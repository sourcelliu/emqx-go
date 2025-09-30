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
	"testing"
	"time"

	"github.com/mochi-mqtt/server/v2/packets"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/turtacn/emqx-go/pkg/actor"
)

func TestPublishWithTopicAlias(t *testing.T) {
	tests := []struct {
		name               string
		topic              string
		topicAlias         uint16
		expectedTopic      string
		expectedTopicAlias uint16
	}{
		{
			name:               "No topic alias",
			topic:              "test/topic",
			topicAlias:         0,
			expectedTopic:      "test/topic",
			expectedTopicAlias: 0,
		},
		{
			name:               "With topic alias",
			topic:              "sensors/temperature",
			topicAlias:         1,
			expectedTopic:      "sensors/temperature",
			expectedTopicAlias: 1,
		},
		{
			name:               "Topic alias only (empty topic)",
			topic:              "",
			topicAlias:         5,
			expectedTopic:      "",
			expectedTopicAlias: 5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a buffer to capture output
			var buf bytes.Buffer
			session := New("test-client", &buf)

			// Create a mailbox for the session
			mb := actor.NewMailbox(10)

			// Start the session in a goroutine
			ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
			defer cancel()

			done := make(chan error, 1)
			go func() {
				done <- session.Start(ctx, mb)
			}()

			// Send a publish message with topic alias
			publishMsg := Publish{
				Topic:      tt.topic,
				Payload:    []byte("test payload"),
				QoS:        1,
				Retain:     false,
				TopicAlias: tt.topicAlias,
			}

			mb.Send(publishMsg)

			// Give the session time to process the message
			time.Sleep(10 * time.Millisecond)

			// Cancel context to stop the session
			cancel()

			// Wait for session to finish
			select {
			case err := <-done:
				if err != context.Canceled {
					t.Fatalf("Session returned unexpected error: %v", err)
				}
			case <-time.After(100 * time.Millisecond):
				t.Fatal("Session did not terminate in time")
			}

			// Parse the output to verify topic alias was set correctly
			if buf.Len() > 0 {
				// Decode the packet from the buffer
				reader := bytes.NewReader(buf.Bytes())
				fh := new(packets.FixedHeader)

				// Read first byte for fixed header
				firstByte, err := reader.ReadByte()
				require.NoError(t, err)

				err = fh.Decode(firstByte)
				require.NoError(t, err)

				// Read remaining length
				rem, _, err := packets.DecodeLength(reader)
				require.NoError(t, err)
				fh.Remaining = rem

				// Read remaining bytes
				remaining := make([]byte, fh.Remaining)
				_, err = reader.Read(remaining)
				require.NoError(t, err)

				// Create and decode packet
				pk := &packets.Packet{
					FixedHeader:     *fh,
					ProtocolVersion: 5, // Set to MQTT 5.0 for topic aliases
				}
				err = pk.PublishDecode(remaining)
				require.NoError(t, err)

				// Verify topic name
				assert.Equal(t, tt.expectedTopic, pk.TopicName)

				// Verify topic alias
				if tt.expectedTopicAlias > 0 {
					assert.Equal(t, tt.expectedTopicAlias, pk.Properties.TopicAlias)
					assert.True(t, pk.Properties.TopicAliasFlag)
				} else {
					assert.Equal(t, uint16(0), pk.Properties.TopicAlias)
					assert.False(t, pk.Properties.TopicAliasFlag)
				}

				// Verify other packet fields
				assert.Equal(t, []byte("test payload"), pk.Payload)
				assert.Equal(t, byte(1), pk.FixedHeader.Qos)
			}
		})
	}
}

func TestSessionTopicAliasManagement(t *testing.T) {
	// Create a buffer to capture output
	var buf bytes.Buffer
	session := New("test-client", &buf)

	// Test SetTopicAliasMaximum and GetTopicAliasMaximum
	assert.Equal(t, uint16(65535), session.GetTopicAliasMaximum()) // Default

	session.SetTopicAliasMaximum(100)
	assert.Equal(t, uint16(100), session.GetTopicAliasMaximum())

	// Test topic alias assignment
	alias1, isNew1 := session.assignTopicAlias("sensors/temperature")
	assert.Equal(t, uint16(1), alias1)
	assert.True(t, isNew1)

	// Same topic should return same alias
	alias2, isNew2 := session.assignTopicAlias("sensors/temperature")
	assert.Equal(t, uint16(1), alias2)
	assert.False(t, isNew2) // Not new

	// Different topic should get different alias
	alias3, isNew3 := session.assignTopicAlias("sensors/humidity")
	assert.Equal(t, uint16(2), alias3)
	assert.True(t, isNew3)
}

func TestTopicAliasAssignmentLimit(t *testing.T) {
	// Create a buffer to capture output
	var buf bytes.Buffer
	session := New("test-client", &buf)

	// Set a low limit for testing
	session.SetTopicAliasMaximum(2)

	// Assign maximum number of aliases
	alias1, isNew1 := session.assignTopicAlias("topic1")
	assert.Equal(t, uint16(1), alias1)
	assert.True(t, isNew1)

	alias2, isNew2 := session.assignTopicAlias("topic2")
	assert.Equal(t, uint16(2), alias2)
	assert.True(t, isNew2)

	// Should fail to assign third alias
	alias3, isNew3 := session.assignTopicAlias("topic3")
	assert.Equal(t, uint16(0), alias3)
	assert.False(t, isNew3)
}

func TestPublishStructTopicAlias(t *testing.T) {
	// Test the Publish struct itself with topic alias
	publishMsg := Publish{
		Topic:      "test/topic",
		Payload:    []byte("test payload"),
		QoS:        2,
		Retain:     true,
		TopicAlias: 42,
		UserProperties: map[string][]byte{
			"key1": []byte("value1"),
		},
	}

	assert.Equal(t, "test/topic", publishMsg.Topic)
	assert.Equal(t, []byte("test payload"), publishMsg.Payload)
	assert.Equal(t, byte(2), publishMsg.QoS)
	assert.True(t, publishMsg.Retain)
	assert.Equal(t, uint16(42), publishMsg.TopicAlias)
	assert.Equal(t, 1, len(publishMsg.UserProperties))
	assert.Equal(t, []byte("value1"), publishMsg.UserProperties["key1"])
}

func TestPublishWithZeroTopicAlias(t *testing.T) {
	// Test that zero topic alias doesn't set any alias properties
	publishMsg := Publish{
		Topic:      "test/topic",
		Payload:    []byte("test payload"),
		QoS:        0,
		Retain:     false,
		TopicAlias: 0, // No alias
	}

	assert.Equal(t, "test/topic", publishMsg.Topic)
	assert.Equal(t, []byte("test payload"), publishMsg.Payload)
	assert.Equal(t, byte(0), publishMsg.QoS)
	assert.False(t, publishMsg.Retain)
	assert.Equal(t, uint16(0), publishMsg.TopicAlias)
}