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

package session

import (
	"bytes"
	"context"
	"io"
	"log"

	"github.com/mochi-mqtt/server/v2/packets"
	"github.com/turtacn/emqx-go/pkg/actor"
)

// Publish represents a message to be published to a client.
// It is sent to a Session actor, which then writes the message to the
// client's connection.
type Publish struct {
	// Topic is the topic to which the message is published.
	Topic string
	// Payload is the message content.
	Payload []byte
}

// Session is an actor responsible for managing a single client's connection.
// Its primary role is to receive messages from the broker and write them to the
// client's network connection. Each connected client has its own Session actor.
type Session struct {
	// ID is the unique identifier for the client session, typically the MQTT
	// ClientID.
	ID string
	// conn is the network connection to the client. It is used to write
	// outgoing messages.
	conn io.Writer
}

// New creates a new Session actor.
//
// id is the client identifier.
// conn is the client's network connection.
func New(id string, conn io.Writer) *Session {
	return &Session{
		ID:   id,
		conn: conn,
	}
}

// Start is the main loop for the Session actor. It implements the actor.Actor
// interface.
//
// The loop continuously receives messages from its mailbox. When it receives a
// Publish message, it encodes it into an MQTT PUBLISH packet and writes it to
// the client's connection.
//
// The actor terminates when the context is canceled or an error occurs during
// message processing or writing to the connection.
func (s *Session) Start(ctx context.Context, mb *actor.Mailbox) error {
	log.Printf("Session actor started for client %s", s.ID)
	for {
		msg, err := mb.Receive(ctx)
		if err != nil {
			log.Printf("Session actor for client %s shutting down: %v", s.ID, err)
			return err
		}

		switch m := msg.(type) {
		case Publish:
			pk := &packets.Packet{
				FixedHeader: packets.FixedHeader{Type: packets.Publish, Qos: 0},
				TopicName:   m.Topic,
				Payload:     m.Payload,
			}
			var buf bytes.Buffer
			if err := pk.PublishEncode(&buf); err != nil {
				log.Printf("Error encoding publish packet for %s: %v", s.ID, err)
				continue
			}
			if _, err := s.conn.Write(buf.Bytes()); err != nil {
				log.Printf("Error writing to client %s: %v", s.ID, err)
				// Returning an error will cause the supervisor to restart this actor,
				// but the connection will likely be dead. A real implementation
				// would need more sophisticated error handling here.
				return err
			}
		default:
			log.Printf("Session actor for %s received unknown message type: %T", s.ID, m)
		}
	}
}
