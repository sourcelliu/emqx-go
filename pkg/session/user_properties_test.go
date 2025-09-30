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

func TestPublishWithUserProperties(t *testing.T) {
	tests := []struct {
		name           string
		userProperties map[string][]byte
		expectedCount  int
	}{
		{
			name:           "No user properties",
			userProperties: nil,
			expectedCount:  0,
		},
		{
			name: "Single user property",
			userProperties: map[string][]byte{
				"key1": []byte("value1"),
			},
			expectedCount: 1,
		},
		{
			name: "Multiple user properties",
			userProperties: map[string][]byte{
				"key1":    []byte("value1"),
				"key2":    []byte("value2"),
				"content": []byte("application/json"),
			},
			expectedCount: 3,
		},
		{
			name: "User properties with special characters",
			userProperties: map[string][]byte{
				"unicode":     []byte("测试"),
				"empty":       []byte(""),
				"with-hyphen": []byte("dash-value"),
			},
			expectedCount: 3,
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

			// Send a publish message with user properties
			publishMsg := Publish{
				Topic:          "test/topic",
				Payload:        []byte("test payload"),
				QoS:            1,
				Retain:         false,
				UserProperties: tt.userProperties,
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

			// Parse the output to verify user properties were included
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
					ProtocolVersion: 5, // Set to MQTT 5.0 for user properties
				}
				err = pk.PublishDecode(remaining)
				require.NoError(t, err)

				// Verify user properties
				assert.Equal(t, tt.expectedCount, len(pk.Properties.User))

				if tt.expectedCount > 0 {
					// Create a map of received properties for easy verification
					receivedProps := make(map[string]string)
					for _, prop := range pk.Properties.User {
						receivedProps[prop.Key] = prop.Val
					}

					// Verify all expected properties are present
					for key, value := range tt.userProperties {
						assert.Equal(t, string(value), receivedProps[key])
					}
				}

				// Verify other packet fields
				assert.Equal(t, "test/topic", pk.TopicName)
				assert.Equal(t, []byte("test payload"), pk.Payload)
				assert.Equal(t, byte(1), pk.FixedHeader.Qos)
			}
		})
	}
}

func TestPublishStructUserProperties(t *testing.T) {
	// Test the Publish struct itself
	publishMsg := Publish{
		Topic:   "test/topic",
		Payload: []byte("test payload"),
		QoS:     2,
		Retain:  true,
		UserProperties: map[string][]byte{
			"key1": []byte("value1"),
			"key2": []byte("value2"),
		},
	}

	assert.Equal(t, "test/topic", publishMsg.Topic)
	assert.Equal(t, []byte("test payload"), publishMsg.Payload)
	assert.Equal(t, byte(2), publishMsg.QoS)
	assert.True(t, publishMsg.Retain)
	assert.Equal(t, 2, len(publishMsg.UserProperties))
	assert.Equal(t, []byte("value1"), publishMsg.UserProperties["key1"])
	assert.Equal(t, []byte("value2"), publishMsg.UserProperties["key2"])
}

func TestPublishWithNilUserProperties(t *testing.T) {
	// Test that nil user properties don't cause issues
	publishMsg := Publish{
		Topic:          "test/topic",
		Payload:        []byte("test payload"),
		QoS:            0,
		Retain:         false,
		UserProperties: nil,
	}

	assert.Equal(t, "test/topic", publishMsg.Topic)
	assert.Equal(t, []byte("test payload"), publishMsg.Payload)
	assert.Equal(t, byte(0), publishMsg.QoS)
	assert.False(t, publishMsg.Retain)
	assert.Nil(t, publishMsg.UserProperties)
}

func TestPublishWithEmptyUserProperties(t *testing.T) {
	// Test that empty user properties map works correctly
	publishMsg := Publish{
		Topic:          "test/topic",
		Payload:        []byte("test payload"),
		QoS:            1,
		Retain:         false,
		UserProperties: make(map[string][]byte),
	}

	assert.Equal(t, "test/topic", publishMsg.Topic)
	assert.Equal(t, []byte("test payload"), publishMsg.Payload)
	assert.Equal(t, byte(1), publishMsg.QoS)
	assert.False(t, publishMsg.Retain)
	assert.NotNil(t, publishMsg.UserProperties)
	assert.Equal(t, 0, len(publishMsg.UserProperties))
}