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

package storage

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMemStore(t *testing.T) {
	s := NewMemStore()
	assert.NotNil(t, s)

	// Test Set and Get
	err := s.Set("key1", "value1")
	assert.NoError(t, err)

	value, err := s.Get("key1")
	assert.NoError(t, err)
	assert.Equal(t, "value1", value)

	// Test Get not found
	_, err = s.Get("nonexistent")
	assert.ErrorIs(t, err, ErrNotFound)

	// Test Delete
	err = s.Delete("key1")
	assert.NoError(t, err)

	_, err = s.Get("key1")
	assert.ErrorIs(t, err, ErrNotFound)
}