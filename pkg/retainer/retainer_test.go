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

package retainer

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/turtacn/emqx-go/pkg/storage/messages"
)

func createTestRetainer(t *testing.T) *Retainer {
	config := DefaultConfig()
	config.CleanupInterval = 100 * time.Millisecond // Faster cleanup for testing

	backend, err := messages.NewMemoryBackend(messages.DefaultConfig())
	require.NoError(t, err)

	return New(backend, config)
}

func TestRetainerStoreAndGetRetained(t *testing.T) {
	retainer := createTestRetainer(t)
	defer retainer.Close()

	ctx := context.Background()

	// Test storing a retained message
	topic := "test/retained"
	payload := []byte("hello world")
	qos := byte(1)
	clientID := "test-client"

	err := retainer.StoreRetained(ctx, topic, payload, qos, clientID)
	assert.NoError(t, err)

	// Test retrieving the retained message
	messages, err := retainer.GetRetainedMessages(ctx, topic)
	assert.NoError(t, err)
	assert.Len(t, messages, 1)

	msg := messages[0]
	assert.Equal(t, topic, msg.Topic)
	assert.Equal(t, payload, msg.Payload)
	assert.Equal(t, qos, msg.QoS)
	assert.Equal(t, clientID, msg.ClientID)
	assert.False(t, msg.Timestamp.IsZero())
}

func TestRetainerDeleteRetained(t *testing.T) {
	retainer := createTestRetainer(t)
	defer retainer.Close()

	ctx := context.Background()
	topic := "test/delete"

	// Store a retained message
	err := retainer.StoreRetained(ctx, topic, []byte("test"), 0, "client")
	assert.NoError(t, err)

	// Verify it exists
	messages, err := retainer.GetRetainedMessages(ctx, topic)
	assert.NoError(t, err)
	assert.Len(t, messages, 1)

	// Delete it by storing empty payload
	err = retainer.StoreRetained(ctx, topic, []byte{}, 0, "client")
	assert.NoError(t, err)

	// Verify it's deleted
	messages, err = retainer.GetRetainedMessages(ctx, topic)
	assert.NoError(t, err)
	assert.Len(t, messages, 0)
}

func TestRetainerExplicitDelete(t *testing.T) {
	retainer := createTestRetainer(t)
	defer retainer.Close()

	ctx := context.Background()
	topic := "test/explicit/delete"

	// Store a retained message
	err := retainer.StoreRetained(ctx, topic, []byte("test"), 0, "client")
	assert.NoError(t, err)

	// Verify it exists
	messages, err := retainer.GetRetainedMessages(ctx, topic)
	assert.NoError(t, err)
	assert.Len(t, messages, 1)

	// Delete explicitly
	err = retainer.DeleteRetained(ctx, topic)
	assert.NoError(t, err)

	// Verify it's deleted
	messages, err = retainer.GetRetainedMessages(ctx, topic)
	assert.NoError(t, err)
	assert.Len(t, messages, 0)
}

func TestRetainerPayloadSizeLimit(t *testing.T) {
	config := DefaultConfig()
	config.MaxPayloadSize = 10 // Very small limit

	backend, err := messages.NewMemoryBackend(messages.DefaultConfig())
	require.NoError(t, err)

	retainer := New(backend, config)
	defer retainer.Close()

	ctx := context.Background()

	// Try to store a message that exceeds the limit
	largePayload := make([]byte, 20)
	err = retainer.StoreRetained(ctx, "test/large", largePayload, 0, "client")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "payload size")

	// Verify it wasn't stored
	messages, err := retainer.GetRetainedMessages(ctx, "test/large")
	assert.NoError(t, err)
	assert.Len(t, messages, 0)
}

func TestRetainerMessageExpiry(t *testing.T) {
	config := DefaultConfig()
	config.MessageExpiryInterval = 50 * time.Millisecond

	backend, err := messages.NewMemoryBackend(messages.DefaultConfig())
	require.NoError(t, err)

	retainer := New(backend, config)
	defer retainer.Close()

	ctx := context.Background()

	// Store a message that will expire
	err = retainer.StoreRetained(ctx, "test/expiry", []byte("expiring"), 0, "client")
	assert.NoError(t, err)

	// Verify it exists initially
	messages, err := retainer.GetRetainedMessages(ctx, "test/expiry")
	assert.NoError(t, err)
	assert.Len(t, messages, 1)

	// Wait for expiry
	time.Sleep(100 * time.Millisecond)

	// Should be filtered out during retrieval
	messages, err = retainer.GetRetainedMessages(ctx, "test/expiry")
	assert.NoError(t, err)
	assert.Len(t, messages, 0, "Expired message should be filtered out")
}

func TestRetainerMaxRetainedMessages(t *testing.T) {
	config := DefaultConfig()
	config.MaxRetainedMessages = 2 // Very small limit

	backend, err := messages.NewMemoryBackend(messages.DefaultConfig())
	require.NoError(t, err)

	retainer := New(backend, config)
	defer retainer.Close()

	ctx := context.Background()

	// Store up to the limit
	err = retainer.StoreRetained(ctx, "test/1", []byte("1"), 0, "client")
	assert.NoError(t, err)

	err = retainer.StoreRetained(ctx, "test/2", []byte("2"), 0, "client")
	assert.NoError(t, err)

	// Try to store one more (should fail)
	err = retainer.StoreRetained(ctx, "test/3", []byte("3"), 0, "client")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "maximum retained messages limit")
}

func TestRetainerStats(t *testing.T) {
	retainer := createTestRetainer(t)
	defer retainer.Close()

	ctx := context.Background()

	// Initial stats
	stats := retainer.GetStats()
	assert.Equal(t, uint64(0), stats.RetainedMessages)

	// Store some messages
	err := retainer.StoreRetained(ctx, "test/stats/1", []byte("1"), 0, "client")
	assert.NoError(t, err)

	err = retainer.StoreRetained(ctx, "test/stats/2", []byte("2"), 1, "client")
	assert.NoError(t, err)

	// Check updated stats
	stats = retainer.GetStats()
	assert.Equal(t, uint64(2), stats.RetainedMessages)
	assert.Equal(t, retainer.config.MaxRetainedMessages, stats.MaxMessages)
	assert.Equal(t, retainer.config.MaxPayloadSize, stats.MaxPayloadSize)
}

func TestRetainerCleanupRoutine(t *testing.T) {
	config := DefaultConfig()
	config.MessageExpiryInterval = 50 * time.Millisecond
	config.CleanupInterval = 25 * time.Millisecond

	backend, err := messages.NewMemoryBackend(messages.DefaultConfig())
	require.NoError(t, err)

	retainer := New(backend, config)
	defer retainer.Close()

	ctx := context.Background()

	// Store a message that will expire
	err = retainer.StoreRetained(ctx, "test/cleanup", []byte("expiring"), 0, "client")
	assert.NoError(t, err)

	// Verify it exists
	stats := retainer.GetStats()
	assert.Equal(t, uint64(1), stats.RetainedMessages)

	// Wait for cleanup to run
	time.Sleep(100 * time.Millisecond)

	// Stats should show the message was cleaned up
	// Note: This depends on the backend's implementation of cleanup
	messages, err := retainer.GetRetainedMessages(ctx, "test/cleanup")
	assert.NoError(t, err)
	assert.Len(t, messages, 0, "Expired message should be cleaned up")
}

func TestMatchTopicFilter(t *testing.T) {
	tests := []struct {
		topic  string
		filter string
		match  bool
	}{
		{"test/topic", "test/topic", true},
		{"test/topic", "test/other", false},
		{"any/topic", "#", true},
		{"test/topic", "test/+", true}, // Single-level wildcard should match
		{"test/topic/sub", "test/+", false}, // + doesn't match multiple levels
		{"test/topic", "+/topic", true},
		{"foo/bar/baz", "foo/+/baz", true},
		{"foo/bar/baz", "foo/+", false}, // + only matches one level
		{"foo/bar", "foo/bar/#", true},
		{"foo/bar/baz", "foo/bar/#", true},
		{"foo", "foo/#", true},
	}

	for _, test := range tests {
		result := MatchTopicFilter(test.topic, test.filter)
		assert.Equal(t, test.match, result,
			"MatchTopicFilter(%q, %q) = %v, want %v",
			test.topic, test.filter, result, test.match)
	}
}

func TestRetainerConcurrentAccess(t *testing.T) {
	retainer := createTestRetainer(t)
	defer retainer.Close()

	ctx := context.Background()
	const numGoroutines = 10
	const messagesPerGoroutine = 10

	// Concurrent writes
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			for j := 0; j < messagesPerGoroutine; j++ {
				topic := fmt.Sprintf("concurrent/%d/%d", id, j)
				payload := []byte(fmt.Sprintf("message-%d-%d", id, j))
				retainer.StoreRetained(ctx, topic, payload, 0, fmt.Sprintf("client-%d", id))
			}
		}(i)
	}

	// Concurrent reads
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			for j := 0; j < messagesPerGoroutine; j++ {
				topic := fmt.Sprintf("concurrent/%d/%d", id, j)
				retainer.GetRetainedMessages(ctx, topic)
			}
		}(i)
	}

	// Give time for all operations to complete
	time.Sleep(100 * time.Millisecond)

	// Verify some messages were stored
	messages, err := retainer.GetRetainedMessages(ctx, "#")
	assert.NoError(t, err)
	assert.Greater(t, len(messages), 0, "Should have stored some messages")
}