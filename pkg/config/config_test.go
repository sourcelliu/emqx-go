package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/turtacn/emqx-go/pkg/auth"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	require.NotNil(t, cfg)

	// Test broker defaults
	assert.Equal(t, "emqx-go-node", cfg.Broker.NodeID)
	assert.Equal(t, ":1883", cfg.Broker.MQTTPort)
	assert.Equal(t, ":8081", cfg.Broker.GRPCPort)
	assert.Equal(t, ":8082", cfg.Broker.MetricsPort)

	// Test auth defaults
	assert.True(t, cfg.Broker.Auth.Enabled)
	assert.Len(t, cfg.Broker.Auth.Users, 3) // admin, user1, test

	// Verify default users
	adminUser := findUser(cfg.Broker.Auth.Users, "admin")
	require.NotNil(t, adminUser)
	assert.Equal(t, "admin", adminUser.Username)
	assert.Equal(t, "bcrypt", string(adminUser.Algorithm))
	assert.True(t, adminUser.Enabled)

	user1 := findUser(cfg.Broker.Auth.Users, "user1")
	require.NotNil(t, user1)
	assert.Equal(t, "sha256", string(user1.Algorithm))

	testUser := findUser(cfg.Broker.Auth.Users, "test")
	require.NotNil(t, testUser)
	assert.Equal(t, "plain", string(testUser.Algorithm))
}

func TestLoadConfigYAML(t *testing.T) {
	// Create a temporary YAML config file
	yamlContent := `
broker:
  node_id: test-node
  mqtt_port: ":1884"
  grpc_port: ":8082"
  metrics_port: ":8083"
  auth:
    enabled: true
    users:
    - username: testuser
      password: testpass
      algorithm: bcrypt
      enabled: true
    - username: disabled_user
      password: disabled_pass
      algorithm: sha256
      enabled: false
`

	tmpFile := createTempFile(t, "config.yaml", yamlContent)
	defer os.Remove(tmpFile)

	cfg, err := LoadConfig(tmpFile)
	require.NoError(t, err)
	require.NotNil(t, cfg)

	// Verify loaded config
	assert.Equal(t, "test-node", cfg.Broker.NodeID)
	assert.Equal(t, ":1884", cfg.Broker.MQTTPort)
	assert.Equal(t, ":8082", cfg.Broker.GRPCPort)
	assert.Equal(t, ":8083", cfg.Broker.MetricsPort)
	assert.True(t, cfg.Broker.Auth.Enabled)
	assert.Len(t, cfg.Broker.Auth.Users, 2)

	testuser := findUser(cfg.Broker.Auth.Users, "testuser")
	require.NotNil(t, testuser)
	assert.Equal(t, "bcrypt", string(testuser.Algorithm))
	assert.True(t, testuser.Enabled)

	disabledUser := findUser(cfg.Broker.Auth.Users, "disabled_user")
	require.NotNil(t, disabledUser)
	assert.False(t, disabledUser.Enabled)
}

func TestLoadConfigJSON(t *testing.T) {
	// Create a temporary JSON config file
	jsonContent := `{
  "broker": {
    "node_id": "json-test-node",
    "mqtt_port": ":1885",
    "grpc_port": ":8083",
    "metrics_port": ":8084",
    "auth": {
      "enabled": false,
      "users": [
        {
          "username": "jsonuser",
          "password": "jsonpass",
          "algorithm": "plain",
          "enabled": true
        }
      ]
    }
  }
}`

	tmpFile := createTempFile(t, "config.json", jsonContent)
	defer os.Remove(tmpFile)

	cfg, err := LoadConfig(tmpFile)
	require.NoError(t, err)
	require.NotNil(t, cfg)

	// Verify loaded config
	assert.Equal(t, "json-test-node", cfg.Broker.NodeID)
	assert.Equal(t, ":1885", cfg.Broker.MQTTPort)
	assert.False(t, cfg.Broker.Auth.Enabled)
	assert.Len(t, cfg.Broker.Auth.Users, 1)

	jsonuser := findUser(cfg.Broker.Auth.Users, "jsonuser")
	require.NotNil(t, jsonuser)
	assert.Equal(t, "plain", string(jsonuser.Algorithm))
}

func TestLoadConfigNonExistent(t *testing.T) {
	_, err := LoadConfig("nonexistent.yaml")
	assert.Error(t, err) // Should return error for non-existent file
	assert.Contains(t, err.Error(), "failed to read config file")

	// Test empty path returns default config
	cfg, err := LoadConfig("")
	assert.NoError(t, err) // Empty path should return default config
	require.NotNil(t, cfg)

	// Should be default config
	assert.Equal(t, "emqx-go-node", cfg.Broker.NodeID)
	assert.True(t, cfg.Broker.Auth.Enabled)
}

func TestLoadConfigInvalid(t *testing.T) {
	// Create invalid YAML file
	invalidYAML := `
broker:
  node_id: test
  invalid_yaml: [unclosed array
`
	tmpFile := createTempFile(t, "invalid.yaml", invalidYAML)
	defer os.Remove(tmpFile)

	_, err := LoadConfig(tmpFile)
	assert.Error(t, err)
}

func TestSaveConfigYAML(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Broker.NodeID = "save-test-node"

	tmpFile := filepath.Join(os.TempDir(), "save_test.yaml")
	defer os.Remove(tmpFile)

	err := SaveConfig(cfg, tmpFile)
	require.NoError(t, err)

	// Verify file was created
	_, err = os.Stat(tmpFile)
	assert.NoError(t, err)

	// Load it back
	loadedCfg, err := LoadConfig(tmpFile)
	require.NoError(t, err)
	assert.Equal(t, "save-test-node", loadedCfg.Broker.NodeID)
}

func TestSaveConfigJSON(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Broker.NodeID = "save-test-json-node"

	tmpFile := filepath.Join(os.TempDir(), "save_test.json")
	defer os.Remove(tmpFile)

	err := SaveConfig(cfg, tmpFile)
	require.NoError(t, err)

	// Verify file was created
	_, err = os.Stat(tmpFile)
	assert.NoError(t, err)

	// Load it back
	loadedCfg, err := LoadConfig(tmpFile)
	require.NoError(t, err)
	assert.Equal(t, "save-test-json-node", loadedCfg.Broker.NodeID)
}

func TestConfigureAuth(t *testing.T) {
	cfg := &Config{
		Broker: BrokerConfig{
			Auth: AuthConfig{
				Enabled: true,
				Users: []UserConfig{
					{
						Username:  "testuser1",
						Password:  "testpass1",
						Algorithm: "plain",
						Enabled:   true,
					},
					{
						Username:  "testuser2",
						Password:  "testpass2",
						Algorithm: "sha256",
						Enabled:   true,
					},
					{
						Username:  "disabled_user",
						Password:  "disabled_pass",
						Algorithm: "bcrypt",
						Enabled:   false,
					},
				},
			},
		},
	}

	authChain := auth.NewAuthChain()
	err := cfg.ConfigureAuth(authChain)
	require.NoError(t, err)

	// Verify authentication chain was configured
	assert.Equal(t, 1, authChain.Count()) // Should have one memory authenticator

	// Test authentication
	result := authChain.Authenticate("testuser1", "testpass1")
	assert.Equal(t, auth.AuthSuccess, result)

	result = authChain.Authenticate("testuser2", "testpass2")
	assert.Equal(t, auth.AuthSuccess, result)

	result = authChain.Authenticate("disabled_user", "disabled_pass")
	assert.Equal(t, auth.AuthFailure, result) // Disabled user should fail

	result = authChain.Authenticate("nonexistent", "password")
	assert.Equal(t, auth.AuthFailure, result)
}

func TestConfigureAuthDisabled(t *testing.T) {
	cfg := &Config{
		Broker: BrokerConfig{
			Auth: AuthConfig{
				Enabled: false,
				Users: []UserConfig{
					{
						Username:  "testuser",
						Password:  "testpass",
						Algorithm: "plain",
						Enabled:   true,
					},
				},
			},
		},
	}

	authChain := auth.NewAuthChain()
	err := cfg.ConfigureAuth(authChain)
	require.NoError(t, err)

	// When auth is disabled, chain should be disabled
	assert.False(t, authChain.IsEnabled())
}

func TestAddUser(t *testing.T) {
	cfg := DefaultConfig()
	initialCount := len(cfg.Broker.Auth.Users)

	err := cfg.AddUser("newuser", "newpass", "bcrypt", true)
	require.NoError(t, err)

	assert.Len(t, cfg.Broker.Auth.Users, initialCount+1)

	newUser := findUser(cfg.Broker.Auth.Users, "newuser")
	require.NotNil(t, newUser)
	assert.Equal(t, "newuser", newUser.Username)
	assert.Equal(t, "bcrypt", string(newUser.Algorithm))
	assert.True(t, newUser.Enabled)

	// Test adding duplicate user
	err = cfg.AddUser("newuser", "anotherpass", "plain", true)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")
}

func TestUpdateUser(t *testing.T) {
	cfg := DefaultConfig()

	// Update existing user
	err := cfg.UpdateUser("admin", "newadminpass", "sha256", false)
	require.NoError(t, err)

	adminUser := findUser(cfg.Broker.Auth.Users, "admin")
	require.NotNil(t, adminUser)
	assert.Equal(t, "sha256", string(adminUser.Algorithm))
	assert.False(t, adminUser.Enabled)

	// Test updating non-existent user
	err = cfg.UpdateUser("nonexistent", "password", "plain", true)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestRemoveUser(t *testing.T) {
	cfg := DefaultConfig()
	initialCount := len(cfg.Broker.Auth.Users)

	err := cfg.RemoveUser("admin")
	require.NoError(t, err)

	assert.Len(t, cfg.Broker.Auth.Users, initialCount-1)

	adminUser := findUser(cfg.Broker.Auth.Users, "admin")
	assert.Nil(t, adminUser)

	// Test removing non-existent user
	err = cfg.RemoveUser("nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestListUsers(t *testing.T) {
	cfg := DefaultConfig()

	users := cfg.ListUsers()
	assert.Len(t, users, 3) // admin, user1, test

	// Check each default user
	foundAdmin := false
	foundUser1 := false
	foundTest := false

	for _, user := range users {
		switch user.Username {
		case "admin":
			foundAdmin = true
			assert.Equal(t, "bcrypt", string(user.Algorithm))
		case "user1":
			foundUser1 = true
			assert.Equal(t, "sha256", string(user.Algorithm))
		case "test":
			foundTest = true
			assert.Equal(t, "plain", string(user.Algorithm))
		}
	}

	assert.True(t, foundAdmin)
	assert.True(t, foundUser1)
	assert.True(t, foundTest)
}

func TestGetFileFormat(t *testing.T) {
	testCases := []struct {
		filename string
		expected string
	}{
		{"config.yaml", "yaml"},
		{"config.yml", "yaml"},
		{"config.json", "json"},
		{"config.txt", "unsupported"}, // Should be unsupported
		{"config", "unsupported"},     // No extension should be unsupported
		{".yaml", "yaml"},
		{".json", "json"},
	}

	for _, tc := range testCases {
		t.Run(tc.filename, func(t *testing.T) {
			// Test by trying to determine format through SaveConfig behavior
			cfg := DefaultConfig()
			tmpFile := filepath.Join(os.TempDir(), tc.filename)
			defer os.Remove(tmpFile)

			err := SaveConfig(cfg, tmpFile)

			if tc.expected == "unsupported" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "unsupported config file format")
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// Helper functions
func createTempFile(t *testing.T, filename, content string) string {
	tmpFile := filepath.Join(os.TempDir(), filename)
	err := os.WriteFile(tmpFile, []byte(content), 0644)
	require.NoError(t, err)
	return tmpFile
}

func findUser(users []UserConfig, username string) *UserConfig {
	for i := range users {
		if users[i].Username == username {
			return &users[i]
		}
	}
	return nil
}