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
// implementation.
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
// This allows for different storage backends (e.g., in-memory, Redis, etc.).
type Store interface {
	Get(key string) (any, error)
	Set(key string, value any) error
	Delete(key string) error
}

// MemStore is a thread-safe, in-memory implementation of the Store interface.
type MemStore struct {
	data map[string]any
	mu   sync.RWMutex
}

// NewMemStore creates a new in-memory store.
func NewMemStore() *MemStore {
	return &MemStore{
		data: make(map[string]any),
	}
}

// Get retrieves a value from the store by its key.
func (s *MemStore) Get(key string) (any, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.data[key]
	if !ok {
		return nil, ErrNotFound
	}
	return value, nil
}

// Set adds or updates a value in the store.
func (s *MemStore) Set(key string, value any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[key] = value
	return nil
}

// Delete removes a value from the store by its key.
func (s *MemStore) Delete(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.data, key)
	return nil
}