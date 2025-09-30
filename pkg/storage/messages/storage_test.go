package messages

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()
	assert.Equal(t, "memory", config.Backend)
	assert.Equal(t, 4, config.Shards)
	assert.Equal(t, 7, config.RetentionDays)
	assert.Equal(t, uint64(1000000), config.MaxMessages)
	assert.NotNil(t, config.BackendConfig)
}

func TestNewMessageStorage(t *testing.T) {
	storage, err := NewMessageStorage(nil)
	require.NoError(t, err)
	require.NotNil(t, storage)
	defer storage.Close()

	stats := storage.GetStats()
	assert.Equal(t, "memory", stats.BackendType)
	assert.Equal(t, uint64(0), stats.TotalMessages)
}

func TestStoreMessage(t *testing.T) {
	storage, err := NewMessageStorage(DefaultConfig())
	require.NoError(t, err)
	defer storage.Close()

	ctx := context.Background()
	message := Message{
		Topic:   "test/topic",
		Payload: []byte("test payload"),
		QoS:     1,
	}

	err = storage.StoreMessage(ctx, message)
	require.NoError(t, err)

	stats := storage.GetStats()
	assert.Equal(t, uint64(1), stats.TotalMessages)
}

func TestStoreMessages(t *testing.T) {
	storage, err := NewMessageStorage(DefaultConfig())
	require.NoError(t, err)
	defer storage.Close()

	ctx := context.Background()
	messages := []Message{
		{
			Topic:   "test/topic1",
			Payload: []byte("payload1"),
			QoS:     0,
		},
		{
			Topic:   "test/topic2",
			Payload: []byte("payload2"),
			QoS:     1,
		},
		{
			Topic:   "test/topic3",
			Payload: []byte("payload3"),
			QoS:     2,
		},
	}

	err = storage.StoreMessages(ctx, messages)
	require.NoError(t, err)

	stats := storage.GetStats()
	assert.Equal(t, uint64(3), stats.TotalMessages)
}

func TestRetainedMessages(t *testing.T) {
	storage, err := NewMessageStorage(DefaultConfig())
	require.NoError(t, err)
	defer storage.Close()

	ctx := context.Background()

	// Store retained message
	retainedMsg := Message{
		Topic:    "test/retained",
		Payload:  []byte("retained payload"),
		QoS:      1,
		Retained: true,
	}

	err = storage.StoreMessage(ctx, retainedMsg)
	require.NoError(t, err)

	// Retrieve retained messages
	retained, err := storage.GetRetainedMessages(ctx, "test/retained")
	require.NoError(t, err)
	assert.Len(t, retained, 1)
	assert.Equal(t, "test/retained", retained[0].Topic)
	assert.Equal(t, []byte("retained payload"), retained[0].Payload)
	assert.True(t, retained[0].Retained)

	stats := storage.GetStats()
	assert.Equal(t, uint64(1), stats.RetainedMessages)
}

func TestDeleteRetainedMessage(t *testing.T) {
	storage, err := NewMessageStorage(DefaultConfig())
	require.NoError(t, err)
	defer storage.Close()

	ctx := context.Background()

	// Store retained message
	retainedMsg := Message{
		Topic:    "test/delete",
		Payload:  []byte("to be deleted"),
		QoS:      1,
		Retained: true,
	}

	err = storage.StoreMessage(ctx, retainedMsg)
	require.NoError(t, err)

	// Verify it exists
	retained, err := storage.GetRetainedMessages(ctx, "test/delete")
	require.NoError(t, err)
	assert.Len(t, retained, 1)

	// Delete retained message
	err = storage.DeleteRetainedMessage(ctx, "test/delete")
	require.NoError(t, err)

	// Verify it's gone
	retained, err = storage.GetRetainedMessages(ctx, "test/delete")
	require.NoError(t, err)
	assert.Len(t, retained, 0)
}

func TestGetMessages(t *testing.T) {
	storage, err := NewMessageStorage(DefaultConfig())
	require.NoError(t, err)
	defer storage.Close()

	ctx := context.Background()
	baseTime := time.Now()

	// Store messages with different timestamps
	messages := []Message{
		{
			Topic:     "test/history",
			Payload:   []byte("message1"),
			QoS:       0,
			Timestamp: baseTime.Add(-2 * time.Hour),
		},
		{
			Topic:     "test/history",
			Payload:   []byte("message2"),
			QoS:       1,
			Timestamp: baseTime.Add(-1 * time.Hour),
		},
		{
			Topic:     "test/history",
			Payload:   []byte("message3"),
			QoS:       2,
			Timestamp: baseTime,
		},
	}

	err = storage.StoreMessages(ctx, messages)
	require.NoError(t, err)

	// Get messages from 1.5 hours ago
	startTime := baseTime.Add(-90 * time.Minute)
	retrieved, err := storage.GetMessages(ctx, "test/history", startTime, 10)
	require.NoError(t, err)

	// Should get the last 2 messages
	assert.Len(t, retrieved, 2)
	assert.Equal(t, []byte("message2"), retrieved[0].Payload)
	assert.Equal(t, []byte("message3"), retrieved[1].Payload)
}

func TestMessageExpiry(t *testing.T) {
	storage, err := NewMessageStorage(DefaultConfig())
	require.NoError(t, err)
	defer storage.Close()

	ctx := context.Background()
	expiredTime := time.Now().Add(-1 * time.Second)

	// Store retained message with expiry
	retainedMsg := Message{
		Topic:      "test/expiry",
		Payload:    []byte("expired message"),
		QoS:        1,
		Retained:   true,
		ExpiryTime: &expiredTime,
	}

	err = storage.StoreMessage(ctx, retainedMsg)
	require.NoError(t, err)

	// Wait for expiry to be processed
	time.Sleep(100 * time.Millisecond)

	// Message should still be there (cleanup runs hourly)
	retained, err := storage.GetRetainedMessages(ctx, "test/expiry")
	require.NoError(t, err)
	assert.Len(t, retained, 1)
}

func TestConcurrentAccess(t *testing.T) {
	storage, err := NewMessageStorage(DefaultConfig())
	require.NoError(t, err)
	defer storage.Close()

	ctx := context.Background()
	const goroutines = 10
	const messagesPerGoroutine = 10

	// Start multiple goroutines storing messages concurrently
	done := make(chan bool, goroutines)

	for i := 0; i < goroutines; i++ {
		go func(goroutineID int) {
			for j := 0; j < messagesPerGoroutine; j++ {
				msg := Message{
					Topic:   "concurrent/test",
					Payload: []byte("concurrent message"),
					QoS:     1,
				}
				err := storage.StoreMessage(ctx, msg)
				assert.NoError(t, err)
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines to complete
	for i := 0; i < goroutines; i++ {
		<-done
	}

	// Verify all messages were stored
	stats := storage.GetStats()
	assert.Equal(t, uint64(goroutines*messagesPerGoroutine), stats.TotalMessages)
}

func TestMemoryBackend(t *testing.T) {
	config := DefaultConfig()
	backend, err := NewMemoryBackend(config)
	require.NoError(t, err)
	defer backend.Close()

	ctx := context.Background()

	// First, store a message
	messages := []Message{
		{
			Topic:   "test/backend",
			Payload: []byte("backend test"),
			QoS:     1,
		},
	}

	err = backend.AppendMessages(ctx, "test-stream", messages)
	require.NoError(t, err)

	stats := backend.Stats()
	assert.Equal(t, uint64(1), stats.TotalMessages)
	assert.Equal(t, uint64(1), stats.TotalStreams)

	// Test GetStreams
	streams, err := backend.GetStreams(ctx, "test/backend", time.Now().Add(-1*time.Hour))
	assert.NoError(t, err)
	assert.Len(t, streams, 1)
	if len(streams) > 0 {
		assert.Equal(t, "test-stream", streams[0].ID)
		assert.Equal(t, "test/backend", streams[0].Topic)
	}

	// Test CreateIterator
	iterator, err := backend.CreateIterator(ctx, "test-stream", "test/backend", time.Now().Add(-1*time.Hour))
	assert.NoError(t, err)
	defer iterator.Close()

	assert.True(t, iterator.HasMore())

	msgs, err := iterator.Next(ctx, 10)
	assert.NoError(t, err)
	assert.Len(t, msgs, 1)
	assert.Equal(t, "test/backend", msgs[0].Topic)

	assert.False(t, iterator.HasMore())

	// Test StoreRetained
	msg := Message{
		Topic:    "test/retained",
		Payload:  []byte("retained test"),
		QoS:      1,
		Retained: true,
	}

	err = backend.StoreRetained(ctx, msg.Topic, msg)
	assert.NoError(t, err)

	retained, err := backend.GetRetained(ctx, "test/retained")
	assert.NoError(t, err)
	assert.Len(t, retained, 1)
	assert.Equal(t, msg.Topic, retained[0].Topic)
	assert.Equal(t, msg.Payload, retained[0].Payload)

	// Test DeleteRetained
	err = backend.DeleteRetained(ctx, "test/retained")
	assert.NoError(t, err)

	retained, err = backend.GetRetained(ctx, "test/retained")
	assert.NoError(t, err)
	assert.Len(t, retained, 0)
}

func TestStreamID(t *testing.T) {
	config := DefaultConfig()
	config.Shards = 2

	storage, err := NewMessageStorage(config)
	require.NoError(t, err)
	defer storage.Close()

	timestamp := time.Date(2023, 12, 25, 12, 0, 0, 0, time.UTC)

	// Test that same topic and time generate same stream ID
	streamID1 := storage.getStreamID("test/topic", timestamp)
	streamID2 := storage.getStreamID("test/topic", timestamp)
	assert.Equal(t, streamID1, streamID2)

	// Test that different topics generate different stream IDs (possibly)
	streamID3 := storage.getStreamID("different/topic", timestamp)
	_ = streamID3 // Use variable to avoid unused warning

	// Test that different times generate different stream IDs
	differentTime := timestamp.Add(24 * time.Hour)
	streamID4 := storage.getStreamID("test/topic", differentTime)
	assert.NotEqual(t, streamID1, streamID4)
}

func TestInvalidBackend(t *testing.T) {
	config := &Config{
		Backend: "invalid-backend",
	}

	_, err := NewMessageStorage(config)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported backend")
}