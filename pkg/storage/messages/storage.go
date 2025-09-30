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

// Package messages provides comprehensive MQTT message storage functionality
// inspired by EMQX's dual-layer storage architecture. It supports both
// durable message streams for real-time data and retained message storage
// with topic-based indexing for efficient retrieval.
package messages

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// Message represents a stored MQTT message with metadata
type Message struct {
	// Core message data
	ID        string            `json:"id"`
	Topic     string            `json:"topic"`
	Payload   []byte            `json:"payload"`
	QoS       byte              `json:"qos"`
	Retained  bool              `json:"retained"`
	Timestamp time.Time         `json:"timestamp"`

	// Optional metadata
	Headers    map[string]string `json:"headers,omitempty"`
	ExpiryTime *time.Time        `json:"expiry_time,omitempty"`
	ClientID   string            `json:"client_id,omitempty"`

	// Storage metadata
	StreamID   string `json:"stream_id,omitempty"`
	SequenceNo uint64 `json:"sequence_no,omitempty"`
}

// StreamInfo represents metadata about a message stream
type StreamInfo struct {
	ID          string    `json:"id"`
	Topic       string    `json:"topic"`
	StartTime   time.Time `json:"start_time"`
	EndTime     time.Time `json:"end_time"`
	MessageCount uint64   `json:"message_count"`
	SizeBytes   uint64    `json:"size_bytes"`
}

// SubscriptionOptions configures stream subscription behavior
type SubscriptionOptions struct {
	StartTime   *time.Time `json:"start_time,omitempty"`
	EndTime     *time.Time `json:"end_time,omitempty"`
	BatchSize   int        `json:"batch_size,omitempty"`
	MaxMessages int        `json:"max_messages,omitempty"`
}

// Iterator provides sequential access to stored messages
type Iterator interface {
	// Next returns the next batch of messages
	Next(ctx context.Context, limit int) ([]Message, error)

	// HasMore returns true if more messages are available
	HasMore() bool

	// Close releases iterator resources
	Close() error
}

// Subscription represents a real-time message subscription
type Subscription interface {
	// Messages returns a channel for receiving new messages
	Messages() <-chan []Message

	// Ack acknowledges processing of messages up to sequence number
	Ack(seqno uint64) error

	// Close terminates the subscription
	Close() error
}

// StorageBackend defines the interface for message storage implementations
type StorageBackend interface {
	// Durable storage operations
	AppendMessages(ctx context.Context, streamID string, messages []Message) error
	GetStreams(ctx context.Context, topicFilter string, startTime time.Time) ([]StreamInfo, error)
	CreateIterator(ctx context.Context, streamID string, topicFilter string, startTime time.Time) (Iterator, error)
	Subscribe(ctx context.Context, streamID string, opts SubscriptionOptions) (Subscription, error)

	// Retained message operations
	StoreRetained(ctx context.Context, topic string, message Message) error
	GetRetained(ctx context.Context, topicFilter string) ([]Message, error)
	DeleteRetained(ctx context.Context, topic string) error

	// Management operations
	Close() error
	Stats() StorageStats
}

// StorageStats provides storage backend statistics
type StorageStats struct {
	TotalMessages     uint64            `json:"total_messages"`
	TotalStreams      uint64            `json:"total_streams"`
	RetainedMessages  uint64            `json:"retained_messages"`
	StorageSize       uint64            `json:"storage_size_bytes"`
	BackendType       string            `json:"backend_type"`
	BackendSpecific   map[string]any    `json:"backend_specific,omitempty"`
}

// MessageStorage provides the main interface for MQTT message storage
type MessageStorage struct {
	backend StorageBackend
	config  *Config
	mu      sync.RWMutex
}

// Config defines storage configuration options
type Config struct {
	Backend       string            `yaml:"backend" json:"backend"`             // memory, persistent, distributed
	Shards        int               `yaml:"shards" json:"shards"`               // number of storage shards
	RetentionDays int               `yaml:"retention_days" json:"retention_days"` // message retention period
	MaxMessages   uint64            `yaml:"max_messages" json:"max_messages"`   // maximum messages per stream
	BackendConfig map[string]any    `yaml:"backend_config" json:"backend_config"` // backend-specific config
}

// DefaultConfig returns a default storage configuration
func DefaultConfig() *Config {
	return &Config{
		Backend:       "memory",
		Shards:        4,
		RetentionDays: 7,
		MaxMessages:   1000000,
		BackendConfig: make(map[string]any),
	}
}

// NewMessageStorage creates a new message storage instance
func NewMessageStorage(config *Config) (*MessageStorage, error) {
	if config == nil {
		config = DefaultConfig()
	}

	var backend StorageBackend
	var err error

	switch config.Backend {
	case "memory":
		backend, err = NewMemoryBackend(config)
	case "persistent":
		return nil, fmt.Errorf("persistent backend not yet implemented")
	case "distributed":
		return nil, fmt.Errorf("distributed backend not yet implemented")
	default:
		return nil, fmt.Errorf("unsupported backend: %s", config.Backend)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to create backend: %w", err)
	}

	return &MessageStorage{
		backend: backend,
		config:  config,
	}, nil
}

// StoreMessage stores a single message
func (ms *MessageStorage) StoreMessage(ctx context.Context, message Message) error {
	ms.mu.RLock()
	defer ms.mu.RUnlock()

	// Generate message ID if not provided
	if message.ID == "" {
		message.ID = generateMessageID()
	}

	// Set timestamp if not provided
	if message.Timestamp.IsZero() {
		message.Timestamp = time.Now()
	}

	if message.Retained {
		// Store as retained message
		return ms.backend.StoreRetained(ctx, message.Topic, message)
	} else {
		// Store in durable stream
		streamID := ms.getStreamID(message.Topic, message.Timestamp)
		return ms.backend.AppendMessages(ctx, streamID, []Message{message})
	}
}

// StoreMessages stores multiple messages efficiently
func (ms *MessageStorage) StoreMessages(ctx context.Context, messages []Message) error {
	ms.mu.RLock()
	defer ms.mu.RUnlock()

	// Group messages by stream and retained status
	streamMessages := make(map[string][]Message)
	var retainedMessages []Message

	for _, msg := range messages {
		// Generate message ID if not provided
		if msg.ID == "" {
			msg.ID = generateMessageID()
		}

		// Set timestamp if not provided
		if msg.Timestamp.IsZero() {
			msg.Timestamp = time.Now()
		}

		if msg.Retained {
			retainedMessages = append(retainedMessages, msg)
		} else {
			streamID := ms.getStreamID(msg.Topic, msg.Timestamp)
			streamMessages[streamID] = append(streamMessages[streamID], msg)
		}
	}

	// Store retained messages
	for _, msg := range retainedMessages {
		if err := ms.backend.StoreRetained(ctx, msg.Topic, msg); err != nil {
			return fmt.Errorf("failed to store retained message: %w", err)
		}
	}

	// Store stream messages
	for streamID, msgs := range streamMessages {
		if err := ms.backend.AppendMessages(ctx, streamID, msgs); err != nil {
			return fmt.Errorf("failed to store stream messages: %w", err)
		}
	}

	return nil
}

// GetRetainedMessages retrieves retained messages matching the topic filter
func (ms *MessageStorage) GetRetainedMessages(ctx context.Context, topicFilter string) ([]Message, error) {
	ms.mu.RLock()
	defer ms.mu.RUnlock()

	return ms.backend.GetRetained(ctx, topicFilter)
}

// GetMessages retrieves messages from streams matching the criteria
func (ms *MessageStorage) GetMessages(ctx context.Context, topicFilter string, startTime time.Time, limit int) ([]Message, error) {
	ms.mu.RLock()
	defer ms.mu.RUnlock()

	streams, err := ms.backend.GetStreams(ctx, topicFilter, startTime)
	if err != nil {
		return nil, fmt.Errorf("failed to get streams: %w", err)
	}

	var allMessages []Message
	remaining := limit

	for _, stream := range streams {
		if remaining <= 0 {
			break
		}

		iterator, err := ms.backend.CreateIterator(ctx, stream.ID, topicFilter, startTime)
		if err != nil {
			return nil, fmt.Errorf("failed to create iterator: %w", err)
		}
		defer iterator.Close()

		for iterator.HasMore() && remaining > 0 {
			batchSize := remaining
			if batchSize > 100 {
				batchSize = 100
			}

			messages, err := iterator.Next(ctx, batchSize)
			if err != nil {
				return nil, fmt.Errorf("failed to read messages: %w", err)
			}

			allMessages = append(allMessages, messages...)
			remaining -= len(messages)
		}
	}

	return allMessages, nil
}

// Subscribe creates a real-time subscription to messages
func (ms *MessageStorage) Subscribe(ctx context.Context, topicFilter string, opts SubscriptionOptions) (Subscription, error) {
	ms.mu.RLock()
	defer ms.mu.RUnlock()

	// For now, use a single stream ID based on topic filter
	streamID := ms.getStreamID(topicFilter, time.Now())
	return ms.backend.Subscribe(ctx, streamID, opts)
}

// DeleteRetainedMessage removes a retained message
func (ms *MessageStorage) DeleteRetainedMessage(ctx context.Context, topic string) error {
	ms.mu.RLock()
	defer ms.mu.RUnlock()

	return ms.backend.DeleteRetained(ctx, topic)
}

// GetStats returns storage statistics
func (ms *MessageStorage) GetStats() StorageStats {
	ms.mu.RLock()
	defer ms.mu.RUnlock()

	return ms.backend.Stats()
}

// Close shuts down the storage system
func (ms *MessageStorage) Close() error {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	if ms.backend != nil {
		return ms.backend.Close()
	}
	return nil
}

// getStreamID generates a stream ID based on topic and time
// This implements a simple sharding strategy
func (ms *MessageStorage) getStreamID(topic string, timestamp time.Time) string {
	// Use daily streams by default
	date := timestamp.Format("2006-01-02")
	shard := hash(topic) % ms.config.Shards
	return fmt.Sprintf("stream-%s-shard-%d", date, shard)
}

// generateMessageID creates a unique message identifier
func generateMessageID() string {
	return fmt.Sprintf("msg-%d-%d", time.Now().UnixNano(), randomInt())
}

// hash computes a simple hash for string values
func hash(s string) int {
	h := 0
	for _, c := range s {
		h = 31*h + int(c)
	}
	if h < 0 {
		h = -h
	}
	return h
}

// randomInt generates a random integer for ID generation
func randomInt() int64 {
	return time.Now().UnixNano() % 1000000
}