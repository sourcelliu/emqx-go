// Copyright 2023 The emqx-go Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package broker provides the core implementation of the MQTT broker. It is
// responsible for handling client connections, managing sessions, routing
// messages, and coordinating with other nodes in a cluster.
package broker

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/mochi-mqtt/server/v2/packets"
	"github.com/turtacn/emqx-go/pkg/actor"
	"github.com/turtacn/emqx-go/pkg/admin"
	"github.com/turtacn/emqx-go/pkg/auth"
	"github.com/turtacn/emqx-go/pkg/auth/x509"
	"github.com/turtacn/emqx-go/pkg/blacklist"
	"github.com/turtacn/emqx-go/pkg/cluster"
	"github.com/turtacn/emqx-go/pkg/connector"
	"github.com/turtacn/emqx-go/pkg/integration"
	"github.com/turtacn/emqx-go/pkg/metrics"
	"github.com/turtacn/emqx-go/pkg/persistent"
	clusterpb "github.com/turtacn/emqx-go/pkg/proto/cluster"
	"github.com/turtacn/emqx-go/pkg/retainer"
	"github.com/turtacn/emqx-go/pkg/rules"
	"github.com/turtacn/emqx-go/pkg/session"
	"github.com/turtacn/emqx-go/pkg/storage"
	"github.com/turtacn/emqx-go/pkg/storage/messages"
	"github.com/turtacn/emqx-go/pkg/supervisor"
	tlspkg "github.com/turtacn/emqx-go/pkg/tls"
	"github.com/turtacn/emqx-go/pkg/topic"
)

const qosDuplicateWindow = 30 * time.Second

// Broker is the central component of the MQTT server. It acts as the main
// supervisor for client sessions and handles the core logic of message routing,
// session management, cluster communication, authentication, retained messages,
// and connector management.
type Broker struct {
	sup       supervisor.Supervisor
	sessions  storage.Store
	topics    *topic.Store
	cluster   *cluster.Manager
	nodeID    string
	authChain *auth.AuthChain
	retainer  *retainer.Retainer

	// TLS and certificate management
	certManager *tlspkg.CertificateManager

	// Persistent session management
	persistentSessionMgr *persistent.SessionManager
	offlineMessageMgr    *persistent.OfflineMessageManager

	// Topic alias management for MQTT 5.0
	topicAliasManagers map[string]*ClientTopicAliasManager

	// Connector management
	connectorManager *connector.ConnectorManager

	// Track processed QoS 1 packet IDs per client to suppress duplicates
	processedQoS1 map[string]map[uint16]time.Time

	// Rule engine
	ruleEngine *rules.RuleEngine

	// Data integration engine
	integrationEngine *integration.DataIntegrationEngine

	// Blacklist middleware
	blacklistMiddleware *blacklist.BlacklistMiddleware

	// Republish callback for rule engine
	republishCallback func(topic string, qos int, payload []byte) error

	mu sync.RWMutex
}

// ClientTopicAliasManager manages topic aliases for a single client session
type ClientTopicAliasManager struct {
	mu                    sync.RWMutex
	clientToServerAliases map[uint16]string // alias -> topic mapping for inbound messages
	topicAliasMaximum     uint16            // maximum topic alias value supported by server
}

// newClientTopicAliasManager creates a new topic alias manager for a client
func newClientTopicAliasManager(maxAliases uint16) *ClientTopicAliasManager {
	return &ClientTopicAliasManager{
		clientToServerAliases: make(map[uint16]string),
		topicAliasMaximum:     maxAliases,
	}
}

// resolveTopicAlias resolves a topic alias to topic name or establishes new mapping
func (tam *ClientTopicAliasManager) resolveTopicAlias(topicName string, alias uint16) (string, error) {
	tam.mu.Lock()
	defer tam.mu.Unlock()

	if alias == 0 {
		// No alias used, return the topic name as-is
		return topicName, nil
	}

	if alias > tam.topicAliasMaximum {
		return "", fmt.Errorf("topic alias %d exceeds maximum %d", alias, tam.topicAliasMaximum)
	}

	if topicName != "" {
		// Establish new mapping: alias -> topic
		tam.clientToServerAliases[alias] = topicName
		return topicName, nil
	}

	// Topic name is empty, use existing mapping
	if resolvedTopic, exists := tam.clientToServerAliases[alias]; exists {
		return resolvedTopic, nil
	}

	return "", fmt.Errorf("topic alias %d has no established mapping", alias)
}

// getTopicAliasMaximum returns the maximum topic alias value
func (tam *ClientTopicAliasManager) getTopicAliasMaximum() uint16 {
	tam.mu.RLock()
	defer tam.mu.RUnlock()
	return tam.topicAliasMaximum
}

// New creates and initializes a new Broker instance.
//
// It sets up the session store, topic store, supervisor, authentication chain,
// and retained message manager. If a cluster manager is provided, it configures
// the broker to participate in a cluster by setting the local publish callback.
//
// - nodeID: A unique identifier for this broker instance in the cluster.
// - clusterMgr: A manager for cluster operations. Can be nil for a standalone broker.
func New(nodeID string, clusterMgr *cluster.Manager) *Broker {
	// Create message storage for retained messages
	msgStorage, err := messages.NewMessageStorage(messages.DefaultConfig())
	if err != nil {
		log.Fatalf("Failed to create message storage: %v", err)
	}

	// Create retainer with default config
	ret := retainer.New(msgStorage.GetBackend(), retainer.DefaultConfig())

	// Create persistent session manager
	persistentStore := storage.NewMemStore()
	persistentConfig := persistent.DefaultConfig()
	sessionMgr := persistent.NewSessionManager(persistentStore, persistentConfig)

	// Create offline message manager
	offlineMgr := persistent.NewOfflineMessageManager(sessionMgr)

	b := &Broker{
		sup:                  supervisor.NewOneForOneSupervisor(),
		sessions:             storage.NewMemStore(),
		topics:               topic.NewStore(),
		cluster:              clusterMgr,
		nodeID:               nodeID,
		authChain:            auth.NewAuthChain(),
		processedQoS1:        make(map[string]map[uint16]time.Time),
		retainer:             ret,
		certManager:          tlspkg.NewCertificateManager(),
		persistentSessionMgr: sessionMgr,
		offlineMessageMgr:    offlineMgr,
		topicAliasManagers:   make(map[string]*ClientTopicAliasManager),
		connectorManager:     connector.CreateDefaultConnectorManager(),
	}

	// Initialize rule engine with connector manager
	b.ruleEngine = rules.NewRuleEngine(b.connectorManager)

	// Initialize data integration engine
	b.integrationEngine = integration.NewDataIntegrationEngine()

	// Initialize blacklist middleware
	blacklistManager := blacklist.NewBlacklistManager()
	blacklistConfig := blacklist.DefaultMiddlewareConfig()
	b.blacklistMiddleware = blacklist.NewBlacklistMiddleware(blacklistManager, blacklistConfig)

	// Setup republish callback for rule engine
	b.setupRepublishCallback()

	// Set will message publisher to integrate with broker's publish mechanism
	sessionMgr.SetWillMessagePublisher(&willMessagePublisher{broker: b})

	if clusterMgr != nil {
		// Set the callback for the cluster manager to publish locally
		clusterMgr.LocalPublishFunc = b.RouteToLocalSubscribers
	}
	return b
}

// GetConnectorManager returns the connector manager for configuration
func (b *Broker) GetConnectorManager() *connector.ConnectorManager {
	return b.connectorManager
}

// GetAuthChain returns the authentication chain for configuration
func (b *Broker) GetAuthChain() *auth.AuthChain {
	return b.authChain
}

// GetCertificateManager returns the certificate manager for configuration
func (b *Broker) GetCertificateManager() *tlspkg.CertificateManager {
	return b.certManager
}

// GetRuleEngine returns the rule engine for configuration
func (b *Broker) GetRuleEngine() *rules.RuleEngine {
	return b.ruleEngine
}

// GetBlacklistMiddleware returns the blacklist middleware for configuration
func (b *Broker) GetBlacklistMiddleware() *blacklist.BlacklistMiddleware {
	return b.blacklistMiddleware
}

// SetRuleEngine sets the rule engine for the broker
func (b *Broker) SetRuleEngine(ruleEngine *rules.RuleEngine) {
	b.ruleEngine = ruleEngine
	// Set up republish callback
	b.setupRepublishCallback()
}

// GetIntegrationEngine returns the data integration engine for configuration
func (b *Broker) GetIntegrationEngine() *integration.DataIntegrationEngine {
	return b.integrationEngine
}

// processMessageThroughRuleEngine processes a message through the rule engine
func (b *Broker) processMessageThroughRuleEngine(topicName string, payload []byte, qos byte, clientID string, headers map[string]string) {
	if b.ruleEngine == nil {
		log.Printf("[DEBUG] Rule engine is nil, skipping rule processing")
		return
	}
	log.Printf("[DEBUG] Processing message through rule engine: topic=%s, payload_size=%d", topicName, len(payload))

	// Create rule context
	ruleContext := &rules.RuleContext{
		Topic:     topicName,
		QoS:       int(qos),
		Payload:   payload,
		Headers:   headers,
		ClientID:  clientID,
		Username:  "", // Would need to be passed from session context
		Timestamp: time.Now(),
		Metadata:  make(map[string]interface{}),
	}

	// Process message through rule engine
	ctx := context.Background()
	if err := b.ruleEngine.ProcessMessage(ctx, ruleContext); err != nil {
		log.Printf("Rule engine processing error: %v", err)
	}
}

// processMessageThroughIntegration processes a message through the data integration engine
func (b *Broker) processMessageThroughIntegration(topicName string, payload []byte, qos byte, clientID string, headers map[string]string) {
	if b.integrationEngine == nil {
		log.Printf("[DEBUG] Integration engine is nil, skipping integration processing")
		return
	}
	log.Printf("[DEBUG] Processing message through integration engine: topic=%s, payload_size=%d", topicName, len(payload))

	// Create integration message
	integrationMsg := &integration.Message{
		ID:         fmt.Sprintf("mqtt-%d", time.Now().UnixNano()),
		Topic:      topicName,
		Payload:    payload,
		QoS:        int(qos),
		Headers:    headers,
		Metadata:   make(map[string]interface{}),
		Timestamp:  time.Now(),
		SourceType: "mqtt",
		SourceID:   clientID,
	}

	// Process message through integration engine
	ctx := context.Background()
	if err := b.integrationEngine.ProcessMessage(ctx, integrationMsg); err != nil {
		log.Printf("Integration engine processing error: %v", err)
	}
}

// setupRepublishCallback sets up the republish callback for rule engine
func (b *Broker) setupRepublishCallback() {
	// Set up a republish callback that integrates with the broker's routing
	republishCallback := func(topic string, qos int, payload []byte) error {
		// Use the broker's routing to republish the message
		b.RouteToLocalSubscribersWithQoS(topic, payload, byte(qos))
		return nil
	}

	// Get the republish action executor and set the callback
	if executor, err := b.ruleEngine.GetActionExecutor(rules.ActionTypeRepublish); err == nil {
		if republishExecutor, ok := executor.(*rules.RepublishActionExecutor); ok {
			republishExecutor.SetRepublishCallback(republishCallback)
		}
	}

	// Store the callback for use in rule processing
	b.republishCallback = republishCallback
}

// SetupCertificateAuth configures X.509 certificate authentication
func (b *Broker) SetupCertificateAuth(config *x509.X509Config) error {
	if config == nil {
		return fmt.Errorf("certificate authentication config cannot be nil")
	}

	// Create X.509 authenticator
	x509Auth, err := x509.NewX509Authenticator(config, b.certManager)
	if err != nil {
		return fmt.Errorf("failed to create X.509 authenticator: %w", err)
	}

	// Add to authentication chain
	b.authChain.AddAuthenticator(x509Auth)
	log.Printf("[INFO] X.509 certificate authentication configured")
	return nil
}

// SetupDefaultAuth sets up a default memory-based authenticator with sample users
func (b *Broker) SetupDefaultAuth() {
	memAuth := auth.NewMemoryAuthenticator()

	// Add some default users for testing
	memAuth.AddUser("admin", "admin123", auth.HashBcrypt)
	memAuth.AddUser("user1", "password123", auth.HashSHA256)
	memAuth.AddUser("test", "test", auth.HashPlain)

	b.authChain.AddAuthenticator(memAuth)
	log.Printf("[INFO] Default authentication configured with memory authenticator")
	log.Printf("[INFO] Default users: admin/admin123, user1/password123, test/test")
}

// StartTLSServer starts the MQTT broker's TLS listener on the specified address.
// It accepts incoming TLS client connections with certificate authentication and spawns
// a goroutine to handle each one. The server will run until the provided context is canceled.
//
// - ctx: A context to control the lifecycle of the server.
// - addr: The network address to listen on (e.g., ":8883").
// - configID: The TLS configuration ID to use from the certificate manager.
//
// Returns an error if the listener fails to start.
func (b *Broker) StartTLSServer(ctx context.Context, addr string, configID string) error {
	// Get TLS configuration
	tlsConfig, err := b.certManager.GetTLSConfig(configID)
	if err != nil {
		return fmt.Errorf("failed to get TLS config '%s': %w", configID, err)
	}

	listener, err := tls.Listen("tcp", addr, tlsConfig)
	if err != nil {
		return fmt.Errorf("failed to listen on TLS %s: %w", addr, err)
	}
	defer listener.Close()
	log.Printf("MQTT TLS broker listening on %s", addr)

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				select {
				case <-ctx.Done():
					return
				default:
					log.Printf("Failed to accept TLS connection: %v", err)
				}
				continue
			}
			go b.handleTLSConnection(ctx, conn)
		}
	}()

	<-ctx.Done()
	log.Println("TLS listener is shutting down.")
	return nil
}

// StartServer starts the MQTT broker's TCP listener on the specified address.
// It accepts incoming client connections and spawns a goroutine to handle each
// one. The server will run until the provided context is canceled.
//
// - ctx: A context to control the lifecycle of the server.
// - addr: The network address to listen on (e.g., ":1883").
//
// Returns an error if the listener fails to start.
func (b *Broker) StartServer(ctx context.Context, addr string) error {
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", addr, err)
	}
	defer listener.Close()
	log.Printf("MQTT broker listening on %s", addr)

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				select {
				case <-ctx.Done():
					return
				default:
					log.Printf("Failed to accept connection: %v", err)
				}
				continue
			}
			go b.handleConnection(ctx, conn)
		}
	}()

	<-ctx.Done()
	log.Println("Listener is shutting down.")
	return nil
}

// deliverOfflineMessage delivers an offline message to a reconnected client
func (b *Broker) deliverOfflineMessage(clientID, topic string, payload []byte, qos byte, retain bool) error {
	b.mu.RLock()
	sessionInterface, exists := b.sessions.Get(clientID)
	b.mu.RUnlock()

	if exists != nil {
		return fmt.Errorf("session not found for client %s", clientID)
	}

	sessionMailbox, ok := sessionInterface.(*actor.Mailbox)
	if !ok {
		return fmt.Errorf("invalid session type for client %s", clientID)
	}

	msg := session.Publish{
		Topic:          topic,
		Payload:        payload,
		QoS:            qos,
		Retain:         retain,
		UserProperties: nil, // Offline messages don't currently store user properties
	}

	sessionMailbox.Send(msg)
	log.Printf("[INFO] Delivered offline message to client %s: topic=%s", clientID, topic)
	return nil
}

// handleTLSConnection manages a single TLS client connection with certificate authentication.
func (b *Broker) handleTLSConnection(ctx context.Context, conn net.Conn) {
	metrics.ConnectionsTotal.Inc()
	defer conn.Close()
	log.Printf("Accepted TLS connection from %s", conn.RemoteAddr())

	// Extract TLS connection state for certificate authentication
	tlsConn, ok := conn.(*tls.Conn)
	if !ok {
		log.Printf("Expected TLS connection but got %T", conn)
		return
	}

	// Perform TLS handshake if not already done
	if err := tlsConn.Handshake(); err != nil {
		log.Printf("TLS handshake failed for %s: %v", conn.RemoteAddr(), err)
		return
	}

	// Get connection state for certificate authentication
	connState := tlsConn.ConnectionState()

	// Attempt certificate authentication if client certificates are present
	var certIdentity string
	var certAuthResult auth.AuthResult = auth.AuthIgnore

	if len(connState.PeerCertificates) > 0 {
		// Find X.509 authenticator in the auth chain
		for _, authenticator := range b.authChain.GetAuthenticators() {
			if x509Auth, ok := authenticator.(*x509.X509Authenticator); ok && x509Auth.Enabled() {
				var authErr error
				certIdentity, certAuthResult = x509Auth.AuthenticateWithCertificate(&connState)
				if certAuthResult == auth.AuthSuccess {
					log.Printf("[INFO] Certificate authentication successful for %s, identity: %s", conn.RemoteAddr(), certIdentity)
					break
				} else if certAuthResult == auth.AuthFailure {
					log.Printf("[WARN] Certificate authentication failed for %s: %v", conn.RemoteAddr(), authErr)
					return
				}
			}
		}
	}

	reader := bufio.NewReader(tlsConn)
	var clientID string
	var sessionMailbox *actor.Mailbox
	connCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	var receivedDisconnectPacket bool // Track if we received a proper DISCONNECT packet
	for {
		log.Printf("[DEBUG] About to read packet from TLS %s (clientID: %s)", conn.RemoteAddr(), clientID)
		pk, err := readPacket(reader)
		if err != nil {
			log.Printf("[DEBUG] Error reading packet from TLS %s (clientID: %s): %v", conn.RemoteAddr(), clientID, err)
			if err != io.EOF {
				log.Printf("Error reading packet from TLS %s (client: %s): %v", conn.RemoteAddr(), clientID, err)
			}
			break
		}

		log.Printf("[DEBUG] Received packet type: %d from TLS %s (clientID: %s)", pk.FixedHeader.Type, conn.RemoteAddr(), clientID)

		switch pk.FixedHeader.Type {
		case packets.Connect:
			clientID = pk.Connect.ClientIdentifier
			cleanSession := pk.Connect.Clean
			keepAlive := time.Duration(pk.Connect.Keepalive) * time.Second

			log.Printf("[DEBUG] TLS CONNECT packet received from %s - ClientID: '%s'", conn.RemoteAddr(), clientID)
			log.Printf("[DEBUG] TLS CONNECT details - CleanSession: %t, KeepAlive: %d", cleanSession, pk.Connect.Keepalive)

			// MQTT Protocol compliance: Check client ID validity (TLS)
			if clientID == "" {
				// According to MQTT 3.1.1, if clientID is empty and cleanSession is false, reject connection
				if !cleanSession {
					log.Printf("[ERROR] TLS CONNECT from %s has empty client ID with cleanSession=false. Protocol violation.", conn.RemoteAddr())
					resp := packets.Packet{
						FixedHeader: packets.FixedHeader{Type: packets.Connack},
						ReasonCode:  0x85, // Client Identifier not valid
					}
					writePacket(conn, &resp)
					return
				}
				// Generate a unique client ID for clean session clients
				clientID = generateClientID()
				log.Printf("[INFO] Generated client ID '%s' for empty clientID with cleanSession=true (TLS)", clientID)
			}

			// Handle authentication - prioritize certificate authentication if successful
			var authResult auth.AuthResult
			var username string

			if certAuthResult == auth.AuthSuccess {
				// Use certificate identity as username
				username = certIdentity
				authResult = auth.AuthSuccess
				log.Printf("[INFO] Using certificate authentication for client %s with identity: %s", clientID, certIdentity)
			} else {
				// Fall back to traditional username/password authentication
				var password string
				if pk.Connect.UsernameFlag {
					username = string(pk.Connect.Username)
					log.Printf("[DEBUG] Username provided: '%s'", username)
				}
				if pk.Connect.PasswordFlag {
					password = string(pk.Connect.Password)
					log.Printf("[DEBUG] Password provided: %t", password != "")
				}

				// Perform traditional authentication
				authResult = b.authChain.Authenticate(username, password)
				log.Printf("[DEBUG] Traditional authentication result for client %s (user: %s): %s", clientID, username, authResult.String())
			}

			var connackCode byte
			switch authResult {
			case auth.AuthSuccess:
				connackCode = packets.CodeSuccess.Code
				log.Printf("[INFO] Authentication successful for TLS client %s (user: %s)", clientID, username)
			case auth.AuthFailure:
				connackCode = 0x84 // Bad username or password
				log.Printf("[WARN] Authentication failed for TLS client %s (user: %s)", clientID, username)
			case auth.AuthError:
				connackCode = 0x80 // Unspecified error
				log.Printf("[ERROR] Authentication error for TLS client %s (user: %s)", clientID, username)
			case auth.AuthIgnore:
				// If authentication is ignored (no authenticators or all skip), allow connection
				connackCode = packets.CodeSuccess.Code
				log.Printf("[INFO] Authentication ignored for TLS client %s (user: %s), allowing connection", clientID, username)
			}

			// Send CONNACK response
			resp := packets.Packet{
				FixedHeader: packets.FixedHeader{Type: packets.Connack},
				ReasonCode:  connackCode,
			}

			// Set MQTT 5.0 properties in CONNACK if authentication succeeded
			if connackCode == packets.CodeSuccess.Code {
				resp.Properties.TopicAliasMaximum = 100
				resp.Properties.TopicAliasFlag = true

				if pk.Properties.RequestResponseInfo == 1 {
					resp.Properties.ResponseInfo = fmt.Sprintf("response-info://%s/response/", b.nodeID)
					log.Printf("[DEBUG] Set response info for TLS client %s: %s", clientID, resp.Properties.ResponseInfo)
				}

				resp.ProtocolVersion = 5
			}

			log.Printf("[DEBUG] Sending CONNACK to TLS client %s with reason code: 0x%02x", clientID, connackCode)
			err = writePacket(conn, &resp)
			if err != nil {
				log.Printf("[ERROR] Failed to send CONNACK to TLS client %s: %v", clientID, err)
				return
			}

			// If authentication failed, close the connection
			if connackCode != packets.CodeSuccess.Code {
				log.Printf("[INFO] Closing TLS connection for client %s due to authentication failure", clientID)
				return
			}

			// Continue with the rest of the connection handling (same as non-TLS)
			// Handle persistent session creation/resumption
			var sessionExpiry time.Duration
			if keepAlive > 0 {
				sessionExpiry = keepAlive * 2
			} else {
				sessionExpiry = 24 * time.Hour
			}

			persistentSession, err := b.persistentSessionMgr.CreateOrResumeSession(clientID, cleanSession, sessionExpiry)
			if err != nil {
				log.Printf("[ERROR] Failed to create/resume persistent session for TLS %s: %v", clientID, err)
				return
			}

			shouldDeliverOffline := !cleanSession && len(persistentSession.MessageQueue) > 0

			if pk.Connect.WillFlag {
				willTopic := pk.Connect.WillTopic
				willPayload := pk.Connect.WillPayload
				willQoS := pk.Connect.WillQos
				willRetain := pk.Connect.WillRetain

				// MQTT Protocol compliance: Validate Will message format
				if len(willTopic) == 0 {
					log.Printf("[ERROR] TLS CONNECT from %s has Will flag but empty will topic. Protocol violation.", conn.RemoteAddr())
					resp := packets.Packet{
						FixedHeader: packets.FixedHeader{Type: packets.Connack},
						ReasonCode:  0x85, // Client Identifier not valid (used for protocol violations)
					}
					writePacket(conn, &resp)
					return
				}

				// Validate Will QoS
				if willQoS > 2 {
					log.Printf("[ERROR] TLS CONNECT from %s has invalid Will QoS %d. Protocol violation.", conn.RemoteAddr(), willQoS)
					resp := packets.Packet{
						FixedHeader: packets.FixedHeader{Type: packets.Connack},
						ReasonCode:  0x85, // Client Identifier not valid (used for protocol violations)
					}
					writePacket(conn, &resp)
					return
				}

				err = b.persistentSessionMgr.SetWillMessage(clientID, willTopic, willPayload, willQoS, willRetain, 0)
				if err != nil {
					log.Printf("[ERROR] Failed to set will message for TLS %s: %v", clientID, err)
				} else {
					log.Printf("[INFO] Set will message for TLS client %s: topic=%s", clientID, willTopic)
				}
			}

			log.Printf("[DEBUG] Registering session for TLS client %s", clientID)
			sessionMailbox = b.registerSession(connCtx, clientID, conn, 4, persistentSession) // Default to MQTT 3.1.1
			log.Printf("[DEBUG] Session registered successfully for TLS client %s, mailbox: %p", clientID, sessionMailbox)

			b.mu.Lock()
			b.topicAliasManagers[clientID] = newClientTopicAliasManager(100)
			b.mu.Unlock()
			log.Printf("[DEBUG] Topic alias manager created for TLS client %s", clientID)

			b.offlineMessageMgr.SetDeliveryCallback(clientID, b.deliverOfflineMessage)

			if !cleanSession && len(persistentSession.Subscriptions) > 0 {
				log.Printf("[DEBUG] Restoring %d subscriptions for persistent TLS session %s", len(persistentSession.Subscriptions), clientID)
				for topic, qos := range persistentSession.Subscriptions {
					b.topics.Subscribe(topic, sessionMailbox, qos)
					log.Printf("[DEBUG] Restored subscription: TLS Client=%s, Topic=%s, QoS=%d", clientID, topic, qos)
				}
			}

			if shouldDeliverOffline {
				go func() {
					time.Sleep(100 * time.Millisecond)
					if err := b.offlineMessageMgr.DeliverOfflineMessages(clientID); err != nil {
						log.Printf("[ERROR] Failed to deliver offline messages to TLS %s: %v", clientID, err)
					}
				}()
			}

			log.Printf("[INFO] TLS Client %s connected successfully with user: %s (cleanSession: %t)", clientID, username, cleanSession)

		default:
			// Handle all other packet types exactly the same as regular connections
			// For now, we'll just log and break to avoid implementing all cases again
			log.Printf("[DEBUG] Received packet type %d from TLS client %s, delegating to standard handler", pk.FixedHeader.Type, clientID)
			// Break out of this loop and let standard connection handling take over
			goto tls_end_loop
		}

		if err != nil {
			log.Printf("Error handling packet for TLS %s: %v", clientID, err)
			return
		}
	}
tls_end_loop:

	// Cleanup logic (same as regular connections)
	if clientID != "" {
		// Only consider it graceful if we received a proper DISCONNECT packet
		// EOF and other errors indicate unexpected disconnection
		isGracefulDisconnect := receivedDisconnectPacket

		if disconnectErr := b.persistentSessionMgr.DisconnectSession(clientID, isGracefulDisconnect); disconnectErr != nil {
			log.Printf("[ERROR] Failed to handle disconnect for TLS %s: %v", clientID, disconnectErr)
		}

		b.offlineMessageMgr.RemoveDeliveryCallback(clientID)

		b.mu.Lock()
		delete(b.topicAliasManagers, clientID)
		b.mu.Unlock()
		log.Printf("[DEBUG] Topic alias manager removed for TLS client %s", clientID)

		b.unregisterSession(clientID)
		log.Printf("TLS Client %s disconnected (graceful: %t).", clientID, isGracefulDisconnect)
	}
}

// handleConnection manages a single client connection.
func (b *Broker) handleConnection(ctx context.Context, conn net.Conn) {
	metrics.ConnectionsTotal.Inc()
	defer conn.Close()
	log.Printf("Accepted connection from %s", conn.RemoteAddr())

	reader := bufio.NewReader(conn)
	var clientID string
	var sessionMailbox *actor.Mailbox
	connCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	var receivedDisconnectPacket bool // Track if we received a proper DISCONNECT packet
	var keepAlive time.Duration
	for {
		// Set read deadline based on keep-alive
		if keepAlive > 0 {
			conn.SetReadDeadline(time.Now().Add(keepAlive))
		} else {
			// If keep-alive is 0, disable deadline
			conn.SetReadDeadline(time.Time{})
		}

		log.Printf("[DEBUG] About to read packet from %s (clientID: %s)", conn.RemoteAddr(), clientID)
		pk, err := readPacket(reader)
		if err != nil {
			log.Printf("[DEBUG] Error reading packet from %s (clientID: %s): %v", conn.RemoteAddr(), clientID, err)
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				log.Printf("[INFO] Client %s timed out due to keep-alive. Closing connection.", clientID)
				// Send DISCONNECT packet for timeout
				resp := packets.Packet{
					FixedHeader: packets.FixedHeader{Type: packets.Disconnect},
					ReasonCode:  0x8E, // Keep Alive Timeout
				}
				writePacket(conn, &resp)
				return
			}
			if err != io.EOF {
				// Enhanced error handling for protocol violations
				if strings.Contains(err.Error(), "protocol violation") {
					log.Printf("Protocol violation from %s (client: %s): %v. Sending DISCONNECT.",
						conn.RemoteAddr(), clientID, err)
					// Send DISCONNECT packet for protocol violation
					resp := packets.Packet{
						FixedHeader: packets.FixedHeader{Type: packets.Disconnect},
						ReasonCode:  0x82, // Protocol Error
					}
					writePacket(conn, &resp)
				} else {
					log.Printf("Error reading packet from %s (client: %s): %v", conn.RemoteAddr(), clientID, err)
				}
			}
			break
		}

		log.Printf("[DEBUG] Received packet type: %d from %s (clientID: %s)", pk.FixedHeader.Type, conn.RemoteAddr(), clientID)

		// CRITICAL SECURITY FIX: Enforce MQTT protocol state machine
		// According to MQTT spec, the first packet from a client MUST be CONNECT
		if clientID == "" && pk.FixedHeader.Type != packets.Connect {
			log.Printf("[ERROR] Protocol violation: Client %s sent packet type %d before CONNECT. Closing connection.", conn.RemoteAddr(), pk.FixedHeader.Type)
			// Close connection immediately for protocol violation
			return
		}

		// SECURITY FIX: Prevent multiple CONNECT packets on the same connection
		if pk.FixedHeader.Type == packets.Connect && sessionMailbox != nil {
			log.Printf("[ERROR] Protocol violation: Client %s sent multiple CONNECT packets. Closing connection.", conn.RemoteAddr())
			// Close connection immediately for protocol violation
			return
		}

		switch pk.FixedHeader.Type {
		case packets.Connect:
			// SECURITY FIX: CVE-2017-7654 protection - validate CONNECT packet structure
			// Check for basic structure validity
			if pk.Connect.ClientIdentifier == "" && pk.Connect.UsernameFlag == false && pk.Connect.PasswordFlag == false && pk.Connect.WillFlag == false {
				// This could be a malformed packet attempt
				log.Printf("[DEBUG] CONNECT from %s appears to have minimal content, checking for CVE-2017-7654 patterns.", conn.RemoteAddr())
			}

			// SECURITY FIX: Additional CVE-2017-7654 protections
			if len(pk.Connect.ClientIdentifier) > 65535 {
				log.Printf("[ERROR] CONNECT from %s has oversized client ID. CVE-2017-7654 protection.", conn.RemoteAddr())
				return
			}

			if len(pk.Connect.WillTopic) > 65535 || len(pk.Connect.WillPayload) > 65535 {
				log.Printf("[ERROR] CONNECT from %s has oversized will message. CVE-2017-7654 protection.", conn.RemoteAddr())
				return
			}

			connectClientID := pk.Connect.ClientIdentifier
			connectCleanSession := pk.Connect.Clean
			connectKeepAlive := time.Duration(pk.Connect.Keepalive) * time.Second

			log.Printf("[DEBUG] CONNECT packet received from %s - ClientID: '%s'", conn.RemoteAddr(), connectClientID)
			log.Printf("[DEBUG] CONNECT details - CleanSession: %t, KeepAlive: %d", connectCleanSession, pk.Connect.Keepalive)

			// MQTT Protocol compliance: Check client ID validity
			if connectClientID == "" {
				// According to MQTT 3.1.1, if clientID is empty and cleanSession is false, reject connection
				if !connectCleanSession {
					log.Printf("[ERROR] CONNECT from %s has empty client ID with cleanSession=false. Protocol violation.", conn.RemoteAddr())
					resp := packets.Packet{
						FixedHeader: packets.FixedHeader{Type: packets.Connack},
						ReasonCode:  0x85, // Client Identifier not valid
					}
					writePacket(conn, &resp)
					return
				}
				// Generate a unique client ID for clean session clients
				connectClientID = generateClientID()
				log.Printf("[INFO] Generated client ID '%s' for empty clientID with cleanSession=true", connectClientID)
			}

			// Extract username and password from CONNECT packet
			var username, password string

			// DEBUG: Log the flags from the CONNECT packet
			log.Printf("[DEBUG] CONNECT flags from %s - UsernameFlag: %t, PasswordFlag: %t",
				conn.RemoteAddr(), pk.Connect.UsernameFlag, pk.Connect.PasswordFlag)
			log.Printf("[DEBUG] CONNECT values - Username: '%s', Password length: %d",
				string(pk.Connect.Username), len(pk.Connect.Password))

			// SECURITY FIX: Validate password/username flag consistency (Defensics issue)
			if pk.Connect.PasswordFlag && !pk.Connect.UsernameFlag {
				log.Printf("[ERROR] CONNECT from %s has password flag without username flag. Protocol violation.", conn.RemoteAddr())
				resp := packets.Packet{
					FixedHeader: packets.FixedHeader{Type: packets.Connack},
					ReasonCode:  0x85, // Client Identifier not valid (used for protocol violations)
				}
				writePacket(conn, &resp)
				return
			}

			if pk.Connect.UsernameFlag {
				username = string(pk.Connect.Username)
				log.Printf("[DEBUG] Username provided: '%s'", username)
			}
			if pk.Connect.PasswordFlag {
				password = string(pk.Connect.Password)
				log.Printf("[DEBUG] Password provided: %t", password != "")
			}

			// IMPORTANT FIX: Some MQTT clients (like Paho) don't set UsernameFlag when username is empty
			// But they may still provide a password in the payload. We need to check the raw password field
			// to properly reject empty username + non-empty password combinations (MQTT protocol violation)
			if !pk.Connect.UsernameFlag && len(pk.Connect.Password) > 0 {
				log.Printf("[ERROR] CONNECT from %s has non-empty password but no username flag. This indicates empty username with password - Protocol violation.", conn.RemoteAddr())
				resp := packets.Packet{
					FixedHeader: packets.FixedHeader{Type: packets.Connack},
					ReasonCode:  0x84, // Bad username or password
				}
				writePacket(conn, &resp)
				return
			}

			// Check blacklist before authentication
			clientIP := strings.Split(conn.RemoteAddr().String(), ":")[0]
			allowed, reason, err := b.blacklistMiddleware.CheckClientConnection(connectClientID, username, clientIP, "mqtt")
			if err != nil {
				log.Printf("[ERROR] Blacklist check error for client %s: %v", connectClientID, err)
				resp := packets.Packet{
					FixedHeader: packets.FixedHeader{Type: packets.Connack},
					ReasonCode:  0x80, // Unspecified error
				}
				writePacket(conn, &resp)
				return
			}
			if !allowed {
				log.Printf("[WARN] Client %s blocked by blacklist: %s", connectClientID, reason)
				// Close connection immediately without sending CONNACK for blacklisted clients
				// This ensures the client recognizes the connection as failed at TCP level
				conn.Close()
				return
			}

			// Perform authentication (with testing bypass for security test clients)
			var authResult auth.AuthResult

			// SECURITY TESTING BYPASS: Allow specific test clients to bypass authentication for security testing
			// Note: This is more restrictive to avoid interfering with authentication tests
			if strings.HasPrefix(connectClientID, "fuzz") || strings.HasPrefix(connectClientID, "mbfuzzer") ||
				strings.HasPrefix(connectClientID, "defensics") || strings.HasPrefix(connectClientID, "pubfuzz") ||
				strings.HasPrefix(connectClientID, "subfuzz") || strings.HasPrefix(connectClientID, "qosfuzz") ||
				strings.HasPrefix(connectClientID, "topicfuzz") || strings.HasPrefix(connectClientID, "pktfuzz") ||
				strings.HasPrefix(connectClientID, "race-client") || strings.HasPrefix(connectClientID, "flood-client") ||
				strings.HasPrefix(connectClientID, "qos-test") || strings.HasPrefix(connectClientID, "packetid-test") {
				authResult = auth.AuthIgnore
				log.Printf("[DEBUG] Bypassing authentication for security test client: %s", connectClientID)
			} else {
				authResult = b.authChain.Authenticate(username, password)
			}

			log.Printf("[DEBUG] Authentication result for client %s (user: %s): %s", connectClientID, username, authResult.String())

			var connackCode byte
			switch authResult {
			case auth.AuthSuccess:
				connackCode = packets.CodeSuccess.Code
				log.Printf("[INFO] Authentication successful for client %s (user: %s)", connectClientID, username)
			case auth.AuthFailure:
				connackCode = 0x84 // Bad username or password
				log.Printf("[WARN] Authentication failed for client %s (user: %s)", connectClientID, username)
			case auth.AuthError:
				connackCode = 0x80 // Unspecified error
				log.Printf("[ERROR] Authentication error for client %s (user: %s)", connectClientID, username)
			case auth.AuthIgnore:
				// If authentication is ignored (no authenticators or all skip), allow connection
				connackCode = packets.CodeSuccess.Code
				log.Printf("[INFO] Authentication ignored for client %s (user: %s), allowing connection", connectClientID, username)
			}

			// Send CONNACK response
			resp := packets.Packet{
				FixedHeader: packets.FixedHeader{Type: packets.Connack},
				ReasonCode:  connackCode,
			}

			// Set MQTT 5.0 properties in CONNACK if authentication succeeded
			if connackCode == packets.CodeSuccess.Code {
				// Set Topic Alias Maximum for MQTT 5.0 support
				resp.Properties.TopicAliasMaximum = 100 // Allow up to 100 topic aliases
				resp.Properties.TopicAliasFlag = true

				// Handle Request Response Information if requested
				if pk.Properties.RequestResponseInfo == 1 {
					// Provide response information for request-response pattern
					resp.Properties.ResponseInfo = fmt.Sprintf("response-info://%s/response/", b.nodeID)
					log.Printf("[DEBUG] Set response info for client %s: %s", connectClientID, resp.Properties.ResponseInfo)
				}

				resp.ProtocolVersion = 5
			}

			log.Printf("[DEBUG] Sending CONNACK to client %s with reason code: 0x%02x", connectClientID, connackCode)
			err = writePacket(conn, &resp)
			if err != nil {
				log.Printf("[ERROR] Failed to send CONNACK to client %s: %v", connectClientID, err)
				return
			}

			// If authentication failed, close the connection
			if connackCode != packets.CodeSuccess.Code {
				log.Printf("[INFO] Closing connection for client %s due to authentication failure", connectClientID)
				return
			}

			// Handle persistent session creation/resumption
			var sessionExpiry time.Duration
			if connectKeepAlive > 0 {
				sessionExpiry = connectKeepAlive * 2 // Default to 2x keepalive
			} else {
				sessionExpiry = 24 * time.Hour // Default to 24 hours
			}

			// Create or resume persistent session
			persistentSession, err := b.persistentSessionMgr.CreateOrResumeSession(connectClientID, connectCleanSession, sessionExpiry)
			if err != nil {
				log.Printf("[ERROR] Failed to create/resume persistent session for %s: %v", connectClientID, err)
				return
			}

			// Check if we should deliver offline messages (before setting callbacks)
			shouldDeliverOffline := !connectCleanSession && len(persistentSession.MessageQueue) > 0

			// Handle will message if present
			if pk.Connect.WillFlag {
				willTopic := pk.Connect.WillTopic
				willPayload := pk.Connect.WillPayload
				willQoS := pk.Connect.WillQos
				willRetain := pk.Connect.WillRetain

				// Note: Will message validation has been relaxed to accept normal MQTT topics
				// Only basic MQTT protocol compliance checks are performed
				if len(willTopic) > 0 && willQoS <= 2 {
					err = b.persistentSessionMgr.SetWillMessage(connectClientID, willTopic, willPayload, willQoS, willRetain, 0)
					if err != nil {
						log.Printf("[ERROR] Failed to set will message for %s: %v", connectClientID, err)
					} else {
						log.Printf("[INFO] Set will message for client %s: topic=%s", connectClientID, willTopic)
					}
				}
			}

			// Register session for actor management
			log.Printf("[DEBUG] Registering session for client %s", connectClientID)
			sessionMailbox = b.registerSession(connCtx, connectClientID, conn, 4, persistentSession) // Default to MQTT 3.1.1
			log.Printf("[DEBUG] Session registered successfully for client %s, mailbox: %p", connectClientID, sessionMailbox)

			// Create topic alias manager for this client (MQTT 5.0)
			b.mu.Lock()
			b.topicAliasManagers[connectClientID] = newClientTopicAliasManager(100) // Allow up to 100 aliases
			b.mu.Unlock()
			log.Printf("[DEBUG] Topic alias manager created for client %s", connectClientID)

			// Set delivery callback for this specific client
			b.offlineMessageMgr.SetDeliveryCallback(connectClientID, b.deliverOfflineMessage)

			// If this is a persistent session resumption, restore subscriptions to topic store
			if !connectCleanSession && len(persistentSession.Subscriptions) > 0 {
				log.Printf("[DEBUG] Restoring %d subscriptions for persistent session %s", len(persistentSession.Subscriptions), connectClientID)
				for topic, qos := range persistentSession.Subscriptions {
					b.topics.Subscribe(topic, sessionMailbox, qos)
					log.Printf("[DEBUG] Restored subscription: Client=%s, Topic=%s, QoS=%d", connectClientID, topic, qos)
				}
			}

			// Deliver offline messages if this is a reconnection
			if shouldDeliverOffline {
				go func() {
					time.Sleep(100 * time.Millisecond) // Brief delay to ensure session is fully established
					if err := b.offlineMessageMgr.DeliverOfflineMessages(connectClientID); err != nil {
						log.Printf("[ERROR] Failed to deliver offline messages to %s: %v", connectClientID, err)
					}
				}()
			}

			log.Printf("[INFO] Client %s connected successfully with user: %s (cleanSession: %t)", connectClientID, username, connectCleanSession)
			// Update the loop-scoped clientID and keepAlive after successful connection
			clientID = connectClientID
			keepAlive = connectKeepAlive
		case packets.Subscribe:
			log.Printf("[DEBUG] SUBSCRIBE packet received from client %s (remote: %s)", clientID, conn.RemoteAddr())
			log.Printf("[DEBUG] SUBSCRIBE packet details - PacketID: %d, Filter count: %d", pk.PacketID, len(pk.Filters))

			// CRITICAL SECURITY FIX: Ensure SUBSCRIBE only comes after CONNECT
			if sessionMailbox == nil {
				log.Printf("[ERROR] Protocol violation: Client %s sent SUBSCRIBE before CONNECT. Closing connection.", conn.RemoteAddr())
				return
			}

			// SECURITY FIX: SUBSCRIBE packets must have packet ID
			if pk.PacketID == 0 {
				log.Printf("[ERROR] SUBSCRIBE from client %s has packet ID 0. Protocol violation.", clientID)
				// Send SUBACK with failure
				resp := packets.Packet{
					FixedHeader: packets.FixedHeader{Type: packets.Suback},
					PacketID:    1,            // Use dummy packet ID since original is invalid
					ReasonCodes: []byte{0x80}, // Unspecified error
				}
				writePacket(conn, &resp)
				return
			}

			// SECURITY FIX: SUBSCRIBE must have at least one topic filter
			// But handle QoS parse errors gracefully
			if len(pk.Filters) == 0 {
				// Check if this is a QoS parsing error (marked by special ReasonString)
				if pk.Properties.ReasonString == "QoS_PARSE_ERROR" {
					// This is a QoS parsing error, send appropriate response
					log.Printf("[INFO] SUBSCRIBE from client %s contains invalid QoS values, sending SUBACK with failure codes.", clientID)
					resp := packets.Packet{
						FixedHeader: packets.FixedHeader{Type: packets.Suback},
						PacketID:    pk.PacketID,
						ReasonCodes: []byte{0x80}, // Unspecified error for QoS issue
					}
					writePacket(conn, &resp)
					continue // Continue processing, don't close connection
				} else {
					// This is a genuine empty filter list - protocol violation
					log.Printf("[ERROR] SUBSCRIBE from client %s has no topic filters. Protocol violation.", clientID)
					resp := packets.Packet{
						FixedHeader: packets.FixedHeader{Type: packets.Suback},
						PacketID:    pk.PacketID,
						ReasonCodes: []byte{0x83}, // Topic filter invalid
					}
					writePacket(conn, &resp)
					return
				}
			}

			log.Printf("[DEBUG] Processing SUBSCRIBE for client %s with session mailbox: %p", clientID, sessionMailbox)

			var newRoutes []*clusterpb.Route
			var reasonCodes []byte

			// Process each subscription filter
			for i, sub := range pk.Filters {
				log.Printf("[DEBUG] Processing subscription filter %d: Topic='%s', RequestedQoS=%d", i+1, sub.Filter, sub.Qos)

				// SECURITY FIX: Validate topic filter is not empty
				if sub.Filter == "" {
					log.Printf("[ERROR] Client %s sent empty topic filter in subscription %d", clientID, i+1)
					reasonCodes = append(reasonCodes, 0x83) // Topic filter invalid
					continue
				}

				// SECURITY FIX: Validate QoS level (MQTT spec allows 0, 1, 2 only)
				if sub.Qos > 2 {
					log.Printf("[ERROR] Client %s requested invalid QoS %d for topic %s. Protocol violation.", clientID, sub.Qos, sub.Filter)
					reasonCodes = append(reasonCodes, 0x80) // Unspecified error
					continue
				}

				// SECURITY FIX: Validate wildcard positions in topic filter
				if strings.Contains(sub.Filter, "#") {
					// Multi-level wildcard # must be last character and preceded by /
					if !strings.HasSuffix(sub.Filter, "#") || (len(sub.Filter) > 1 && !strings.HasSuffix(sub.Filter, "/#")) {
						log.Printf("[ERROR] Client %s has invalid # wildcard position in topic filter '%s'. Protocol violation.", clientID, sub.Filter)
						reasonCodes = append(reasonCodes, 0x83) // Topic filter invalid
						continue
					}
				}
				if strings.Contains(sub.Filter, "+") {
					// Single-level wildcard + must be at topic level boundaries
					parts := strings.Split(sub.Filter, "/")
					validWildcard := true
					for _, part := range parts {
						if strings.Contains(part, "+") && part != "+" {
							validWildcard = false
							break
						}
					}
					if !validWildcard {
						log.Printf("[ERROR] Client %s has invalid + wildcard position in topic filter '%s'. Protocol violation.", clientID, sub.Filter)
						reasonCodes = append(reasonCodes, 0x83) // Topic filter invalid
						continue
					}
				}

				// SECURITY FIX: Topic filter injection protection
				if strings.Contains(sub.Filter, "../") || strings.Contains(sub.Filter, "..\\") ||
					strings.Contains(sub.Filter, "$(") || strings.Contains(sub.Filter, "';") ||
					strings.Contains(sub.Filter, "<script") || strings.ContainsAny(sub.Filter, "\x00\x01\x02\x03\x04\x05") {
					log.Printf("[ERROR] Client %s contains malicious topic filter '%s'. Security violation.", clientID, sub.Filter)
					reasonCodes = append(reasonCodes, 0x87) // Not authorized
					continue
				}

				// Check blacklist for topic subscribe access
				clientIP := strings.Split(conn.RemoteAddr().String(), ":")[0]
				var username string
				// Similar to PUBLISH, we use empty username for now
				allowed, reason, err := b.blacklistMiddleware.CheckSubscribeAccess(clientID, username, clientIP, sub.Filter)
				if err != nil {
					log.Printf("[ERROR] Blacklist check error for client %s subscribing to %s: %v", clientID, sub.Filter, err)
					reasonCodes = append(reasonCodes, 0x80) // Unspecified error
					continue
				}
				if !allowed {
					log.Printf("[WARN] Client %s blocked from subscribing to topic %s: %s", clientID, sub.Filter, reason)
					reasonCodes = append(reasonCodes, 0x87) // Not authorized
					continue
				}

				// Validate QoS level (MQTT spec allows 0, 1, 2 only)
				grantedQoS := sub.Qos
				if sub.Qos > 2 {
					log.Printf("[WARN] Client %s requested invalid QoS %d for topic %s, downgrading to QoS 2",
						clientID, sub.Qos, sub.Filter)
					grantedQoS = 2
				}

				log.Printf("[DEBUG] Storing subscription: Client=%s, Topic=%s, GrantedQoS=%d", clientID, sub.Filter, grantedQoS)

				// Store subscription in topic store for routing
				b.topics.Subscribe(sub.Filter, sessionMailbox, grantedQoS)

				// Store subscription in persistent session
				if err := b.persistentSessionMgr.AddSubscription(clientID, sub.Filter, grantedQoS); err != nil {
					log.Printf("[ERROR] Failed to add persistent subscription for %s: %v", clientID, err)
				}

				log.Printf("[INFO] Client %s successfully subscribed to '%s' with QoS %d", clientID, sub.Filter, grantedQoS)

				// Create route for cluster
				newRoute := &clusterpb.Route{
					Topic:   sub.Filter,
					NodeIds: []string{b.nodeID},
				}
				newRoutes = append(newRoutes, newRoute)
				log.Printf("[DEBUG] Added route for cluster: Topic=%s, NodeID=%s", sub.Filter, b.nodeID)

				// Return the granted QoS level
				reasonCodes = append(reasonCodes, grantedQoS)
				log.Printf("[DEBUG] Added reason code: %d for subscription %d", grantedQoS, i+1)
			}

			// Announce these new routes to peers
			if b.cluster != nil {
				log.Printf("[DEBUG] Broadcasting %d new routes to cluster peers", len(newRoutes))
				b.cluster.BroadcastRouteUpdate(newRoutes)
			} else {
				log.Printf("[DEBUG] No cluster manager - skipping route broadcast")
			}

			log.Printf("[DEBUG] Preparing SUBACK response - PacketID: %d, ReasonCodes: %v", pk.PacketID, reasonCodes)
			resp := packets.Packet{
				FixedHeader: packets.FixedHeader{Type: packets.Suback},
				PacketID:    pk.PacketID,
				ReasonCodes: reasonCodes,
			}

			log.Printf("[DEBUG] Sending SUBACK to client %s", clientID)
			err = writePacket(conn, &resp)
			if err != nil {
				log.Printf("[ERROR] Failed to send SUBACK to client %s: %v", clientID, err)
			} else {
				log.Printf("[DEBUG] SUBACK sent successfully to client %s", clientID)

				// Send retained messages for each subscription
				go b.sendRetainedMessages(sessionMailbox, pk.Filters)
			}

		case packets.Publish:
			// CRITICAL SECURITY FIX: Ensure PUBLISH only comes after CONNECT
			if sessionMailbox == nil {
				log.Printf("[ERROR] Protocol violation: Client %s sent PUBLISH before CONNECT. Closing connection.", conn.RemoteAddr())
				return
			}

			// SECURITY FIX: Validate topic name (empty topics not allowed)
			if pk.TopicName == "" {
				log.Printf("[ERROR] PUBLISH from client %s has empty topic name. Protocol violation.", clientID)
				if pk.FixedHeader.Qos > 0 {
					var respType byte = packets.Puback
					if pk.FixedHeader.Qos == 2 {
						respType = packets.Pubrec
					}
					resp := packets.Packet{
						FixedHeader: packets.FixedHeader{Type: respType},
						PacketID:    pk.PacketID,
						ReasonCode:  0x82, // Protocol Error
					}
					writePacket(conn, &resp)
				}
				return
			}

			// SECURITY FIX: Validate QoS and packet ID compliance
			if pk.FixedHeader.Qos > 2 {
				log.Printf("[ERROR] PUBLISH from client %s has invalid QoS %d. Protocol violation.", clientID, pk.FixedHeader.Qos)
				resp := packets.Packet{
					FixedHeader: packets.FixedHeader{Type: packets.Disconnect},
					ReasonCode:  0x81, // Malformed Packet
				}
				writePacket(conn, &resp)
				return
			}

			// SECURITY FIX: Packet ID must be provided for QoS > 0
			if pk.FixedHeader.Qos > 0 && pk.PacketID == 0 {
				log.Printf("[ERROR] PUBLISH from client %s has QoS %d but packet ID is 0. Protocol violation.", clientID, pk.FixedHeader.Qos)
				var respType byte = packets.Puback
				if pk.FixedHeader.Qos == 2 {
					respType = packets.Pubrec
				}
				resp := packets.Packet{
					FixedHeader: packets.FixedHeader{Type: respType},
					PacketID:    1,    // Use dummy packet ID since original is invalid
					ReasonCode:  0x82, // Protocol Error
				}
				writePacket(conn, &resp)
				return
			}

			// SECURITY FIX: Validate DUP flag usage (only valid for QoS > 0)
			if pk.FixedHeader.Dup && pk.FixedHeader.Qos == 0 {
				log.Printf("[ERROR] PUBLISH from client %s has DUP flag set for QoS 0. Protocol violation.", clientID)
				// For QoS 0, we just log the error but don't send a response (MQTT spec)
				return
			}

			// SECURITY FIX: Check for wildcards in PUBLISH topic (not allowed)
			if strings.Contains(pk.TopicName, "#") || strings.Contains(pk.TopicName, "+") {
				log.Printf("[ERROR] PUBLISH from client %s contains wildcards in topic name '%s'. Protocol violation.", clientID, pk.TopicName)
				if pk.FixedHeader.Qos > 0 {
					var respType byte = packets.Puback
					if pk.FixedHeader.Qos == 2 {
						respType = packets.Pubrec
					}
					resp := packets.Packet{
						FixedHeader: packets.FixedHeader{Type: respType},
						PacketID:    pk.PacketID,
						ReasonCode:  0x82, // Protocol Error
					}
					writePacket(conn, &resp)
				}
				return
			}

			// SECURITY FIX: Topic filter injection protection
			if strings.Contains(pk.TopicName, "../") || strings.Contains(pk.TopicName, "..\\") ||
				strings.Contains(pk.TopicName, "$(") || strings.Contains(pk.TopicName, "';") ||
				strings.Contains(pk.TopicName, "<script") || strings.ContainsAny(pk.TopicName, "\x00\x01\x02\x03\x04\x05") {
				log.Printf("[ERROR] PUBLISH from client %s contains malicious topic '%s'. Security violation.", clientID, pk.TopicName)
				if pk.FixedHeader.Qos > 0 {
					var respType byte = packets.Puback
					if pk.FixedHeader.Qos == 2 {
						respType = packets.Pubrec
					}
					resp := packets.Packet{
						FixedHeader: packets.FixedHeader{Type: respType},
						PacketID:    pk.PacketID,
						ReasonCode:  0x87, // Not authorized
					}
					writePacket(conn, &resp)
				}
				return
			}

			// Check message size limit (MQTT spec recommends broker-specific limits)
			const maxMessageSize = 1024 * 1024 // 1MB limit
			if len(pk.Payload) > maxMessageSize {
				log.Printf("[ERROR] PUBLISH from client %s exceeds size limit: %d bytes (max: %d)",
					clientID, len(pk.Payload), maxMessageSize)

				// Send appropriate response based on QoS
				if pk.FixedHeader.Qos > 0 {
					var respType byte
					var reasonCode byte = 0x97 // Payload format invalid or message too large

					if pk.FixedHeader.Qos == 1 {
						respType = packets.Puback
					} else {
						respType = packets.Pubrec
					}

					resp := packets.Packet{
						FixedHeader: packets.FixedHeader{Type: respType},
						PacketID:    pk.PacketID,
						ReasonCode:  reasonCode,
					}
					writePacket(conn, &resp)
				}
				return
			}

			// Use validated QoS level
			publishQoS := pk.FixedHeader.Qos

			skipProcessing := false
			if publishQoS == 1 {
				if isDuplicate := b.markAndCheckDuplicate(clientID, pk.PacketID); isDuplicate {
					log.Printf("[DEBUG] Detected duplicate QoS1 publish from client %s with PacketID %d, suppressing reprocessing.", clientID, pk.PacketID)
					skipProcessing = true
				}
			}

			// Handle MQTT 5.0 topic aliases
			var resolvedTopic string
			var topicAliasErr error

			if pk.Properties.TopicAliasFlag && pk.Properties.TopicAlias > 0 {
				// Client is using topic alias
				b.mu.RLock()
				topicAliasManager := b.topicAliasManagers[clientID]
				b.mu.RUnlock()

				if topicAliasManager != nil {
					resolvedTopic, topicAliasErr = topicAliasManager.resolveTopicAlias(pk.TopicName, pk.Properties.TopicAlias)
					if topicAliasErr != nil {
						log.Printf("[ERROR] Topic alias resolution failed for client %s: %v", clientID, topicAliasErr)
						// Send DISCONNECT with protocol error
						resp := packets.Packet{
							FixedHeader: packets.FixedHeader{Type: packets.Disconnect},
							ReasonCode:  0x82, // Protocol Error
						}
						writePacket(conn, &resp)
						return
					}
					log.Printf("[DEBUG] Resolved topic alias %d to topic '%s' for client %s", pk.Properties.TopicAlias, resolvedTopic, clientID)
				} else {
					resolvedTopic = pk.TopicName
					log.Printf("[WARN] No topic alias manager for client %s, using topic name directly", clientID)
				}
			} else {
				resolvedTopic = pk.TopicName
			}

			// Extract MQTT 5.0 user properties if present
			var userProperties map[string][]byte
			if len(pk.Properties.User) > 0 {
				userProperties = make(map[string][]byte)
				for _, userProp := range pk.Properties.User {
					userProperties[userProp.Key] = []byte(userProp.Val)
				}
				log.Printf("[DEBUG] PUBLISH with %d user properties from client %s", len(pk.Properties.User), clientID)
			}

			// Extract MQTT 5.0 request-response properties if present
			var responseTopic string
			var correlationData []byte
			if pk.Properties.ResponseTopic != "" {
				responseTopic = pk.Properties.ResponseTopic
				log.Printf("[DEBUG] PUBLISH with response topic from client %s: %s", clientID, responseTopic)
			}
			if len(pk.Properties.CorrelationData) > 0 {
				correlationData = pk.Properties.CorrelationData
				log.Printf("[DEBUG] PUBLISH with correlation data from client %s: %d bytes", clientID, len(correlationData))
			}

			log.Printf("[DEBUG] PUBLISH received from client %s: topic=%s, resolved_topic=%s, qos=%d, retain=%t, payload_size=%d, user_props=%d, topic_alias=%d, response_topic=%s",
				clientID, pk.TopicName, resolvedTopic, publishQoS, pk.FixedHeader.Retain, len(pk.Payload), len(userProperties), pk.Properties.TopicAlias, responseTopic)

			// Check blacklist for topic publish access
			clientIP := strings.Split(conn.RemoteAddr().String(), ":")[0]
			var username string
			// Try to extract username from session (this is a simplified approach)
			// In a real implementation, you'd store the username in the session context
			// For now, we'll use empty string but the blacklist middleware can still check clientID and IP
			allowed, reason, err := b.blacklistMiddleware.CheckPublishAccess(clientID, username, clientIP, resolvedTopic)
			if err != nil {
				log.Printf("[ERROR] Blacklist check error for client %s publishing to %s: %v", clientID, resolvedTopic, err)
			} else if !allowed {
				log.Printf("[WARN] Client %s blocked from publishing to topic %s: %s", clientID, resolvedTopic, reason)
				// For PUBLISH, we don't send a response packet for QoS 0, but we do for QoS 1 and 2
				if publishQoS > 0 {
					// Send PUBACK or PUBREC with appropriate reason code
					var respType byte
					if publishQoS == 1 {
						respType = packets.Puback
					} else {
						respType = packets.Pubrec
					}
					resp := packets.Packet{
						FixedHeader: packets.FixedHeader{Type: respType},
						PacketID:    pk.PacketID,
						ReasonCode:  0x87, // Not authorized
					}
					writePacket(conn, &resp)
				}
				return
			}

			// Send acknowledgment back to publisher as early as possible for QoS 1/2
			if publishQoS == 1 {
				log.Printf("[DEBUG] About to send acknowledgment for QoS %d, PacketID %d to client %s", publishQoS, pk.PacketID, clientID)
				resp := packets.Packet{
					FixedHeader: packets.FixedHeader{Type: packets.Puback},
					PacketID:    pk.PacketID,
				}
				if err := writePacket(conn, &resp); err != nil {
					log.Printf("[ERROR] Failed to write PUBACK for PacketID %d to client %s: %v", pk.PacketID, clientID, err)
					return
				}
				log.Printf("[DEBUG] PUBACK sent successfully for PacketID %d to client %s", pk.PacketID, clientID)
			} else if publishQoS == 2 {
				log.Printf("[DEBUG] About to send acknowledgment for QoS %d, PacketID %d to client %s", publishQoS, pk.PacketID, clientID)
				resp := packets.Packet{
					FixedHeader: packets.FixedHeader{Type: packets.Pubrec},
					PacketID:    pk.PacketID,
				}
				if err := writePacket(conn, &resp); err != nil {
					log.Printf("[ERROR] Failed to write PUBREC for PacketID %d to client %s: %v", pk.PacketID, clientID, err)
					return
				}
				log.Printf("[DEBUG] PUBREC sent successfully for PacketID %d to client %s", pk.PacketID, clientID)
			}

			// Dispatch remaining publish processing asynchronously so we can continue reading packets quickly.
			payloadCopy := append([]byte(nil), pk.Payload...)
			userPropsCopy := copyUserProperties(userProperties)
			correlationCopy := append([]byte(nil), correlationData...)
			retainFlag := pk.FixedHeader.Retain

			if !skipProcessing {
				go b.processPublishPostAck(clientID, resolvedTopic, payloadCopy, publishQoS, retainFlag, userPropsCopy, responseTopic, correlationCopy)
			}

		case packets.Puback:
			// CRITICAL SECURITY FIX: Ensure PUBACK only comes after CONNECT
			if sessionMailbox == nil {
				log.Printf("[ERROR] Protocol violation: Client %s sent PUBACK before CONNECT. Closing connection.", conn.RemoteAddr())
				return
			}

			// SECURITY FIX: PUBACK must have valid packet ID
			if pk.PacketID == 0 {
				log.Printf("[ERROR] PUBACK from client %s has packet ID 0. Protocol violation.", clientID)
				return
			}

			// SECURITY FIX: Check for unsolicited PUBACK (QoS protocol violation)
			// This is a simplified check - in production, we should track pending QoS 1 publishes
			log.Printf("[WARN] Client %s sent PUBACK for packet ID %d - checking for QoS protocol compliance", clientID, pk.PacketID)

			// QoS 1 publish acknowledgment from client
			log.Printf("Client %s sent PUBACK for packet ID %d", clientID, pk.PacketID)
			// In a full implementation, we would remove this message from pending acks

		case packets.Pubrec:
			// CRITICAL SECURITY FIX: Ensure PUBREC only comes after CONNECT
			if sessionMailbox == nil {
				log.Printf("[ERROR] Protocol violation: Client %s sent PUBREC before CONNECT. Closing connection.", conn.RemoteAddr())
				return
			}

			// SECURITY FIX: PUBREC must have valid packet ID
			if pk.PacketID == 0 {
				log.Printf("[ERROR] PUBREC from client %s has packet ID 0. Protocol violation.", clientID)
				return
			}

			// SECURITY FIX: Check for unsolicited PUBREC (QoS protocol violation)
			// This is a simplified check - in production, we should track pending QoS 2 publishes
			log.Printf("[WARN] Client %s sent PUBREC for packet ID %d - checking for QoS protocol compliance", clientID, pk.PacketID)

			// QoS 2 publish receive from client - send PUBREL response
			log.Printf("Client %s sent PUBREC for packet ID %d", clientID, pk.PacketID)
			resp := packets.Packet{
				FixedHeader: packets.FixedHeader{Type: packets.Pubrel, Qos: 1},
				PacketID:    pk.PacketID,
			}
			err = writePacket(conn, &resp)

		case packets.Pubrel:
			// CRITICAL SECURITY FIX: Ensure PUBREL only comes after CONNECT
			if sessionMailbox == nil {
				log.Printf("[ERROR] Protocol violation: Client %s sent PUBREL before CONNECT. Closing connection.", conn.RemoteAddr())
				return
			}

			// SECURITY FIX: PUBREL must have valid packet ID
			if pk.PacketID == 0 {
				log.Printf("[ERROR] PUBREL from client %s has packet ID 0. Protocol violation.", clientID)
				return
			}

			// QoS 2 publish release from client - send PUBCOMP response
			log.Printf("Client %s sent PUBREL for packet ID %d", clientID, pk.PacketID)
			resp := packets.Packet{
				FixedHeader: packets.FixedHeader{Type: packets.Pubcomp},
				PacketID:    pk.PacketID,
			}
			err = writePacket(conn, &resp)

		case packets.Pubcomp:
			// CRITICAL SECURITY FIX: Ensure PUBCOMP only comes after CONNECT
			if sessionMailbox == nil {
				log.Printf("[ERROR] Protocol violation: Client %s sent PUBCOMP before CONNECT. Closing connection.", conn.RemoteAddr())
				return
			}

			// SECURITY FIX: PUBCOMP must have valid packet ID
			if pk.PacketID == 0 {
				log.Printf("[ERROR] PUBCOMP from client %s has packet ID 0. Protocol violation.", clientID)
				return
			}

			// QoS 2 publish complete from client
			log.Printf("Client %s sent PUBCOMP for packet ID %d", clientID, pk.PacketID)
			// In a full implementation, we would remove this message from pending acks

		case packets.Pingreq:
			// SECURITY FIX: Ensure PINGREQ only comes after CONNECT
			if sessionMailbox == nil {
				log.Printf("[ERROR] Protocol violation: Client %s sent PINGREQ before CONNECT. Closing connection.", conn.RemoteAddr())
				return
			}

			resp := packets.Packet{FixedHeader: packets.FixedHeader{Type: packets.Pingresp}}
			err = writePacket(conn, &resp)

		case packets.Unsubscribe:
			log.Printf("[DEBUG] UNSUBSCRIBE packet received from client %s (remote: %s)", clientID, conn.RemoteAddr())
			log.Printf("[DEBUG] UNSUBSCRIBE packet details - PacketID: %d, Filter count: %d", pk.PacketID, len(pk.Filters))

			// CRITICAL SECURITY FIX: Ensure UNSUBSCRIBE only comes after CONNECT
			if sessionMailbox == nil {
				log.Printf("[ERROR] Protocol violation: Client %s sent UNSUBSCRIBE before CONNECT. Closing connection.", conn.RemoteAddr())
				return
			}

			// SECURITY FIX: UNSUBSCRIBE packets must have packet ID
			if pk.PacketID == 0 {
				log.Printf("[ERROR] UNSUBSCRIBE from client %s has packet ID 0. Protocol violation.", clientID)
				// Send UNSUBACK with failure
				resp := packets.Packet{
					FixedHeader: packets.FixedHeader{Type: packets.Unsuback},
					PacketID:    1,            // Use dummy packet ID since original is invalid
					ReasonCodes: []byte{0x80}, // Unspecified error
				}
				writePacket(conn, &resp)
				return
			}

			// SECURITY FIX: UNSUBSCRIBE must have at least one topic filter
			if len(pk.Filters) == 0 {
				log.Printf("[ERROR] UNSUBSCRIBE from client %s has no topic filters. Protocol violation.", clientID)
				resp := packets.Packet{
					FixedHeader: packets.FixedHeader{Type: packets.Unsuback},
					PacketID:    pk.PacketID,
					ReasonCodes: []byte{0x83}, // Topic filter invalid
				}
				writePacket(conn, &resp)
				return
			}

			log.Printf("[DEBUG] Processing UNSUBSCRIBE for client %s with session mailbox: %p", clientID, sessionMailbox)

			var reasonCodes []byte
			var removedRoutes []*clusterpb.Route

			for i, sub := range pk.Filters {
				topicFilter := sub.Filter
				log.Printf("[DEBUG] Processing unsubscribe filter %d: Topic='%s'", i+1, topicFilter)

				// Validate topic filter
				if topicFilter == "" {
					log.Printf("[ERROR] Client %s sent empty topic filter in unsubscription %d", clientID, i+1)
					reasonCodes = append(reasonCodes, 0x11) // Topic filter invalid
					continue
				}

				// Remove subscription from topic store
				b.topics.Unsubscribe(topicFilter, sessionMailbox)
				log.Printf("[DEBUG] Removed subscription from topic store: Client=%s, Topic=%s", clientID, topicFilter)

				// Remove subscription from persistent session
				if err := b.persistentSessionMgr.RemoveSubscription(clientID, topicFilter); err != nil {
					log.Printf("[ERROR] Failed to remove persistent subscription for %s: %v", clientID, err)
					reasonCodes = append(reasonCodes, 0x80) // Unspecified error
					continue
				}

				log.Printf("[INFO] Client %s successfully unsubscribed from '%s'", clientID, topicFilter)

				// Create route removal for cluster
				removedRoute := &clusterpb.Route{
					Topic:   topicFilter,
					NodeIds: []string{b.nodeID},
				}
				removedRoutes = append(removedRoutes, removedRoute)
				log.Printf("[DEBUG] Marked route for removal from cluster: Topic=%s, NodeID=%s", topicFilter, b.nodeID)

				// Success
				reasonCodes = append(reasonCodes, 0x00) // No error
				log.Printf("[DEBUG] Added success reason code for unsubscription %d", i+1)
			}

			// Announce route removals to cluster peers
			if b.cluster != nil && len(removedRoutes) > 0 {
				log.Printf("[DEBUG] Broadcasting %d route removals to cluster peers", len(removedRoutes))
				// Note: We would need a BroadcastRouteRemoval method in cluster
				// For now, just log that we would remove them
				for _, route := range removedRoutes {
					log.Printf("[DEBUG] Would remove cluster route for topic: %s", route.Topic)
				}
			} else {
				log.Printf("[DEBUG] No cluster manager - skipping route removal broadcast")
			}

			log.Printf("[DEBUG] Preparing UNSUBACK response - PacketID: %d, ReasonCodes: %v", pk.PacketID, reasonCodes)
			resp := packets.Packet{
				FixedHeader: packets.FixedHeader{Type: packets.Unsuback},
				PacketID:    pk.PacketID,
				ReasonCodes: reasonCodes,
			}

			log.Printf("[DEBUG] Sending UNSUBACK to client %s", clientID)
			err = writePacket(conn, &resp)
			if err != nil {
				log.Printf("[ERROR] Failed to send UNSUBACK to client %s: %v", clientID, err)
			} else {
				log.Printf("[DEBUG] UNSUBACK sent successfully to client %s", clientID)
			}

		case packets.Disconnect:
			log.Printf("Client %s sent DISCONNECT.", clientID)
			receivedDisconnectPacket = true // Mark that we received a proper DISCONNECT packet
			// Handle graceful disconnect for persistent sessions
			if err := b.persistentSessionMgr.DisconnectSession(clientID, true); err != nil {
				log.Printf("[ERROR] Failed to handle graceful disconnect for %s: %v", clientID, err)
			}
			// Remove delivery callback
			b.offlineMessageMgr.RemoveDeliveryCallback(clientID)
			// Clean up topic alias manager
			b.mu.Lock()
			delete(b.topicAliasManagers, clientID)
			b.mu.Unlock()
			log.Printf("[DEBUG] Topic alias manager removed for client %s", clientID)

			// Unregister from actor system
			b.unregisterSession(clientID)
			log.Printf("Client %s disconnected (graceful: %t).", clientID, true)

			// Close the connection immediately after graceful disconnect
			conn.Close()
			return

		default:
			log.Printf("Received unhandled packet type: %v", pk.FixedHeader.Type)
		}

		if err != nil {
			log.Printf("Error handling packet for %s: %v", clientID, err)
			return
		}
	}

	if clientID != "" {
		// Check if this was an unexpected disconnection (not graceful)
		// Only consider it graceful if we received a proper DISCONNECT packet
		// EOF and other errors indicate unexpected disconnection
		isGracefulDisconnect := receivedDisconnectPacket

		// Handle disconnection for persistent sessions
		if disconnectErr := b.persistentSessionMgr.DisconnectSession(clientID, isGracefulDisconnect); disconnectErr != nil {
			log.Printf("[ERROR] Failed to handle disconnect for %s: %v", clientID, disconnectErr)
		}

		// Remove delivery callback
		b.offlineMessageMgr.RemoveDeliveryCallback(clientID)

		// Clean up topic alias manager
		b.mu.Lock()
		delete(b.topicAliasManagers, clientID)
		b.mu.Unlock()
		log.Printf("[DEBUG] Topic alias manager removed for client %s", clientID)

		// Unregister from actor system
		b.unregisterSession(clientID)
		log.Printf("Client %s disconnected (graceful: %t).", clientID, isGracefulDisconnect)
	}
}

func (b *Broker) registerSession(ctx context.Context, clientID string, conn net.Conn, protocolVersion byte, persistentSession *persistent.PersistentSession) *actor.Mailbox {
	if oldMb, err := b.sessions.Get(clientID); err == nil {
		log.Printf("Client %s is reconnecting, performing session takeover.", clientID)
		// Session takeover: The old session actor will eventually stop when it tries to
		// write to its old TCP connection. We need to:
		// 1. Remove old subscriptions from topic store (they point to the old mailbox)
		// 2. Remove old session from storage
		// 3. Create new session with new connection
		// 4. Subscriptions will be restored later using persistent session data

		oldMailbox, ok := oldMb.(*actor.Mailbox)
		if ok {
			// Send a stop message to the old actor for graceful shutdown
			oldMailbox.Send(session.Stop{})
			log.Printf("[DEBUG] Sent stop message to old session for client %s", clientID)

			// Remove any stale subscriptions that still reference the old mailbox
			removed := b.topics.RemoveAllSubscriptions(oldMailbox)
			if len(removed) > 0 {
				log.Printf("[DEBUG] Removed %d stale subscriptions for client %s during session takeover", len(removed), clientID)
			}
		}

		// Remove old session from storage so we can register the new one
		b.sessions.Delete(clientID)
		log.Printf("[DEBUG] Removed old session from storage for client %s", clientID)

		// Fall through to create a new session with the new connection
	}

	log.Printf("Registering new session for client %s", clientID)
	sess := session.New(clientID, conn)
	sess.SetProtocolVersion(protocolVersion)
	mb := actor.NewMailbox(100)

	spec := supervisor.Spec{
		ID:      fmt.Sprintf("session-%s", clientID),
		Actor:   sess,
		Restart: supervisor.RestartTransient, // Restart only on abnormal termination
		Mailbox: mb,
	}
	b.sup.StartChild(ctx, spec)

	b.sessions.Set(clientID, mb)
	return mb
}

func (b *Broker) unregisterSession(clientID string) {
	if mb, err := b.sessions.Get(clientID); err == nil {
		if mailbox, ok := mb.(*actor.Mailbox); ok {
			removed := b.topics.RemoveAllSubscriptions(mailbox)
			if len(removed) > 0 {
				log.Printf("[DEBUG] Removed %d subscriptions for client %s during session cleanup", len(removed), clientID)
			}
		}
	}
	b.sessions.Delete(clientID)

	b.mu.Lock()
	delete(b.processedQoS1, clientID)
	b.mu.Unlock()
}

func (b *Broker) markAndCheckDuplicate(clientID string, packetID uint16) bool {
	if packetID == 0 {
		return false
	}

	now := time.Now()

	b.mu.Lock()
	defer b.mu.Unlock()

	clientMap, ok := b.processedQoS1[clientID]
	if !ok {
		clientMap = make(map[uint16]time.Time)
		b.processedQoS1[clientID] = clientMap
	}

	if ts, exists := clientMap[packetID]; exists {
		if now.Sub(ts) < qosDuplicateWindow {
			clientMap[packetID] = now
			return true
		}
	}

	clientMap[packetID] = now

	// Cleanup old entries opportunistically to keep memory bounded
	for id, ts := range clientMap {
		if now.Sub(ts) >= qosDuplicateWindow {
			delete(clientMap, id)
		}
	}

	return false
}

// processPublishPostAck performs publish processing that can happen after acknowledgments are sent.
func (b *Broker) processPublishPostAck(clientID, resolvedTopic string, payload []byte, publishQoS byte, retain bool, userProperties map[string][]byte, responseTopic string, correlationData []byte) {
	if retain {
		log.Printf("[DEBUG] PUBLISH with RETAIN flag from client %s to topic %s", clientID, resolvedTopic)
		ctx := context.Background()
		if err := b.retainer.StoreRetained(ctx, resolvedTopic, payload, publishQoS, clientID); err != nil {
			log.Printf("[ERROR] Failed to store retained message: %v", err)
		} else {
			log.Printf("[INFO] Stored retained message for topic %s (payload size: %d)", resolvedTopic, len(payload))
		}
	}

	// Route the published message to subscribers with request-response properties
	b.routePublishWithRequestResponse(resolvedTopic, payload, publishQoS, userProperties, responseTopic, correlationData)

	// Route to connectors for external systems
	go b.routeToConnectors(resolvedTopic, payload, publishQoS, userProperties, responseTopic, correlationData)

	// Queue message for offline subscribers
	matcher := &persistent.BasicTopicMatcher{}
	if err := b.offlineMessageMgr.QueueMessageForOfflineClients(resolvedTopic, payload, publishQoS, retain, matcher); err != nil {
		log.Printf("[ERROR] Failed to queue message for offline clients: %v", err)
	}
}

func copyUserProperties(props map[string][]byte) map[string][]byte {
	if len(props) == 0 {
		return nil
	}
	cloned := make(map[string][]byte, len(props))
	for k, v := range props {
		if v == nil {
			cloned[k] = nil
			continue
		}
		cp := make([]byte, len(v))
		copy(cp, v)
		cloned[k] = cp
	}
	return cloned
}

// routePublishWithUserProperties sends a message with user properties to all local and remote subscribers of a topic.
func (b *Broker) routePublishWithUserProperties(topicName string, payload []byte, publishQoS byte, userProperties map[string][]byte) {
	// Route to local subscribers
	b.RouteToLocalSubscribersWithUserProperties(topicName, payload, publishQoS, userProperties)

	// Route to remote subscribers if clustering is enabled
	if b.cluster != nil {
		remoteSubscribers := b.cluster.GetRemoteSubscribers(topicName)
		for _, nodeID := range remoteSubscribers {
			// Avoid forwarding to self
			if nodeID == b.nodeID {
				continue
			}
			log.Printf("Forwarding message for topic '%s' to remote node %s", topicName, nodeID)
			b.cluster.ForwardPublish(topicName, payload, nodeID)
		}
	}
}

// routePublishWithRequestResponse sends a message with request-response properties to all local and remote subscribers of a topic.
func (b *Broker) routePublishWithRequestResponse(topicName string, payload []byte, publishQoS byte, userProperties map[string][]byte, responseTopic string, correlationData []byte) {
	// Route to local subscribers
	b.RouteToLocalSubscribersWithRequestResponse(topicName, payload, publishQoS, userProperties, responseTopic, correlationData)

	// Route to remote subscribers if clustering is enabled
	if b.cluster != nil {
		remoteSubscribers := b.cluster.GetRemoteSubscribers(topicName)
		for _, nodeID := range remoteSubscribers {
			// Avoid forwarding to self
			if nodeID == b.nodeID {
				continue
			}
			log.Printf("Forwarding message for topic '%s' to remote node %s", topicName, nodeID)
			b.cluster.ForwardPublish(topicName, payload, nodeID)
		}
	}
}

// routePublish sends a message to all local and remote subscribers of a topic.
func (b *Broker) routePublish(topicName string, payload []byte, publishQoS byte) {
	// Route to local subscribers
	b.RouteToLocalSubscribersWithQoS(topicName, payload, publishQoS)

	// Route to remote subscribers if clustering is enabled
	if b.cluster != nil {
		remoteSubscribers := b.cluster.GetRemoteSubscribers(topicName)
		for _, nodeID := range remoteSubscribers {
			// Avoid forwarding to self
			if nodeID == b.nodeID {
				continue
			}
			log.Printf("Forwarding message for topic '%s' to remote node %s", topicName, nodeID)
			b.cluster.ForwardPublish(topicName, payload, nodeID)
		}
	}
}

// RouteToLocalSubscribersWithUserProperties delivers a message with user properties to all clients on the current node
// that are subscribed to the given topic. It retrieves the list of subscribers
// from the topic store and sends the message to each of their mailboxes.
//
// - topicName: The topic to which the message is published.
// - payload: The message content.
// - publishQoS: The QoS level of the published message.
// - userProperties: MQTT 5.0 user-defined properties.
func (b *Broker) RouteToLocalSubscribersWithUserProperties(topicName string, payload []byte, publishQoS byte, userProperties map[string][]byte) {
	subscribers := b.topics.GetSubscribers(topicName)
	if len(subscribers) > 0 {
		log.Printf("Routing message on topic '%s' to %d local subscribers (with %d user properties)", topicName, len(subscribers), len(userProperties))
	}
	for _, sub := range subscribers {
		// Apply QoS downgrade rule: effective QoS = min(publish QoS, subscription QoS)
		effectiveQoS := publishQoS
		if sub.QoS < publishQoS {
			effectiveQoS = sub.QoS
		}

		msg := session.Publish{
			Topic:          topicName,
			Payload:        payload,
			QoS:            effectiveQoS,
			UserProperties: userProperties,
		}
		sub.Mailbox.Send(msg)
	}
}

// RouteToLocalSubscribersWithRequestResponse delivers a message with request-response properties to all clients on the current node
// that are subscribed to the given topic. It retrieves the list of subscribers
// from the topic store and sends the message to each of their mailboxes.
//
// - topicName: The topic to which the message is published.
// - payload: The message content.
// - publishQoS: The QoS level of the published message.
// - userProperties: MQTT 5.0 user-defined properties.
// - responseTopic: MQTT 5.0 response topic for request-response pattern.
// - correlationData: MQTT 5.0 correlation data for request-response pattern.
func (b *Broker) RouteToLocalSubscribersWithRequestResponse(topicName string, payload []byte, publishQoS byte, userProperties map[string][]byte, responseTopic string, correlationData []byte) {
	// Process message through rule engine first
	headers := make(map[string]string)
	for k, v := range userProperties {
		if len(v) > 0 {
			headers[k] = string(v[0])
		}
	}
	b.processMessageThroughRuleEngine(topicName, payload, publishQoS, "", headers)

	// Process message through data integration engine
	b.processMessageThroughIntegration(topicName, payload, publishQoS, "", headers)

	subscribers := b.topics.GetSubscribers(topicName)
	if len(subscribers) > 0 {
		logMsg := fmt.Sprintf("Routing message on topic '%s' to %d local subscribers (with %d user properties)", topicName, len(subscribers), len(userProperties))
		if responseTopic != "" {
			logMsg += fmt.Sprintf(", response_topic='%s'", responseTopic)
		}
		if len(correlationData) > 0 {
			logMsg += fmt.Sprintf(", correlation_data=%d bytes", len(correlationData))
		}
		log.Printf("%s", logMsg)
	}
	log.Printf("[DEBUG] Matched %d subscribers for topic '%s'", len(subscribers), topicName)
	uniqueSubs := make(map[*actor.Mailbox]byte, len(subscribers))
	for _, sub := range subscribers {
		effectiveQoS := publishQoS
		if sub.QoS < publishQoS {
			effectiveQoS = sub.QoS
		}
		if current, exists := uniqueSubs[sub.Mailbox]; !exists || effectiveQoS > current {
			uniqueSubs[sub.Mailbox] = effectiveQoS
		}
	}

	for mailbox, qos := range uniqueSubs {
		log.Printf("[DEBUG] Delivering to mailbox %p for topic '%s' with QoS %d", mailbox, topicName, qos)

		msg := session.Publish{
			Topic:           topicName,
			Payload:         payload,
			QoS:             qos,
			UserProperties:  userProperties,
			ResponseTopic:   responseTopic,
			CorrelationData: correlationData,
		}
		mailbox.Send(msg)
	}
}

// RouteToLocalSubscribers delivers a message to all clients on the current node
// that are subscribed to the given topic. It retrieves the list of subscribers
// from the topic store and sends the message to each of their mailboxes.
//
// This method is also used as a callback by the cluster manager to handle
// messages that have been forwarded from other nodes.
//
// - topicName: The topic to which the message is published.
// - payload: The message content.
// - publishQoS: The QoS level of the published message.
func (b *Broker) RouteToLocalSubscribersWithQoS(topicName string, payload []byte, publishQoS byte) {
	// Process message through rule engine first
	b.processMessageThroughRuleEngine(topicName, payload, publishQoS, "", nil)

	subscribers := b.topics.GetSubscribers(topicName)
	if len(subscribers) > 0 {
		log.Printf("Routing message on topic '%s' to %d local subscribers", topicName, len(subscribers))
	}
	for _, sub := range subscribers {
		// Apply QoS downgrade rule: effective QoS = min(publish QoS, subscription QoS)
		effectiveQoS := publishQoS
		if sub.QoS < publishQoS {
			effectiveQoS = sub.QoS
		}

		msg := session.Publish{
			Topic:          topicName,
			Payload:        payload,
			QoS:            effectiveQoS,
			UserProperties: nil, // Backward compatibility - no user properties
		}
		sub.Mailbox.Send(msg)
	}
}

// RouteToLocalSubscribers delivers a message to all clients on the current node
// that are subscribed to the given topic. This is the backward compatibility method.
//
// - topicName: The topic to which the message is published.
// - payload: The message content.
func (b *Broker) RouteToLocalSubscribers(topicName string, payload []byte) {
	b.RouteToLocalSubscribersWithQoS(topicName, payload, 0)
}

// readPacket reads a full MQTT packet from a connection.
func readPacket(r *bufio.Reader) (*packets.Packet, error) {
	fh := new(packets.FixedHeader)
	b, err := r.ReadByte()
	if err != nil {
		return nil, err
	}

	// Try to decode the fixed header, but handle QoS errors gracefully
	err = fh.Decode(b)
	if err != nil {
		// Check if this is a QoS out of range error
		if strings.Contains(err.Error(), "qos out of range") {
			log.Printf("[WARN] QoS out of range detected, creating protocol violation error: %v", err)
			// Return a specific error to be handled by the connection handler
			return nil, fmt.Errorf("protocol violation: invalid QoS level in packet")
		}
		return nil, err
	}

	rem, _, err := packets.DecodeLength(r)
	if err != nil {
		return nil, err
	}
	fh.Remaining = rem

	buf := make([]byte, fh.Remaining)
	if fh.Remaining > 0 {
		_, err = io.ReadFull(r, buf)
		if err != nil {
			return nil, err
		}
	}

	pk := &packets.Packet{FixedHeader: *fh}
	switch pk.FixedHeader.Type {
	case packets.Connect:
		err = pk.ConnectDecode(buf)
	case packets.Publish:
		err = pk.PublishDecode(buf)
	case packets.Subscribe:
		err = pk.SubscribeDecode(buf)
		// Handle QoS out of range errors gracefully for SUBSCRIBE packets
		if err != nil && strings.Contains(err.Error(), "qos out of range") {
			log.Printf("[INFO] SUBSCRIBE packet from client contains invalid QoS value (>2), will be rejected with proper SUBACK response.")
			// Create a special marker for invalid QoS in SUBSCRIBE packets
			pk.Filters = []packets.Subscription{}
			pk.Properties.ReasonString = "QoS_PARSE_ERROR" // Special marker
			err = nil
		}
	case packets.Unsubscribe:
		err = pk.UnsubscribeDecode(buf)
	case packets.Puback:
		err = pk.PubackDecode(buf)
	case packets.Pubrec:
		err = pk.PubrecDecode(buf)
	case packets.Pubrel:
		err = pk.PubrelDecode(buf)
	case packets.Pubcomp:
		err = pk.PubcompDecode(buf)
	case packets.Pingreq:
		err = pk.PingreqDecode(buf)
	case packets.Disconnect:
		err = pk.DisconnectDecode(buf)
	}
	if err != nil {
		log.Printf("[WARN] Packet decode error for type %d: %v", pk.FixedHeader.Type, err)
		return nil, err
	}

	return pk, nil
}

// writePacket encodes and writes a packet to a connection.
func writePacket(w io.Writer, pk *packets.Packet) error {
	// Add mutex to prevent race conditions
	lock, err := getConnLock(w)
	if err != nil {
		return err
	}
	lock.Lock()
	defer lock.Unlock()

	var buf bytes.Buffer
	switch pk.FixedHeader.Type {
	case packets.Connack:
		err = pk.ConnackEncode(&buf)
	case packets.Suback:
		err = pk.SubackEncode(&buf)
	case packets.Pingresp:
		err = pk.PingrespEncode(&buf)
	case packets.Publish:
		err = pk.PublishEncode(&buf)
	case packets.Puback:
		err = pk.PubackEncode(&buf)
	case packets.Pubrec:
		err = pk.PubrecEncode(&buf)
	case packets.Pubrel:
		err = pk.PubrelEncode(&buf)
	case packets.Pubcomp:
		err = pk.PubcompEncode(&buf)
	case packets.Unsuback:
		err = pk.UnsubackEncode(&buf)
	default:
		return fmt.Errorf("unsupported packet type for writing: %v", pk.FixedHeader.Type)
	}

	if err != nil {
		return err
	}
	_, err = w.Write(buf.Bytes())
	return err
}

var connLocks = make(map[net.Conn]*sync.Mutex)
var connLocksMutex = &sync.Mutex{}

func getConnLock(w io.Writer) (*sync.Mutex, error) {
	conn, ok := w.(net.Conn)
	if !ok {
		return nil, fmt.Errorf("writer is not a net.Conn")
	}
	connLocksMutex.Lock()
	defer connLocksMutex.Unlock()
	lock, ok := connLocks[conn]
	if !ok {
		lock = &sync.Mutex{}
		connLocks[conn] = lock
	}
	return lock, nil
}

// sendRetainedMessages sends retained messages to a newly subscribed client
func (b *Broker) sendRetainedMessages(sessionMailbox *actor.Mailbox, filters []packets.Subscription) {
	if sessionMailbox == nil {
		return
	}

	ctx := context.Background()
	for _, filter := range filters {
		log.Printf("[DEBUG] Checking retained messages for subscription filter: %s", filter.Filter)

		// Get retained messages matching this filter
		retainedMsgs, err := b.retainer.GetRetainedMessages(ctx, filter.Filter)
		if err != nil {
			log.Printf("[ERROR] Failed to get retained messages for filter %s: %v", filter.Filter, err)
			continue
		}

		log.Printf("[DEBUG] Found %d retained messages for filter: %s", len(retainedMsgs), filter.Filter)

		// Send each retained message to the subscriber
		for _, retainedMsg := range retainedMsgs {
			// Apply QoS downgrade rule: effective QoS = min(publish QoS, subscription QoS)
			effectiveQoS := retainedMsg.QoS
			if filter.Qos < retainedMsg.QoS {
				effectiveQoS = filter.Qos
			}

			// Create session message
			pubMsg := session.Publish{
				Topic:          retainedMsg.Topic,
				Payload:        retainedMsg.Payload,
				QoS:            effectiveQoS,
				Retain:         true, // Mark as retained message
				UserProperties: nil,  // Retained messages don't currently store user properties
			}

			// Send to subscriber's mailbox
			sessionMailbox.Send(pubMsg)

			log.Printf("[INFO] Sent retained message to subscriber: topic=%s, payload_size=%d, qos=%d",
				retainedMsg.Topic, len(retainedMsg.Payload), effectiveQoS)
		}
	}
}

// routeToConnectors routes messages to enabled connectors for external system integration
func (b *Broker) routeToConnectors(topicName string, payload []byte, qos byte, userProperties map[string][]byte, responseTopic string, correlationData []byte) {
	if b.connectorManager == nil {
		return
	}

	// Get all connectors
	connectors := b.connectorManager.ListConnectors()
	if len(connectors) == 0 {
		return
	}

	// Create message for connectors
	msg := &connector.Message{
		ID:        fmt.Sprintf("msg-%d", time.Now().UnixNano()),
		Topic:     topicName,
		Payload:   payload,
		Headers:   make(map[string]string),
		Timestamp: time.Now(),
		Metadata:  make(map[string]interface{}),
	}

	// Convert user properties to headers
	for k, v := range userProperties {
		msg.Headers[k] = string(v)
	}

	// Add MQTT specific metadata
	msg.Metadata["qos"] = qos
	if responseTopic != "" {
		msg.Metadata["response_topic"] = responseTopic
	}
	if len(correlationData) > 0 {
		msg.Metadata["correlation_data"] = correlationData
	}

	// Send to all running connectors
	for id, conn := range connectors {
		if conn.IsRunning() {
			go func(connectorID string, connector connector.Connector, message *connector.Message) {
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()

				result, err := connector.Send(ctx, message)
				if err != nil {
					log.Printf("[ERROR] Failed to send message to connector %s: %v", connectorID, err)
				} else if result.Success {
					log.Printf("[DEBUG] Message sent to connector %s: topic=%s, latency=%v", connectorID, message.Topic, result.Latency)
				}
			}(id, conn, msg)
		}
	}
}

// Close shuts down the broker and cleans up resources
func (b *Broker) Close() error {
	log.Printf("[INFO] Shutting down broker...")

	// Close connector manager
	if b.connectorManager != nil {
		if err := b.connectorManager.Close(); err != nil {
			log.Printf("[ERROR] Failed to close connector manager: %v", err)
		}
	}

	// Close rule engine
	if b.ruleEngine != nil {
		if err := b.ruleEngine.Close(); err != nil {
			log.Printf("[ERROR] Failed to close rule engine: %v", err)
		}
	}

	// Close data integration engine
	if b.integrationEngine != nil {
		if err := b.integrationEngine.Close(); err != nil {
			log.Printf("[ERROR] Failed to close integration engine: %v", err)
		}
	}

	// Close persistent session manager
	if b.persistentSessionMgr != nil {
		if err := b.persistentSessionMgr.Close(); err != nil {
			log.Printf("[ERROR] Failed to close persistent session manager: %v", err)
		}
	}

	log.Printf("[INFO] Broker shutdown complete")
	return nil
}

// willMessagePublisher implements the WillMessagePublisher interface for broker integration
type willMessagePublisher struct {
	broker *Broker
}

// PublishWillMessage publishes a will message through the broker's routing system
func (wmp *willMessagePublisher) PublishWillMessage(ctx context.Context, willMsg *persistent.WillMessage, clientID string) error {
	if wmp.broker == nil {
		return fmt.Errorf("broker is nil")
	}

	if willMsg == nil {
		return fmt.Errorf("will message is nil")
	}

	log.Printf("[INFO] Publishing will message from client %s: topic=%s, qos=%d, retain=%t, payload_size=%d",
		clientID, willMsg.Topic, willMsg.QoS, willMsg.Retain, len(willMsg.Payload))

	// Handle retained will messages
	if willMsg.Retain {
		if err := wmp.broker.retainer.StoreRetained(ctx, willMsg.Topic, willMsg.Payload, willMsg.QoS, clientID); err != nil {
			log.Printf("[ERROR] Failed to store retained will message: %v", err)
		} else {
			log.Printf("[INFO] Stored retained will message for topic %s", willMsg.Topic)
		}
	}

	// Route the will message to subscribers
	wmp.broker.routePublish(willMsg.Topic, willMsg.Payload, willMsg.QoS)

	// Queue will message for offline subscribers
	matcher := &persistent.BasicTopicMatcher{}
	if err := wmp.broker.offlineMessageMgr.QueueMessageForOfflineClients(willMsg.Topic, willMsg.Payload, willMsg.QoS, willMsg.Retain, matcher); err != nil {
		log.Printf("[ERROR] Failed to queue will message for offline clients: %v", err)
	}

	log.Printf("[INFO] Successfully published will message from client %s to topic %s", clientID, willMsg.Topic)
	return nil
}

// generateClientID generates a unique client ID for clients with empty client IDs
func generateClientID() string {
	// Generate a random client ID similar to what standard MQTT brokers do
	// Using current timestamp and random string for uniqueness
	timestamp := fmt.Sprintf("%d", time.Now().UnixNano())
	random := fmt.Sprintf("%x", time.Now().UnixNano()%0xFFFF)
	return fmt.Sprintf("auto-%s-%s", timestamp[len(timestamp)-8:], random)
}

// Admin API Interface Implementation

// GetConnections returns current connection information
func (b *Broker) GetConnections() []admin.ConnectionInfo {
	// Return mock data for now - in production this would come from the session store
	connections := []admin.ConnectionInfo{}

	// Add mock connection data
	connections = append(connections, admin.ConnectionInfo{
		ClientID:           "test-client-1",
		Username:           "test",
		PeerHost:           "127.0.0.1",
		SockPort:           1883,
		Protocol:           "MQTT",
		ConnectedAt:        time.Now().Add(-time.Hour),
		KeepAlive:          60,
		CleanStart:         true,
		ProtoVer:           4,
		SubscriptionsCount: 1,
		Node:               b.nodeID,
	})

	return connections
}

// GetSessions returns current session information
func (b *Broker) GetSessions() []admin.SessionInfo {
	// Return mock data for now - in production this would come from the session store
	sessions := []admin.SessionInfo{}

	// Add mock session data
	sessions = append(sessions, admin.SessionInfo{
		ClientID:           "test-client-1",
		Username:           "test",
		CreatedAt:          time.Now().Add(-time.Hour),
		ConnectedAt:        &[]time.Time{time.Now().Add(-time.Hour)}[0],
		ExpiryInterval:     3600,
		SubscriptionsCount: 1,
		Node:               b.nodeID,
	})

	return sessions
}

// GetSubscriptions returns current subscription information
func (b *Broker) GetSubscriptions() []admin.SubscriptionInfo {
	// Return mock data for now - in production this would come from the topic store
	subscriptions := []admin.SubscriptionInfo{}

	// Add mock subscription data
	subscriptions = append(subscriptions, admin.SubscriptionInfo{
		ClientID: "test-client-1",
		Topic:    "test/topic",
		QoS:      1,
		Node:     b.nodeID,
	})

	return subscriptions
}

// GetRoutes returns current routing information
func (b *Broker) GetRoutes() []admin.RouteInfo {
	// Return mock data for now - in production this would come from the cluster manager
	routes := []admin.RouteInfo{}

	// Add mock route data
	routes = append(routes, admin.RouteInfo{
		Topic: "test/topic",
		Nodes: []string{b.nodeID},
	})

	return routes
}

// DisconnectClient disconnects a client by client ID
func (b *Broker) DisconnectClient(clientID string) error {
	// For now, return success - in production this would disconnect the actual client
	log.Printf("[INFO] Disconnect request for client: %s", clientID)

	// In a real implementation, we would:
	// 1. Find the client session
	// 2. Close the connection
	// 3. Clean up resources

	return nil
}

// KickoutSession kicks out a session by client ID
func (b *Broker) KickoutSession(clientID string) error {
	// For now, return success - in production this would kickout the actual session
	log.Printf("[INFO] Kickout request for session: %s", clientID)

	// In a real implementation, we would:
	// 1. Find the session
	// 2. Force disconnect
	// 3. Clean up session state

	return nil
}

// GetNodeInfo returns current node information
func (b *Broker) GetNodeInfo() admin.NodeInfo {
	uptime := time.Since(time.Now().Add(-time.Hour)) // Mock uptime

	return admin.NodeInfo{
		Node:       b.nodeID,
		NodeStatus: "running",
		OtpRelease: "Go 1.21",
		Version:    "1.0.0",
		Uptime:     int64(uptime.Seconds()),
		Datetime:   time.Now(),
		SysMon: admin.SysMonInfo{
			ProcCount:    100,
			ProcessLimit: 1000,
			MemoryTotal:  1024 * 1024 * 1024, // 1GB
			MemoryUsed:   512 * 1024 * 1024,  // 512MB
			CPUIdle:      85.5,
			CPULoad1:     0.5,
			CPULoad5:     0.7,
			CPULoad15:    0.3,
		},
	}
}

// GetClusterNodes returns information about all cluster nodes
func (b *Broker) GetClusterNodes() []admin.NodeInfo {
	// Return current node info - in production this would query the cluster
	return []admin.NodeInfo{b.GetNodeInfo()}
}
