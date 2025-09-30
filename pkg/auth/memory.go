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

package auth

import (
	"fmt"
	"log"
	"sync"
)

// MemoryAuthenticator provides username/password authentication using in-memory storage
// This is similar to EMQX's internal authenticator but simplified for this implementation
type MemoryAuthenticator struct {
	users   map[string]*User
	enabled bool
	mu      sync.RWMutex
}

// NewMemoryAuthenticator creates a new memory-based authenticator
func NewMemoryAuthenticator() *MemoryAuthenticator {
	return &MemoryAuthenticator{
		users:   make(map[string]*User),
		enabled: true,
	}
}

// Name returns the name of this authenticator
func (ma *MemoryAuthenticator) Name() string {
	return "memory"
}

// Enabled returns whether this authenticator is enabled
func (ma *MemoryAuthenticator) Enabled() bool {
	ma.mu.RLock()
	defer ma.mu.RUnlock()
	return ma.enabled
}

// SetEnabled enables or disables this authenticator
func (ma *MemoryAuthenticator) SetEnabled(enabled bool) {
	ma.mu.Lock()
	defer ma.mu.Unlock()
	ma.enabled = enabled
}

// AddUser adds a user to the authenticator
func (ma *MemoryAuthenticator) AddUser(username, password string, algorithm HashAlgorithm) error {
	ma.mu.Lock()
	defer ma.mu.Unlock()

	if username == "" {
		return fmt.Errorf("username cannot be empty")
	}

	// Generate salt for SHA256
	salt := ""
	if algorithm == HashSHA256 {
		salt = username // Simple salt using username, in production use random salt
	}

	// Hash the password
	passwordHash, err := hashPassword(password, salt, algorithm)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}

	user := &User{
		Username:     username,
		PasswordHash: passwordHash,
		Algorithm:    algorithm,
		Salt:         salt,
		Enabled:      true,
	}

	ma.users[username] = user
	log.Printf("[INFO] Added user: %s with algorithm: %s", username, algorithm)
	return nil
}

// RemoveUser removes a user from the authenticator
func (ma *MemoryAuthenticator) RemoveUser(username string) error {
	ma.mu.Lock()
	defer ma.mu.Unlock()

	if _, exists := ma.users[username]; !exists {
		return fmt.Errorf("user not found: %s", username)
	}

	delete(ma.users, username)
	log.Printf("[INFO] Removed user: %s", username)
	return nil
}

// UpdateUser updates an existing user's password
func (ma *MemoryAuthenticator) UpdateUser(username, password string, algorithm HashAlgorithm) error {
	ma.mu.Lock()
	defer ma.mu.Unlock()

	user, exists := ma.users[username]
	if !exists {
		return fmt.Errorf("user not found: %s", username)
	}

	// Generate salt for SHA256
	salt := ""
	if algorithm == HashSHA256 {
		salt = username
	}

	// Hash the password
	passwordHash, err := hashPassword(password, salt, algorithm)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}

	user.PasswordHash = passwordHash
	user.Algorithm = algorithm
	user.Salt = salt

	log.Printf("[INFO] Updated user: %s with algorithm: %s", username, algorithm)
	return nil
}

// GetUser retrieves user information (without password hash for security)
func (ma *MemoryAuthenticator) GetUser(username string) (*User, error) {
	ma.mu.RLock()
	defer ma.mu.RUnlock()

	user, exists := ma.users[username]
	if !exists {
		return nil, fmt.Errorf("user not found: %s", username)
	}

	// Return a copy without the password hash for security
	return &User{
		Username:  user.Username,
		Algorithm: user.Algorithm,
		Enabled:   user.Enabled,
	}, nil
}

// ListUsers returns a list of all usernames
func (ma *MemoryAuthenticator) ListUsers() []string {
	ma.mu.RLock()
	defer ma.mu.RUnlock()

	users := make([]string, 0, len(ma.users))
	for username := range ma.users {
		users = append(users, username)
	}
	return users
}

// SetUserEnabled enables or disables a specific user
func (ma *MemoryAuthenticator) SetUserEnabled(username string, enabled bool) error {
	ma.mu.Lock()
	defer ma.mu.Unlock()

	user, exists := ma.users[username]
	if !exists {
		return fmt.Errorf("user not found: %s", username)
	}

	user.Enabled = enabled
	log.Printf("[INFO] User %s enabled status set to: %t", username, enabled)
	return nil
}

// Authenticate verifies the provided credentials
func (ma *MemoryAuthenticator) Authenticate(username, password string) AuthResult {
	ma.mu.RLock()
	defer ma.mu.RUnlock()

	if !ma.enabled {
		return AuthIgnore
	}

	// If username is empty, ignore authentication
	if username == "" {
		log.Printf("[DEBUG] Empty username provided, ignoring authentication")
		return AuthIgnore
	}

	// Check if user exists
	user, exists := ma.users[username]
	if !exists {
		log.Printf("[DEBUG] User not found in memory authenticator: %s", username)
		return AuthIgnore
	}

	// Check if user is enabled
	if !user.Enabled {
		log.Printf("[WARN] User %s is disabled", username)
		return AuthFailure
	}

	// Verify password
	if verifyPassword(password, user.PasswordHash, user.Salt, user.Algorithm) {
		log.Printf("[INFO] Password verification successful for user: %s", username)
		return AuthSuccess
	}

	log.Printf("[WARN] Password verification failed for user: %s", username)
	return AuthFailure
}

// Clear removes all users
func (ma *MemoryAuthenticator) Clear() {
	ma.mu.Lock()
	defer ma.mu.Unlock()

	ma.users = make(map[string]*User)
	log.Printf("[INFO] Cleared all users from memory authenticator")
}

// Count returns the number of users
func (ma *MemoryAuthenticator) Count() int {
	ma.mu.RLock()
	defer ma.mu.RUnlock()
	return len(ma.users)
}