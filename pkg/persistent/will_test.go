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
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/turtacn/emqx-go/pkg/storage"
)

// mockWillMessagePublisher implements WillMessagePublisher for testing
type mockWillMessagePublisher struct {
	publishedMessages []PublishedWillMessage
	publishDelay      time.Duration
	shouldFail        bool
	mu                sync.Mutex
}

type PublishedWillMessage struct {
	WillMessage *WillMessage
	ClientID    string
	Timestamp   time.Time
}

func (m *mockWillMessagePublisher) PublishWillMessage(ctx context.Context, willMsg *WillMessage, clientID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.shouldFail {
		return fmt.Errorf("mock publisher error")
	}

	if m.publishDelay > 0 {
		time.Sleep(m.publishDelay)
	}

	m.publishedMessages = append(m.publishedMessages, PublishedWillMessage{
		WillMessage: &WillMessage{
			Topic:         willMsg.Topic,
			Payload:       append([]byte(nil), willMsg.Payload...),
			QoS:           willMsg.QoS,
			Retain:        willMsg.Retain,
			DelayInterval: willMsg.DelayInterval,
		},
		ClientID:  clientID,
		Timestamp: time.Now(),
	})

	return nil
}

func (m *mockWillMessagePublisher) getPublishedMessages() []PublishedWillMessage {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]PublishedWillMessage(nil), m.publishedMessages...)
}

func (m *mockWillMessagePublisher) reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.publishedMessages = nil
	m.shouldFail = false
}

func TestWillMessageManager_PublishImmediately(t *testing.T) {
	publisher := &mockWillMessagePublisher{}
	wmm := NewWillMessageManager(publisher)
	defer wmm.Close()

	willMsg := &WillMessage{
		Topic:         "test/will",
		Payload:       []byte("client disconnected"),
		QoS:           1,
		Retain:        true,
		DelayInterval: 0, // Immediate publish
	}

	ctx := context.Background()
	err := wmm.PublishWillMessage(ctx, willMsg, "test-client")
	require.NoError(t, err)

	published := publisher.getPublishedMessages()
	require.Len(t, published, 1)

	assert.Equal(t, "test/will", published[0].WillMessage.Topic)
	assert.Equal(t, []byte("client disconnected"), published[0].WillMessage.Payload)
	assert.Equal(t, byte(1), published[0].WillMessage.QoS)
	assert.True(t, published[0].WillMessage.Retain)
	assert.Equal(t, "test-client", published[0].ClientID)
}

func TestWillMessageManager_PublishDelayed(t *testing.T) {
	publisher := &mockWillMessagePublisher{}
	wmm := NewWillMessageManager(publisher)
	defer wmm.Close()

	willMsg := &WillMessage{
		Topic:         "test/delayed",
		Payload:       []byte("delayed will message"),
		QoS:           2,
		Retain:        false,
		DelayInterval: 100 * time.Millisecond,
	}

	ctx := context.Background()
	err := wmm.PublishWillMessage(ctx, willMsg, "delayed-client")
	require.NoError(t, err)

	// Should not be published immediately
	published := publisher.getPublishedMessages()
	assert.Len(t, published, 0)

	// Check scheduled messages
	scheduled := wmm.GetScheduledWillMessages()
	assert.Len(t, scheduled, 1)

	// Wait for delayed publish
	time.Sleep(150 * time.Millisecond)

	published = publisher.getPublishedMessages()
	require.Len(t, published, 1)

	assert.Equal(t, "test/delayed", published[0].WillMessage.Topic)
	assert.Equal(t, []byte("delayed will message"), published[0].WillMessage.Payload)
	assert.Equal(t, byte(2), published[0].WillMessage.QoS)
	assert.False(t, published[0].WillMessage.Retain)
	assert.Equal(t, "delayed-client", published[0].ClientID)

	// Scheduled messages should be cleared after publishing
	scheduled = wmm.GetScheduledWillMessages()
	assert.Len(t, scheduled, 0)
}

func TestWillMessageManager_CancelDelayedMessage(t *testing.T) {
	publisher := &mockWillMessagePublisher{}
	wmm := NewWillMessageManager(publisher)
	defer wmm.Close()

	willMsg := &WillMessage{
		Topic:         "test/cancelled",
		Payload:       []byte("should not be published"),
		QoS:           1,
		Retain:        false,
		DelayInterval: 200 * time.Millisecond,
	}

	ctx := context.Background()
	err := wmm.PublishWillMessage(ctx, willMsg, "cancel-client")
	require.NoError(t, err)

	// Verify message is scheduled
	scheduled := wmm.GetScheduledWillMessages()
	assert.Len(t, scheduled, 1)

	// Cancel the message before it publishes
	wmm.CancelWillMessage("cancel-client")

	// Verify message is no longer scheduled
	scheduled = wmm.GetScheduledWillMessages()
	assert.Len(t, scheduled, 0)

	// Wait longer than delay interval
	time.Sleep(250 * time.Millisecond)

	// Message should not have been published
	published := publisher.getPublishedMessages()
	assert.Len(t, published, 0)
}

func TestWillMessageManager_MultipleDelayedMessages(t *testing.T) {
	publisher := &mockWillMessagePublisher{}
	wmm := NewWillMessageManager(publisher)
	defer wmm.Close()

	// Schedule multiple will messages for the same client
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		willMsg := &WillMessage{
			Topic:         fmt.Sprintf("test/multi/%d", i),
			Payload:       []byte(fmt.Sprintf("message %d", i)),
			QoS:           1,
			Retain:        false,
			DelayInterval: 50 * time.Millisecond,
		}

		err := wmm.PublishWillMessage(ctx, willMsg, "multi-client")
		require.NoError(t, err)
	}

	// All messages should be scheduled
	scheduled := wmm.GetScheduledWillMessages()
	assert.Len(t, scheduled, 3)

	// Cancel all messages for the client
	wmm.CancelWillMessage("multi-client")

	// No messages should be scheduled
	scheduled = wmm.GetScheduledWillMessages()
	assert.Len(t, scheduled, 0)

	// Wait and verify no messages were published
	time.Sleep(100 * time.Millisecond)
	published := publisher.getPublishedMessages()
	assert.Len(t, published, 0)
}

func TestWillMessageManager_PublishError(t *testing.T) {
	publisher := &mockWillMessagePublisher{shouldFail: true}
	wmm := NewWillMessageManager(publisher)
	defer wmm.Close()

	willMsg := &WillMessage{
		Topic:         "test/error",
		Payload:       []byte("error test"),
		QoS:           1,
		Retain:        false,
		DelayInterval: 0,
	}

	ctx := context.Background()
	err := wmm.PublishWillMessage(ctx, willMsg, "error-client")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "mock publisher error")
}

func TestWillMessageManager_NilMessage(t *testing.T) {
	publisher := &mockWillMessagePublisher{}
	wmm := NewWillMessageManager(publisher)
	defer wmm.Close()

	ctx := context.Background()
	err := wmm.PublishWillMessage(ctx, nil, "nil-client")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "will message is nil")
}

func TestWillMessageManager_Close(t *testing.T) {
	publisher := &mockWillMessagePublisher{}
	wmm := NewWillMessageManager(publisher)

	// Schedule some delayed messages
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		willMsg := &WillMessage{
			Topic:         fmt.Sprintf("test/close/%d", i),
			Payload:       []byte(fmt.Sprintf("message %d", i)),
			QoS:           1,
			Retain:        false,
			DelayInterval: 1 * time.Second, // Long delay
		}

		err := wmm.PublishWillMessage(ctx, willMsg, fmt.Sprintf("close-client-%d", i))
		require.NoError(t, err)
	}

	// Verify messages are scheduled
	scheduled := wmm.GetScheduledWillMessages()
	assert.Len(t, scheduled, 3)

	// Close should cancel all scheduled messages
	wmm.Close()

	// Wait a bit and verify no messages were published
	time.Sleep(100 * time.Millisecond)
	published := publisher.getPublishedMessages()
	assert.Len(t, published, 0)
}

func TestWillMessageManager_ConcurrentOperations(t *testing.T) {
	publisher := &mockWillMessagePublisher{}
	wmm := NewWillMessageManager(publisher)
	defer wmm.Close()

	const numClients = 10
	const messagesPerClient = 5

	var wg sync.WaitGroup
	ctx := context.Background()

	// Concurrently schedule messages
	for i := 0; i < numClients; i++ {
		wg.Add(1)
		go func(clientNum int) {
			defer wg.Done()
			clientID := fmt.Sprintf("concurrent-client-%d", clientNum)

			for j := 0; j < messagesPerClient; j++ {
				willMsg := &WillMessage{
					Topic:         fmt.Sprintf("test/concurrent/%d/%d", clientNum, j),
					Payload:       []byte(fmt.Sprintf("message %d-%d", clientNum, j)),
					QoS:           1,
					Retain:        false,
					DelayInterval: 50 * time.Millisecond,
				}

				err := wmm.PublishWillMessage(ctx, willMsg, clientID)
				assert.NoError(t, err)
			}
		}(i)
	}

	wg.Wait()

	// Verify all messages are scheduled
	scheduled := wmm.GetScheduledWillMessages()
	assert.Len(t, scheduled, numClients*messagesPerClient)

	// Concurrently cancel some clients
	for i := 0; i < numClients/2; i++ {
		wg.Add(1)
		go func(clientNum int) {
			defer wg.Done()
			clientID := fmt.Sprintf("concurrent-client-%d", clientNum)
			wmm.CancelWillMessage(clientID)
		}(i)
	}

	wg.Wait()

	// Wait for remaining messages to publish
	time.Sleep(100 * time.Millisecond)

	// Should have messages from the remaining clients
	published := publisher.getPublishedMessages()
	expectedMessages := (numClients - numClients/2) * messagesPerClient
	assert.Len(t, published, expectedMessages)
}

func TestSessionManager_WillMessageIntegration(t *testing.T) {
	store := storage.NewMemStore()
	config := DefaultConfig()
	config.CleanupInterval = 1 * time.Second

	sm := NewSessionManager(store, config)
	defer sm.Close()

	publisher := &mockWillMessagePublisher{}
	sm.SetWillMessagePublisher(publisher)

	clientID := "will-integration-client"

	// Create session with will message
	session, err := sm.CreateOrResumeSession(clientID, false, time.Hour)
	require.NoError(t, err)

	willTopic := "test/integration/will"
	willPayload := []byte("integration test will message")
	willQoS := byte(2)
	willRetain := true
	willDelay := 100 * time.Millisecond

	err = sm.SetWillMessage(clientID, willTopic, willPayload, willQoS, willRetain, willDelay)
	require.NoError(t, err)

	// Verify will message is set
	assert.NotNil(t, session.WillMessage)
	assert.Equal(t, willTopic, session.WillMessage.Topic)
	assert.Equal(t, willPayload, session.WillMessage.Payload)
	assert.Equal(t, willQoS, session.WillMessage.QoS)
	assert.Equal(t, willRetain, session.WillMessage.Retain)
	assert.Equal(t, willDelay, session.WillMessage.DelayInterval)

	// Test abnormal disconnection (should trigger will message)
	err = sm.DisconnectSession(clientID, false) // false = abnormal disconnect
	require.NoError(t, err)

	// Wait for delayed will message
	time.Sleep(150 * time.Millisecond)

	// Verify will message was published
	published := publisher.getPublishedMessages()
	require.Len(t, published, 1)

	assert.Equal(t, willTopic, published[0].WillMessage.Topic)
	assert.Equal(t, willPayload, published[0].WillMessage.Payload)
	assert.Equal(t, willQoS, published[0].WillMessage.QoS)
	assert.Equal(t, willRetain, published[0].WillMessage.Retain)
	assert.Equal(t, clientID, published[0].ClientID)

	// Will message should be cleared after disconnection
	assert.Nil(t, session.WillMessage)
}

func TestSessionManager_WillMessageGracefulDisconnect(t *testing.T) {
	store := storage.NewMemStore()
	sm := NewSessionManager(store, DefaultConfig())
	defer sm.Close()

	publisher := &mockWillMessagePublisher{}
	sm.SetWillMessagePublisher(publisher)

	clientID := "graceful-disconnect-client"

	// Create session with will message
	session, err := sm.CreateOrResumeSession(clientID, false, time.Hour)
	require.NoError(t, err)

	err = sm.SetWillMessage(clientID, "test/graceful", []byte("should not publish"), 1, false, 50*time.Millisecond)
	require.NoError(t, err)

	// Verify will message is set
	assert.NotNil(t, session.WillMessage)

	// Test graceful disconnection (should NOT trigger will message)
	err = sm.DisconnectSession(clientID, true) // true = graceful disconnect
	require.NoError(t, err)

	// Wait to ensure no delayed message is published
	time.Sleep(100 * time.Millisecond)

	// No will message should be published
	published := publisher.getPublishedMessages()
	assert.Len(t, published, 0)

	// Will message should be cleared
	assert.Nil(t, session.WillMessage)
}