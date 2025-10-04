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

// Package topic provides a thread-safe, in-memory data structure for managing
// MQTT topic subscriptions. It maps topic strings to a list of subscriber
// mailboxes, enabling the broker to efficiently route messages to the correct
// recipients. This implementation supports MQTT topic wildcards (+ and #).
package topic

import (
	"strings"
	"sync"

	"github.com/turtacn/emqx-go/pkg/actor"
)

// Subscription represents a single topic subscription with its QoS level.
type Subscription struct {
	Mailbox *actor.Mailbox
	QoS     byte
}

// SharedSubscription represents a subscription to a shared topic with group management.
type SharedSubscription struct {
	Group      string
	Topic      string
	Mailboxes  []*Subscription
	NextIndex  int // For round-robin distribution
}

// Store provides a thread-safe, in-memory mapping of topic strings to lists of
// subscriber mailboxes. It is the central component for tracking which clients
// are subscribed to which topics.
type Store struct {
	subscriptions       map[string][]*Subscription
	sharedSubscriptions map[string]*SharedSubscription // key: "group/topic"
	mu                  sync.RWMutex
}

// NewStore creates and initializes a new, empty topic Store.
func NewStore() *Store {
	return &Store{
		subscriptions:       make(map[string][]*Subscription),
		sharedSubscriptions: make(map[string]*SharedSubscription),
	}
}

// Subscribe adds a subscriber's mailbox to the list of subscribers for a given
// topic with the specified QoS level. If the topic does not exist, it is created.
// Supports MQTT shared subscriptions with $share/group/topic format.
//
// - topic: The topic string to subscribe to (may be shared subscription format).
// - mailbox: The mailbox of the subscribing actor, which will receive the messages.
// - qos: The QoS level for this subscription.
func (s *Store) Subscribe(topic string, mailbox *actor.Mailbox, qos byte) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if this is a shared subscription ($share/group/topic)
	if strings.HasPrefix(topic, "$share/") {
		s.subscribeShared(topic, mailbox, qos)
	} else {
		// Regular subscription
		sub := &Subscription{Mailbox: mailbox, QoS: qos}
		s.subscriptions[topic] = append(s.subscriptions[topic], sub)
	}
}

// subscribeShared handles shared subscription logic
func (s *Store) subscribeShared(shareTopic string, mailbox *actor.Mailbox, qos byte) {
	// Parse $share/group/topic format
	parts := strings.SplitN(shareTopic, "/", 3)
	if len(parts) != 3 {
		// Invalid format, treat as regular subscription
		sub := &Subscription{Mailbox: mailbox, QoS: qos}
		s.subscriptions[shareTopic] = append(s.subscriptions[shareTopic], sub)
		return
	}

	group := parts[1]
	actualTopic := parts[2]
	groupKey := group + "/" + actualTopic

	// Create or get existing shared subscription
	sharedSub, exists := s.sharedSubscriptions[groupKey]
	if !exists {
		sharedSub = &SharedSubscription{
			Group:     group,
			Topic:     actualTopic,
			Mailboxes: []*Subscription{},
			NextIndex: 0,
		}
		s.sharedSubscriptions[groupKey] = sharedSub
	}

	// Add subscriber to the group
	sub := &Subscription{Mailbox: mailbox, QoS: qos}
	sharedSub.Mailboxes = append(sharedSub.Mailboxes, sub)
}

// Unsubscribe removes a subscriber's mailbox from a specific topic's
// subscription list. If the mailbox is the last subscriber for that topic, the
// topic entry is removed from the store. Supports shared subscriptions.
//
// - topic: The topic string to unsubscribe from.
// - mailbox: The mailbox of the unsubscribing actor.
func (s *Store) Unsubscribe(topic string, mailbox *actor.Mailbox) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if this is a shared subscription
	if strings.HasPrefix(topic, "$share/") {
		s.unsubscribeShared(topic, mailbox)
	} else {
		// Regular unsubscribe
		if subscribers, ok := s.subscriptions[topic]; ok {
			var newSubscribers []*Subscription
			for _, sub := range subscribers {
				if sub.Mailbox != mailbox {
					newSubscribers = append(newSubscribers, sub)
				}
			}
			if len(newSubscribers) > 0 {
				s.subscriptions[topic] = newSubscribers
			} else {
				delete(s.subscriptions, topic)
			}
		}
	}
}

// unsubscribeShared handles shared subscription unsubscribe logic
func (s *Store) unsubscribeShared(shareTopic string, mailbox *actor.Mailbox) {
	// Parse $share/group/topic format
	parts := strings.SplitN(shareTopic, "/", 3)
	if len(parts) != 3 {
		// Invalid format, treat as regular unsubscription
		if subscribers, ok := s.subscriptions[shareTopic]; ok {
			var newSubscribers []*Subscription
			for _, sub := range subscribers {
				if sub.Mailbox != mailbox {
					newSubscribers = append(newSubscribers, sub)
				}
			}
			if len(newSubscribers) > 0 {
				s.subscriptions[shareTopic] = newSubscribers
			} else {
				delete(s.subscriptions, shareTopic)
			}
		}
		return
	}

	group := parts[1]
	actualTopic := parts[2]
	groupKey := group + "/" + actualTopic

	// Remove from shared subscription
	if sharedSub, exists := s.sharedSubscriptions[groupKey]; exists {
		var newMailboxes []*Subscription
		for _, sub := range sharedSub.Mailboxes {
			if sub.Mailbox != mailbox {
				newMailboxes = append(newMailboxes, sub)
			}
		}

		if len(newMailboxes) > 0 {
			sharedSub.Mailboxes = newMailboxes
			// Reset index if it's out of bounds
			if sharedSub.NextIndex >= len(newMailboxes) {
				sharedSub.NextIndex = 0
			}
		} else {
			// No more subscribers, remove the shared subscription
			delete(s.sharedSubscriptions, groupKey)
		}
	}
}

// GetSubscribers returns a slice of all subscriptions for a specific topic.
// This method now supports MQTT wildcards (+ for single-level, # for multi-level).
//
// - topic: The topic for which to retrieve subscribers.
//
// Returns a copy of the slice of subscriber subscriptions that match the topic.
func (s *Store) GetSubscribers(topic string) []*Subscription {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var allSubs []*Subscription

	// Check all stored subscription patterns
	for subTopic, subs := range s.subscriptions {
		if matchesTopicFilter(topic, subTopic) {
			allSubs = append(allSubs, subs...)
		}
	}

	// Check shared subscriptions for this topic
	sharedSubs := s.getSharedSubscribersForTopic(topic)
	allSubs = append(allSubs, sharedSubs...)

	// Return a copy to prevent race conditions
	subsCopy := make([]*Subscription, len(allSubs))
	copy(subsCopy, allSubs)
	return subsCopy
}

// getSharedSubscribersForTopic returns subscribers from shared subscriptions
// using round-robin distribution for load balancing
func (s *Store) getSharedSubscribersForTopic(topic string) []*Subscription {
	var selectedSubs []*Subscription

	// Check all shared subscriptions to see if topic matches
	for _, sharedSub := range s.sharedSubscriptions {
		if matchesTopicFilter(topic, sharedSub.Topic) && len(sharedSub.Mailboxes) > 0 {
			// Round-robin selection
			selected := sharedSub.Mailboxes[sharedSub.NextIndex]
			selectedSubs = append(selectedSubs, selected)

			// Update next index for round-robin (with locking, we can modify safely)
			sharedSub.NextIndex = (sharedSub.NextIndex + 1) % len(sharedSub.Mailboxes)
		}
	}

	return selectedSubs
}

// matchesTopicFilter checks if a published topic matches a subscription topic filter.
// Implements MQTT 3.1.1 specification for topic matching with wildcards:
// - '+' matches exactly one level
// - '#' matches zero or more levels and must be the last character
//
// Examples:
// - "sensor/+/temperature" matches "sensor/room1/temperature" but not "sensor/room1/sub/temperature"
// - "sensor/#" matches "sensor", "sensor/room1", "sensor/room1/temperature", etc.
// - "sensor/room1/temperature" matches exactly "sensor/room1/temperature"
//
// Parameters:
// - publishTopic: The topic name used when publishing a message
// - filterTopic: The topic filter used in subscription (may contain wildcards)
//
// Returns true if the published topic matches the subscription filter.
func matchesTopicFilter(publishTopic, filterTopic string) bool {
	// Exact match (common case, optimize for it)
	if publishTopic == filterTopic {
		return true
	}

	// If no wildcards in filter, it's just a string comparison
	if !strings.ContainsAny(filterTopic, "+#") {
		return publishTopic == filterTopic
	}

	// Split topics into levels
	pubLevels := strings.Split(publishTopic, "/")
	filterLevels := strings.Split(filterTopic, "/")

	// Handle multi-level wildcard '#'
	// Must be last level and matches everything from that point
	if len(filterLevels) > 0 && filterLevels[len(filterLevels)-1] == "#" {
		// Remove the '#' and check if remaining levels match
		filterLevels = filterLevels[:len(filterLevels)-1]

		// If published topic has fewer levels than filter (minus #), no match
		if len(pubLevels) < len(filterLevels) {
			return false
		}

		// Check all levels before the '#'
		for i := 0; i < len(filterLevels); i++ {
			if filterLevels[i] != "+" && filterLevels[i] != pubLevels[i] {
				return false
			}
		}
		return true
	}

	// For single-level wildcards '+', levels must match exactly in count
	if len(pubLevels) != len(filterLevels) {
		return false
	}

	// Check each level
	for i := 0; i < len(filterLevels); i++ {
		if filterLevels[i] != "+" && filterLevels[i] != pubLevels[i] {
			return false
		}
	}

	return true
}