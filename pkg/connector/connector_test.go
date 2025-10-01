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

package connector

import (
	"context"
	"testing"
	"time"
)

func TestConnectorManager(t *testing.T) {
	// Create connector manager
	manager := NewConnectorManager()

	// Register HTTP factory
	manager.RegisterFactory(&HTTPConnectorFactory{})

	t.Run("TestCreateConnector", func(t *testing.T) {
		config := ConnectorConfig{
			ID:      "test-http",
			Name:    "Test HTTP Connector",
			Type:    ConnectorTypeHTTP,
			Enabled: true,
			Parameters: map[string]interface{}{
				"url":    "http://example.com/webhook",
				"method": "POST",
			},
			HealthCheck: DefaultHealthCheckConfig(),
			Retry:       DefaultRetryConfig(),
		}

		err := manager.CreateConnector(config)
		if err != nil {
			t.Fatalf("Failed to create connector: %v", err)
		}

		// Verify connector exists
		conn, err := manager.GetConnector("test-http")
		if err != nil {
			t.Fatalf("Failed to get connector: %v", err)
		}

		if conn == nil {
			t.Fatal("Connector is nil")
		}

		// Verify configuration
		connConfig := conn.GetConfig()
		if connConfig.ID != "test-http" {
			t.Errorf("Expected ID 'test-http', got '%s'", connConfig.ID)
		}

		if connConfig.Type != ConnectorTypeHTTP {
			t.Errorf("Expected type '%s', got '%s'", ConnectorTypeHTTP, connConfig.Type)
		}
	})

	t.Run("TestListConnectors", func(t *testing.T) {
		connectors := manager.ListConnectors()
		if len(connectors) != 1 {
			t.Errorf("Expected 1 connector, got %d", len(connectors))
		}

		if _, exists := connectors["test-http"]; !exists {
			t.Error("Expected connector 'test-http' to exist")
		}
	})

	t.Run("TestGetConnectorInfo", func(t *testing.T) {
		info, err := manager.GetConnectorInfo("test-http")
		if err != nil {
			t.Fatalf("Failed to get connector info: %v", err)
		}

		if info.Config.ID != "test-http" {
			t.Errorf("Expected ID 'test-http', got '%s'", info.Config.ID)
		}

		if info.Status.ID != "test-http" {
			t.Errorf("Expected status ID 'test-http', got '%s'", info.Status.ID)
		}
	})

	t.Run("TestDeleteConnector", func(t *testing.T) {
		err := manager.DeleteConnector("test-http")
		if err != nil {
			t.Fatalf("Failed to delete connector: %v", err)
		}

		// Verify connector is deleted
		_, err = manager.GetConnector("test-http")
		if err == nil {
			t.Error("Expected error when getting deleted connector")
		}

		connectors := manager.ListConnectors()
		if len(connectors) != 0 {
			t.Errorf("Expected 0 connectors after deletion, got %d", len(connectors))
		}
	})

	t.Run("TestCreateDuplicateConnector", func(t *testing.T) {
		config := ConnectorConfig{
			ID:   "test-duplicate",
			Name: "Test Duplicate",
			Type: ConnectorTypeHTTP,
			Parameters: map[string]interface{}{
				"url": "http://example.com",
			},
			HealthCheck: DefaultHealthCheckConfig(),
			Retry:       DefaultRetryConfig(),
		}

		// Create first connector
		err := manager.CreateConnector(config)
		if err != nil {
			t.Fatalf("Failed to create first connector: %v", err)
		}

		// Try to create duplicate
		err = manager.CreateConnector(config)
		if err == nil {
			t.Error("Expected error when creating duplicate connector")
		}

		if err != ErrConnectorExists {
			t.Errorf("Expected ErrConnectorExists, got %v", err)
		}

		// Cleanup
		manager.DeleteConnector("test-duplicate")
	})
}

func TestHTTPConnectorFactory(t *testing.T) {
	factory := &HTTPConnectorFactory{}

	t.Run("TestType", func(t *testing.T) {
		if factory.Type() != ConnectorTypeHTTP {
			t.Errorf("Expected type %s, got %s", ConnectorTypeHTTP, factory.Type())
		}
	})

	t.Run("TestValidateConfig", func(t *testing.T) {
		// Valid config
		config := ConnectorConfig{
			Parameters: map[string]interface{}{
				"url":    "http://example.com/webhook",
				"method": "POST",
			},
		}

		err := factory.ValidateConfig(config)
		if err != nil {
			t.Errorf("Expected valid config to pass validation, got error: %v", err)
		}

		// Invalid config - missing URL
		invalidConfig := ConnectorConfig{
			Parameters: map[string]interface{}{
				"method": "POST",
			},
		}

		err = factory.ValidateConfig(invalidConfig)
		if err == nil {
			t.Error("Expected error for config missing URL")
		}
	})

	t.Run("TestGetDefaultConfig", func(t *testing.T) {
		defaultConfig := factory.GetDefaultConfig()

		if defaultConfig.Type != ConnectorTypeHTTP {
			t.Errorf("Expected type %s, got %s", ConnectorTypeHTTP, defaultConfig.Type)
		}

		if defaultConfig.Enabled {
			t.Error("Expected default config to be disabled")
		}

		// Check default parameters
		params := defaultConfig.Parameters
		if url, ok := params["url"].(string); !ok || url == "" {
			t.Error("Expected default URL parameter")
		}

		if method, ok := params["method"].(string); !ok || method != "POST" {
			t.Errorf("Expected default method 'POST', got '%v'", method)
		}
	})

	t.Run("TestGetConfigSchema", func(t *testing.T) {
		schema := factory.GetConfigSchema()

		if schema["type"] != "object" {
			t.Error("Expected schema type to be 'object'")
		}

		properties, ok := schema["properties"].(map[string]interface{})
		if !ok {
			t.Fatal("Expected properties in schema")
		}

		// Check URL property
		if _, exists := properties["url"]; !exists {
			t.Error("Expected 'url' property in schema")
		}

		// Check method property
		if _, exists := properties["method"]; !exists {
			t.Error("Expected 'method' property in schema")
		}

		required, ok := schema["required"].([]string)
		if !ok {
			t.Fatal("Expected required fields in schema")
		}

		urlRequired := false
		for _, field := range required {
			if field == "url" {
				urlRequired = true
				break
			}
		}
		if !urlRequired {
			t.Error("Expected 'url' to be required field")
		}
	})

	t.Run("TestCreateConnector", func(t *testing.T) {
		config := ConnectorConfig{
			ID:   "test-create",
			Name: "Test Create",
			Type: ConnectorTypeHTTP,
			Parameters: map[string]interface{}{
				"url":     "http://example.com/webhook",
				"method":  "POST",
				"timeout": "30s",
				"headers": map[string]string{
					"Content-Type": "application/json",
				},
			},
			HealthCheck: DefaultHealthCheckConfig(),
			Retry:       DefaultRetryConfig(),
		}

		connector, err := factory.Create(config)
		if err != nil {
			t.Fatalf("Failed to create connector: %v", err)
		}

		if connector == nil {
			t.Fatal("Created connector is nil")
		}

		// Verify connector type
		httpConn, ok := connector.(*HTTPConnector)
		if !ok {
			t.Fatal("Expected HTTPConnector type")
		}

		// Verify configuration
		connConfig := httpConn.GetConfig()
		if connConfig.ID != "test-create" {
			t.Errorf("Expected ID 'test-create', got '%s'", connConfig.ID)
		}

		// Verify HTTP config
		if httpConn.httpConfig.URL != "http://example.com/webhook" {
			t.Errorf("Expected URL 'http://example.com/webhook', got '%s'", httpConn.httpConfig.URL)
		}

		if httpConn.httpConfig.Method != "POST" {
			t.Errorf("Expected method 'POST', got '%s'", httpConn.httpConfig.Method)
		}

		if httpConn.httpConfig.Timeout != 30*time.Second {
			t.Errorf("Expected timeout 30s, got %v", httpConn.httpConfig.Timeout)
		}
	})
}

func TestBaseConnector(t *testing.T) {
	config := ConnectorConfig{
		ID:          "test-base",
		Name:        "Test Base",
		Type:        ConnectorTypeHTTP,
		HealthCheck: DefaultHealthCheckConfig(),
		Retry:       DefaultRetryConfig(),
	}

	base := NewBaseConnector(config)

	t.Run("TestInitialState", func(t *testing.T) {
		if base.IsRunning() {
			t.Error("Expected connector to not be running initially")
		}

		status := base.Status()
		if status.State != StateCreated {
			t.Errorf("Expected state %s, got %s", StateCreated, status.State)
		}

		if status.ID != "test-base" {
			t.Errorf("Expected ID 'test-base', got '%s'", status.ID)
		}
	})

	t.Run("TestGetConfig", func(t *testing.T) {
		retrievedConfig := base.GetConfig()

		if retrievedConfig.ID != config.ID {
			t.Errorf("Expected ID '%s', got '%s'", config.ID, retrievedConfig.ID)
		}

		if retrievedConfig.Name != config.Name {
			t.Errorf("Expected name '%s', got '%s'", config.Name, retrievedConfig.Name)
		}

		if retrievedConfig.Type != config.Type {
			t.Errorf("Expected type '%s', got '%s'", config.Type, retrievedConfig.Type)
		}
	})

	t.Run("TestMetrics", func(t *testing.T) {
		metrics := base.GetMetrics()

		// Initially all metrics should be zero
		if metrics.MessagesSent != 0 {
			t.Errorf("Expected MessagesSent 0, got %d", metrics.MessagesSent)
		}

		if metrics.ErrorCount != 0 {
			t.Errorf("Expected ErrorCount 0, got %d", metrics.ErrorCount)
		}

		// Test metric recording
		base.recordSentMessage()
		base.recordSuccess(100 * time.Millisecond)

		updatedMetrics := base.GetMetrics()
		if updatedMetrics.MessagesSent != 1 {
			t.Errorf("Expected MessagesSent 1, got %d", updatedMetrics.MessagesSent)
		}

		if updatedMetrics.SuccessCount != 1 {
			t.Errorf("Expected SuccessCount 1, got %d", updatedMetrics.SuccessCount)
		}

		if updatedMetrics.AvgLatency != 100*time.Millisecond {
			t.Errorf("Expected AvgLatency 100ms, got %v", updatedMetrics.AvgLatency)
		}

		// Test reset
		base.ResetMetrics()
		resetMetrics := base.GetMetrics()
		if resetMetrics.MessagesSent != 0 {
			t.Errorf("Expected MessagesSent 0 after reset, got %d", resetMetrics.MessagesSent)
		}
	})

	t.Run("TestUpdateConfig", func(t *testing.T) {
		newConfig := config
		newConfig.Name = "Updated Name"
		newConfig.Description = "Updated Description"

		err := base.UpdateConfig(context.Background(), newConfig)
		if err != nil {
			t.Fatalf("Failed to update config: %v", err)
		}

		updatedConfig := base.GetConfig()
		if updatedConfig.Name != "Updated Name" {
			t.Errorf("Expected name 'Updated Name', got '%s'", updatedConfig.Name)
		}

		if updatedConfig.Description != "Updated Description" {
			t.Errorf("Expected description 'Updated Description', got '%s'", updatedConfig.Description)
		}

		// Test invalid updates
		invalidConfig := newConfig
		invalidConfig.ID = "different-id"
		err = base.UpdateConfig(context.Background(), invalidConfig)
		if err == nil {
			t.Error("Expected error when changing connector ID")
		}

		invalidConfig = newConfig
		invalidConfig.Type = ConnectorTypeMySQL
		err = base.UpdateConfig(context.Background(), invalidConfig)
		if err == nil {
			t.Error("Expected error when changing connector type")
		}
	})
}

func TestConnectorRegistry(t *testing.T) {
	t.Run("TestGetRegisteredConnectorTypes", func(t *testing.T) {
		types := GetRegisteredConnectorTypes()

		expectedTypes := []ConnectorType{
			ConnectorTypeHTTP,
			ConnectorTypeMySQL,
			ConnectorTypePostgreSQL,
			ConnectorTypeRedis,
			ConnectorTypeKafka,
		}

		if len(types) != len(expectedTypes) {
			t.Errorf("Expected %d types, got %d", len(expectedTypes), len(types))
		}

		for _, expectedType := range expectedTypes {
			found := false
			for _, actualType := range types {
				if actualType == expectedType {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("Expected type %s not found in registered types", expectedType)
			}
		}
	})

	t.Run("TestCreateDefaultConnectorManager", func(t *testing.T) {
		manager := CreateDefaultConnectorManager()

		if manager == nil {
			t.Fatal("Created manager is nil")
		}

		// Verify all types are registered
		registeredTypes := manager.GetConnectorTypes()
		expectedTypes := GetRegisteredConnectorTypes()

		if len(registeredTypes) != len(expectedTypes) {
			t.Errorf("Expected %d registered types, got %d", len(expectedTypes), len(registeredTypes))
		}

		for _, expectedType := range expectedTypes {
			found := false
			for _, actualType := range registeredTypes {
				if actualType == expectedType {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("Expected type %s not registered in manager", expectedType)
			}
		}
	})
}

func TestMessage(t *testing.T) {
	t.Run("TestMessageCreation", func(t *testing.T) {
		msg := &Message{
			ID:        "test-msg-1",
			Topic:     "test/topic",
			Payload:   []byte("test payload"),
			Headers:   map[string]string{"Content-Type": "application/json"},
			Timestamp: time.Now(),
			Metadata:  map[string]interface{}{"qos": 1},
		}

		if msg.ID != "test-msg-1" {
			t.Errorf("Expected ID 'test-msg-1', got '%s'", msg.ID)
		}

		if msg.Topic != "test/topic" {
			t.Errorf("Expected topic 'test/topic', got '%s'", msg.Topic)
		}

		if string(msg.Payload) != "test payload" {
			t.Errorf("Expected payload 'test payload', got '%s'", string(msg.Payload))
		}

		if msg.Headers["Content-Type"] != "application/json" {
			t.Errorf("Expected Content-Type 'application/json', got '%s'", msg.Headers["Content-Type"])
		}

		if qos, ok := msg.Metadata["qos"].(int); !ok || qos != 1 {
			t.Errorf("Expected QoS 1, got %v", msg.Metadata["qos"])
		}
	})
}

func TestDefaultConfigs(t *testing.T) {
	t.Run("TestDefaultHealthCheckConfig", func(t *testing.T) {
		config := DefaultHealthCheckConfig()

		if !config.Enabled {
			t.Error("Expected health check to be enabled by default")
		}

		if config.Interval != 30*time.Second {
			t.Errorf("Expected interval 30s, got %v", config.Interval)
		}

		if config.Timeout != 10*time.Second {
			t.Errorf("Expected timeout 10s, got %v", config.Timeout)
		}

		if config.MaxFails != 3 {
			t.Errorf("Expected max fails 3, got %d", config.MaxFails)
		}
	})

	t.Run("TestDefaultRetryConfig", func(t *testing.T) {
		config := DefaultRetryConfig()

		if config.MaxAttempts != 3 {
			t.Errorf("Expected max attempts 3, got %d", config.MaxAttempts)
		}

		if config.InitialBackoff != 1*time.Second {
			t.Errorf("Expected initial backoff 1s, got %v", config.InitialBackoff)
		}

		if config.MaxBackoff != 60*time.Second {
			t.Errorf("Expected max backoff 60s, got %v", config.MaxBackoff)
		}

		if config.BackoffMultiplier != 2.0 {
			t.Errorf("Expected backoff multiplier 2.0, got %f", config.BackoffMultiplier)
		}
	})

	t.Run("TestDefaultPoolConfig", func(t *testing.T) {
		config := DefaultPoolConfig()

		if config.MaxSize != 10 {
			t.Errorf("Expected max size 10, got %d", config.MaxSize)
		}

		if config.MinSize != 1 {
			t.Errorf("Expected min size 1, got %d", config.MinSize)
		}

		if config.MaxIdle != 5*time.Minute {
			t.Errorf("Expected max idle 5m, got %v", config.MaxIdle)
		}

		if config.MaxLifetime != 30*time.Minute {
			t.Errorf("Expected max lifetime 30m, got %v", config.MaxLifetime)
		}
	})
}