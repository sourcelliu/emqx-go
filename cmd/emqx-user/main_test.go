package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/turtacn/emqx-go/pkg/config"
)

func TestGenerateConfig(t *testing.T) {
	tmpFile := filepath.Join(os.TempDir(), "test_generate.yaml")
	defer os.Remove(tmpFile)

	// Capture stdout to suppress output during test
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	generateConfig(tmpFile)

	w.Close()
	os.Stdout = old

	// Read captured output
	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	// Verify output message
	assert.Contains(t, output, "Sample configuration saved to")
	assert.Contains(t, output, tmpFile)

	// Verify file was created
	_, err := os.Stat(tmpFile)
	assert.NoError(t, err)

	// Verify file contents by loading it
	cfg, err := config.LoadConfig(tmpFile)
	require.NoError(t, err)
	assert.Equal(t, "emqx-go-node", cfg.Broker.NodeID)
}

func TestListUsers(t *testing.T) {
	// Create a test config file
	testConfig := createTestConfigFile(t)
	defer os.Remove(testConfig)

	// Capture stdout
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	listUsers(testConfig)

	w.Close()
	os.Stdout = old

	// Read captured output
	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	// Verify output contains expected users
	assert.Contains(t, output, "USERNAME")
	assert.Contains(t, output, "ALGORITHM")
	assert.Contains(t, output, "ENABLED")
	assert.Contains(t, output, "admin")
	assert.Contains(t, output, "bcrypt")
	assert.Contains(t, output, "✓") // Enabled symbol
}

func TestListUsersEmptyConfig(t *testing.T) {
	// Create a config file with no users
	emptyConfig := createEmptyConfigFile(t)
	defer os.Remove(emptyConfig)

	// Capture stdout
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	listUsers(emptyConfig)

	w.Close()
	os.Stdout = old

	// Read captured output
	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	assert.Contains(t, output, "No users configured")
}

func TestAddUser(t *testing.T) {
	testConfig := createTestConfigFile(t)
	defer os.Remove(testConfig)

	// Capture stdout
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	addUser(testConfig, "newuser", "newpass", "sha256", true)

	w.Close()
	os.Stdout = old

	// Read captured output
	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	assert.Contains(t, output, "User 'newuser' added successfully")
	assert.Contains(t, output, "algorithm: sha256")
	assert.Contains(t, output, "status: enabled")

	// Verify user was actually added
	cfg, err := config.LoadConfig(testConfig)
	require.NoError(t, err)

	users := cfg.ListUsers()
	found := false
	for _, user := range users {
		if user.Username == "newuser" {
			found = true
			assert.Equal(t, "sha256", string(user.Algorithm))
			assert.True(t, user.Enabled)
			break
		}
	}
	assert.True(t, found, "New user should be found in config")
}

func TestAddUserDisabled(t *testing.T) {
	testConfig := createTestConfigFile(t)
	defer os.Remove(testConfig)

	// Capture stdout
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	addUser(testConfig, "disableduser", "disabledpass", "bcrypt", false)

	w.Close()
	os.Stdout = old

	// Read captured output
	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	assert.Contains(t, output, "User 'disableduser' added successfully")
	assert.Contains(t, output, "status: disabled")

	// Verify user was added as disabled
	cfg, err := config.LoadConfig(testConfig)
	require.NoError(t, err)

	users := cfg.ListUsers()
	found := false
	for _, user := range users {
		if user.Username == "disableduser" {
			found = true
			assert.False(t, user.Enabled)
			break
		}
	}
	assert.True(t, found)
}

func TestUpdateUser(t *testing.T) {
	testConfig := createTestConfigFile(t)
	defer os.Remove(testConfig)

	// Capture stdout
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	updateUser(testConfig, "admin", "newadminpass", "plain", false)

	w.Close()
	os.Stdout = old

	// Read captured output
	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	assert.Contains(t, output, "User 'admin' updated successfully")

	// Verify user was actually updated
	cfg, err := config.LoadConfig(testConfig)
	require.NoError(t, err)

	users := cfg.ListUsers()
	found := false
	for _, user := range users {
		if user.Username == "admin" {
			found = true
			assert.Equal(t, "plain", string(user.Algorithm))
			assert.False(t, user.Enabled)
			break
		}
	}
	assert.True(t, found)
}

func TestRemoveUser(t *testing.T) {
	testConfig := createTestConfigFile(t)
	defer os.Remove(testConfig)

	// Capture stdout
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	removeUser(testConfig, "test")

	w.Close()
	os.Stdout = old

	// Read captured output
	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	assert.Contains(t, output, "User 'test' removed successfully")

	// Verify user was actually removed
	cfg, err := config.LoadConfig(testConfig)
	require.NoError(t, err)

	users := cfg.ListUsers()
	found := false
	for _, user := range users {
		if user.Username == "test" {
			found = true
			break
		}
	}
	assert.False(t, found, "User 'test' should have been removed")
}

func TestEnableDisableUser(t *testing.T) {
	testConfig := createTestConfigFile(t)
	defer os.Remove(testConfig)

	// Test disabling a user
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	enableUser(testConfig, "admin", false)

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	assert.Contains(t, output, "User 'admin' disabled successfully")

	// Verify user was disabled
	cfg, err := config.LoadConfig(testConfig)
	require.NoError(t, err)

	users := cfg.ListUsers()
	for _, user := range users {
		if user.Username == "admin" {
			assert.False(t, user.Enabled)
			break
		}
	}

	// Test enabling the user again
	r, w, _ = os.Pipe()
	os.Stdout = w

	enableUser(testConfig, "admin", true)

	w.Close()
	os.Stdout = old

	buf.Reset()
	io.Copy(&buf, r)
	output = buf.String()

	assert.Contains(t, output, "User 'admin' enabled successfully")

	// Verify user was enabled
	cfg, err = config.LoadConfig(testConfig)
	require.NoError(t, err)

	users = cfg.ListUsers()
	for _, user := range users {
		if user.Username == "admin" {
			assert.True(t, user.Enabled)
			break
		}
	}
}

func TestErrorCases(t *testing.T) {
	testConfig := createTestConfigFile(t)
	defer os.Remove(testConfig)

	// Test adding duplicate user - this should cause a panic from log.Fatalf
	// We'll test this by checking that calling addUser with an existing user
	// would cause an error, but since log.Fatalf exits the program, we can't
	// easily test this in a unit test without complex setup.

	// Instead, we can test the underlying config functionality
	cfg, err := config.LoadConfig(testConfig)
	require.NoError(t, err)

	// Try to add duplicate user via config
	err = cfg.AddUser("admin", "newpass", "bcrypt", true)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")

	// Test updating non-existent user via config
	err = cfg.UpdateUser("nonexistent", "pass", "plain", true)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")

	// Test removing non-existent user via config
	err = cfg.RemoveUser("nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestPasswordMasking(t *testing.T) {
	testConfig := createTestConfigFile(t)
	defer os.Remove(testConfig)

	// Add a user with a long password
	addUser(testConfig, "longpassuser", "verylongpassword123456", "plain", true)

	// Capture stdout
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	listUsers(testConfig)

	w.Close()
	os.Stdout = old

	// Read captured output
	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	// Verify password is masked
	assert.Contains(t, output, "longpassuser")
	assert.Contains(t, output, "****")                      // Should contain masked password
	assert.NotContains(t, output, "verylongpassword123456") // Should not contain full password
}

// Helper functions
func createTestConfigFile(t *testing.T) string {
	tmpFile := filepath.Join(os.TempDir(), "test_config.yaml")

	yamlContent := `
broker:
  node_id: test-node
  mqtt_port: ":1883"
  grpc_port: ":8081"
  metrics_port: ":8082"
  auth:
    enabled: true
    users:
    - username: admin
      password: admin123
      algorithm: bcrypt
      enabled: true
    - username: user1
      password: password123
      algorithm: sha256
      enabled: true
    - username: test
      password: test
      algorithm: plain
      enabled: true
`

	err := os.WriteFile(tmpFile, []byte(strings.TrimSpace(yamlContent)), 0644)
	require.NoError(t, err)
	return tmpFile
}

func createEmptyConfigFile(t *testing.T) string {
	tmpFile := filepath.Join(os.TempDir(), "empty_config.yaml")

	yamlContent := `
broker:
  node_id: test-node
  mqtt_port: ":1883"
  grpc_port: ":8081"
  metrics_port: ":8082"
  auth:
    enabled: true
    users: []
`

	err := os.WriteFile(tmpFile, []byte(strings.TrimSpace(yamlContent)), 0644)
	require.NoError(t, err)
	return tmpFile
}
