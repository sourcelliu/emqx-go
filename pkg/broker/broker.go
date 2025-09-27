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

// package broker contains the main MQTT broker service.
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
	"github.com/turtacn/emqx-go/pkg/metrics"
	clusterpb "github.com/turtacn/emqx-go/pkg/proto/cluster"
	"github.com/turtacn/emqx-go/pkg/session"
	"github.com/turtacn/emqx-go/pkg/storage"
	"github.com/turtacn/emqx-go/pkg/supervisor"
	"github.com/turtacn/emqx-go/pkg/topic"
)

// Broker is the main actor responsible for managing client sessions and routing messages.
type Broker struct {
	sup      supervisor.Supervisor
	sessions storage.Store
	topics   *topic.Store
	cluster  *cluster.Manager
	nodeID   string
	mu       sync.RWMutex
}

// New creates a new Broker.
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

// StartServer begins listening for incoming TCP connections on the specified address.
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
				log.Printf("Error reading packet from %s: %v", conn.RemoteAddr(), err)
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
			for _, sub := range pk.Filters {
				b.topics.Subscribe(sub.Filter, sessionMailbox)
				log.Printf("Client %s subscribed to %s", clientID, sub.Filter)
				newRoutes = append(newRoutes, &clusterpb.Route{
					Topic:   sub.Filter,
					NodeIds: []string{b.nodeID},
				})
			}
			// Announce these new routes to peers
			if b.cluster != nil {
				b.cluster.BroadcastRouteUpdate(newRoutes)
			}

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
func (b *Broker) routePublish(topicName string, payload []byte) {
	// Route to local subscribers
	b.RouteToLocalSubscribers(topicName, payload)

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

// RouteToLocalSubscribers sends a message to all local subscribers of a topic.
// This function is also used as a callback by the cluster manager for forwarded messages.
func (b *Broker) RouteToLocalSubscribers(topicName string, payload []byte) {
	subscribers := b.topics.GetSubscribers(topicName)
	if len(subscribers) > 0 {
		log.Printf("Routing message on topic '%s' to %d local subscribers", topicName, len(subscribers))
	}
	msg := session.Publish{
		Topic:   topicName,
		Payload: payload,
	}
	for _, mb := range subscribers {
		mb.Send(msg)
	}
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
	default:
		return fmt.Errorf("unsupported packet type for writing: %v", pk.FixedHeader.Type)
	}

	if err != nil {
		return err
	}
	_, err = w.Write(buf.Bytes())
	return err
}