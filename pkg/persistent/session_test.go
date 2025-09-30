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

package persistent

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/turtacn/emqx-go/pkg/storage"
)

func createTestSessionManager(t *testing.T) *SessionManager {
	config := DefaultConfig()
	config.CleanupInterval = 100 * time.Millisecond // Faster cleanup for testing
	config.RetryInterval = 50 * time.Millisecond    // Faster retry for testing

	store := storage.NewMemStore()
	return NewSessionManager(store, config)
}

func TestCreateCleanSession(t *testing.T) {
	sm := createTestSessionManager(t)
	defer sm.Close()

	clientID := "test-client-clean"
	session, err := sm.CreateOrResumeSession(clientID, true, 0)
	require.NoError(t, err)

	assert.Equal(t, clientID, session.ClientID)
	assert.True(t, session.CleanSession)
	assert.True(t, session.Connected)
	assert.Empty(t, session.MessageQueue)
}

func TestCreatePersistentSession(t *testing.T) {
	sm := createTestSessionManager(t)
	defer sm.Close()

	clientID := "test-client-persistent"
	expiryInterval := time.Hour

	session, err := sm.CreateOrResumeSession(clientID, false, expiryInterval)
	require.NoError(t, err)

	assert.Equal(t, clientID, session.ClientID)
	assert.False(t, session.CleanSession)
	assert.True(t, session.Connected)
	assert.Equal(t, expiryInterval, session.ExpiryInterval)
	assert.Empty(t, session.MessageQueue)
}

func TestResumePersistentSession(t *testing.T) {
	sm := createTestSessionManager(t)
	defer sm.Close()

	clientID := "test-client-resume"

	// Create initial session
	session1, err := sm.CreateOrResumeSession(clientID, false, time.Hour)
	require.NoError(t, err)

	// Add subscription
	err = sm.AddSubscription(clientID, "test/topic", 1)
	require.NoError(t, err)

	// Add offline message
	err = sm.QueueMessage(clientID, "test/offline", []byte("offline message"), 1, false)
	require.NoError(t, err)

	// Disconnect
	err = sm.DisconnectSession(clientID, true)
	require.NoError(t, err)

	// Resume session
	session2, err := sm.CreateOrResumeSession(clientID, false, time.Hour)
	require.NoError(t, err)

	assert.Equal(t, session1.ClientID, session2.ClientID)
	assert.False(t, session2.CleanSession)
	assert.True(t, session2.Connected)
	assert.Len(t, session2.Subscriptions, 1)
	assert.Contains(t, session2.Subscriptions, "test/topic")
	assert.Equal(t, byte(1), session2.Subscriptions["test/topic"])
}

func TestCleanSessionOverridesPersistentSession(t *testing.T) {
	sm := createTestSessionManager(t)
	defer sm.Close()

	clientID := "test-client-override"

	// Create persistent session with data
	session1, err := sm.CreateOrResumeSession(clientID, false, time.Hour)
	require.NoError(t, err)

	err = sm.AddSubscription(clientID, "test/topic", 1)
	require.NoError(t, err)

	// Disconnect
	err = sm.DisconnectSession(clientID, true)
	require.NoError(t, err)

	// Reconnect with clean session
	session2, err := sm.CreateOrResumeSession(clientID, true, 0)
	require.NoError(t, err)

	assert.Equal(t, session1.ClientID, session2.ClientID)
	assert.True(t, session2.CleanSession)
	assert.Empty(t, session2.Subscriptions) // Should be cleared
	assert.Empty(t, session2.MessageQueue)  // Should be cleared
}

func TestAddRemoveSubscriptions(t *testing.T) {
	sm := createTestSessionManager(t)
	defer sm.Close()

	clientID := "test-client-subs"
	session, err := sm.CreateOrResumeSession(clientID, false, time.Hour)
	require.NoError(t, err)

	// Add subscriptions
	err = sm.AddSubscription(clientID, "test/topic1", 0)
	require.NoError(t, err)

	err = sm.AddSubscription(clientID, "test/topic2", 1)
	require.NoError(t, err)

	err = sm.AddSubscription(clientID, "test/topic3", 2)
	require.NoError(t, err)

	assert.Len(t, session.Subscriptions, 3)
	assert.Equal(t, byte(0), session.Subscriptions["test/topic1"])
	assert.Equal(t, byte(1), session.Subscriptions["test/topic2"])
	assert.Equal(t, byte(2), session.Subscriptions["test/topic3"])

	// Remove subscription
	err = sm.RemoveSubscription(clientID, "test/topic2")
	require.NoError(t, err)

	assert.Len(t, session.Subscriptions, 2)
	assert.NotContains(t, session.Subscriptions, "test/topic2")
}

func TestWillMessage(t *testing.T) {
	sm := createTestSessionManager(t)
	defer sm.Close()

	clientID := "test-client-will"
	session, err := sm.CreateOrResumeSession(clientID, false, time.Hour)
	require.NoError(t, err)

	// Set will message
	willTopic := "test/will"
	willPayload := []byte("client disconnected unexpectedly")
	willQoS := byte(1)
	willRetain := true
	willDelay := 5 * time.Second

	err = sm.SetWillMessage(clientID, willTopic, willPayload, willQoS, willRetain, willDelay)
	require.NoError(t, err)

	assert.NotNil(t, session.WillMessage)
	assert.Equal(t, willTopic, session.WillMessage.Topic)
	assert.Equal(t, willPayload, session.WillMessage.Payload)
	assert.Equal(t, willQoS, session.WillMessage.QoS)
	assert.Equal(t, willRetain, session.WillMessage.Retain)
	assert.Equal(t, willDelay, session.WillMessage.DelayInterval)

	// Graceful disconnect should clear will message
	err = sm.DisconnectSession(clientID, true)
	require.NoError(t, err)

	assert.Nil(t, session.WillMessage)
}

func TestOfflineMessageQueue(t *testing.T) {
	sm := createTestSessionManager(t)
	defer sm.Close()

	clientID := "test-client-offline"
	session, err := sm.CreateOrResumeSession(clientID, false, time.Hour)
	require.NoError(t, err)

	// Disconnect client
	err = sm.DisconnectSession(clientID, true)
	require.NoError(t, err)

	// Queue messages for offline client
	for i := 0; i < 5; i++ {
		topic := "test/offline"
		payload := []byte("offline message " + string(rune('0'+i)))
		err = sm.QueueMessage(clientID, topic, payload, 1, false)
		require.NoError(t, err)
	}

	assert.Len(t, session.MessageQueue, 5)

	// Verify message content
	for i, msg := range session.MessageQueue {
		assert.Equal(t, "test/offline", msg.Topic)
		expectedPayload := "offline message " + string(rune('0'+i))
		assert.Equal(t, expectedPayload, string(msg.Payload))
		assert.Equal(t, byte(1), msg.QoS)
		assert.False(t, msg.Retain)
		assert.False(t, msg.Timestamp.IsZero())
	}
}

func TestOfflineMessageQueueLimit(t *testing.T) {
	config := DefaultConfig()
	config.MaxOfflineMessages = 3 // Set small limit

	store := storage.NewMemStore()
	sm := NewSessionManager(store, config)
	defer sm.Close()

	clientID := "test-client-limit"
	session, err := sm.CreateOrResumeSession(clientID, false, time.Hour)
	require.NoError(t, err)

	// Disconnect client
	err = sm.DisconnectSession(clientID, true)
	require.NoError(t, err)

	// Queue more messages than limit
	for i := 0; i < 5; i++ {
		topic := "test/overflow"
		payload := []byte("message " + string(rune('0'+i)))
		err = sm.QueueMessage(clientID, topic, payload, 1, false)
		require.NoError(t, err)
	}

	// Should only keep the last 3 messages
	assert.Len(t, session.MessageQueue, 3)

	// Check that oldest messages were dropped
	for i, msg := range session.MessageQueue {
		expectedPayload := "message " + string(rune('0'+i+2)) // Should start from message 2
		assert.Equal(t, expectedPayload, string(msg.Payload))
	}
}

func TestPacketIDGeneration(t *testing.T) {
	sm := createTestSessionManager(t)
	defer sm.Close()

	clientID := "test-client-packet-id"
	_, err := sm.CreateOrResumeSession(clientID, false, time.Hour)
	require.NoError(t, err)

	// Generate packet IDs
	var packetIDs []uint16
	for i := 0; i < 10; i++ {
		id, err := sm.NextPacketID(clientID)
		require.NoError(t, err)
		packetIDs = append(packetIDs, id)
	}

	// Check that IDs are unique and sequential
	for i, id := range packetIDs {
		assert.Equal(t, uint16(i+1), id)
	}
}

func TestPacketIDWrapAround(t *testing.T) {
	sm := createTestSessionManager(t)
	defer sm.Close()

	clientID := "test-client-wrap"
	session, err := sm.CreateOrResumeSession(clientID, false, time.Hour)
	require.NoError(t, err)

	// Set packet ID counter near max value
	session.PacketIDCounter = 65535

	// Generate next ID - should wrap to 1 (skip 0)
	id, err := sm.NextPacketID(clientID)
	require.NoError(t, err)
	assert.Equal(t, uint16(1), id)
}

func TestSessionExpiry(t *testing.T) {
	config := DefaultConfig()
	config.CleanupInterval = 10 * time.Millisecond
	config.DefaultSessionExpiry = 50 * time.Millisecond

	store := storage.NewMemStore()
	sm := NewSessionManager(store, config)
	defer sm.Close()

	clientID := "test-client-expiry"
	_, err := sm.CreateOrResumeSession(clientID, false, config.DefaultSessionExpiry)
	require.NoError(t, err)

	// Disconnect client
	err = sm.DisconnectSession(clientID, true)
	require.NoError(t, err)

	// Wait for expiry
	time.Sleep(100 * time.Millisecond)

	// Session should be cleaned up
	sm.mu.RLock()
	_, exists := sm.activeSessions[clientID]
	sm.mu.RUnlock()

	assert.False(t, exists)
}

func TestMessageExpiry(t *testing.T) {
	config := DefaultConfig()
	config.CleanupInterval = 10 * time.Millisecond
	config.MessageExpiryInterval = 50 * time.Millisecond

	store := storage.NewMemStore()
	sm := NewSessionManager(store, config)
	defer sm.Close()

	clientID := "test-client-msg-expiry"
	session, err := sm.CreateOrResumeSession(clientID, false, time.Hour)
	require.NoError(t, err)

	// Disconnect and queue message
	err = sm.DisconnectSession(clientID, true)
	require.NoError(t, err)

	err = sm.QueueMessage(clientID, "test/expiry", []byte("expiring message"), 1, false)
	require.NoError(t, err)

	assert.Len(t, session.MessageQueue, 1)

	// Wait for message expiry
	time.Sleep(100 * time.Millisecond)

	// Message should be cleaned up
	assert.Empty(t, session.MessageQueue)
}

func TestConcurrentAccess(t *testing.T) {
	sm := createTestSessionManager(t)
	defer sm.Close()

	clientID := "test-client-concurrent"
	_, err := sm.CreateOrResumeSession(clientID, false, time.Hour)
	require.NoError(t, err)

	// Concurrent operations
	const numGoroutines = 10
	const opsPerGoroutine = 10

	// Concurrent subscription operations
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			for j := 0; j < opsPerGoroutine; j++ {
				topic := "test/concurrent/" + string(rune('0'+id)) + "/" + string(rune('0'+j))
				sm.AddSubscription(clientID, topic, byte(j%3))
			}
		}(i)
	}

	// Concurrent message queueing after disconnection
	sm.DisconnectSession(clientID, true)

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			for j := 0; j < opsPerGoroutine; j++ {
				topic := "test/offline"
				payload := []byte("concurrent message")
				sm.QueueMessage(clientID, topic, payload, 1, false)
			}
		}(i)
	}

	// Give time for operations to complete
	time.Sleep(100 * time.Millisecond)

	// Verify session state is consistent
	sm.mu.RLock()
	session := sm.activeSessions[clientID]
	sm.mu.RUnlock()

	assert.NotNil(t, session)
	assert.Greater(t, len(session.Subscriptions), 0)
	assert.Greater(t, len(session.MessageQueue), 0)
}

func TestSessionPersistence(t *testing.T) {
	store := storage.NewMemStore()

	// Create first session manager
	sm1 := NewSessionManager(store, DefaultConfig())

	clientID := "test-client-persistence"
	session1, err := sm1.CreateOrResumeSession(clientID, false, time.Hour)
	require.NoError(t, err)

	// Add data to session
	err = sm1.AddSubscription(clientID, "test/persistence", 2)
	require.NoError(t, err)

	err = sm1.SetWillMessage(clientID, "test/will", []byte("will message"), 1, true, 0)
	require.NoError(t, err)

	// Disconnect and close session manager
	err = sm1.DisconnectSession(clientID, true)
	require.NoError(t, err)
	sm1.Close()

	// Create new session manager with same store
	sm2 := NewSessionManager(store, DefaultConfig())
	defer sm2.Close()

	// Resume session
	session2, err := sm2.CreateOrResumeSession(clientID, false, time.Hour)
	require.NoError(t, err)

	// Verify data was persisted
	assert.Equal(t, session1.ClientID, session2.ClientID)
	assert.False(t, session2.CleanSession)
	assert.Len(t, session2.Subscriptions, 1)
	assert.Contains(t, session2.Subscriptions, "test/persistence")
	assert.Equal(t, byte(2), session2.Subscriptions["test/persistence"])
}