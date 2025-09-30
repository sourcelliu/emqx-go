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

func TestClientTopicAliasManager(t *testing.T) {
	tam := newClientTopicAliasManager(10)

	// Test initial state
	assert.Equal(t, uint16(10), tam.getTopicAliasMaximum())

	// Test establishing new mapping
	resolvedTopic, err := tam.resolveTopicAlias("sensors/temperature", 1)
	require.NoError(t, err)
	assert.Equal(t, "sensors/temperature", resolvedTopic)

	// Test using existing mapping with empty topic
	resolvedTopic2, err := tam.resolveTopicAlias("", 1)
	require.NoError(t, err)
	assert.Equal(t, "sensors/temperature", resolvedTopic2)

	// Test different alias
	resolvedTopic3, err := tam.resolveTopicAlias("sensors/humidity", 2)
	require.NoError(t, err)
	assert.Equal(t, "sensors/humidity", resolvedTopic3)
}

func TestClientTopicAliasManagerErrors(t *testing.T) {
	tam := newClientTopicAliasManager(5)

	// Test alias exceeding maximum
	_, err := tam.resolveTopicAlias("test/topic", 10)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "exceeds maximum")

	// Test unmapped alias with empty topic
	_, err = tam.resolveTopicAlias("", 3)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no established mapping")

	// Test zero alias (should work)
	resolvedTopic, err := tam.resolveTopicAlias("test/topic", 0)
	require.NoError(t, err)
	assert.Equal(t, "test/topic", resolvedTopic)
}

func TestTopicAliasOverwrite(t *testing.T) {
	tam := newClientTopicAliasManager(10)

	// Establish mapping
	resolvedTopic1, err := tam.resolveTopicAlias("original/topic", 1)
	require.NoError(t, err)
	assert.Equal(t, "original/topic", resolvedTopic1)

	// Overwrite mapping with same alias
	resolvedTopic2, err := tam.resolveTopicAlias("new/topic", 1)
	require.NoError(t, err)
	assert.Equal(t, "new/topic", resolvedTopic2)

	// Verify new mapping is used
	resolvedTopic3, err := tam.resolveTopicAlias("", 1)
	require.NoError(t, err)
	assert.Equal(t, "new/topic", resolvedTopic3)
}

func TestRouteToLocalSubscribersWithTopicAlias(t *testing.T) {
	// Create a mock broker
	broker := New("test-node", nil)
	defer broker.Close()

	// Create a test mailbox to simulate a subscriber
	testMailbox := actor.NewMailbox(10)

	// Add subscriber to topic store manually for testing
	broker.topics.Subscribe("test/topic", testMailbox, 1)

	// Test routing with topic alias
	userProperties := map[string][]byte{
		"sender": []byte("test-client"),
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
		assert.Equal(t, uint16(0), publishMsg.TopicAlias) // No alias should be set for regular routing
	default:
		t.Fatal("Expected message was not received")
	}
}

func TestTopicAliasExtraction(t *testing.T) {
	tests := []struct {
		name            string
		packet          *packets.Packet
		expectedAlias   uint16
		expectedFlag    bool
		expectedTopic   string
	}{
		{
			name: "No topic alias",
			packet: &packets.Packet{
				FixedHeader: packets.FixedHeader{
					Type: packets.Publish,
					Qos:  1,
				},
				TopicName: "test/topic",
				Payload:   []byte("test payload"),
				Properties: packets.Properties{
					TopicAliasFlag: false,
					TopicAlias:     0,
				},
			},
			expectedAlias: 0,
			expectedFlag:  false,
			expectedTopic: "test/topic",
		},
		{
			name: "With topic alias",
			packet: &packets.Packet{
				FixedHeader: packets.FixedHeader{
					Type: packets.Publish,
					Qos:  1,
				},
				TopicName: "sensors/temperature",
				Payload:   []byte("25.5"),
				Properties: packets.Properties{
					TopicAliasFlag: true,
					TopicAlias:     1,
				},
			},
			expectedAlias: 1,
			expectedFlag:  true,
			expectedTopic: "sensors/temperature",
		},
		{
			name: "Topic alias with empty topic name",
			packet: &packets.Packet{
				FixedHeader: packets.FixedHeader{
					Type: packets.Publish,
					Qos:  0,
				},
				TopicName: "",
				Payload:   []byte("26.0"),
				Properties: packets.Properties{
					TopicAliasFlag: true,
					TopicAlias:     1,
				},
			},
			expectedAlias: 1,
			expectedFlag:  true,
			expectedTopic: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expectedAlias, tt.packet.Properties.TopicAlias)
			assert.Equal(t, tt.expectedFlag, tt.packet.Properties.TopicAliasFlag)
			assert.Equal(t, tt.expectedTopic, tt.packet.TopicName)
		})
	}
}

func TestBrokerTopicAliasManagerLifecycle(t *testing.T) {
	broker := New("test-node", nil)
	defer broker.Close()

	clientID := "test-client"

	// Initially no manager should exist
	broker.mu.RLock()
	_, exists := broker.topicAliasManagers[clientID]
	broker.mu.RUnlock()
	assert.False(t, exists)

	// Simulate client connection (normally done in handleConnection)
	broker.mu.Lock()
	broker.topicAliasManagers[clientID] = newClientTopicAliasManager(50)
	broker.mu.Unlock()

	// Verify manager exists
	broker.mu.RLock()
	manager, exists := broker.topicAliasManagers[clientID]
	broker.mu.RUnlock()
	assert.True(t, exists)
	assert.NotNil(t, manager)
	assert.Equal(t, uint16(50), manager.getTopicAliasMaximum())

	// Simulate client disconnection cleanup
	broker.mu.Lock()
	delete(broker.topicAliasManagers, clientID)
	broker.mu.Unlock()

	// Verify manager is removed
	broker.mu.RLock()
	_, exists = broker.topicAliasManagers[clientID]
	broker.mu.RUnlock()
	assert.False(t, exists)
}

func TestTopicAliasResolutionIntegration(t *testing.T) {
	broker := New("test-node", nil)
	defer broker.Close()

	clientID := "test-client"

	// Create topic alias manager for client
	broker.mu.Lock()
	broker.topicAliasManagers[clientID] = newClientTopicAliasManager(10)
	broker.mu.Unlock()

	// Get the manager
	broker.mu.RLock()
	manager := broker.topicAliasManagers[clientID]
	broker.mu.RUnlock()

	// Test resolution scenarios
	tests := []struct {
		name          string
		topicName     string
		alias         uint16
		expectedTopic string
		expectError   bool
	}{
		{
			name:          "Establish new mapping",
			topicName:     "sensors/temperature",
			alias:         1,
			expectedTopic: "sensors/temperature",
			expectError:   false,
		},
		{
			name:          "Use existing mapping",
			topicName:     "",
			alias:         1,
			expectedTopic: "sensors/temperature",
			expectError:   false,
		},
		{
			name:          "Alias exceeds maximum",
			topicName:     "test/topic",
			alias:         15,
			expectedTopic: "",
			expectError:   true,
		},
		{
			name:          "No alias used",
			topicName:     "direct/topic",
			alias:         0,
			expectedTopic: "direct/topic",
			expectError:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolvedTopic, err := manager.resolveTopicAlias(tt.topicName, tt.alias)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedTopic, resolvedTopic)
			}
		})
	}
}