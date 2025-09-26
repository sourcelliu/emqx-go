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

// package topic provides an in-memory store for managing MQTT topic subscriptions.
package topic

import (
	"sync"

	"github.com/turtacn/emqx-go/pkg/actor"
)

// Store manages topic subscriptions and message routing.
// This is a simple in-memory implementation for the PoC.
type Store struct {
	subscriptions map[string][]*actor.Mailbox
	mu            sync.RWMutex
}

// NewStore creates a new topic store.
func NewStore() *Store {
	return &Store{
		subscriptions: make(map[string][]*actor.Mailbox),
	}
}

// Subscribe adds a subscriber's mailbox to a topic.
func (s *Store) Subscribe(topic string, mailbox *actor.Mailbox) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.subscriptions[topic] = append(s.subscriptions[topic], mailbox)
}

// Unsubscribe removes a subscriber's mailbox from a topic.
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

// GetSubscribers returns all mailboxes subscribed to a topic.
// NOTE: This is a simplified implementation that does not handle wildcards.
func (s *Store) GetSubscribers(topic string) []*actor.Mailbox {
	s.mu.RLock()
	defer s.mu.RUnlock()
	// Return a copy to prevent race conditions on the slice itself.
	subs := s.subscriptions[topic]
	subsCopy := make([]*actor.Mailbox, len(subs))
	copy(subsCopy, subs)
	return subsCopy
}