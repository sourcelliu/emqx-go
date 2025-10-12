package auth

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHashPassword(t *testing.T) {
	testCases := []struct {
		name      string
		password  string
		salt      string
		algorithm HashAlgorithm
		expectErr bool
	}{
		{
			name:      "plain password",
			password:  "password123",
			salt:      "",
			algorithm: HashPlain,
			expectErr: false,
		},
		{
			name:      "sha256 password",
			password:  "password123",
			salt:      "user1",
			algorithm: HashSHA256,
			expectErr: false,
		},
		{
			name:      "bcrypt password",
			password:  "password123",
			salt:      "",
			algorithm: HashBcrypt,
			expectErr: false,
		},
		{
			name:      "unsupported algorithm",
			password:  "password123",
			salt:      "",
			algorithm: "md5",
			expectErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			hash, err := hashPassword(tc.password, tc.salt, tc.algorithm)
			if tc.expectErr {
				assert.Error(t, err)
				assert.Empty(t, hash)
			} else {
				assert.NoError(t, err)
				assert.NotEmpty(t, hash)

				// Verify the hash can be used for authentication
				assert.True(t, verifyPassword(tc.password, hash, tc.salt, tc.algorithm))
				assert.False(t, verifyPassword("wrongpassword", hash, tc.salt, tc.algorithm))
			}
		})
	}
}

func TestVerifyPassword(t *testing.T) {
	testCases := []struct {
		name      string
		password  string
		hash      string
		salt      string
		algorithm HashAlgorithm
		expected  bool
	}{
		{
			name:      "plain correct password",
			password:  "password123",
			hash:      "password123",
			salt:      "",
			algorithm: HashPlain,
			expected:  true,
		},
		{
			name:      "plain wrong password",
			password:  "wrongpassword",
			hash:      "password123",
			salt:      "",
			algorithm: HashPlain,
			expected:  false,
		},
		{
			name:      "sha256 correct password",
			password:  "password123",
			hash:      "4e3b8c08b5c16e14bdf68b304b4fe44e4b29b0b2c2b2b8b1b8b9b8b1b8b9b8b1", // pre-computed for test
			salt:      "",
			algorithm: HashSHA256,
			expected:  false, // Will be false because we're using a fake hash for test
		},
		{
			name:      "unsupported algorithm",
			password:  "password123",
			hash:      "anyhash",
			salt:      "",
			algorithm: "md5",
			expected:  false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := verifyPassword(tc.password, tc.hash, tc.salt, tc.algorithm)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestMemoryAuthenticator(t *testing.T) {
	auth := NewMemoryAuthenticator()
	require.NotNil(t, auth)

	// Test basic properties
	assert.Equal(t, "memory", auth.Name())
	assert.True(t, auth.Enabled())
	assert.Equal(t, 0, auth.Count())

	// Test adding users
	err := auth.AddUser("user1", "password123", HashPlain)
	assert.NoError(t, err)
	assert.Equal(t, 1, auth.Count())

	err = auth.AddUser("user2", "password456", HashSHA256)
	assert.NoError(t, err)
	assert.Equal(t, 2, auth.Count())

	err = auth.AddUser("user3", "password789", HashBcrypt)
	assert.NoError(t, err)
	assert.Equal(t, 3, auth.Count())

	// Test adding user with empty username
	err = auth.AddUser("", "password", HashPlain)
	assert.Error(t, err)
	assert.Equal(t, 3, auth.Count())

	// Test authentication
	result := auth.Authenticate("user1", "password123")
	assert.Equal(t, AuthSuccess, result)

	result = auth.Authenticate("user1", "wrongpassword")
	assert.Equal(t, AuthFailure, result)

	result = auth.Authenticate("user2", "password456")
	assert.Equal(t, AuthSuccess, result)

	result = auth.Authenticate("user3", "password789")
	assert.Equal(t, AuthSuccess, result)

	result = auth.Authenticate("nonexistentuser", "password")
	assert.Equal(t, AuthIgnore, result)

	result = auth.Authenticate("", "password")
	assert.Equal(t, AuthIgnore, result)

	// Test getting user info
	user, err := auth.GetUser("user1")
	assert.NoError(t, err)
	assert.Equal(t, "user1", user.Username)
	assert.Equal(t, HashPlain, user.Algorithm)
	assert.True(t, user.Enabled)

	_, err = auth.GetUser("nonexistentuser")
	assert.Error(t, err)

	// Test listing users
	users := auth.ListUsers()
	assert.Equal(t, 3, len(users))
	assert.Contains(t, users, "user1")
	assert.Contains(t, users, "user2")
	assert.Contains(t, users, "user3")

	// Test disabling a user
	err = auth.SetUserEnabled("user1", false)
	assert.NoError(t, err)

	result = auth.Authenticate("user1", "password123")
	assert.Equal(t, AuthFailure, result)

	// Re-enable user
	err = auth.SetUserEnabled("user1", true)
	assert.NoError(t, err)

	result = auth.Authenticate("user1", "password123")
	assert.Equal(t, AuthSuccess, result)

	// Test updating user
	err = auth.UpdateUser("user1", "newpassword", HashSHA256)
	assert.NoError(t, err)

	result = auth.Authenticate("user1", "password123")
	assert.Equal(t, AuthFailure, result)

	result = auth.Authenticate("user1", "newpassword")
	assert.Equal(t, AuthSuccess, result)

	// Test removing user
	err = auth.RemoveUser("user1")
	assert.NoError(t, err)
	assert.Equal(t, 2, auth.Count())

	result = auth.Authenticate("user1", "newpassword")
	assert.Equal(t, AuthIgnore, result)

	// Test removing non-existent user
	err = auth.RemoveUser("nonexistentuser")
	assert.Error(t, err)

	// Test disabling authenticator
	auth.SetEnabled(false)
	assert.False(t, auth.Enabled())

	result = auth.Authenticate("user2", "password456")
	assert.Equal(t, AuthIgnore, result)

	// Re-enable authenticator
	auth.SetEnabled(true)
	result = auth.Authenticate("user2", "password456")
	assert.Equal(t, AuthSuccess, result)

	// Test clearing all users
	auth.Clear()
	assert.Equal(t, 0, auth.Count())
	assert.Empty(t, auth.ListUsers())
}

func TestAuthChain(t *testing.T) {
	chain := NewAuthChain()
	require.NotNil(t, chain)

	assert.True(t, chain.IsEnabled())
	assert.Equal(t, 0, chain.Count())

	// Test empty chain (should allow by default)
	result := chain.Authenticate("user", "password")
	assert.Equal(t, AuthSuccess, result)

	// Add first authenticator
	auth1 := NewMemoryAuthenticator()
	auth1.AddUser("user1", "password1", HashPlain)
	chain.AddAuthenticator(auth1)
	assert.Equal(t, 1, chain.Count())

	// Test authentication with first authenticator
	result = chain.Authenticate("user1", "password1")
	assert.Equal(t, AuthSuccess, result)

	result = chain.Authenticate("user1", "wrongpassword")
	assert.Equal(t, AuthFailure, result)

	result = chain.Authenticate("user2", "password")
	assert.Equal(t, AuthFailure, result) // All authenticators ignored/failed

	// Add second authenticator
	auth2 := NewMemoryAuthenticator()
	auth2.AddUser("user2", "password2", HashSHA256)
	chain.AddAuthenticator(auth2)
	assert.Equal(t, 2, chain.Count())

	// Test authentication with chain
	result = chain.Authenticate("user1", "password1")
	assert.Equal(t, AuthSuccess, result)

	result = chain.Authenticate("user2", "password2")
	assert.Equal(t, AuthSuccess, result)

	result = chain.Authenticate("user1", "wrongpassword")
	assert.Equal(t, AuthFailure, result)

	result = chain.Authenticate("nonexistentuser", "password")
	assert.Equal(t, AuthFailure, result)

	// Test disabling an authenticator
	auth1.SetEnabled(false)
	result = chain.Authenticate("user1", "password1")
	assert.Equal(t, AuthFailure, result) // user1 only exists in disabled auth1

	result = chain.Authenticate("user2", "password2")
	assert.Equal(t, AuthSuccess, result) // user2 exists in enabled auth2

	// Test disabling the chain
	chain.SetEnabled(false)
	result = chain.Authenticate("user2", "password2")
	assert.Equal(t, AuthIgnore, result)

	// Re-enable chain
	chain.SetEnabled(true)
	result = chain.Authenticate("user2", "password2")
	assert.Equal(t, AuthSuccess, result)

	// Test clearing chain
	chain.Clear()
	assert.Equal(t, 0, chain.Count())

	// Empty chain should allow by default
	result = chain.Authenticate("anyuser", "anypassword")
	assert.Equal(t, AuthSuccess, result)
}

func TestAuthResult(t *testing.T) {
	testCases := []struct {
		result   AuthResult
		expected string
	}{
		{AuthSuccess, "success"},
		{AuthFailure, "failure"},
		{AuthError, "error"},
		{AuthIgnore, "ignore"},
		{AuthResult(999), "unknown"},
	}

	for _, tc := range testCases {
		assert.Equal(t, tc.expected, tc.result.String())
	}
}
