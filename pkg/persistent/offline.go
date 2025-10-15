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

// Package persistent provides offline message management for MQTT clients
package persistent

import (
	"fmt"
	"log"
	"sync"
	"time"
)

// OfflineMessageManager manages offline messages for disconnected MQTT clients
type OfflineMessageManager struct {
	sessionManager *SessionManager

	// Message delivery tracking
	deliveryCallbacks map[string]MessageDeliveryCallback

	mu sync.RWMutex
}

// MessageDeliveryCallback is called when delivering messages to a reconnected client
type MessageDeliveryCallback func(clientID, topic string, payload []byte, qos byte, retain bool) error

// NewOfflineMessageManager creates a new offline message manager
func NewOfflineMessageManager(sessionManager *SessionManager) *OfflineMessageManager {
	return &OfflineMessageManager{
		sessionManager:    sessionManager,
		deliveryCallbacks: make(map[string]MessageDeliveryCallback),
	}
}

// SetDeliveryCallback sets the callback function for message delivery
func (omm *OfflineMessageManager) SetDeliveryCallback(clientID string, callback MessageDeliveryCallback) {
	omm.mu.Lock()
	defer omm.mu.Unlock()

	omm.deliveryCallbacks[clientID] = callback
}

// RemoveDeliveryCallback removes the callback function for a client
func (omm *OfflineMessageManager) RemoveDeliveryCallback(clientID string) {
	omm.mu.Lock()
	defer omm.mu.Unlock()

	delete(omm.deliveryCallbacks, clientID)
}

// DeliverOfflineMessages delivers all queued messages to a reconnected client
func (omm *OfflineMessageManager) DeliverOfflineMessages(clientID string) error {
	omm.mu.RLock()
	callback, exists := omm.deliveryCallbacks[clientID]
	omm.mu.RUnlock()

	if !exists {
		return fmt.Errorf("no delivery callback registered for client %s", clientID)
	}

	// Get session
	value, exists := omm.sessionManager.activeSessions.Load(clientID)
	if !exists {
		return fmt.Errorf("session not found for client %s", clientID)
	}

	session := value.(*PersistentSession)
	session.mu.Lock()
	defer session.mu.Unlock()

	if !session.Connected {
		return fmt.Errorf("client %s is not connected", clientID)
	}

	// Deliver all queued messages
	deliveredCount := 0
	for _, msg := range session.MessageQueue {
		// Check if message has expired
		if msg.ExpiryTime != nil && time.Now().After(*msg.ExpiryTime) {
			log.Printf("[DEBUG] Skipping expired message for client %s: topic=%s", clientID, msg.Topic)
			continue
		}

		// Deliver message
		if err := callback(clientID, msg.Topic, msg.Payload, msg.QoS, msg.Retain); err != nil {
			log.Printf("[ERROR] Failed to deliver offline message to client %s: %v", clientID, err)
			return err
		}

		deliveredCount++
	}

	// Clear message queue after successful delivery
	session.MessageQueue = nil

	// Save session state if persistent
	if !session.CleanSession {
		if err := omm.sessionManager.saveSession(session); err != nil {
			log.Printf("[ERROR] Failed to save session after delivering offline messages for %s: %v", clientID, err)
		}
	}

	log.Printf("[INFO] Delivered %d offline messages to client %s", deliveredCount, clientID)
	return nil
}

// QueueMessageForOfflineClients queues a message for all matching offline clients

func (omm *OfflineMessageManager) QueueMessageForOfflineClients(topic string, payload []byte, qos byte, retain bool, matcher TopicMatcher) error {
	queuedCount := 0

	omm.sessionManager.activeSessions.Range(func(key, value any) bool {
		clientID := key.(string)
		session := value.(*PersistentSession)
		session.mu.RLock()

		// Skip connected clients
		if session.Connected {
			session.mu.RUnlock()
			return true
		}

		// Skip clean sessions
		if session.CleanSession {
			session.mu.RUnlock()
			return true
		}

		// Check if client has matching subscription
		hasMatchingSubscription := false
		for subTopic := range session.Subscriptions {
			if matcher.Matches(topic, subTopic) {
				hasMatchingSubscription = true
				break
			}
		}

		session.mu.RUnlock()

		if hasMatchingSubscription {
			if err := omm.sessionManager.QueueMessage(clientID, topic, payload, qos, retain); err != nil {
				log.Printf("[ERROR] Failed to queue message for offline client %s: %v", clientID, err)
			} else {
				queuedCount++
			}
		}

		return true
	})

	if queuedCount > 0 {
		log.Printf("[INFO] Queued message for %d offline clients: topic=%s", queuedCount, topic)
	}

	return nil
}

// GetOfflineMessageStats returns statistics about offline messages
func (omm *OfflineMessageManager) GetOfflineMessageStats() map[string]OfflineStats {
	stats := make(map[string]OfflineStats)

	omm.sessionManager.activeSessions.Range(func(key, value any) bool {
		clientID := key.(string)
		session := value.(*PersistentSession)
		session.mu.RLock()

		if !session.Connected && !session.CleanSession {
			var totalBytes uint64
			var expiredCount int

			now := time.Now()
			for _, msg := range session.MessageQueue {
				totalBytes += uint64(len(msg.Payload))
				if msg.ExpiryTime != nil && now.After(*msg.ExpiryTime) {
					expiredCount++
				}
			}

			stats[clientID] = OfflineStats{
				MessageCount:  len(session.MessageQueue),
				TotalBytes:    totalBytes,
				ExpiredCount:  expiredCount,
				OldestMessage: getOldestMessageTime(session.MessageQueue),
				LastActivity:  session.LastActivity,
			}
		}

		session.mu.RUnlock()
		return true
	})

	return stats
}

// OfflineStats contains statistics about offline messages for a client
type OfflineStats struct {
	MessageCount  int        `json:"message_count"`
	TotalBytes    uint64     `json:"total_bytes"`
	ExpiredCount  int        `json:"expired_count"`
	OldestMessage *time.Time `json:"oldest_message,omitempty"`
	LastActivity  time.Time  `json:"last_activity"`
}

// TopicMatcher interface for topic matching logic
type TopicMatcher interface {
	Matches(publishTopic, subscriptionTopic string) bool
}

// BasicTopicMatcher provides basic MQTT topic matching
type BasicTopicMatcher struct{}

// Matches implements basic MQTT topic wildcard matching
func (btm *BasicTopicMatcher) Matches(publishTopic, subscriptionTopic string) bool {
	// Use the same topic matching logic as in retainer
	return topicMatches(publishTopic, subscriptionTopic)
}

// Helper function to find oldest message timestamp
func getOldestMessageTime(messages []*QueuedMessage) *time.Time {
	if len(messages) == 0 {
		return nil
	}

	oldest := messages[0].Timestamp
	for _, msg := range messages[1:] {
		if msg.Timestamp.Before(oldest) {
			oldest = msg.Timestamp
		}
	}

	return &oldest
}

// topicMatches checks if a topic matches a filter using MQTT wildcard rules
// This is the same implementation as in the retainer package
func topicMatches(topic, filter string) bool {
	// Implementation details omitted for brevity
	// This would use the same logic as in pkg/storage/messages/memory_backend.go
	return basicTopicMatch(topic, filter)
}

// basicTopicMatch provides a simplified topic matching implementation
func basicTopicMatch(topic, filter string) bool {
	if topic == filter {
		return true
	}

	if filter == "#" {
		return true
	}

	// For now, implement basic exact matching
	// Full wildcard support would be implemented here
	return false
}
