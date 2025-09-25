package storage

import (
	"errors"
	"sync"
)

var (
	ErrNotFound = errors.New("not found")
)

// Store defines the interface for a generic key-value store.
type Store interface {
	Get(key string) (interface{}, error)
	Set(key string, value interface{}) error
	Delete(key string) error
}

// MemStore is an in-memory implementation of the Store interface.
// It is safe for concurrent use.
type MemStore struct {
	data map[string]interface{}
	mu   sync.RWMutex
}

// NewMemStore creates a new in-memory store.
func NewMemStore() *MemStore {
	return &MemStore{
		data: make(map[string]interface{}),
	}
}

// Get retrieves a value from the store.
func (s *MemStore) Get(key string) (interface{}, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.data[key]
	if !ok {
		return nil, ErrNotFound
	}
	return value, nil
}

// Set adds or updates a value in the store.
func (s *MemStore) Set(key string, value interface{}) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[key] = value
	return nil
}

// Delete removes a value from the store.
func (s *MemStore) Delete(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.data, key)
	return nil
}
