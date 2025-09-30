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
	"testing"

	"github.com/mochi-mqtt/server/v2/packets"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/turtacn/emqx-go/pkg/actor"
	"github.com/turtacn/emqx-go/pkg/session"
)

func TestUserPropertiesExtraction(t *testing.T) {
	tests := []struct {
		name           string
		packet         *packets.Packet
		expectedProps  map[string][]byte
		expectedCount  int
	}{
		{
			name: "No user properties",
			packet: &packets.Packet{
				FixedHeader: packets.FixedHeader{
					Type: packets.Publish,
					Qos:  1,
				},
				TopicName: "test/topic",
				Payload:   []byte("test payload"),
				Properties: packets.Properties{
					User: []packets.UserProperty{},
				},
			},
			expectedProps: nil,
			expectedCount: 0,
		},
		{
			name: "Single user property",
			packet: &packets.Packet{
				FixedHeader: packets.FixedHeader{
					Type: packets.Publish,
					Qos:  1,
				},
				TopicName: "test/topic",
				Payload:   []byte("test payload"),
				Properties: packets.Properties{
					User: []packets.UserProperty{
						{Key: "content-type", Val: "application/json"},
					},
				},
			},
			expectedProps: map[string][]byte{
				"content-type": []byte("application/json"),
			},
			expectedCount: 1,
		},
		{
			name: "Multiple user properties",
			packet: &packets.Packet{
				FixedHeader: packets.FixedHeader{
					Type: packets.Publish,
					Qos:  2,
				},
				TopicName: "test/topic",
				Payload:   []byte("test payload"),
				Properties: packets.Properties{
					User: []packets.UserProperty{
						{Key: "sender", Val: "client1"},
						{Key: "priority", Val: "high"},
						{Key: "encoding", Val: "utf-8"},
					},
				},
			},
			expectedProps: map[string][]byte{
				"sender":   []byte("client1"),
				"priority": []byte("high"),
				"encoding": []byte("utf-8"),
			},
			expectedCount: 3,
		},
		{
			name: "User properties with special characters",
			packet: &packets.Packet{
				FixedHeader: packets.FixedHeader{
					Type: packets.Publish,
					Qos:  0,
				},
				TopicName: "test/topic",
				Payload:   []byte("test payload"),
				Properties: packets.Properties{
					User: []packets.UserProperty{
						{Key: "unicode", Val: "测试"},
						{Key: "empty", Val: ""},
						{Key: "with-hyphen", Val: "dash-value"},
					},
				},
			},
			expectedProps: map[string][]byte{
				"unicode":     []byte("测试"),
				"empty":       []byte(""),
				"with-hyphen": []byte("dash-value"),
			},
			expectedCount: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Extract user properties using the same logic as in broker
			var userProperties map[string][]byte
			if len(tt.packet.Properties.User) > 0 {
				userProperties = make(map[string][]byte)
				for _, userProp := range tt.packet.Properties.User {
					userProperties[userProp.Key] = []byte(userProp.Val)
				}
			}

			assert.Equal(t, tt.expectedCount, len(userProperties))

			if tt.expectedCount > 0 {
				require.NotNil(t, userProperties)
				for key, expectedValue := range tt.expectedProps {
					actualValue, exists := userProperties[key]
					assert.True(t, exists, "Expected key %s not found", key)
					assert.Equal(t, expectedValue, actualValue)
				}
			} else {
				assert.Nil(t, userProperties)
			}
		})
	}
}

func TestRouteToLocalSubscribersWithUserProperties(t *testing.T) {
	// Create a mock broker
	broker := New("test-node", nil)
	defer broker.Close()

	// Create a test mailbox to simulate a subscriber
	testMailbox := actor.NewMailbox(10)

	// Add subscriber to topic store manually for testing
	broker.topics.Subscribe("test/topic", testMailbox, 1)

	// Test routing with user properties
	userProperties := map[string][]byte{
		"sender":   []byte("test-client"),
		"priority": []byte("high"),
	}

	// Call the method under test
	broker.RouteToLocalSubscribersWithUserProperties("test/topic", []byte("test message"), 2, userProperties)

	// Verify the message was sent to the subscriber
	select {
	case msg := <-testMailbox.Chan():
		publishMsg, ok := msg.(session.Publish)
		require.True(t, ok, "Expected session.Publish message")

		assert.Equal(t, "test/topic", publishMsg.Topic)
		assert.Equal(t, []byte("test message"), publishMsg.Payload)
		assert.Equal(t, byte(1), publishMsg.QoS) // Should be downgraded to subscriber QoS
		assert.Equal(t, userProperties, publishMsg.UserProperties)
	default:
		t.Fatal("Expected message was not received")
	}
}

func TestRouteToLocalSubscribersBackwardCompatibility(t *testing.T) {
	// Test that the original method still works without user properties
	broker := New("test-node", nil)
	defer broker.Close()

	// Create a test mailbox to simulate a subscriber
	testMailbox := actor.NewMailbox(10)

	// Add subscriber to topic store manually for testing
	broker.topics.Subscribe("test/topic", testMailbox, 2)

	// Call the original method (backward compatibility)
	broker.RouteToLocalSubscribersWithQoS("test/topic", []byte("test message"), 1)

	// Verify the message was sent to the subscriber
	select {
	case msg := <-testMailbox.Chan():
		publishMsg, ok := msg.(session.Publish)
		require.True(t, ok, "Expected session.Publish message")

		assert.Equal(t, "test/topic", publishMsg.Topic)
		assert.Equal(t, []byte("test message"), publishMsg.Payload)
		assert.Equal(t, byte(1), publishMsg.QoS)
		assert.Nil(t, publishMsg.UserProperties) // Should be nil for backward compatibility
	default:
		t.Fatal("Expected message was not received")
	}
}

func TestQoSDowngradeWithUserProperties(t *testing.T) {
	// Test QoS downgrade behavior with user properties
	broker := New("test-node", nil)
	defer broker.Close()

	testCases := []struct {
		name           string
		publishQoS     byte
		subscribeQoS   byte
		expectedQoS    byte
	}{
		{"QoS 2 to QoS 1", 2, 1, 1},
		{"QoS 2 to QoS 0", 2, 0, 0},
		{"QoS 1 to QoS 0", 1, 0, 0},
		{"QoS 1 to QoS 1", 1, 1, 1},
		{"QoS 0 to QoS 2", 0, 2, 0}, // Published QoS is the maximum
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create a test mailbox to simulate a subscriber
			testMailbox := actor.NewMailbox(10)

			// Add subscriber with specific QoS
			broker.topics.Subscribe("test/topic", testMailbox, tc.subscribeQoS)

			userProperties := map[string][]byte{
				"test": []byte("qos-downgrade"),
			}

			// Call the method with user properties
			broker.RouteToLocalSubscribersWithUserProperties("test/topic", []byte("test"), tc.publishQoS, userProperties)

			// Verify the message QoS was correctly downgraded
			select {
			case msg := <-testMailbox.Chan():
				publishMsg, ok := msg.(session.Publish)
				require.True(t, ok, "Expected session.Publish message")
				assert.Equal(t, tc.expectedQoS, publishMsg.QoS)
				assert.Equal(t, userProperties, publishMsg.UserProperties)
			default:
				t.Fatal("Expected message was not received")
			}

			// Clean up for next test
			broker.topics.Unsubscribe("test/topic", testMailbox)
		})
	}
}