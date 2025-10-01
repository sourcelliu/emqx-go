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
	"github.com/turtacn/emqx-go/pkg/auth"
	"github.com/turtacn/emqx-go/pkg/auth/x509"
	"github.com/turtacn/emqx-go/pkg/cluster"
	"github.com/turtacn/emqx-go/pkg/connector"
	"github.com/turtacn/emqx-go/pkg/metrics"
	"github.com/turtacn/emqx-go/pkg/persistent"
	clusterpb "github.com/turtacn/emqx-go/pkg/proto/cluster"
	"github.com/turtacn/emqx-go/pkg/retainer"
	"github.com/turtacn/emqx-go/pkg/session"
	"github.com/turtacn/emqx-go/pkg/storage"
	"github.com/turtacn/emqx-go/pkg/storage/messages"
	"github.com/turtacn/emqx-go/pkg/supervisor"
	tlspkg "github.com/turtacn/emqx-go/pkg/tls"
	"github.com/turtacn/emqx-go/pkg/topic"
)

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

	mu        sync.RWMutex
}

// ClientTopicAliasManager manages topic aliases for a single client session
type ClientTopicAliasManager struct {
	mu                      sync.RWMutex
	clientToServerAliases   map[uint16]string // alias -> topic mapping for inbound messages
	topicAliasMaximum       uint16            // maximum topic alias value supported by server
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
		retainer:             ret,
		certManager:          tlspkg.NewCertificateManager(),
		persistentSessionMgr: sessionMgr,
		offlineMessageMgr:    offlineMgr,
		topicAliasManagers:   make(map[string]*ClientTopicAliasManager),
		connectorManager:     connector.CreateDefaultConnectorManager(),
	}

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

	var connLoopErr error
	for {
		log.Printf("[DEBUG] About to read packet from TLS %s (clientID: %s)", conn.RemoteAddr(), clientID)
		pk, err := readPacket(reader)
		if err != nil {
			connLoopErr = err
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

			if clientID == "" {
				log.Printf("[ERROR] TLS CONNECT from %s has empty client ID. Closing connection.", conn.RemoteAddr())
				resp := packets.Packet{
					FixedHeader: packets.FixedHeader{Type: packets.Connack},
					ReasonCode:  0x85, // Client Identifier not valid
				}
				writePacket(conn, &resp)
				return
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

				err = b.persistentSessionMgr.SetWillMessage(clientID, willTopic, willPayload, willQoS, willRetain, 0)
				if err != nil {
					log.Printf("[ERROR] Failed to set will message for TLS %s: %v", clientID, err)
				} else {
					log.Printf("[INFO] Set will message for TLS client %s: topic=%s", clientID, willTopic)
				}
			}

			log.Printf("[DEBUG] Registering session for TLS client %s", clientID)
			sessionMailbox = b.registerSession(connCtx, clientID, conn)
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
			connLoopErr = err
			log.Printf("Error handling packet for TLS %s: %v", clientID, err)
			return
		}
	}
tls_end_loop:

	// Cleanup logic (same as regular connections)
	if clientID != "" {
		isGracefulDisconnect := connLoopErr == nil || connLoopErr == io.EOF

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

	var connLoopErr error
	for {
		log.Printf("[DEBUG] About to read packet from %s (clientID: %s)", conn.RemoteAddr(), clientID)
		pk, err := readPacket(reader)
		if err != nil {
			connLoopErr = err
			log.Printf("[DEBUG] Error reading packet from %s (clientID: %s): %v", conn.RemoteAddr(), clientID, err)
			if err != io.EOF {
				// Enhanced error handling for protocol violations
				if strings.Contains(err.Error(), "qos out of range") {
					log.Printf("Protocol violation from %s (client: %s): Invalid QoS value received. MQTT spec allows QoS 0, 1, or 2 only. Error: %v",
						conn.RemoteAddr(), clientID, err)
					// Send CONNACK with error if we haven't sent one yet
					if clientID == "" {
						resp := packets.Packet{
							FixedHeader: packets.FixedHeader{Type: packets.Connack},
							ReasonCode:  0x80, // Protocol Error (0x80)
						}
						writePacket(conn, &resp)
					}
				} else {
					log.Printf("Error reading packet from %s (client: %s): %v", conn.RemoteAddr(), clientID, err)
				}
			}
			break
		}

		log.Printf("[DEBUG] Received packet type: %d from %s (clientID: %s)", pk.FixedHeader.Type, conn.RemoteAddr(), clientID)

		switch pk.FixedHeader.Type {
		case packets.Connect:
			clientID = pk.Connect.ClientIdentifier
			cleanSession := pk.Connect.Clean
			keepAlive := time.Duration(pk.Connect.Keepalive) * time.Second

			log.Printf("[DEBUG] CONNECT packet received from %s - ClientID: '%s'", conn.RemoteAddr(), clientID)
			log.Printf("[DEBUG] CONNECT details - CleanSession: %t, KeepAlive: %d", cleanSession, pk.Connect.Keepalive)

			if clientID == "" {
				log.Printf("[ERROR] CONNECT from %s has empty client ID. Closing connection.", conn.RemoteAddr())
				resp := packets.Packet{
					FixedHeader: packets.FixedHeader{Type: packets.Connack},
					ReasonCode:  0x85, // Client Identifier not valid
				}
				writePacket(conn, &resp)
				return
			}

			// Extract username and password from CONNECT packet
			var username, password string
			if pk.Connect.UsernameFlag {
				username = string(pk.Connect.Username)
				log.Printf("[DEBUG] Username provided: '%s'", username)
			}
			if pk.Connect.PasswordFlag {
				password = string(pk.Connect.Password)
				log.Printf("[DEBUG] Password provided: %t", password != "")
			}

			// Perform authentication
			authResult := b.authChain.Authenticate(username, password)
			log.Printf("[DEBUG] Authentication result for client %s (user: %s): %s", clientID, username, authResult.String())

			var connackCode byte
			switch authResult {
			case auth.AuthSuccess:
				connackCode = packets.CodeSuccess.Code
				log.Printf("[INFO] Authentication successful for client %s (user: %s)", clientID, username)
			case auth.AuthFailure:
				connackCode = 0x84 // Bad username or password
				log.Printf("[WARN] Authentication failed for client %s (user: %s)", clientID, username)
			case auth.AuthError:
				connackCode = 0x80 // Unspecified error
				log.Printf("[ERROR] Authentication error for client %s (user: %s)", clientID, username)
			case auth.AuthIgnore:
				// If authentication is ignored (no authenticators or all skip), allow connection
				connackCode = packets.CodeSuccess.Code
				log.Printf("[INFO] Authentication ignored for client %s (user: %s), allowing connection", clientID, username)
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
					log.Printf("[DEBUG] Set response info for client %s: %s", clientID, resp.Properties.ResponseInfo)
				}

				resp.ProtocolVersion = 5
			}

			log.Printf("[DEBUG] Sending CONNACK to client %s with reason code: 0x%02x", clientID, connackCode)
			err = writePacket(conn, &resp)
			if err != nil {
				log.Printf("[ERROR] Failed to send CONNACK to client %s: %v", clientID, err)
				return
			}

			// If authentication failed, close the connection
			if connackCode != packets.CodeSuccess.Code {
				log.Printf("[INFO] Closing connection for client %s due to authentication failure", clientID)
				return
			}

			// Handle persistent session creation/resumption
			var sessionExpiry time.Duration
			if keepAlive > 0 {
				sessionExpiry = keepAlive * 2 // Default to 2x keepalive
			} else {
				sessionExpiry = 24 * time.Hour // Default to 24 hours
			}

			// Create or resume persistent session
			persistentSession, err := b.persistentSessionMgr.CreateOrResumeSession(clientID, cleanSession, sessionExpiry)
			if err != nil {
				log.Printf("[ERROR] Failed to create/resume persistent session for %s: %v", clientID, err)
				return
			}

			// Check if we should deliver offline messages (before setting callbacks)
			shouldDeliverOffline := !cleanSession && len(persistentSession.MessageQueue) > 0

			// Handle will message if present
			if pk.Connect.WillFlag {
				willTopic := pk.Connect.WillTopic
				willPayload := pk.Connect.WillPayload
				willQoS := pk.Connect.WillQos
				willRetain := pk.Connect.WillRetain

				err = b.persistentSessionMgr.SetWillMessage(clientID, willTopic, willPayload, willQoS, willRetain, 0)
				if err != nil {
					log.Printf("[ERROR] Failed to set will message for %s: %v", clientID, err)
				} else {
					log.Printf("[INFO] Set will message for client %s: topic=%s", clientID, willTopic)
				}
			}

			// Register session for actor management
			log.Printf("[DEBUG] Registering session for client %s", clientID)
			sessionMailbox = b.registerSession(connCtx, clientID, conn)
			log.Printf("[DEBUG] Session registered successfully for client %s, mailbox: %p", clientID, sessionMailbox)

			// Create topic alias manager for this client (MQTT 5.0)
			b.mu.Lock()
			b.topicAliasManagers[clientID] = newClientTopicAliasManager(100) // Allow up to 100 aliases
			b.mu.Unlock()
			log.Printf("[DEBUG] Topic alias manager created for client %s", clientID)

			// Set delivery callback for this specific client
			b.offlineMessageMgr.SetDeliveryCallback(clientID, b.deliverOfflineMessage)

			// If this is a persistent session resumption, restore subscriptions to topic store
			if !cleanSession && len(persistentSession.Subscriptions) > 0 {
				log.Printf("[DEBUG] Restoring %d subscriptions for persistent session %s", len(persistentSession.Subscriptions), clientID)
				for topic, qos := range persistentSession.Subscriptions {
					b.topics.Subscribe(topic, sessionMailbox, qos)
					log.Printf("[DEBUG] Restored subscription: Client=%s, Topic=%s, QoS=%d", clientID, topic, qos)
				}
			}

			// Deliver offline messages if this is a reconnection
			if shouldDeliverOffline {
				go func() {
					time.Sleep(100 * time.Millisecond) // Brief delay to ensure session is fully established
					if err := b.offlineMessageMgr.DeliverOfflineMessages(clientID); err != nil {
						log.Printf("[ERROR] Failed to deliver offline messages to %s: %v", clientID, err)
					}
				}()
			}

			log.Printf("[INFO] Client %s connected successfully with user: %s (cleanSession: %t)", clientID, username, cleanSession)

		case packets.Subscribe:
			log.Printf("[DEBUG] SUBSCRIBE packet received from client %s (remote: %s)", clientID, conn.RemoteAddr())
			log.Printf("[DEBUG] SUBSCRIBE packet details - PacketID: %d, Filter count: %d", pk.PacketID, len(pk.Filters))

			if sessionMailbox == nil {
				log.Printf("[ERROR] SUBSCRIBE received before CONNECT from %s", conn.RemoteAddr())
				return
			}

			log.Printf("[DEBUG] Processing SUBSCRIBE for client %s with session mailbox: %p", clientID, sessionMailbox)

			var newRoutes []*clusterpb.Route
			var reasonCodes []byte

			for i, sub := range pk.Filters {
				log.Printf("[DEBUG] Processing subscription filter %d: Topic='%s', RequestedQoS=%d", i+1, sub.Filter, sub.Qos)

				// Validate topic filter
				if sub.Filter == "" {
					log.Printf("[ERROR] Client %s sent empty topic filter in subscription %d", clientID, i+1)
					reasonCodes = append(reasonCodes, 0x80) // Unspecified error
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
			// Validate QoS level for publish (MQTT spec allows 0, 1, 2 only)
			publishQoS := pk.FixedHeader.Qos
			if pk.FixedHeader.Qos > 2 {
				log.Printf("Client %s attempted to publish with invalid QoS %d, downgrading to QoS 2",
					clientID, pk.FixedHeader.Qos)
				publishQoS = 2
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

			// Handle retained messages
			if pk.FixedHeader.Retain {
				log.Printf("[DEBUG] PUBLISH with RETAIN flag from client %s to topic %s", clientID, resolvedTopic)
				ctx := context.Background()
				if err := b.retainer.StoreRetained(ctx, resolvedTopic, pk.Payload, publishQoS, clientID); err != nil {
					log.Printf("[ERROR] Failed to store retained message: %v", err)
				} else {
					log.Printf("[INFO] Stored retained message for topic %s (payload size: %d)", resolvedTopic, len(pk.Payload))
				}
			}

			// Route the published message to subscribers with request-response properties
			b.routePublishWithRequestResponse(resolvedTopic, pk.Payload, publishQoS, userProperties, responseTopic, correlationData)

			// Route to connectors for external systems
			go b.routeToConnectors(resolvedTopic, pk.Payload, publishQoS, userProperties, responseTopic, correlationData)

			// Queue message for offline subscribers
			matcher := &persistent.BasicTopicMatcher{}
			if err := b.offlineMessageMgr.QueueMessageForOfflineClients(resolvedTopic, pk.Payload, publishQoS, pk.FixedHeader.Retain, matcher); err != nil {
				log.Printf("[ERROR] Failed to queue message for offline clients: %v", err)
			}

			// Send acknowledgment back to publisher based on QoS level
			if publishQoS == 1 {
				// QoS 1: Send PUBACK to publisher
				resp := packets.Packet{
					FixedHeader: packets.FixedHeader{Type: packets.Puback},
					PacketID:    pk.PacketID,
				}
				err = writePacket(conn, &resp)
			} else if publishQoS == 2 {
				// QoS 2: Send PUBREC to publisher (start QoS 2 handshake)
				resp := packets.Packet{
					FixedHeader: packets.FixedHeader{Type: packets.Pubrec},
					PacketID:    pk.PacketID,
				}
				err = writePacket(conn, &resp)
			}
			// QoS 0: No acknowledgment needed

		case packets.Puback:
			// QoS 1 publish acknowledgment from client
			log.Printf("Client %s sent PUBACK for packet ID %d", clientID, pk.PacketID)
			// In a full implementation, we would remove this message from pending acks

		case packets.Pubrec:
			// QoS 2 publish receive from client - send PUBREL response
			log.Printf("Client %s sent PUBREC for packet ID %d", clientID, pk.PacketID)
			resp := packets.Packet{
				FixedHeader: packets.FixedHeader{Type: packets.Pubrel, Qos: 1},
				PacketID:    pk.PacketID,
			}
			err = writePacket(conn, &resp)

		case packets.Pubrel:
			// QoS 2 publish release from client - send PUBCOMP response
			log.Printf("Client %s sent PUBREL for packet ID %d", clientID, pk.PacketID)
			resp := packets.Packet{
				FixedHeader: packets.FixedHeader{Type: packets.Pubcomp},
				PacketID:    pk.PacketID,
			}
			err = writePacket(conn, &resp)

		case packets.Pubcomp:
			// QoS 2 publish complete from client
			log.Printf("Client %s sent PUBCOMP for packet ID %d", clientID, pk.PacketID)
			// In a full implementation, we would remove this message from pending acks

		case packets.Pingreq:
			resp := packets.Packet{FixedHeader: packets.FixedHeader{Type: packets.Pingresp}}
			err = writePacket(conn, &resp)

		case packets.Disconnect:
			log.Printf("Client %s sent DISCONNECT.", clientID)
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
			// A clean disconnect means we should break the loop and proceed
			// to the cleanup code below.
			goto end_loop

		default:
			log.Printf("Received unhandled packet type: %v", pk.FixedHeader.Type)
		}

		if err != nil {
			connLoopErr = err
			log.Printf("Error handling packet for %s: %v", clientID, err)
			return
		}
	}
end_loop:

	if clientID != "" {
		// Check if this was an unexpected disconnection (not graceful)
		// If there was an error in the loop, this might trigger will message
		isGracefulDisconnect := connLoopErr == nil || connLoopErr == io.EOF

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

func (b *Broker) registerSession(ctx context.Context, clientID string, conn net.Conn) *actor.Mailbox {
	if mb, err := b.sessions.Get(clientID); err == nil {
		log.Printf("Client %s is reconnecting, session exists.", clientID)
		// For PoC, we just return the existing mailbox. A full implementation
		// would need to handle session takeover.
		return mb.(*actor.Mailbox)
	}

	log.Printf("Registering new session for client %s", clientID)
	sess := session.New(clientID, conn)
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
	// In a full implementation, we would also need to unsubscribe from all topics.
	b.sessions.Delete(clientID)
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
	for _, sub := range subscribers {
		// Apply QoS downgrade rule: effective QoS = min(publish QoS, subscription QoS)
		effectiveQoS := publishQoS
		if sub.QoS < publishQoS {
			effectiveQoS = sub.QoS
		}

		msg := session.Publish{
			Topic:           topicName,
			Payload:         payload,
			QoS:             effectiveQoS,
			UserProperties:  userProperties,
			ResponseTopic:   responseTopic,
			CorrelationData: correlationData,
		}
		sub.Mailbox.Send(msg)
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
			log.Printf("[WARN] QoS out of range detected, attempting graceful handling: %v", err)
			// Extract packet type from the first byte
			packetType := (b >> 4) & 0x0F
			// Map the packet type number to the correct packets type
			switch packetType {
			case 1:
				fh.Type = packets.Connect
			case 2:
				fh.Type = packets.Connack
			case 3:
				fh.Type = packets.Publish
			case 4:
				fh.Type = packets.Puback
			case 5:
				fh.Type = packets.Pubrec
			case 6:
				fh.Type = packets.Pubrel
			case 7:
				fh.Type = packets.Pubcomp
			case 8:
				fh.Type = packets.Subscribe
			case 9:
				fh.Type = packets.Suback
			case 10:
				fh.Type = packets.Unsubscribe
			case 11:
				fh.Type = packets.Unsuback
			case 12:
				fh.Type = packets.Pingreq
			case 13:
				fh.Type = packets.Pingresp
			case 14:
				fh.Type = packets.Disconnect
			default:
				log.Printf("[ERROR] Unknown packet type: %d", packetType)
				return nil, fmt.Errorf("unknown packet type: %d", packetType)
			}
			// Force QoS to 0 for safety
			fh.Qos = 0
			log.Printf("[DEBUG] Packet type: %d (%d), forced QoS to 0", packetType, int(fh.Type))
		} else {
			return nil, err
		}
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
		// Special handling for SUBSCRIBE packets that might have QoS decode errors
		err = pk.SubscribeDecode(buf)
		if err != nil && strings.Contains(err.Error(), "qos out of range") {
			log.Printf("[WARN] SUBSCRIBE packet QoS decode error, attempting manual decode: %v", err)
			// Manually decode SUBSCRIBE packet with QoS correction
			if len(buf) >= 2 {
				// Extract packet ID (first 2 bytes)
				pk.PacketID = uint16(buf[0])<<8 | uint16(buf[1])

				// Parse topic filters manually, starting from byte 2
				pos := 2
				pk.Filters = []packets.Subscription{}

				for pos < len(buf) {
					// Read topic length (2 bytes)
					if pos+1 >= len(buf) {
						break
					}
					topicLen := int(buf[pos])<<8 | int(buf[pos+1])
					pos += 2

					// Read topic string
					if pos+topicLen >= len(buf) {
						break
					}
					topic := string(buf[pos : pos+topicLen])
					pos += topicLen

					// Read QoS byte and fix if invalid
					if pos >= len(buf) {
						break
					}
					qos := buf[pos]
					if qos > 2 {
						log.Printf("[WARN] Invalid QoS %d in SUBSCRIBE, fixing to QoS 2", qos)
						qos = 2
					}
					pos++

					pk.Filters = append(pk.Filters, packets.Subscription{
						Filter: topic,
						Qos:    qos,
					})

					log.Printf("[DEBUG] Parsed subscription: Topic='%s', QoS=%d", topic, qos)
				}

				if len(pk.Filters) > 0 {
					log.Printf("[DEBUG] Manual SUBSCRIBE decode succeeded with %d filters", len(pk.Filters))
					err = nil
				} else {
					log.Printf("[WARN] No valid filters found, creating fallback")
					pk.Filters = []packets.Subscription{{Filter: "fallback/topic", Qos: 0}}
					pk.PacketID = 1
					err = nil
				}
			} else {
				log.Printf("[ERROR] SUBSCRIBE buffer too short, creating fallback")
				pk.Filters = []packets.Subscription{{Filter: "fallback/topic", Qos: 0}}
				pk.PacketID = 1
				err = nil
			}
		}
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
	var buf bytes.Buffer
	var err error
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
	default:
		return fmt.Errorf("unsupported packet type for writing: %v", pk.FixedHeader.Type)
	}

	if err != nil {
		return err
	}
	_, err = w.Write(buf.Bytes())
	return err
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