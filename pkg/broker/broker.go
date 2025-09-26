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

package broker

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"sync"

	"github.com/mochi-mqtt/server/v2/packets"
	"github.com/turtacn/emqx-go/pkg/actor"
	"github.com/turtacn/emqx-go/pkg/cluster"
	"github.com/turtacn/emqx-go/pkg/session"
	"github.com/turtacn/emqx-go/pkg/storage"
	"github.com/turtacn/emqx-go/pkg/supervisor"
	"github.com/turtacn/emqx-go/pkg/topic"
	clusterpb "github.com/turtacn/emqx-go/pkg/proto/cluster"
)

// Broker is the central component of the MQTT server. It is responsible for
// accepting client connections, managing client sessions, and routing messages
// between clients. It acts as a supervisor for all session actors.
type Broker struct {
	sup      *supervisor.OneForOneSupervisor
	sessions storage.Store // Store for client sessions, mapping ClientID to mailbox
	topics   *topic.Store
	cluster  *cluster.Manager
	nodeID   string
	mu       sync.RWMutex
}

// New creates a new Broker instance.
// It initializes the supervisor, session storage, topic store, and cluster manager.
//
// nodeID is the unique identifier for this broker instance in the cluster.
// clusterMgr is the manager responsible for cluster communication.
func New(nodeID string, clusterMgr *cluster.Manager) *Broker {
	return &Broker{
		sup:      supervisor.NewOneForOneSupervisor(),
		sessions: storage.NewMemStore(),
		topics:   topic.NewStore(),
		cluster:  clusterMgr,
		nodeID:   nodeID,
	}
}

// StartServer begins listening for incoming TCP connections on the specified address.
// It spawns a new goroutine for each incoming connection to handle the MQTT protocol.
// This method blocks until the provided context is canceled.
//
// ctx is the context to control the lifecycle of the server.
// addr is the network address to listen on, e.g., ":1883".
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

// handleConnection is responsible for the lifecycle of a single client connection.
// It reads MQTT packets from the connection, processes them, and handles the
// registration and unregistration of the client's session.
func (b *Broker) handleConnection(ctx context.Context, conn net.Conn) {
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
				log.Printf("Error reading packet from %s: %v", conn.RemoteAddr(), err)
			}
			break
		}

		switch pk.FixedHeader.Type {
		case packets.Connect:
			if pk.Connect.ClientIdentifier == "" {
				log.Printf("CONNECT from %s has empty client ID. Closing.", conn.RemoteAddr())
				return
			}
			clientID = pk.Connect.ClientIdentifier
			sessionMailbox = b.registerSession(connCtx, clientID, conn)

			resp := packets.Packet{
				FixedHeader:    packets.FixedHeader{Type: packets.Connack},
				SessionPresent: false,
				ReasonCode:     packets.CodeSuccess.Code,
			}
			err = writePacket(conn, &resp)

		case packets.Subscribe:
			if sessionMailbox == nil {
				log.Println("SUBSCRIBE received before CONNECT")
				return
			}
			var newRoutes []*clusterpb.Route
			for _, sub := range pk.Filters {
				b.topics.Subscribe(sub.Filter, sessionMailbox)
				newRoutes = append(newRoutes, &clusterpb.Route{
					Topic:   sub.Filter,
					NodeIds: []string{b.nodeID},
				})
			}
			// Announce these new routes to peers
			b.cluster.BroadcastRouteUpdate(newRoutes)

			resp := packets.Packet{
				FixedHeader: packets.FixedHeader{Type: packets.Suback},
				PacketID:    pk.PacketID,
				ReasonCodes: []byte{packets.CodeGrantedQos0.Code},
			}
			err = writePacket(conn, &resp)

		case packets.Publish:
			b.routePublish(pk.TopicName, pk.Payload)

		case packets.Pingreq:
			resp := packets.Packet{FixedHeader: packets.FixedHeader{Type: packets.Pingresp}}
			err = writePacket(conn, &resp)

		case packets.Disconnect:
			log.Printf("Client %s sent DISCONNECT.", clientID)
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
		b.unregisterSession(clientID)
		log.Printf("Client %s disconnected.", clientID)
	}
}

// registerSession creates a new session actor for a client or retrieves an existing one.
// It sets up the session actor under the broker's supervisor and stores the session's
// mailbox for message routing.
func (b *Broker) registerSession(ctx context.Context, clientID string, conn net.Conn) *actor.Mailbox {
	if mb, err := b.sessions.Get(clientID); err == nil {
		log.Printf("Client %s is reconnecting, session exists.", clientID)
		return mb.(*actor.Mailbox)
	}

	log.Printf("Registering new session for client %s", clientID)
	sess := session.New(clientID, conn)
	mb := actor.NewMailbox(100)

	spec := supervisor.Spec{
		ID:      fmt.Sprintf("session-%s", clientID),
		Actor:   sess,
		Restart: supervisor.RestartPermanent,
		Mailbox: mb,
	}
	b.sup.StartChild(ctx, spec)

	b.sessions.Set(clientID, mb)
	return mb
}

// unregisterSession removes a client's session from the broker.
// This is called when a client disconnects.
func (b *Broker) unregisterSession(clientID string) {
	b.sessions.Delete(clientID)
}

// routePublish sends a published message to all subscribers of a topic.
// It handles both local subscribers on the same broker instance and forwards
// messages to remote subscribers on other nodes in the cluster.
func (b *Broker) routePublish(topicName string, payload []byte) {
	// Route to local subscribers
	localSubscribers := b.topics.GetSubscribers(topicName)
	msg := session.Publish{
		Topic:   topicName,
		Payload: payload,
	}
	for _, mb := range localSubscribers {
		mb.Send(msg)
	}

	// Route to remote subscribers
	remoteSubscribers := b.cluster.GetRemoteSubscribers(topicName)
	for _, nodeID := range remoteSubscribers {
		log.Printf("Forwarding message for topic '%s' to remote node %s", topicName, nodeID)
		// In a real implementation, we would use the gRPC client to send this message.
		// For the PoC, this log is sufficient to show the logic is in place.
	}
}

// readPacket is a helper function to read a full MQTT packet from a buffered reader.
// It decodes the fixed header and the variable part of the packet.
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
	case packets.Pingreq:
		err = pk.PingreqDecode(buf)
	case packets.Disconnect:
		err = pk.DisconnectDecode(buf)
	}

	return pk, err
}

// writePacket is a helper function to encode and write an MQTT packet to a writer.
func writePacket(w io.Writer, pk *packets.Packet) error {
	var buf bytes.Buffer
	var err error
	switch pk.FixedHeader.Type {
	case packets.Connack:
		err = pk.ConnackEncode(&buf)
	case packets.Publish:
		err = pk.PublishEncode(&buf)
	case packets.Suback:
		err = pk.SubackEncode(&buf)
	case packets.Pingresp:
		err = pk.PingrespEncode(&buf)
	default:
		return fmt.Errorf("unsupported packet type for writing: %v", pk.FixedHeader.Type)
	}

	if err != nil {
		return err
	}
	_, err = w.Write(buf.Bytes())
	return err
}