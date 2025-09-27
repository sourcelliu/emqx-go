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

// Package storage provides a generic, abstracted key-value storage system.
// It defines a core `Store` interface, allowing for different backend
// implementations (e.g., in-memory, persistent, distributed) to be used
// interchangeably throughout the application for storing session data,
// subscriptions, and other state.
package storage

import (
	"errors"
	"sync"
)

var (
	// ErrNotFound is the error returned by a Store's Get method when the
	// requested key does not exist in the store.
	ErrNotFound = errors.New("not found")
)

// Store defines the interface for a generic key-value store. This abstraction
// allows for various storage backends, such as in-memory maps or persistent
// databases, to be used for storing application state like session data.
type Store interface {
	// Get retrieves a value from the store using its key. It returns the
	// value and a nil error if the key is found. If the key is not found,
	// it returns nil and ErrNotFound.
	Get(key string) (any, error)

	// Set adds or updates a key-value pair in the store.
	Set(key string, value any) error

	// Delete removes a key-value pair from the store by its key.
	Delete(key string) error
}

// MemStore is a simple, thread-safe, in-memory implementation of the Store
// interface. It uses a map for storage and a RWMutex to handle concurrent
// access.
type MemStore struct {
	data map[string]any
	mu   sync.RWMutex
}

// NewMemStore creates and initializes a new MemStore instance.
func NewMemStore() *MemStore {
	return &MemStore{
		data: make(map[string]any),
	}
}

// Get retrieves a value from the in-memory store. It is safe for concurrent use.
//
// - key: The key of the value to retrieve.
//
// Returns the value associated with the key, or nil and ErrNotFound if the key
// is not present.
func (s *MemStore) Get(key string) (any, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.data[key]
	if !ok {
		return nil, ErrNotFound
	}
	return value, nil
}

// Set adds or updates a value in the in-memory store. It is safe for concurrent
// use.
//
// - key: The key to set.
// - value: The value to associate with the key.
//
// Returns nil.
func (s *MemStore) Set(key string, value any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[key] = value
	return nil
}

// Delete removes a key and its associated value from the in-empty store. It is
// safe for concurrent use. If the key does not exist, the operation is a no-op.
//
// - key: The key to delete.
//
// Returns nil.
func (s *MemStore) Delete(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.data, key)
	return nil
}