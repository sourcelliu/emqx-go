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

// package storage provides a generic key-value store interface and an in-memory
// implementation. This is used by the broker to store session information and
// other data that needs to be persisted or shared.
package storage

import (
	"errors"
	"sync"
)

var (
	// ErrNotFound is returned when a key is not found in the store.
	ErrNotFound = errors.New("not found")
)

// Store defines the interface for a generic key-value store.
// This interface provides basic CRUD operations for storing and retrieving data.
// It is designed to be implementation-agnostic, allowing for different storage
// backends, such as in-memory, disk-based, or distributed stores.
type Store interface {
	// Get retrieves a value from the store by its key.
	// It returns the value and a nil error on success.
	// If the key is not found, it returns nil and ErrNotFound.
	Get(key string) (interface{}, error)
	// Set adds or updates a value in the store.
	// It takes a key and a value, and returns an error if the operation fails.
	Set(key string, value interface{}) error
	// Delete removes a value from the store by its key.
	// It returns an error if the operation fails.
	Delete(key string) error
}

// MemStore is an in-memory implementation of the Store interface.
// It uses a map to store key-value pairs and a RWMutex to ensure thread safety,
// making it safe for concurrent use.
type MemStore struct {
	data map[string]interface{}
	mu   sync.RWMutex
}

// NewMemStore creates and returns a new instance of MemStore.
func NewMemStore() *MemStore {
	return &MemStore{
		data: make(map[string]interface{}),
	}
}

// Get retrieves a value from the in-memory store.
// It uses a read lock to allow for concurrent reads.
func (s *MemStore) Get(key string) (interface{}, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.data[key]
	if !ok {
		return nil, ErrNotFound
	}
	return value, nil
}

// Set adds or updates a value in the in-memory store.
// It uses a write lock to ensure that only one goroutine can modify the store
// at a time.
func (s *MemStore) Set(key string, value interface{}) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[key] = value
	return nil
}

// Delete removes a value from the in-memory store.
// It uses a write lock to ensure that only one goroutine can modify the store
// at a time.
func (s *MemStore) Delete(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.data, key)
	return nil
}
