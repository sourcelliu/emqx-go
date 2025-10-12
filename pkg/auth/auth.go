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

// Package auth provides authentication mechanisms for MQTT clients.
// It supports username/password authentication with configurable password
// hashing algorithms including plain text, SHA256, and bcrypt.
package auth

import (
	"crypto/sha256"
	"fmt"
	"log"

	"golang.org/x/crypto/bcrypt"
)

// HashAlgorithm defines the password hashing algorithm type
type HashAlgorithm string

const (
	// HashPlain represents plain text passwords (not recommended for production)
	HashPlain HashAlgorithm = "plain"
	// HashSHA256 represents SHA256 hashed passwords
	HashSHA256 HashAlgorithm = "sha256"
	// HashBcrypt represents bcrypt hashed passwords (recommended)
	HashBcrypt HashAlgorithm = "bcrypt"
)

// User represents a user credential entry
type User struct {
	Username     string        `json:"username"`
	PasswordHash string        `json:"password_hash"`
	Algorithm    HashAlgorithm `json:"algorithm"`
	Salt         string        `json:"salt,omitempty"`
	Enabled      bool          `json:"enabled"`
}

// AuthResult represents the result of an authentication attempt
type AuthResult int

const (
	// AuthSuccess indicates successful authentication
	AuthSuccess AuthResult = iota
	// AuthFailure indicates authentication failed due to invalid credentials
	AuthFailure
	// AuthError indicates an error occurred during authentication
	AuthError
	// AuthIgnore indicates the authenticator should be skipped
	AuthIgnore
)

// String returns the string representation of AuthResult
func (ar AuthResult) String() string {
	switch ar {
	case AuthSuccess:
		return "success"
	case AuthFailure:
		return "failure"
	case AuthError:
		return "error"
	case AuthIgnore:
		return "ignore"
	default:
		return "unknown"
	}
}

// Authenticator defines the interface for authentication providers
type Authenticator interface {
	// Authenticate verifies the provided credentials
	Authenticate(username, password string) AuthResult
	// Name returns the name of the authenticator
	Name() string
	// Enabled returns whether the authenticator is enabled
	Enabled() bool
}

// AuthChain manages a chain of authenticators following EMQX's authentication model
type AuthChain struct {
	authenticators []Authenticator
	enabled        bool
}

// NewAuthChain creates a new authentication chain
func NewAuthChain() *AuthChain {
	return &AuthChain{
		authenticators: make([]Authenticator, 0),
		enabled:        true,
	}
}

// AddAuthenticator adds an authenticator to the chain
func (ac *AuthChain) AddAuthenticator(auth Authenticator) {
	ac.authenticators = append(ac.authenticators, auth)
}

// Authenticate processes authentication through the chain
// Following EMQX's authentication flow:
// - If any authenticator returns AuthSuccess, authentication succeeds
// - If any authenticator returns AuthFailure, authentication fails
// - If all authenticators return AuthIgnore, authentication fails
// - If any authenticator returns AuthError, log error and continue
func (ac *AuthChain) Authenticate(username, password string) AuthResult {
	if !ac.enabled {
		return AuthIgnore
	}

	if len(ac.authenticators) == 0 {
		log.Printf("[WARN] No authenticators configured, allowing connection")
		return AuthSuccess
	}

	log.Printf("[DEBUG] Starting authentication chain for user: %s", username)

	for i, auth := range ac.authenticators {
		if !auth.Enabled() {
			log.Printf("[DEBUG] Authenticator %d (%s) is disabled, skipping", i+1, auth.Name())
			continue
		}

		log.Printf("[DEBUG] Trying authenticator %d: %s for user: %s", i+1, auth.Name(), username)
		result := auth.Authenticate(username, password)
		log.Printf("[DEBUG] Authenticator %s returned: %s for user: %s", auth.Name(), result.String(), username)

		switch result {
		case AuthSuccess:
			log.Printf("[INFO] Authentication successful for user: %s via %s", username, auth.Name())
			return AuthSuccess
		case AuthFailure:
			log.Printf("[WARN] Authentication failed for user: %s via %s", username, auth.Name())
			return AuthFailure
		case AuthError:
			log.Printf("[ERROR] Authentication error for user: %s via %s", username, auth.Name())
			continue
		case AuthIgnore:
			log.Printf("[DEBUG] Authentication ignored for user: %s via %s", username, auth.Name())
			continue
		}
	}

	log.Printf("[WARN] All authenticators skipped/ignored for user: %s, denying access", username)
	return AuthFailure
}

// SetEnabled enables or disables the authentication chain
func (ac *AuthChain) SetEnabled(enabled bool) {
	ac.enabled = enabled
}

// IsEnabled returns whether the authentication chain is enabled
func (ac *AuthChain) IsEnabled() bool {
	return ac.enabled
}

// Clear removes all authenticators from the chain
func (ac *AuthChain) Clear() {
	ac.authenticators = ac.authenticators[:0]
}

// Count returns the number of authenticators in the chain
func (ac *AuthChain) Count() int {
	return len(ac.authenticators)
}

// GetAuthenticators returns a copy of the authenticators slice
func (ac *AuthChain) GetAuthenticators() []Authenticator {
	result := make([]Authenticator, len(ac.authenticators))
	copy(result, ac.authenticators)
	return result
}

// hashPassword creates a hash of the password using the specified algorithm
func hashPassword(password, salt string, algorithm HashAlgorithm) (string, error) {
	switch algorithm {
	case HashPlain:
		return password, nil
	case HashSHA256:
		hasher := sha256.New()
		hasher.Write([]byte(salt + password))
		return fmt.Sprintf("%x", hasher.Sum(nil)), nil
	case HashBcrypt:
		hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			return "", err
		}
		return string(hash), nil
	default:
		return "", fmt.Errorf("unsupported hash algorithm: %s", algorithm)
	}
}

// verifyPassword verifies a password against a hash using the specified algorithm
func verifyPassword(password, hash, salt string, algorithm HashAlgorithm) bool {
	switch algorithm {
	case HashPlain:
		return password == hash
	case HashSHA256:
		expectedHash, err := hashPassword(password, salt, HashSHA256)
		if err != nil {
			return false
		}
		return expectedHash == hash
	case HashBcrypt:
		err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
		return err == nil
	default:
		return false
	}
}
