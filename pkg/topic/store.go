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

// Store provides a thread-safe, in-memory mapping of topic strings to lists of
// subscriber mailboxes. It is the central component for tracking which clients
// are subscribed to which topics.
type Store struct {
	subscriptions map[string][]*Subscription
	mu            sync.RWMutex
}

// NewStore creates and initializes a new, empty topic Store.
func NewStore() *Store {
	return &Store{
		subscriptions: make(map[string][]*Subscription),
	}
}

// Subscribe adds a subscriber's mailbox to the list of subscribers for a given
// topic with the specified QoS level. If the topic does not exist, it is created.
//
// - topic: The topic string to subscribe to.
// - mailbox: The mailbox of the subscribing actor, which will receive the messages.
// - qos: The QoS level for this subscription.
func (s *Store) Subscribe(topic string, mailbox *actor.Mailbox, qos byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sub := &Subscription{Mailbox: mailbox, QoS: qos}
	s.subscriptions[topic] = append(s.subscriptions[topic], sub)
}

// Unsubscribe removes a subscriber's mailbox from a specific topic's
// subscription list. If the mailbox is the last subscriber for that topic, the
// topic entry is removed from the store.
//
// - topic: The topic string to unsubscribe from.
// - mailbox: The mailbox of the unsubscribing actor.
func (s *Store) Unsubscribe(topic string, mailbox *actor.Mailbox) {
	s.mu.Lock()
	defer s.mu.Unlock()
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

	// Return a copy to prevent race conditions
	subsCopy := make([]*Subscription, len(allSubs))
	copy(subsCopy, allSubs)
	return subsCopy
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