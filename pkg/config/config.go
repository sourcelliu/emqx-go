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

// Package config provides configuration management for emqx-go,
// including user authentication configuration and other broker settings.
package config

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"path/filepath"
	"strings"

	"github.com/turtacn/emqx-go/pkg/auth"
	"gopkg.in/yaml.v2"
)

// UserConfig represents a user configuration entry
type UserConfig struct {
	Username  string `yaml:"username" json:"username"`
	Password  string `yaml:"password" json:"password"`
	Algorithm string `yaml:"algorithm" json:"algorithm"`
	Enabled   bool   `yaml:"enabled" json:"enabled"`
}

// AuthConfig represents the authentication configuration
type AuthConfig struct {
	Enabled bool         `yaml:"enabled" json:"enabled"`
	Users   []UserConfig `yaml:"users" json:"users"`
}

// BrokerConfig represents the overall broker configuration
type BrokerConfig struct {
	NodeID      string     `yaml:"node_id" json:"node_id"`
	MQTTPort    string     `yaml:"mqtt_port" json:"mqtt_port"`
	GRPCPort    string     `yaml:"grpc_port" json:"grpc_port"`
	MetricsPort string     `yaml:"metrics_port" json:"metrics_port"`
	Auth        AuthConfig `yaml:"auth" json:"auth"`
}

// Config holds the complete configuration
type Config struct {
	Broker BrokerConfig `yaml:"broker" json:"broker"`
}

// DefaultConfig returns a default configuration
func DefaultConfig() *Config {
	return &Config{
		Broker: BrokerConfig{
			NodeID:      "emqx-go-node",
			MQTTPort:    ":1883",
			GRPCPort:    ":8081",
			MetricsPort: ":8082",
			Auth: AuthConfig{
				Enabled: true,
				Users: []UserConfig{
					{
						Username:  "admin",
						Password:  "admin123",
						Algorithm: "bcrypt",
						Enabled:   true,
					},
					{
						Username:  "user1",
						Password:  "password123",
						Algorithm: "sha256",
						Enabled:   true,
					},
					{
						Username:  "test",
						Password:  "test",
						Algorithm: "plain",
						Enabled:   true,
					},
				},
			},
		},
	}
}

// LoadConfig loads configuration from a file
func LoadConfig(configPath string) (*Config, error) {
	// If no config file specified, return default config
	if configPath == "" {
		log.Println("[INFO] No config file specified, using default configuration")
		return DefaultConfig(), nil
	}

	// Read config file
	data, err := ioutil.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file %s: %w", configPath, err)
	}

	config := &Config{}
	ext := strings.ToLower(filepath.Ext(configPath))

	switch ext {
	case ".yaml", ".yml":
		err = yaml.Unmarshal(data, config)
	case ".json":
		err = json.Unmarshal(data, config)
	default:
		return nil, fmt.Errorf("unsupported config file format: %s (supported: .yaml, .yml, .json)", ext)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to parse config file %s: %w", configPath, err)
	}

	// Validate configuration
	if err := validateConfig(config); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	log.Printf("[INFO] Configuration loaded from %s", configPath)
	return config, nil
}

// SaveConfig saves configuration to a file
func SaveConfig(config *Config, configPath string) error {
	var data []byte
	var err error

	ext := strings.ToLower(filepath.Ext(configPath))
	switch ext {
	case ".yaml", ".yml":
		data, err = yaml.Marshal(config)
	case ".json":
		data, err = json.MarshalIndent(config, "", "  ")
	default:
		return fmt.Errorf("unsupported config file format: %s (supported: .yaml, .yml, .json)", ext)
	}

	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	err = ioutil.WriteFile(configPath, data, 0644)
	if err != nil {
		return fmt.Errorf("failed to write config file %s: %w", configPath, err)
	}

	log.Printf("[INFO] Configuration saved to %s", configPath)
	return nil
}

// validateConfig validates the configuration
func validateConfig(config *Config) error {
	if config.Broker.NodeID == "" {
		return fmt.Errorf("node_id cannot be empty")
	}

	if config.Broker.MQTTPort == "" {
		return fmt.Errorf("mqtt_port cannot be empty")
	}

	// Validate users
	usernames := make(map[string]bool)
	for i, user := range config.Broker.Auth.Users {
		if user.Username == "" {
			return fmt.Errorf("user %d: username cannot be empty", i)
		}

		if usernames[user.Username] {
			return fmt.Errorf("duplicate username: %s", user.Username)
		}
		usernames[user.Username] = true

		if user.Password == "" {
			return fmt.Errorf("user %s: password cannot be empty", user.Username)
		}

		// Validate algorithm
		switch user.Algorithm {
		case "plain", "sha256", "bcrypt":
			// Valid algorithms
		default:
			return fmt.Errorf("user %s: unsupported algorithm: %s (supported: plain, sha256, bcrypt)", user.Username, user.Algorithm)
		}
	}

	return nil
}

// ConfigureAuth configures authentication from the config
func (c *Config) ConfigureAuth(authChain *auth.AuthChain) error {
	// Clear existing authenticators
	authChain.Clear()

	if !c.Broker.Auth.Enabled {
		authChain.SetEnabled(false)
		log.Println("[INFO] Authentication disabled by configuration")
		return nil
	}

	authChain.SetEnabled(true)

	// Create memory authenticator
	memAuth := auth.NewMemoryAuthenticator()

	// Add users from configuration
	for _, userConfig := range c.Broker.Auth.Users {
		algorithm := auth.HashAlgorithm(userConfig.Algorithm)
		err := memAuth.AddUser(userConfig.Username, userConfig.Password, algorithm)
		if err != nil {
			return fmt.Errorf("failed to add user %s: %w", userConfig.Username, err)
		}

		// Set user enabled status
		err = memAuth.SetUserEnabled(userConfig.Username, userConfig.Enabled)
		if err != nil {
			return fmt.Errorf("failed to set user %s enabled status: %w", userConfig.Username, err)
		}

		log.Printf("[INFO] Configured user: %s (algorithm: %s, enabled: %t)",
			userConfig.Username, userConfig.Algorithm, userConfig.Enabled)
	}

	// Add authenticator to chain
	authChain.AddAuthenticator(memAuth)
	log.Printf("[INFO] Authentication configured with %d users", len(c.Broker.Auth.Users))

	return nil
}

// AddUser adds a new user to the configuration
func (c *Config) AddUser(username, password, algorithm string, enabled bool) error {
	// Check if user already exists
	for _, user := range c.Broker.Auth.Users {
		if user.Username == username {
			return fmt.Errorf("user %s already exists", username)
		}
	}

	// Validate algorithm
	switch algorithm {
	case "plain", "sha256", "bcrypt":
		// Valid algorithms
	default:
		return fmt.Errorf("unsupported algorithm: %s (supported: plain, sha256, bcrypt)", algorithm)
	}

	// Add user
	newUser := UserConfig{
		Username:  username,
		Password:  password,
		Algorithm: algorithm,
		Enabled:   enabled,
	}

	c.Broker.Auth.Users = append(c.Broker.Auth.Users, newUser)
	log.Printf("[INFO] Added user to configuration: %s", username)
	return nil
}

// UpdateUser updates an existing user in the configuration
func (c *Config) UpdateUser(username, password, algorithm string, enabled bool) error {
	for i, user := range c.Broker.Auth.Users {
		if user.Username == username {
			// Validate algorithm if provided
			if algorithm != "" {
				switch algorithm {
				case "plain", "sha256", "bcrypt":
					c.Broker.Auth.Users[i].Algorithm = algorithm
				default:
					return fmt.Errorf("unsupported algorithm: %s (supported: plain, sha256, bcrypt)", algorithm)
				}
			}

			// Update password if provided
			if password != "" {
				c.Broker.Auth.Users[i].Password = password
			}

			c.Broker.Auth.Users[i].Enabled = enabled
			log.Printf("[INFO] Updated user in configuration: %s", username)
			return nil
		}
	}

	return fmt.Errorf("user %s not found", username)
}

// RemoveUser removes a user from the configuration
func (c *Config) RemoveUser(username string) error {
	for i, user := range c.Broker.Auth.Users {
		if user.Username == username {
			c.Broker.Auth.Users = append(c.Broker.Auth.Users[:i], c.Broker.Auth.Users[i+1:]...)
			log.Printf("[INFO] Removed user from configuration: %s", username)
			return nil
		}
	}

	return fmt.Errorf("user %s not found", username)
}

// ListUsers returns all users in the configuration
func (c *Config) ListUsers() []UserConfig {
	return c.Broker.Auth.Users
}