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

package messages

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"
	"time"
)

// MemoryBackend implements StorageBackend using in-memory data structures
type MemoryBackend struct {
	// Durable streams: streamID -> messages
	streams map[string][]Message
	streamMeta map[string]*StreamInfo

	// Retained messages: topic -> message
	retained map[string]Message

	// Subscriptions
	subscriptions map[string]*memorySubscription
	subCounter    uint64

	// Configuration
	config *Config

	// Synchronization
	mu sync.RWMutex
}

// memoryIterator implements Iterator for in-memory storage
type memoryIterator struct {
	messages []Message
	position int
	closed   bool
	mu       sync.Mutex
}

// memorySubscription implements Subscription for in-memory storage
type memorySubscription struct {
	id       string
	streamID string
	opts     SubscriptionOptions
	msgChan  chan []Message
	closed   bool
	mu       sync.Mutex
}

// NewMemoryBackend creates a new in-memory storage backend
func NewMemoryBackend(config *Config) (*MemoryBackend, error) {
	backend := &MemoryBackend{
		streams:       make(map[string][]Message),
		streamMeta:    make(map[string]*StreamInfo),
		retained:      make(map[string]Message),
		subscriptions: make(map[string]*memorySubscription),
		config:        config,
	}

	// Start cleanup routine for expired messages
	go backend.cleanupRoutine()

	return backend, nil
}

// AppendMessages adds messages to a stream
func (mb *MemoryBackend) AppendMessages(ctx context.Context, streamID string, messages []Message) error {
	mb.mu.Lock()
	defer mb.mu.Unlock()

	log.Printf("[DEBUG] Appending %d messages to stream %s", len(messages), streamID)

	// Initialize stream if it doesn't exist
	if _, exists := mb.streams[streamID]; !exists {
		mb.streams[streamID] = make([]Message, 0)
		mb.streamMeta[streamID] = &StreamInfo{
			ID:        streamID,
			StartTime: time.Now(),
		}
	}

	// Add sequence numbers and update metadata
	stream := mb.streams[streamID]
	meta := mb.streamMeta[streamID]

	for i, msg := range messages {
		msg.StreamID = streamID
		msg.SequenceNo = uint64(len(stream) + i + 1)

		// Set timestamp if not provided
		if msg.Timestamp.IsZero() {
			msg.Timestamp = time.Now()
		}

		// Update stream metadata
		if meta.Topic == "" {
			meta.Topic = msg.Topic
		}
		meta.EndTime = msg.Timestamp
		meta.MessageCount++
		meta.SizeBytes += uint64(len(msg.Payload))

		stream = append(stream, msg)
	}

	mb.streams[streamID] = stream

	// Notify subscriptions
	mb.notifySubscriptions(streamID, messages)

	// Apply retention policy
	mb.applyRetention(streamID)

	log.Printf("[INFO] Stored %d messages in stream %s (total: %d)", len(messages), streamID, len(stream))
	return nil
}

// GetStreams returns streams matching the topic filter and time range
func (mb *MemoryBackend) GetStreams(ctx context.Context, topicFilter string, startTime time.Time) ([]StreamInfo, error) {
	mb.mu.RLock()
	defer mb.mu.RUnlock()

	var matchingStreams []StreamInfo

	log.Printf("[DEBUG] GetStreams: looking for filter '%s', have %d streams", topicFilter, len(mb.streamMeta))
	for streamID, meta := range mb.streamMeta {
		log.Printf("[DEBUG] Stream %s: topic='%s', endTime=%s", streamID, meta.Topic, meta.EndTime.Format(time.RFC3339))
		// Simple topic matching (exact match for now)
		if topicMatches(meta.Topic, topicFilter) && meta.EndTime.After(startTime) {
			matchingStreams = append(matchingStreams, *meta)
			log.Printf("[DEBUG] Stream %s matches filter '%s'", streamID, topicFilter)
		}
	}

	// Sort by start time
	sort.Slice(matchingStreams, func(i, j int) bool {
		return matchingStreams[i].StartTime.Before(matchingStreams[j].StartTime)
	})

	log.Printf("[DEBUG] Found %d streams matching filter '%s'", len(matchingStreams), topicFilter)
	return matchingStreams, nil
}

// CreateIterator creates an iterator for reading messages from a stream
func (mb *MemoryBackend) CreateIterator(ctx context.Context, streamID string, topicFilter string, startTime time.Time) (Iterator, error) {
	mb.mu.RLock()
	defer mb.mu.RUnlock()

	messages, exists := mb.streams[streamID]
	if !exists {
		log.Printf("[DEBUG] Stream %s not found", streamID)
		return &memoryIterator{messages: []Message{}}, nil
	}

	// Filter messages by topic and time
	var filteredMessages []Message
	for _, msg := range messages {
		if topicMatches(msg.Topic, topicFilter) && msg.Timestamp.After(startTime) {
			filteredMessages = append(filteredMessages, msg)
		}
	}

	log.Printf("[DEBUG] Created iterator for stream %s with %d messages", streamID, len(filteredMessages))
	return &memoryIterator{
		messages: filteredMessages,
		position: 0,
	}, nil
}

// Subscribe creates a real-time subscription to a stream
func (mb *MemoryBackend) Subscribe(ctx context.Context, streamID string, opts SubscriptionOptions) (Subscription, error) {
	mb.mu.Lock()
	defer mb.mu.Unlock()

	mb.subCounter++
	subID := fmt.Sprintf("sub-%d", mb.subCounter)

	sub := &memorySubscription{
		id:       subID,
		streamID: streamID,
		opts:     opts,
		msgChan:  make(chan []Message, 10), // Buffered channel
	}

	mb.subscriptions[subID] = sub

	log.Printf("[INFO] Created subscription %s for stream %s", subID, streamID)
	return sub, nil
}

// StoreRetained stores a retained message
func (mb *MemoryBackend) StoreRetained(ctx context.Context, topic string, message Message) error {
	mb.mu.Lock()
	defer mb.mu.Unlock()

	// Empty payload deletes the retained message
	if len(message.Payload) == 0 {
		delete(mb.retained, topic)
		log.Printf("[INFO] Deleted retained message for topic %s", topic)
		return nil
	}

	message.Retained = true
	mb.retained[topic] = message

	log.Printf("[INFO] Stored retained message for topic %s", topic)
	return nil
}

// GetRetained retrieves retained messages matching the topic filter
func (mb *MemoryBackend) GetRetained(ctx context.Context, topicFilter string) ([]Message, error) {
	mb.mu.RLock()
	defer mb.mu.RUnlock()

	var messages []Message
	for topic, msg := range mb.retained {
		if topicMatches(topic, topicFilter) {
			messages = append(messages, msg)
		}
	}

	log.Printf("[DEBUG] Found %d retained messages for filter '%s'", len(messages), topicFilter)
	return messages, nil
}

// DeleteRetained removes a retained message
func (mb *MemoryBackend) DeleteRetained(ctx context.Context, topic string) error {
	mb.mu.Lock()
	defer mb.mu.Unlock()

	delete(mb.retained, topic)
	log.Printf("[INFO] Deleted retained message for topic %s", topic)
	return nil
}

// Stats returns storage statistics
func (mb *MemoryBackend) Stats() StorageStats {
	mb.mu.RLock()
	defer mb.mu.RUnlock()

	var totalMessages, totalSize uint64
	for _, meta := range mb.streamMeta {
		totalMessages += meta.MessageCount
		totalSize += meta.SizeBytes
	}

	return StorageStats{
		TotalMessages:    totalMessages,
		TotalStreams:     uint64(len(mb.streams)),
		RetainedMessages: uint64(len(mb.retained)),
		StorageSize:      totalSize,
		BackendType:      "memory",
		BackendSpecific: map[string]any{
			"active_subscriptions": len(mb.subscriptions),
		},
	}
}

// Close shuts down the backend
func (mb *MemoryBackend) Close() error {
	mb.mu.Lock()
	defer mb.mu.Unlock()

	// Close all subscriptions
	for _, sub := range mb.subscriptions {
		sub.Close()
	}

	log.Println("[INFO] Memory backend closed")
	return nil
}

// Helper methods

// notifySubscriptions sends new messages to active subscriptions
func (mb *MemoryBackend) notifySubscriptions(streamID string, messages []Message) {
	for _, sub := range mb.subscriptions {
		if sub.streamID == streamID && !sub.closed {
			select {
			case sub.msgChan <- messages:
			default:
				log.Printf("[WARN] Subscription %s buffer full, dropping messages", sub.id)
			}
		}
	}
}

// applyRetention removes old messages based on retention policy
func (mb *MemoryBackend) applyRetention(streamID string) {
	if mb.config.RetentionDays <= 0 {
		return
	}

	cutoff := time.Now().AddDate(0, 0, -mb.config.RetentionDays)
	messages := mb.streams[streamID]

	// Find first message to keep
	keepFrom := 0
	for i, msg := range messages {
		if msg.Timestamp.After(cutoff) {
			keepFrom = i
			break
		}
	}

	if keepFrom > 0 {
		mb.streams[streamID] = messages[keepFrom:]
		meta := mb.streamMeta[streamID]
		meta.MessageCount = uint64(len(messages) - keepFrom)
		log.Printf("[INFO] Applied retention to stream %s, removed %d old messages", streamID, keepFrom)
	}
}

// cleanupRoutine periodically cleans up expired messages
func (mb *MemoryBackend) cleanupRoutine() {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for range ticker.C {
		mb.mu.Lock()
		now := time.Now()

		// Clean up expired retained messages
		for topic, msg := range mb.retained {
			if msg.ExpiryTime != nil && msg.ExpiryTime.Before(now) {
				delete(mb.retained, topic)
				log.Printf("[INFO] Expired retained message for topic %s", topic)
			}
		}

		// Clean up streams based on retention
		for streamID := range mb.streams {
			mb.applyRetention(streamID)
		}

		mb.mu.Unlock()
	}
}

// topicMatches checks if a topic matches a filter using MQTT wildcard rules
func topicMatches(topic, filter string) bool {
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

// Iterator implementation

func (mi *memoryIterator) Next(ctx context.Context, limit int) ([]Message, error) {
	mi.mu.Lock()
	defer mi.mu.Unlock()

	if mi.closed {
		return nil, fmt.Errorf("iterator is closed")
	}

	if mi.position >= len(mi.messages) {
		return []Message{}, nil
	}

	end := mi.position + limit
	if end > len(mi.messages) {
		end = len(mi.messages)
	}

	messages := mi.messages[mi.position:end]
	mi.position = end

	return messages, nil
}

func (mi *memoryIterator) HasMore() bool {
	mi.mu.Lock()
	defer mi.mu.Unlock()

	return !mi.closed && mi.position < len(mi.messages)
}

func (mi *memoryIterator) Close() error {
	mi.mu.Lock()
	defer mi.mu.Unlock()

	mi.closed = true
	return nil
}

// Subscription implementation

func (ms *memorySubscription) Messages() <-chan []Message {
	return ms.msgChan
}

func (ms *memorySubscription) Ack(seqno uint64) error {
	// For memory backend, ack is a no-op
	return nil
}

func (ms *memorySubscription) Close() error {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	if !ms.closed {
		ms.closed = true
		close(ms.msgChan)
		log.Printf("[INFO] Closed subscription %s", ms.id)
	}

	return nil
}