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
	"github.com/turtacn/emqx-go/pkg/cluster"
	"github.com/turtacn/emqx-go/pkg/metrics"
	clusterpb "github.com/turtacn/emqx-go/pkg/proto/cluster"
	"github.com/turtacn/emqx-go/pkg/session"
	"github.com/turtacn/emqx-go/pkg/storage"
	"github.com/turtacn/emqx-go/pkg/supervisor"
	"github.com/turtacn/emqx-go/pkg/topic"
)

// Broker is the central component of the MQTT server. It acts as the main
// supervisor for client sessions and handles the core logic of message routing,
// session management, and cluster communication.
type Broker struct {
	sup      supervisor.Supervisor
	sessions storage.Store
	topics   *topic.Store
	cluster  *cluster.Manager
	nodeID   string
	mu       sync.RWMutex
}

// New creates and initializes a new Broker instance.
//
// It sets up the session store, topic store, and supervisor. If a cluster
// manager is provided, it configures the broker to participate in a cluster by
// setting the local publish callback.
//
// - nodeID: A unique identifier for this broker instance in the cluster.
// - clusterMgr: A manager for cluster operations. Can be nil for a standalone broker.
func New(nodeID string, clusterMgr *cluster.Manager) *Broker {
	b := &Broker{
		sup:      supervisor.NewOneForOneSupervisor(),
		sessions: storage.NewMemStore(),
		topics:   topic.NewStore(),
		cluster:  clusterMgr,
		nodeID:   nodeID,
	}
	if clusterMgr != nil {
		// Set the callback for the cluster manager to publish locally
		clusterMgr.LocalPublishFunc = b.RouteToLocalSubscribers
	}
	return b
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
		pk, err := readPacket(reader)
		if err != nil {
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

		switch pk.FixedHeader.Type {
		case packets.Connect:
			clientID = pk.Connect.ClientIdentifier
			if clientID == "" {
				log.Printf("CONNECT from %s has empty client ID. Closing.", conn.RemoteAddr())
				return
			}
			sessionMailbox = b.registerSession(connCtx, clientID, conn)
			resp := packets.Packet{
				FixedHeader: packets.FixedHeader{Type: packets.Connack},
				ReasonCode:  packets.CodeSuccess.Code,
			}
			err = writePacket(conn, &resp)

		case packets.Subscribe:
			if sessionMailbox == nil {
				log.Println("SUBSCRIBE received before CONNECT")
				return
			}
			var newRoutes []*clusterpb.Route
			var reasonCodes []byte
			for _, sub := range pk.Filters {
				// Validate QoS level (MQTT spec allows 0, 1, 2 only)
				grantedQoS := sub.Qos
				if sub.Qos > 2 {
					log.Printf("Client %s requested invalid QoS %d for topic %s, downgrading to QoS 2",
						clientID, sub.Qos, sub.Filter)
					grantedQoS = 2
				}

				// Store subscription with validated QoS level
				b.topics.Subscribe(sub.Filter, sessionMailbox, grantedQoS)
				log.Printf("Client %s subscribed to %s with QoS %d", clientID, sub.Filter, grantedQoS)
				newRoutes = append(newRoutes, &clusterpb.Route{
					Topic:   sub.Filter,
					NodeIds: []string{b.nodeID},
				})
				// Return the granted QoS level
				reasonCodes = append(reasonCodes, grantedQoS)
			}
			// Announce these new routes to peers
			if b.cluster != nil {
				b.cluster.BroadcastRouteUpdate(newRoutes)
			}

			resp := packets.Packet{
				FixedHeader: packets.FixedHeader{Type: packets.Suback},
				PacketID:    pk.PacketID,
				ReasonCodes: reasonCodes,
			}
			err = writePacket(conn, &resp)

		case packets.Publish:
			// Validate QoS level for publish (MQTT spec allows 0, 1, 2 only)
			publishQoS := pk.FixedHeader.Qos
			if pk.FixedHeader.Qos > 2 {
				log.Printf("Client %s attempted to publish with invalid QoS %d, downgrading to QoS 2",
					clientID, pk.FixedHeader.Qos)
				publishQoS = 2
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
	err = fh.Decode(b)
	if err != nil {
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