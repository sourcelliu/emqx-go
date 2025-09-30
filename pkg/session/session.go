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
	"sync"

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
	// QoS is the Quality of Service level for the message.
	QoS byte
	// Retain indicates whether this is a retained message.
	Retain bool
	// UserProperties contains MQTT 5.0 user-defined properties
	UserProperties map[string][]byte
	// TopicAlias contains MQTT 5.0 topic alias for this message (0 means no alias)
	TopicAlias uint16
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

	// Topic alias management for MQTT 5.0
	mu                      sync.RWMutex
	brokerToClientAliases   map[uint16]string    // alias -> topic mapping for outbound messages
	nextBrokerAlias         uint16               // next alias number to assign
	topicAliasMaximum       uint16               // maximum topic alias value supported
}

// New creates a new Session instance.
//
// - id: The unique identifier for the client session.
// - conn: The I/O writer for the client's network connection.
func New(id string, conn io.Writer) *Session {
	return &Session{
		ID:                    id,
		conn:                  conn,
		brokerToClientAliases: make(map[uint16]string),
		nextBrokerAlias:       1,
		topicAliasMaximum:     65535, // Default maximum topic alias value
	}
}

// SetTopicAliasMaximum sets the maximum topic alias value for this session
func (s *Session) SetTopicAliasMaximum(maximum uint16) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.topicAliasMaximum = maximum
}

// GetTopicAliasMaximum returns the maximum topic alias value for this session
func (s *Session) GetTopicAliasMaximum() uint16 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.topicAliasMaximum
}

// assignTopicAlias assigns a new topic alias for the given topic
// Returns the alias number and whether a new alias was assigned
func (s *Session) assignTopicAlias(topic string) (uint16, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if we've reached the maximum
	if s.nextBrokerAlias > s.topicAliasMaximum {
		return 0, false
	}

	// Check if this topic already has an alias
	for alias, existingTopic := range s.brokerToClientAliases {
		if existingTopic == topic {
			return alias, false // Return existing alias
		}
	}

	// Assign a new alias
	alias := s.nextBrokerAlias
	s.brokerToClientAliases[alias] = topic
	s.nextBrokerAlias++
	return alias, true
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
				FixedHeader: packets.FixedHeader{
					Type:   packets.Publish,
					Qos:    m.QoS,
					Retain: m.Retain,
				},
				TopicName:       m.Topic,
				Payload:         m.Payload,
				ProtocolVersion: 5, // Enable MQTT 5.0 for topic aliases and user properties support
			}

			// Handle topic aliases for MQTT 5.0
			var useTopicAlias bool
			var topicAlias uint16

			if m.TopicAlias > 0 {
				// Use the provided topic alias
				topicAlias = m.TopicAlias
				useTopicAlias = true

				// If topic alias is provided with empty topic, use alias only
				if m.Topic == "" {
					pk.TopicName = ""
				}
			}
			// Note: Automatic topic alias assignment is disabled for now to maintain
			// compatibility. The broker can still assign aliases by setting TopicAlias
			// in the Publish message explicitly.

			// Set topic alias in properties if we're using one
			if useTopicAlias {
				pk.Properties.TopicAlias = topicAlias
				pk.Properties.TopicAliasFlag = true
			}

			// Add MQTT 5.0 user properties if present
			if len(m.UserProperties) > 0 {
				for key, value := range m.UserProperties {
					pk.Properties.User = append(pk.Properties.User, packets.UserProperty{
						Key: key,
						Val: string(value),
					})
				}
			}

			// QoS 1 and QoS 2 require a packet ID for acknowledgments
			if m.QoS > 0 {
				// For simplicity, use a static packet ID. In a full implementation,
				// this should be managed per-client and incremented.
				pk.PacketID = 1
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