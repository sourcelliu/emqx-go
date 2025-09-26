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

// package connection provides an actor-based implementation for handling
// individual client connections. Each connection is managed by its own actor,
// which is responsible for reading and writing MQTT packets and communicating
// with the central broker actor.
package connection

import (
	"net"

	"github.com/asynkron/protoactor-go/actor"
	"github.com/turtacn/emqx-go/pkg/broker"
	"github.com/turtacn/emqx-go/pkg/protocol/mqtt"
)

// Connection is an actor that manages a single client connection.
// It is responsible for the entire lifecycle of an MQTT client, from handling
// the initial CONNECT packet to processing subscriptions, publications, and
// the final DISCONNECT.
type Connection struct {
	conn      net.Conn
	brokerPID *actor.PID
}

// New creates the properties for a new Connection actor.
// This is used by the actor system to spawn new connection actors.
//
// conn is the network connection from the client.
// brokerPID is the process ID of the broker actor, which the connection actor
// will communicate with for subscriptions and message publishing.
func New(conn net.Conn, brokerPID *actor.PID) *actor.Props {
	return actor.PropsFromProducer(func() actor.Actor {
		return &Connection{
			conn:      conn,
			brokerPID: brokerPID,
		}
	})
}

// Receive is the message handler for the Connection actor. It processes
// messages sent to the actor's mailbox.
//
// It handles the following messages:
// - *actor.Started: Initializes the actor by spawning the connection handler loop.
// - *broker.ForwardedPublish: Receives a publish message from the broker and
//   forwards it to the connected client.
// - *actor.Stopping: Cleans up resources by closing the network connection.
func (c *Connection) Receive(context actor.Context) {
	switch msg := context.Message().(type) {
	case *actor.Started:
		go c.handleConnection(context)
	case *broker.ForwardedPublish:
		c.handleForwardedPublish(msg)
	case *actor.Stopping:
		c.conn.Close()
	}
}

// handleConnection is the main loop for processing incoming data from a single
// client connection. It decodes MQTT packets and acts on them.
// The loop terminates if the connection is closed or an error occurs.
func (c *Connection) handleConnection(context actor.Context) {
	defer context.Stop(context.Self())

	// Handle the initial CONNECT packet
	if _, err := mqtt.DecodeConnect(c.conn); err != nil {
		return
	}
	if err := mqtt.EncodeConnack(c.conn, &mqtt.ConnackPacket{ReturnCode: mqtt.CodeAccepted}); err != nil {
		return
	}

	// Loop to process subsequent packets
	for {
		fh, err := mqtt.DecodeFixedHeader(c.conn)
		if err != nil {
			return // Connection closed or error
		}

		switch fh.PacketType {
		case mqtt.TypeSUBSCRIBE:
			packet, _ := mqtt.DecodeSubscribe(fh, c.conn)
			for _, topic := range packet.Topics {
				// Send a subscription message to the broker
				context.Send(c.brokerPID, &broker.Subscribe{Topic: topic, Sender: context.Self()})
			}
			mqtt.EncodeSuback(c.conn, &mqtt.SubackPacket{MessageID: packet.MessageID, ReturnCodes: []byte{0x00}})
		case mqtt.TypePUBLISH:
			packet, _ := mqtt.DecodePublish(fh, c.conn)
			// Send the published message to the broker for routing
			context.Send(c.brokerPID, &broker.Publish{Topic: packet.TopicName, Payload: packet.Payload})
		case mqtt.TypePINGREQ:
			// Respond to PINGREQ with PINGRESP. (Implementation needed)
		case mqtt.TypeDISCONNECT:
			return // Client initiated disconnect
		}
	}
}

// handleForwardedPublish takes a publish message received from the broker
// and writes it to the client's connection.
func (c *Connection) handleForwardedPublish(msg *broker.ForwardedPublish) {
	packet := &mqtt.PublishPacket{
		TopicName: msg.Topic,
		Payload:   msg.Payload,
	}
	mqtt.EncodePublish(c.conn, packet)
}
