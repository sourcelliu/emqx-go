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

// package session provides the actor implementation for managing a single
// client session.
package session

import (
	"bytes"
	"context"
	"io"
	"log"

	"github.com/mochi-mqtt/server/v2/packets"
	"github.com/turtacn/emqx-go/pkg/actor"
)

// Publish is the message type sent to a Session actor to publish a message
// to the connected client.
type Publish struct {
	Topic   string
	Payload []byte
}

// Session is an actor that manages the state for a single client connection.
// Its primary responsibility is to write outgoing messages to the client.
type Session struct {
	ID   string
	conn io.Writer
}

// New creates a new Session actor.
func New(id string, conn io.Writer) *Session {
	return &Session{
		ID:   id,
		conn: conn,
	}
}

// Start is the main loop for the Session actor. It listens for messages on its
// mailbox and processes them.
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
			// When a Publish message is received, create an MQTT PUBLISH packet
			// and write it to the client's connection.
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
				// Returning an error will cause the supervisor to treat this as a
				// failure and restart the actor if the strategy allows.
				return err
			}
		default:
			log.Printf("Session actor for %s received unknown message type: %T", s.ID, m)
		}
	}
}