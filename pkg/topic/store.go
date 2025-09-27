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
// recipients. This implementation is simplified and does not currently support
// MQTT topic wildcards.
package topic

import (
	"sync"

	"github.com/turtacn/emqx-go/pkg/actor"
)

// Store provides a thread-safe, in-memory mapping of topic strings to lists of
// subscriber mailboxes. It is the central component for tracking which clients
// are subscribed to which topics.
type Store struct {
	subscriptions map[string][]*actor.Mailbox
	mu            sync.RWMutex
}

// NewStore creates and initializes a new, empty topic Store.
func NewStore() *Store {
	return &Store{
		subscriptions: make(map[string][]*actor.Mailbox),
	}
}

// Subscribe adds a subscriber's mailbox to the list of subscribers for a given
// topic. If the topic does not exist, it is created.
//
// - topic: The topic string to subscribe to.
// - mailbox: The mailbox of the subscribing actor, which will receive the messages.
func (s *Store) Subscribe(topic string, mailbox *actor.Mailbox) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.subscriptions[topic] = append(s.subscriptions[topic], mailbox)
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
		var newSubscribers []*actor.Mailbox
		for _, sub := range subscribers {
			if sub != mailbox {
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

// GetSubscribers returns a slice of all mailboxes subscribed to a specific
// topic.
//
// Note: This is a simplified implementation for the proof-of-concept and does
// not support MQTT wildcards (+ or #). It only performs exact string matches.
//
// - topic: The topic for which to retrieve subscribers.
//
// Returns a copy of the slice of subscriber mailboxes.
func (s *Store) GetSubscribers(topic string) []*actor.Mailbox {
	s.mu.RLock()
	defer s.mu.RUnlock()
	// Return a copy to prevent race conditions on the slice itself.
	subs := s.subscriptions[topic]
	subsCopy := make([]*actor.Mailbox, len(subs))
	copy(subsCopy, subs)
	return subsCopy
}