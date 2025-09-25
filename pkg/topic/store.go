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
func (s *Store) GetSubscribers(topic string) []*actor.Mailbox {
	s.mu.RLock()
	defer s.mu.RUnlock()
	// Return a copy to prevent race conditions on the slice itself.
	subs := s.subscriptions[topic]
	subsCopy := make([]*actor.Mailbox, len(subs))
	copy(subsCopy, subs)
	return subsCopy
}
