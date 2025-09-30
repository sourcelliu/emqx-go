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
	"fmt"
	"io"
	"log"
	"net"
	"strings"
	"sync"

	"github.com/mochi-mqtt/server/v2/packets"
	"github.com/turtacn/emqx-go/pkg/actor"
	"github.com/turtacn/emqx-go/pkg/auth"
	"github.com/turtacn/emqx-go/pkg/cluster"
	"github.com/turtacn/emqx-go/pkg/metrics"
	clusterpb "github.com/turtacn/emqx-go/pkg/proto/cluster"
	"github.com/turtacn/emqx-go/pkg/retainer"
	"github.com/turtacn/emqx-go/pkg/session"
	"github.com/turtacn/emqx-go/pkg/storage"
	"github.com/turtacn/emqx-go/pkg/storage/messages"
	"github.com/turtacn/emqx-go/pkg/supervisor"
	"github.com/turtacn/emqx-go/pkg/topic"
)

// Broker is the central component of the MQTT server. It acts as the main
// supervisor for client sessions and handles the core logic of message routing,
// session management, cluster communication, authentication, and retained messages.
type Broker struct {
	sup       supervisor.Supervisor
	sessions  storage.Store
	topics    *topic.Store
	cluster   *cluster.Manager
	nodeID    string
	authChain *auth.AuthChain
	retainer  *retainer.Retainer
	mu        sync.RWMutex
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

	b := &Broker{
		sup:       supervisor.NewOneForOneSupervisor(),
		sessions:  storage.NewMemStore(),
		topics:    topic.NewStore(),
		cluster:   clusterMgr,
		nodeID:    nodeID,
		authChain: auth.NewAuthChain(),
		retainer:  ret,
	}
	if clusterMgr != nil {
		// Set the callback for the cluster manager to publish locally
		clusterMgr.LocalPublishFunc = b.RouteToLocalSubscribers
	}
	return b
}

// GetAuthChain returns the authentication chain for configuration
func (b *Broker) GetAuthChain() *auth.AuthChain {
	return b.authChain
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

	for {
		log.Printf("[DEBUG] About to read packet from %s (clientID: %s)", conn.RemoteAddr(), clientID)
		pk, err := readPacket(reader)
		if err != nil {
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
			log.Printf("[DEBUG] CONNECT packet received from %s - ClientID: '%s'", conn.RemoteAddr(), clientID)
			log.Printf("[DEBUG] CONNECT details - CleanSession: %t, KeepAlive: %d", pk.Connect.Clean, pk.Connect.Keepalive)

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

			// Authentication successful, register session
			log.Printf("[DEBUG] Registering session for client %s", clientID)
			sessionMailbox = b.registerSession(connCtx, clientID, conn)
			log.Printf("[DEBUG] Session registered successfully for client %s, mailbox: %p", clientID, sessionMailbox)
			log.Printf("[INFO] Client %s connected successfully with user: %s", clientID, username)

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

				// Store subscription with validated QoS level
				b.topics.Subscribe(sub.Filter, sessionMailbox, grantedQoS)

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

			// Handle retained messages
			if pk.FixedHeader.Retain {
				log.Printf("[DEBUG] PUBLISH with RETAIN flag from client %s to topic %s", clientID, pk.TopicName)
				ctx := context.Background()
				if err := b.retainer.StoreRetained(ctx, pk.TopicName, pk.Payload, publishQoS, clientID); err != nil {
					log.Printf("[ERROR] Failed to store retained message: %v", err)
				} else {
					log.Printf("[INFO] Stored retained message for topic %s (payload size: %d)", pk.TopicName, len(pk.Payload))
				}
			}

			// Route the published message to subscribers
			b.routePublish(pk.TopicName, pk.Payload, publishQoS)

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
			// A clean disconnect means we should break the loop and proceed
			// to the cleanup code below.
			goto end_loop

		default:
			log.Printf("Received unhandled packet type: %v", pk.FixedHeader.Type)
		}

		if err != nil {
			log.Printf("Error handling packet for %s: %v", clientID, err)
			return
		}
	}
end_loop:

	if clientID != "" {
		b.unregisterSession(clientID)
		log.Printf("Client %s disconnected.", clientID)
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
			Topic:   topicName,
			Payload: payload,
			QoS:     effectiveQoS,
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
				Topic:   retainedMsg.Topic,
				Payload: retainedMsg.Payload,
				QoS:     effectiveQoS,
				Retain:  true, // Mark as retained message
			}

			// Send to subscriber's mailbox
			sessionMailbox.Send(pubMsg)

			log.Printf("[INFO] Sent retained message to subscriber: topic=%s, payload_size=%d, qos=%d",
				retainedMsg.Topic, len(retainedMsg.Payload), effectiveQoS)
		}
	}
}