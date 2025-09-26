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
// It allows for subscribing, unsubscribing, and retrieving subscribers for a given
// topic in a thread-safe manner.
package topic

import (
	"sync"

	"github.com/turtacn/emqx-go/pkg/actor"
)

// Store manages topic subscriptions and facilitates message routing.
// It uses a map to associate topics with a list of subscriber mailboxes.
// This implementation is a simple in-memory store, suitable for a proof-of-concept,
// and is safe for concurrent use.
type Store struct {
	subscriptions map[string][]*actor.Mailbox
	mu            sync.RWMutex
}

// NewStore creates and returns a new instance of a topic Store.
func NewStore() *Store {
	return &Store{
		subscriptions: make(map[string][]*actor.Mailbox),
	}
}

// Subscribe adds a subscriber's mailbox to a topic's subscription list.
// If the topic does not exist, it is created.
func (s *Store) Subscribe(topic string, mailbox *actor.Mailbox) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.subscriptions[topic] = append(s.subscriptions[topic], mailbox)
}

// Unsubscribe removes a subscriber's mailbox from a topic's subscription list.
// If the mailbox is the last subscriber for a topic, the topic is removed from
// the store.
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

// GetSubscribers returns a slice of all mailboxes subscribed to a given topic.
// It returns a copy of the subscriber list to prevent race conditions when the
// list is being iterated over by the caller.
func (s *Store) GetSubscribers(topic string) []*actor.Mailbox {
	s.mu.RLock()
	defer s.mu.RUnlock()
	// Return a copy to prevent race conditions on the slice itself.
	subs := s.subscriptions[topic]
	subsCopy := make([]*actor.Mailbox, len(subs))
	copy(subsCopy, subs)
	return subsCopy
}
