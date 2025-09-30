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

// Package retainer provides MQTT retained message functionality
// inspired by EMQX's retainer implementation.
package retainer

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/turtacn/emqx-go/pkg/storage/messages"
)

// RetainedMessage represents a stored retained message
type RetainedMessage struct {
	Topic      string            `json:"topic"`
	Payload    []byte            `json:"payload"`
	QoS        byte              `json:"qos"`
	Timestamp  time.Time         `json:"timestamp"`
	ExpiryTime *time.Time        `json:"expiry_time,omitempty"`
	ClientID   string            `json:"client_id,omitempty"`
	Headers    map[string]string `json:"headers,omitempty"`
}

// Config defines retainer configuration
type Config struct {
	// Message retention time, 0 means no expiry
	MessageExpiryInterval time.Duration `yaml:"message_expiry_interval" json:"message_expiry_interval"`

	// Maximum message size allowed
	MaxPayloadSize int64 `yaml:"max_payload_size" json:"max_payload_size"`

	// Whether to stop publishing clear messages (empty payload with retain=true)
	StopPublishClearMsg bool `yaml:"stop_publish_clear_msg" json:"stop_publish_clear_msg"`

	// Maximum number of retained messages
	MaxRetainedMessages uint64 `yaml:"max_retained_messages" json:"max_retained_messages"`

	// Cleanup interval for expired messages
	CleanupInterval time.Duration `yaml:"cleanup_interval" json:"cleanup_interval"`
}

// DefaultConfig returns a default retainer configuration
func DefaultConfig() *Config {
	return &Config{
		MessageExpiryInterval: 0, // No expiry by default
		MaxPayloadSize:        1024 * 1024, // 1MB
		StopPublishClearMsg:   false,
		MaxRetainedMessages:   10000,
		CleanupInterval:       5 * time.Minute,
	}
}

// Retainer manages retained messages
type Retainer struct {
	storage messages.StorageBackend
	config  *Config
	mu      sync.RWMutex

	// Cleanup management
	cleanupTicker *time.Ticker
	stopCleanup   chan struct{}
}

// New creates a new retainer instance
func New(storage messages.StorageBackend, config *Config) *Retainer {
	if config == nil {
		config = DefaultConfig()
	}

	r := &Retainer{
		storage:     storage,
		config:      config,
		stopCleanup: make(chan struct{}),
	}

	// Start cleanup routine if expiry is enabled
	if config.CleanupInterval > 0 {
		r.startCleanupRoutine()
	}

	return r
}

// StoreRetained stores a retained message
func (r *Retainer) StoreRetained(ctx context.Context, topic string, payload []byte, qos byte, clientID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Check payload size limit
	if r.config.MaxPayloadSize > 0 && int64(len(payload)) > r.config.MaxPayloadSize {
		return fmt.Errorf("payload size %d exceeds maximum %d", len(payload), r.config.MaxPayloadSize)
	}

	// Empty payload means delete retained message
	if len(payload) == 0 {
		log.Printf("[INFO] Deleting retained message for topic: %s", topic)
		return r.storage.DeleteRetained(ctx, topic)
	}

	// Check max retained messages limit
	if r.config.MaxRetainedMessages > 0 {
		stats := r.storage.Stats()
		if stats.RetainedMessages >= r.config.MaxRetainedMessages {
			return fmt.Errorf("maximum retained messages limit (%d) reached", r.config.MaxRetainedMessages)
		}
	}

	// Create retained message
	msg := messages.Message{
		ID:        generateMessageID(),
		Topic:     topic,
		Payload:   payload,
		QoS:       qos,
		Retained:  true,
		Timestamp: time.Now(),
		ClientID:  clientID,
	}

	// Set expiry time if configured
	if r.config.MessageExpiryInterval > 0 {
		expiryTime := msg.Timestamp.Add(r.config.MessageExpiryInterval)
		msg.ExpiryTime = &expiryTime
	}

	log.Printf("[INFO] Storing retained message for topic: %s, payload size: %d", topic, len(payload))
	return r.storage.StoreRetained(ctx, topic, msg)
}

// GetRetainedMessages retrieves retained messages matching the topic filter
func (r *Retainer) GetRetainedMessages(ctx context.Context, topicFilter string) ([]RetainedMessage, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	msgs, err := r.storage.GetRetained(ctx, topicFilter)
	if err != nil {
		return nil, err
	}

	result := make([]RetainedMessage, 0, len(msgs))
	now := time.Now()

	for _, msg := range msgs {
		// Skip expired messages
		if msg.ExpiryTime != nil && now.After(*msg.ExpiryTime) {
			continue
		}

		retainedMsg := RetainedMessage{
			Topic:      msg.Topic,
			Payload:    msg.Payload,
			QoS:        msg.QoS,
			Timestamp:  msg.Timestamp,
			ExpiryTime: msg.ExpiryTime,
			ClientID:   msg.ClientID,
			Headers:    msg.Headers,
		}
		result = append(result, retainedMsg)
	}

	log.Printf("[DEBUG] Retrieved %d retained messages for filter: %s", len(result), topicFilter)
	return result, nil
}

// DeleteRetained removes a retained message
func (r *Retainer) DeleteRetained(ctx context.Context, topic string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	log.Printf("[INFO] Deleting retained message for topic: %s", topic)
	return r.storage.DeleteRetained(ctx, topic)
}

// GetStats returns retainer statistics
func (r *Retainer) GetStats() RetainerStats {
	r.mu.RLock()
	defer r.mu.RUnlock()

	storageStats := r.storage.Stats()
	return RetainerStats{
		RetainedMessages: storageStats.RetainedMessages,
		TotalSize:        storageStats.StorageSize,
		MaxMessages:      r.config.MaxRetainedMessages,
		MaxPayloadSize:   r.config.MaxPayloadSize,
		ExpiryInterval:   r.config.MessageExpiryInterval,
	}
}

// RetainerStats provides retainer statistics
type RetainerStats struct {
	RetainedMessages uint64        `json:"retained_messages"`
	TotalSize        uint64        `json:"total_size_bytes"`
	MaxMessages      uint64        `json:"max_messages"`
	MaxPayloadSize   int64         `json:"max_payload_size"`
	ExpiryInterval   time.Duration `json:"expiry_interval"`
}

// Close shuts down the retainer
func (r *Retainer) Close() error {
	if r.cleanupTicker != nil {
		r.cleanupTicker.Stop()
		close(r.stopCleanup)
	}
	return nil
}

// startCleanupRoutine starts the background cleanup process
func (r *Retainer) startCleanupRoutine() {
	r.cleanupTicker = time.NewTicker(r.config.CleanupInterval)

	go func() {
		for {
			select {
			case <-r.cleanupTicker.C:
				r.cleanupExpiredMessages()
			case <-r.stopCleanup:
				return
			}
		}
	}()

	log.Printf("[INFO] Started retained message cleanup routine with interval: %v", r.config.CleanupInterval)
}

// cleanupExpiredMessages removes expired retained messages
func (r *Retainer) cleanupExpiredMessages() {
	ctx := context.Background()

	// Get all retained messages
	msgs, err := r.storage.GetRetained(ctx, "#") // Get all topics
	if err != nil {
		log.Printf("[ERROR] Failed to get retained messages for cleanup: %v", err)
		return
	}

	now := time.Now()
	deletedCount := 0

	for _, msg := range msgs {
		if msg.ExpiryTime != nil && now.After(*msg.ExpiryTime) {
			if err := r.storage.DeleteRetained(ctx, msg.Topic); err != nil {
				log.Printf("[ERROR] Failed to delete expired retained message for topic %s: %v", msg.Topic, err)
			} else {
				deletedCount++
			}
		}
	}

	if deletedCount > 0 {
		log.Printf("[INFO] Cleanup completed: deleted %d expired retained messages", deletedCount)
	}
}

// MatchTopicFilter checks if a topic matches a topic filter
// This implementation supports basic MQTT wildcards:
// - # matches any topics at that level and below
// - + matches any single topic level
func MatchTopicFilter(topic, filter string) bool {
	// Exact match
	if topic == filter {
		return true
	}

	// Multi-level wildcard - # matches everything
	if filter == "#" {
		return true
	}

	// Split topic and filter into levels
	topicLevels := strings.Split(topic, "/")
	filterLevels := strings.Split(filter, "/")

	// Multi-level wildcard at the end
	if len(filterLevels) > 0 && filterLevels[len(filterLevels)-1] == "#" {
		// Check that all preceding levels match
		for i := 0; i < len(filterLevels)-1; i++ {
			if i >= len(topicLevels) {
				return false
			}
			if filterLevels[i] != "+" && filterLevels[i] != topicLevels[i] {
				return false
			}
		}
		return true
	}

	// Both must have same number of levels for single-level wildcards
	if len(topicLevels) != len(filterLevels) {
		return false
	}

	// Check each level
	for i := 0; i < len(filterLevels); i++ {
		if filterLevels[i] == "+" {
			// Single-level wildcard matches any single level
			continue
		}
		if filterLevels[i] != topicLevels[i] {
			return false
		}
	}

	return true
}

// generateMessageID creates a unique message identifier
func generateMessageID() string {
	return fmt.Sprintf("retained-%d-%d", time.Now().UnixNano(), randomInt())
}

// randomInt generates a random integer for ID generation
func randomInt() int64 {
	return time.Now().UnixNano() % 1000000
}