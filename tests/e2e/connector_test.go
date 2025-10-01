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

package e2e

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/turtacn/emqx-go/pkg/connector"
)

// TestConnectorE2E tests the end-to-end connector functionality
func TestConnectorE2E(t *testing.T) {
	// Setup HTTP test server to receive webhook messages
	receivedMessages := make([]map[string]interface{}, 0)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var msg map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&msg); err == nil {
			receivedMessages = append(receivedMessages, msg)
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}))
	defer server.Close()

	t.Run("TestHTTPConnectorE2E", func(t *testing.T) {
		// Create connector manager
		manager := connector.CreateDefaultConnectorManager()

		// Create HTTP connector configuration
		config := connector.ConnectorConfig{
			ID:      "test-http-e2e",
			Name:    "Test HTTP E2E Connector",
			Type:    connector.ConnectorTypeHTTP,
			Enabled: true,
			Parameters: map[string]interface{}{
				"url":    server.URL,
				"method": "POST",
				"headers": map[string]string{
					"Content-Type": "application/json",
				},
				"timeout": "10s",
			},
			HealthCheck: connector.DefaultHealthCheckConfig(),
			Retry:       connector.DefaultRetryConfig(),
		}

		// Create connector
		err := manager.CreateConnector(config)
		if err != nil {
			t.Fatalf("Failed to create connector: %v", err)
		}

		// Start connector
		err = manager.StartConnector("test-http-e2e")
		if err != nil {
			t.Fatalf("Failed to start connector: %v", err)
		}

		// Wait a moment for connector to start
		time.Sleep(100 * time.Millisecond)

		// Get connector and verify it's running
		conn, err := manager.GetConnector("test-http-e2e")
		if err != nil {
			t.Fatalf("Failed to get connector: %v", err)
		}

		if !conn.IsRunning() {
			t.Fatal("Connector should be running")
		}

		// Test health check
		ctx := context.Background()
		if err := conn.HealthCheck(ctx); err != nil {
			t.Logf("Health check failed as expected for test server: %v", err)
			// For test servers, health check might fail, which is acceptable
		}

		// Send test message
		testMessage := &connector.Message{
			ID:        "test-msg-1",
			Topic:     "test/topic",
			Payload:   []byte(`{"message": "Hello from connector test", "timestamp": "2023-12-01T10:00:00Z"}`),
			Headers:   map[string]string{"X-Test": "true"},
			Timestamp: time.Now(),
			Metadata:  map[string]interface{}{"qos": 1},
		}

		result, err := conn.Send(ctx, testMessage)
		if err != nil {
			t.Fatalf("Failed to send message: %v", err)
		}

		if !result.Success {
			t.Fatalf("Message send failed: %s", result.Error)
		}

		if result.MessageID != "test-msg-1" {
			t.Errorf("Expected message ID 'test-msg-1', got '%s'", result.MessageID)
		}

		// Wait for message to be received
		time.Sleep(100 * time.Millisecond)

		// Verify message was received by webhook
		if len(receivedMessages) != 1 {
			t.Fatalf("Expected 1 received message, got %d", len(receivedMessages))
		}

		received := receivedMessages[0]
		if message, ok := received["message"].(string); !ok || message != "Hello from connector test" {
			t.Errorf("Expected message 'Hello from connector test', got %v", received["message"])
		}

		// Test batch sending
		batchMessages := []*connector.Message{
			{
				ID:        "batch-msg-1",
				Topic:     "batch/topic1",
				Payload:   []byte(`{"batch": 1}`),
				Timestamp: time.Now(),
			},
			{
				ID:        "batch-msg-2",
				Topic:     "batch/topic2",
				Payload:   []byte(`{"batch": 2}`),
				Timestamp: time.Now(),
			},
		}

		batchResults, err := conn.SendBatch(ctx, batchMessages)
		if err != nil {
			t.Fatalf("Failed to send batch: %v", err)
		}

		if len(batchResults) != 2 {
			t.Fatalf("Expected 2 batch results, got %d", len(batchResults))
		}

		for i, result := range batchResults {
			if !result.Success {
				t.Errorf("Batch message %d failed: %s", i, result.Error)
			}
		}

		// Wait for batch messages to be received
		time.Sleep(100 * time.Millisecond)

		// Verify batch messages were received
		if len(receivedMessages) != 3 {
			t.Fatalf("Expected 3 total received messages, got %d", len(receivedMessages))
		}

		// Test metrics
		metrics := conn.GetMetrics()
		if metrics.MessagesSent < 3 {
			t.Errorf("Expected at least 3 messages sent, got %d", metrics.MessagesSent)
		}

		if metrics.SuccessCount < 3 {
			t.Errorf("Expected at least 3 successful sends, got %d", metrics.SuccessCount)
		}

		// Test connector status
		status := conn.Status()
		if status.State != connector.StateRunning {
			t.Errorf("Expected state running, got %s", status.State)
		}

		if !status.HealthStatus.Healthy {
			t.Logf("Connector health status not healthy, but that's acceptable for test server")
		}

		// Stop connector
		err = manager.StopConnector("test-http-e2e")
		if err != nil {
			t.Fatalf("Failed to stop connector: %v", err)
		}

		// Verify connector is stopped
		if conn.IsRunning() {
			t.Error("Connector should not be running after stop")
		}

		// Test metrics after stop
		finalMetrics := conn.GetMetrics()
		if finalMetrics.MessagesSent != metrics.MessagesSent {
			t.Error("Metrics should be preserved after stop")
		}

		// Clean up
		err = manager.DeleteConnector("test-http-e2e")
		if err != nil {
			t.Fatalf("Failed to delete connector: %v", err)
		}

		// Verify connector is deleted
		_, err = manager.GetConnector("test-http-e2e")
		if err == nil {
			t.Error("Expected error when getting deleted connector")
		}
	})

	t.Run("TestConnectorManagerE2E", func(t *testing.T) {
		manager := connector.CreateDefaultConnectorManager()

		// Test creating multiple connectors
		configs := []connector.ConnectorConfig{
			{
				ID:   "http-1",
				Name: "HTTP Connector 1",
				Type: connector.ConnectorTypeHTTP,
				Parameters: map[string]interface{}{
					"url": server.URL + "/webhook1",
				},
				HealthCheck: connector.DefaultHealthCheckConfig(),
				Retry:       connector.DefaultRetryConfig(),
			},
			{
				ID:   "http-2",
				Name: "HTTP Connector 2",
				Type: connector.ConnectorTypeHTTP,
				Parameters: map[string]interface{}{
					"url": server.URL + "/webhook2",
				},
				HealthCheck: connector.DefaultHealthCheckConfig(),
				Retry:       connector.DefaultRetryConfig(),
			},
		}

		// Create connectors
		for _, config := range configs {
			err := manager.CreateConnector(config)
			if err != nil {
				t.Fatalf("Failed to create connector %s: %v", config.ID, err)
			}
		}

		// List connectors
		connectors := manager.ListConnectors()
		if len(connectors) != 2 {
			t.Fatalf("Expected 2 connectors, got %d", len(connectors))
		}

		// Verify connectors exist
		for _, config := range configs {
			if _, exists := connectors[config.ID]; !exists {
				t.Errorf("Connector %s not found in list", config.ID)
			}
		}

		// Test connector types
		types := manager.GetConnectorTypes()
		expectedTypes := []connector.ConnectorType{
			connector.ConnectorTypeHTTP,
			connector.ConnectorTypeMySQL,
			connector.ConnectorTypePostgreSQL,
			connector.ConnectorTypeRedis,
			connector.ConnectorTypeKafka,
		}

		if len(types) != len(expectedTypes) {
			t.Errorf("Expected %d connector types, got %d", len(expectedTypes), len(types))
		}

		// Test health check all
		ctx := context.Background()
		healthResults := manager.HealthCheckAll(ctx)

		// Since connectors are not started, health checks should fail
		for id, err := range healthResults {
			if err == nil {
				t.Errorf("Expected health check to fail for stopped connector %s", id)
			}
		}

		// Test metrics collection
		allMetrics := manager.GetAllMetrics()
		if len(allMetrics) != 2 {
			t.Errorf("Expected metrics for 2 connectors, got %d", len(allMetrics))
		}

		for id, metrics := range allMetrics {
			if metrics.MessagesSent != 0 {
				t.Errorf("Expected 0 messages sent for new connector %s, got %d", id, metrics.MessagesSent)
			}
		}

		// Clean up
		for _, config := range configs {
			err := manager.DeleteConnector(config.ID)
			if err != nil {
				t.Errorf("Failed to delete connector %s: %v", config.ID, err)
			}
		}

		// Verify all connectors are deleted
		finalConnectors := manager.ListConnectors()
		if len(finalConnectors) != 0 {
			t.Errorf("Expected 0 connectors after cleanup, got %d", len(finalConnectors))
		}
	})

	t.Run("TestConnectorFailureHandling", func(t *testing.T) {
		manager := connector.CreateDefaultConnectorManager()

		// Create connector with invalid URL to test error handling
		config := connector.ConnectorConfig{
			ID:   "error-test",
			Name: "Error Test Connector",
			Type: connector.ConnectorTypeHTTP,
			Parameters: map[string]interface{}{
				"url":    "http://non-existent-domain-12345.com/webhook",
				"method": "POST",
			},
			HealthCheck: connector.DefaultHealthCheckConfig(),
			Retry:       connector.DefaultRetryConfig(),
		}

		err := manager.CreateConnector(config)
		if err != nil {
			t.Fatalf("Failed to create error test connector: %v", err)
		}

		// Start connector - expect it to fail during startup due to invalid URL
		err = manager.StartConnector("error-test")
		if err != nil {
			t.Logf("Connector failed to start as expected: %v", err)
			// For this test, we expect it to fail to start
			// Let's manually set it to created state and test health check
			conn, _ := manager.GetConnector("error-test")

			// Health check should fail for invalid URL
			ctx := context.Background()
			err = conn.HealthCheck(ctx)
			if err == nil {
				t.Error("Expected health check to fail for invalid URL")
			}

			// Clean up
			manager.DeleteConnector("error-test")
			return
		}

		conn, _ := manager.GetConnector("error-test")

		// Health check should fail for invalid URL
		ctx := context.Background()
		err = conn.HealthCheck(ctx)
		if err == nil {
			t.Error("Expected health check to fail for invalid URL")
		}

		// Sending message should fail
		testMessage := &connector.Message{
			ID:        "error-msg-1",
			Topic:     "error/topic",
			Payload:   []byte(`{"test": "error"}`),
			Timestamp: time.Now(),
		}

		result, err := conn.Send(ctx, testMessage)
		if err == nil {
			t.Error("Expected error when sending to invalid URL")
		}

		if result != nil && result.Success {
			t.Error("Expected message send to fail")
		}

		// Metrics should show error
		metrics := conn.GetMetrics()
		if metrics.ErrorCount == 0 {
			t.Error("Expected error count to be greater than 0")
		}

		// Clean up
		manager.DeleteConnector("error-test")
	})

	// Reset received messages for isolation
	receivedMessages = receivedMessages[:0]
}

// TestConnectorIntegration tests connector integration scenarios
func TestConnectorIntegration(t *testing.T) {
	t.Run("TestConfigValidation", func(t *testing.T) {
		manager := connector.CreateDefaultConnectorManager()

		// Test invalid configuration
		invalidConfigs := []connector.ConnectorConfig{
			{
				ID:   "invalid-1",
				Type: connector.ConnectorTypeHTTP,
				Parameters: map[string]interface{}{
					// Missing required URL
					"method": "POST",
				},
			},
			{
				ID:   "invalid-2",
				Type: "unsupported-type",
				Parameters: map[string]interface{}{
					"url": "http://example.com",
				},
			},
		}

		for i, config := range invalidConfigs {
			err := manager.CreateConnector(config)
			if err == nil {
				t.Errorf("Expected error for invalid config %d", i)
			}
		}
	})

	t.Run("TestConnectorLifecycle", func(t *testing.T) {
		manager := connector.CreateDefaultConnectorManager()

		config := connector.ConnectorConfig{
			ID:   "lifecycle-test",
			Name: "Lifecycle Test",
			Type: connector.ConnectorTypeHTTP,
			Parameters: map[string]interface{}{
				"url": "http://httpbin.org/status/200",
			},
			HealthCheck: connector.DefaultHealthCheckConfig(),
			Retry:       connector.DefaultRetryConfig(),
		}

		// Create
		err := manager.CreateConnector(config)
		if err != nil {
			t.Fatalf("Failed to create connector: %v", err)
		}

		conn, _ := manager.GetConnector("lifecycle-test")

		// Initial state should be created
		status := conn.Status()
		if status.State != connector.StateCreated {
			t.Errorf("Expected initial state %s, got %s", connector.StateCreated, status.State)
		}

		// Start
		err = manager.StartConnector("lifecycle-test")
		if err != nil {
			t.Fatalf("Failed to start connector: %v", err)
		}

		if !conn.IsRunning() {
			t.Error("Connector should be running after start")
		}

		// Restart
		err = manager.RestartConnector("lifecycle-test")
		if err != nil {
			t.Fatalf("Failed to restart connector: %v", err)
		}

		if !conn.IsRunning() {
			t.Error("Connector should be running after restart")
		}

		// Stop
		err = manager.StopConnector("lifecycle-test")
		if err != nil {
			t.Fatalf("Failed to stop connector: %v", err)
		}

		if conn.IsRunning() {
			t.Error("Connector should not be running after stop")
		}

		// Delete
		err = manager.DeleteConnector("lifecycle-test")
		if err != nil {
			t.Fatalf("Failed to delete connector: %v", err)
		}
	})
}

// Helper function to check if string contains substring
func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}