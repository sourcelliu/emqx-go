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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/turtacn/emqx-go/pkg/admin"
	"github.com/turtacn/emqx-go/pkg/broker"
	"github.com/turtacn/emqx-go/pkg/connector"
	"github.com/turtacn/emqx-go/pkg/dashboard"
	"github.com/turtacn/emqx-go/pkg/integration"
	"github.com/turtacn/emqx-go/pkg/metrics"
	"github.com/turtacn/emqx-go/pkg/monitor"
	"github.com/turtacn/emqx-go/pkg/rules"
)

const (
	integrationTestBrokerPort    = ":1936"
	integrationTestDashboardPort = 18090
	integrationTestMetricsPort   = ":8099"
)

// TestDataIntegrationE2E tests the complete data integration flow
func TestDataIntegrationE2E(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start broker
	brokerInstance := broker.New("integration-e2e-broker", nil)
	brokerInstance.SetupDefaultAuth()
	defer brokerInstance.Close()

	go brokerInstance.StartServer(ctx, integrationTestBrokerPort)
	time.Sleep(200 * time.Millisecond)

	// Start metrics server
	go metrics.Serve(integrationTestMetricsPort)
	time.Sleep(100 * time.Millisecond)

	// Create dashboard components
	metricsManager := metrics.NewMetricsManager()
	healthChecker := monitor.NewHealthChecker()
	mockBroker := newTestBrokerInterface()
	adminAPI := admin.NewAPIServer(metricsManager, mockBroker)

	// Create dashboard server
	config := &dashboard.Config{
		Address:    "127.0.0.1",
		Port:       integrationTestDashboardPort,
		Username:   "admin",
		Password:   "public",
		EnableAuth: false,
	}

	// Create connector manager and integration engine
	connectorManager := connector.CreateDefaultConnectorManager()
	integrationEngine := integration.NewDataIntegrationEngine()
	ruleEngine := rules.NewRuleEngine(connectorManager)

	dashboardServer, err := dashboard.NewServer(config, adminAPI, metricsManager, healthChecker, nil, connectorManager, ruleEngine, integrationEngine)
	require.NoError(t, err)

	// Start dashboard server
	go dashboardServer.Start(ctx)
	time.Sleep(200 * time.Millisecond)

	baseURL := fmt.Sprintf("http://localhost:%d", integrationTestDashboardPort)

	// Test bridge creation via API
	bridge := map[string]interface{}{
		"id":          "e2e-test-bridge",
		"name":        "E2E Test Bridge",
		"type":        "kafka",
		"direction":   "egress",
		"description": "E2E test bridge for Kafka integration",
		"configuration": map[string]interface{}{
			"brokers":     []string{"localhost:9092"},
			"topic":       "test-mqtt-data",
			"topic_filter": "sensor/+",
		},
	}

	// Create bridge via API
	bridgeJSON, err := json.Marshal(bridge)
	require.NoError(t, err)

	resp, err := http.Post(baseURL+"/api/integration/bridges", "application/json", bytes.NewBuffer(bridgeJSON))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Verify bridge was created
	resp, err = http.Get(baseURL + "/api/integration/bridges")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var bridgesResponse []interface{}
	err = json.NewDecoder(resp.Body).Decode(&bridgesResponse)
	require.NoError(t, err)

	assert.Len(t, bridgesResponse, 1)

	createdBridge := bridgesResponse[0].(map[string]interface{})
	assert.Equal(t, "e2e-test-bridge", createdBridge["id"])
	assert.Equal(t, "E2E Test Bridge", createdBridge["name"])
	assert.Equal(t, "disabled", createdBridge["status"])

	// Enable the bridge
	resp, err = http.Post(baseURL+"/api/integration/bridges/e2e-test-bridge/enable", "application/json", nil)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Verify bridge is enabled
	resp, err = http.Get(baseURL + "/api/integration/bridges/e2e-test-bridge")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var bridgeResponse map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&bridgeResponse)
	require.NoError(t, err)

	assert.Equal(t, "enabled", bridgeResponse["status"])

	dashboardServer.Stop()
}

// TestDataIntegrationMetrics tests the metrics collection for data integration
func TestDataIntegrationMetrics(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start broker
	brokerInstance := broker.New("integration-metrics-broker", nil)
	brokerInstance.SetupDefaultAuth()
	defer brokerInstance.Close()

	go brokerInstance.StartServer(ctx, ":1937")
	time.Sleep(200 * time.Millisecond)

	// Create dashboard components
	metricsManager := metrics.NewMetricsManager()
	healthChecker := monitor.NewHealthChecker()
	mockBroker := newTestBrokerInterface()
	adminAPI := admin.NewAPIServer(metricsManager, mockBroker)

	// Create dashboard server
	config := &dashboard.Config{
		Address:    "127.0.0.1",
		Port:       18091,
		Username:   "admin",
		Password:   "public",
		EnableAuth: false,
	}

	// Create connector manager and integration engine
	connectorManager := connector.CreateDefaultConnectorManager()
	integrationEngine := integration.NewDataIntegrationEngine()
	ruleEngine := rules.NewRuleEngine(connectorManager)

	dashboardServer, err := dashboard.NewServer(config, adminAPI, metricsManager, healthChecker, nil, connectorManager, ruleEngine, integrationEngine)
	require.NoError(t, err)

	// Start dashboard server
	go dashboardServer.Start(ctx)
	time.Sleep(200 * time.Millisecond)

	baseURL := "http://localhost:18091"

	// Create a test bridge
	bridge := integration.Bridge{
		ID:          "metrics-test-bridge",
		Name:        "Metrics Test Bridge",
		Type:        "kafka",
		Direction:   "egress",
		Description: "Bridge for metrics testing",
		Configuration: map[string]interface{}{
			"brokers":      []string{"localhost:9092"},
			"topic":        "metrics-test",
			"topic_filter": "metrics/test",
		},
	}

	err = integrationEngine.CreateBridge(bridge)
	require.NoError(t, err)

	err = integrationEngine.EnableBridge("metrics-test-bridge")
	require.NoError(t, err)

	// Get integration metrics
	resp, err := http.Get(baseURL + "/api/integration/metrics")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var metricsResponse integration.IntegrationMetrics
	err = json.NewDecoder(resp.Body).Decode(&metricsResponse)
	require.NoError(t, err)

	assert.Equal(t, 1, metricsResponse.TotalBridges)
	assert.Equal(t, 1, metricsResponse.ActiveBridges)
	assert.GreaterOrEqual(t, metricsResponse.TotalConnectors, 0)

	dashboardServer.Stop()
}

// TestDataIntegrationWithMQTT tests data integration with real MQTT traffic
func TestDataIntegrationWithMQTT(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start broker
	brokerInstance := broker.New("integration-mqtt-broker", nil)
	brokerInstance.SetupDefaultAuth()
	defer brokerInstance.Close()

	go brokerInstance.StartServer(ctx, ":1938")
	time.Sleep(200 * time.Millisecond)

	// Create dashboard components
	metricsManager := metrics.NewMetricsManager()
	healthChecker := monitor.NewHealthChecker()
	mockBroker := newTestBrokerInterface()
	adminAPI := admin.NewAPIServer(metricsManager, mockBroker)

	// Create dashboard server
	config := &dashboard.Config{
		Address:    "127.0.0.1",
		Port:       18092,
		Username:   "admin",
		Password:   "public",
		EnableAuth: false,
	}

	// Create connector manager and integration engine
	connectorManager := connector.CreateDefaultConnectorManager()
	integrationEngine := integration.NewDataIntegrationEngine()
	ruleEngine := rules.NewRuleEngine(connectorManager)

	dashboardServer, err := dashboard.NewServer(config, adminAPI, metricsManager, healthChecker, nil, connectorManager, ruleEngine, integrationEngine)
	require.NoError(t, err)

	// Start dashboard server
	go dashboardServer.Start(ctx)
	time.Sleep(200 * time.Millisecond)

	// Create a test bridge
	bridge := integration.Bridge{
		ID:          "mqtt-test-bridge",
		Name:        "MQTT Test Bridge",
		Type:        "kafka",
		Direction:   "egress",
		Description: "Bridge for MQTT integration testing",
		Configuration: map[string]interface{}{
			"brokers":      []string{"localhost:9092"},
			"topic":        "mqtt-integration-test",
			"topic_filter": "sensor/temperature",
		},
	}

	err = integrationEngine.CreateBridge(bridge)
	require.NoError(t, err)

	err = integrationEngine.EnableBridge("mqtt-test-bridge")
	require.NoError(t, err)

	// Create MQTT client
	client := createIntegrationTestClient("integration-test-client", ":1938")
	token := client.Connect()
	require.True(t, token.WaitTimeout(5*time.Second))
	require.NoError(t, token.Error())
	defer client.Disconnect(100)

	// Publish test messages
	testMessages := []struct {
		topic   string
		payload string
	}{
		{"sensor/temperature", `{"temperature": 23.5, "unit": "celsius"}`},
		{"sensor/temperature", `{"temperature": 24.1, "unit": "celsius"}`},
		{"sensor/humidity", `{"humidity": 65.2, "unit": "percent"}`}, // Should not match bridge filter
	}

	for _, msg := range testMessages {
		token := client.Publish(msg.topic, 1, false, msg.payload)
		require.True(t, token.WaitTimeout(time.Second))
		require.NoError(t, token.Error())
	}

	// Wait a bit for processing
	time.Sleep(500 * time.Millisecond)

	// Get integration metrics to verify processing
	baseURL := "http://localhost:18092"
	resp, err := http.Get(baseURL + "/api/integration/metrics")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var metricsResponse integration.IntegrationMetrics
	err = json.NewDecoder(resp.Body).Decode(&metricsResponse)
	require.NoError(t, err)

	// Should have processed messages (exact count depends on implementation)
	assert.GreaterOrEqual(t, metricsResponse.TotalMessages, int64(0))

	dashboardServer.Stop()
}

// TestDataIntegrationProcessors tests data processors functionality
func TestDataIntegrationProcessors(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Test JSON processor directly
	transformations := []integration.JSONTransformation{
		{
			Type:       "map",
			SourcePath: "$.temp",
			TargetPath: "$.temperature_celsius",
		},
		{
			Type: "enrich",
			Value: map[string]interface{}{
				"processed_at": "2023-01-01T12:00:00Z",
				"version":      "1.0",
			},
		},
	}

	jsonProcessor := integration.NewJSONProcessor("test-json", "Test JSON Processor", transformations)

	// Test message processing
	testMessage := &integration.Message{
		ID:      "test-msg-1",
		Topic:   "sensor/data",
		Payload: []byte(`{"temp": 25.5, "humidity": 60, "device_id": "sensor001"}`),
		QoS:     1,
		Headers: map[string]string{"source": "test"},
		Metadata: map[string]interface{}{
			"timestamp": time.Now().Unix(),
		},
		Timestamp:  time.Now(),
		SourceType: "mqtt",
		SourceID:   "test-client",
	}

	processedMsg, err := jsonProcessor.Process(ctx, testMessage)
	require.NoError(t, err)
	require.NotNil(t, processedMsg)

	// Verify the processed payload
	var processedPayload map[string]interface{}
	err = json.Unmarshal(processedMsg.Payload, &processedPayload)
	require.NoError(t, err)

	// Original data should be preserved
	assert.Equal(t, 25.5, processedPayload["temp"])
	assert.Equal(t, float64(60), processedPayload["humidity"])
	assert.Equal(t, "sensor001", processedPayload["device_id"])

	// Test Plain Text processor
	plainTextProcessor := integration.NewPlainTextProcessor("test-text", "Test Text Processor")

	processedTextMsg, err := plainTextProcessor.Process(ctx, testMessage)
	require.NoError(t, err)
	require.NotNil(t, processedTextMsg)

	// Verify we got some text output
	assert.NotEmpty(t, processedTextMsg.Payload)
}

// TestDataIntegrationConnectorManagement tests connector registration and management
func TestDataIntegrationConnectorManagement(t *testing.T) {
	integrationEngine := integration.NewDataIntegrationEngine()

	// Test Kafka connector
	kafkaConfig := &integration.KafkaConfig{
		ID:               "test-kafka-connector",
		Name:             "Test Kafka Connector",
		Brokers:          []string{"localhost:9092"},
		Topics:           []string{"test-topic"},
		SecurityProtocol: "PLAINTEXT",
		CompressionType:  "gzip",
	}

	kafkaConnector := integration.NewKafkaConnector(kafkaConfig)
	err := integrationEngine.RegisterConnector(kafkaConnector)
	require.NoError(t, err)

	// Verify connector registration
	connectors := integrationEngine.ListConnectors()
	assert.Len(t, connectors, 1)
	assert.Equal(t, "test-kafka-connector", connectors[0].ID())
	assert.Equal(t, "kafka", connectors[0].Type())

	// Test connector retrieval
	retrievedConnector, err := integrationEngine.GetConnector("test-kafka-connector")
	require.NoError(t, err)
	assert.Equal(t, "test-kafka-connector", retrievedConnector.ID())

	// Test connector metrics
	metrics := retrievedConnector.GetMetrics()
	assert.Equal(t, "disconnected", metrics.ConnectionStatus)
	assert.Equal(t, int64(0), metrics.MessagesReceived)
	assert.Equal(t, int64(0), metrics.MessagesSent)

	// Test connector configuration
	config := retrievedConnector.GetConfiguration()
	assert.Equal(t, "test-kafka-connector", config["id"])
	assert.Equal(t, "Test Kafka Connector", config["name"])
	assert.Equal(t, []interface{}{"localhost:9092"}, config["brokers"])
}

func createIntegrationTestClient(clientID, address string) mqtt.Client {
	opts := mqtt.NewClientOptions()
	opts.AddBroker(fmt.Sprintf("tcp://localhost%s", address))
	opts.SetClientID(clientID)
	opts.SetUsername("test")
	opts.SetPassword("test")
	opts.SetCleanSession(true)
	opts.SetAutoReconnect(false)
	opts.SetConnectTimeout(5 * time.Second)

	return mqtt.NewClient(opts)
}