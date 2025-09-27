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

// Package session provides the actor-based implementation for managing an
// individual MQTT client session. Each connected client is managed by its own
// Session actor, which is responsible for handling the client's state and
// communication, particularly for outbound messages.
package session

import (
	"bytes"
	"context"
	"io"
	"log"

	"github.com/mochi-mqtt/server/v2/packets"
	"github.com/turtacn/emqx-go/pkg/actor"
)

// Publish is an internal message sent to a Session actor, instructing it to
// deliver an MQTT PUBLISH packet to the client it manages.
type Publish struct {
	// Topic is the MQTT topic for the message.
	Topic string
	// Payload is the content of the message.
	Payload []byte
}

// Session is an actor that manages the state and network connection for a single
// MQTT client. Its primary responsibility is to receive Publish messages from
// the broker and write the corresponding MQTT PUBLISH packets to the client's
// network connection.
type Session struct {
	// ID is the unique identifier for the client session, typically the MQTT
	// Client ID.
	ID   string
	conn io.Writer
}

// New creates a new Session instance.
//
// - id: The unique identifier for the client session.
// - conn: The I/O writer for the client's network connection.
func New(id string, conn io.Writer) *Session {
	return &Session{
		ID:   id,
		conn: conn,
	}
}

// Start is the entry point and main loop for the Session actor. It conforms to
// the actor.Actor interface. The method continuously waits for messages on its
// mailbox and processes them.
//
// The actor terminates when the provided context is canceled or when an
// unrecoverable error occurs, such as a failure to write to the client's
// connection.
//
// - ctx: The context that controls the actor's lifecycle.
// - mb: The actor's mailbox for receiving messages.
//
// Returns an error if the actor terminates unexpectedly.
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