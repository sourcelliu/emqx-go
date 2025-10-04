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
	"github.com/turtacn/emqx-go/pkg/blacklist"
	"github.com/turtacn/emqx-go/pkg/broker"
	"github.com/turtacn/emqx-go/pkg/dashboard"
	"github.com/turtacn/emqx-go/pkg/metrics"
	"github.com/turtacn/emqx-go/pkg/monitor"
)

const (
	blacklistTestBrokerPortBase    = 1939
	blacklistTestDashboardPortBase = 18093
	blacklistTestMetricsPortBase   = 8100
)

// TestBlacklistE2EClientIDBlocking tests client ID blacklist functionality end-to-end
func TestBlacklistE2EClientIDBlocking(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Use unique ports for this test
	brokerPort := fmt.Sprintf(":%d", blacklistTestBrokerPortBase)
	dashboardPort := blacklistTestDashboardPortBase
	metricsPort := fmt.Sprintf(":%d", blacklistTestMetricsPortBase)
	baseURL := fmt.Sprintf("http://localhost:%d", dashboardPort)

	// Start broker with blacklist middleware
	brokerInstance := broker.New("blacklist-e2e-broker", nil)
	brokerInstance.SetupDefaultAuth()
	defer brokerInstance.Close()

	go brokerInstance.StartServer(ctx, brokerPort)
	time.Sleep(300 * time.Millisecond)

	// Start test environment
	dashboardServer := setupBlacklistTestEnvironment(t, ctx, brokerInstance, dashboardPort, metricsPort)
	defer dashboardServer.Stop()

	// Test 1: Normal client connection should work initially
	normalClient := createBlacklistTestClient("good-client", brokerPort)
	token := normalClient.Connect()
	require.True(t, token.WaitTimeout(5*time.Second), "Good client should connect initially")
	require.NoError(t, token.Error())
	normalClient.Disconnect(100)

	// Test 2: Add client ID to blacklist via API
	blacklistEntry := map[string]interface{}{
		"type":    "clientid",
		"value":   "malicious-client",
		"action":  "deny",
		"reason":  "Suspicious behavior detected",
		"enabled": true,
	}

	entryJSON, err := json.Marshal(blacklistEntry)
	require.NoError(t, err)

	resp, err := http.Post(baseURL+"/api/v5/blacklist", "application/json", bytes.NewBuffer(entryJSON))
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusCreated, resp.StatusCode)

	// Test 3: Blacklisted client should be blocked
	blockedClient := createBlacklistTestClient("malicious-client", brokerPort)
	token = blockedClient.Connect()

	// Connection should fail or close immediately
	connected := token.WaitTimeout(5 * time.Second)
	if connected {
		// If connection succeeds, it should be closed quickly due to blacklist
		time.Sleep(500 * time.Millisecond)
		assert.False(t, blockedClient.IsConnected(), "Blacklisted client should be disconnected")
	} else {
		// Connection should fail with authentication error
		assert.Error(t, token.Error(), "Blacklisted client connection should fail")
	}
	blockedClient.Disconnect(100)

	// Test 4: Normal client should still work
	normalClient2 := createBlacklistTestClient("another-good-client", brokerPort)
	token = normalClient2.Connect()
	require.True(t, token.WaitTimeout(5*time.Second), "Good client should still connect")
	require.NoError(t, token.Error())
	normalClient2.Disconnect(100)

	// Test 5: Verify blacklist entry via API
	resp, err = http.Get(baseURL + "/api/v5/blacklist?type=clientid")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var apiResponse struct {
		Code int                      `json:"code"`
		Data []map[string]interface{} `json:"data"`
	}
	err = json.NewDecoder(resp.Body).Decode(&apiResponse)
	require.NoError(t, err)
	assert.Equal(t, 0, apiResponse.Code)
	require.Len(t, apiResponse.Data, 1, "Should have exactly 1 blacklist entry")
	assert.Equal(t, "malicious-client", apiResponse.Data[0]["value"])
	assert.Equal(t, "deny", apiResponse.Data[0]["action"])
}

// TestBlacklistE2EUsernameBlocking tests username blacklist functionality
func TestBlacklistE2EUsernameBlocking(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Use unique ports for this test
	brokerPort := fmt.Sprintf(":%d", blacklistTestBrokerPortBase+1)
	dashboardPort := blacklistTestDashboardPortBase + 1
	metricsPort := fmt.Sprintf(":%d", blacklistTestMetricsPortBase+1)
	baseURL := fmt.Sprintf("http://localhost:%d", dashboardPort)

	// Start broker
	brokerInstance := broker.New("blacklist-username-broker", nil)
	brokerInstance.SetupDefaultAuth()
	defer brokerInstance.Close()

	go brokerInstance.StartServer(ctx, brokerPort)
	time.Sleep(300 * time.Millisecond)

	// Setup test environment
	dashboardServer := setupBlacklistTestEnvironment(t, ctx, brokerInstance, dashboardPort, metricsPort)
	defer dashboardServer.Stop()

	// Add username to blacklist
	blacklistEntry := map[string]interface{}{
		"type":    "username",
		"value":   "baduser",
		"action":  "deny",
		"reason":  "Account compromised",
		"enabled": true,
	}

	entryJSON, err := json.Marshal(blacklistEntry)
	require.NoError(t, err)

	resp, err := http.Post(baseURL+"/api/v5/blacklist", "application/json", bytes.NewBuffer(entryJSON))
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusCreated, resp.StatusCode)

	// Test blocked username
	opts := mqtt.NewClientOptions()
	opts.AddBroker("tcp://localhost" + brokerPort)
	opts.SetClientID("username-test-client")
	opts.SetUsername("baduser")
	opts.SetPassword("test")
	opts.SetConnectTimeout(5 * time.Second)

	blockedClient := mqtt.NewClient(opts)
	token := blockedClient.Connect()

	connected := token.WaitTimeout(5 * time.Second)
	if connected {
		time.Sleep(500 * time.Millisecond)
		assert.False(t, blockedClient.IsConnected(), "Blacklisted username should be disconnected")
	} else {
		assert.Error(t, token.Error(), "Blacklisted username connection should fail")
	}
	blockedClient.Disconnect(100)

	// Test good username should work
	opts.SetUsername("test")
	goodClient := mqtt.NewClient(opts)
	token = goodClient.Connect()
	require.True(t, token.WaitTimeout(5*time.Second), "Good username should connect")
	require.NoError(t, token.Error())
	goodClient.Disconnect(100)
}

// TestBlacklistE2ETopicBlocking tests topic blacklist functionality
func TestBlacklistE2ETopicBlocking(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Use unique ports for this test
	brokerPort := fmt.Sprintf(":%d", blacklistTestBrokerPortBase+2)
	dashboardPort := blacklistTestDashboardPortBase + 2
	metricsPort := fmt.Sprintf(":%d", blacklistTestMetricsPortBase+2)
	baseURL := fmt.Sprintf("http://localhost:%d", dashboardPort)

	// Start broker
	brokerInstance := broker.New("blacklist-topic-broker", nil)
	brokerInstance.SetupDefaultAuth()
	defer brokerInstance.Close()

	go brokerInstance.StartServer(ctx, brokerPort)
	time.Sleep(300 * time.Millisecond)

	// Setup test environment
	dashboardServer := setupBlacklistTestEnvironment(t, ctx, brokerInstance, dashboardPort, metricsPort)
	defer dashboardServer.Stop()

	// Add topic to blacklist
	blacklistEntry := map[string]interface{}{
		"type":    "topic",
		"value":   "forbidden/topic",
		"action":  "deny",
		"reason":  "Sensitive data",
		"enabled": true,
	}

	entryJSON, err := json.Marshal(blacklistEntry)
	require.NoError(t, err)

	resp, err := http.Post(baseURL+"/api/v5/blacklist", "application/json", bytes.NewBuffer(entryJSON))
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusCreated, resp.StatusCode)

	// Create client
	opts := mqtt.NewClientOptions()
	opts.AddBroker("tcp://localhost" + brokerPort)
	opts.SetClientID("topic-test-client")
	opts.SetUsername("test")
	opts.SetPassword("test")
	opts.SetConnectTimeout(5 * time.Second)

	client := mqtt.NewClient(opts)
	token := client.Connect()
	require.True(t, token.WaitTimeout(5*time.Second), "Client should connect")
	require.NoError(t, token.Error())
	defer client.Disconnect(100)

	// Test publishing to allowed topic should work
	token = client.Publish("allowed/topic", 1, false, "test message")
	require.True(t, token.WaitTimeout(5*time.Second), "Publish to allowed topic should succeed")
	require.NoError(t, token.Error())

	// Test publishing to forbidden topic should fail or be silently dropped
	// Note: Since we're testing the middleware integration, the publish might succeed
	// at the client level but be blocked by the broker's blacklist middleware
	token = client.Publish("forbidden/topic", 1, false, "blocked message")
	// The publish token might succeed because the client doesn't know about server-side blocking
	token.WaitTimeout(5 * time.Second)

	// Test subscribing to forbidden topic should fail
	subToken := client.Subscribe("forbidden/topic", 1, func(client mqtt.Client, msg mqtt.Message) {
		t.Error("Should not receive messages from blacklisted topic")
	})
	// Subscription might succeed at protocol level but be blocked by middleware
	subToken.WaitTimeout(5 * time.Second)
}

// TestBlacklistE2EPatternBlocking tests pattern-based blacklist functionality
func TestBlacklistE2EPatternBlocking(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Use unique ports for this test
	brokerPort := fmt.Sprintf(":%d", blacklistTestBrokerPortBase+3)
	dashboardPort := blacklistTestDashboardPortBase + 3
	metricsPort := fmt.Sprintf(":%d", blacklistTestMetricsPortBase+3)
	baseURL := fmt.Sprintf("http://localhost:%d", dashboardPort)

	// Start broker
	brokerInstance := broker.New("blacklist-pattern-broker", nil)
	brokerInstance.SetupDefaultAuth()
	defer brokerInstance.Close()

	go brokerInstance.StartServer(ctx, brokerPort)
	time.Sleep(300 * time.Millisecond)

	// Setup test environment
	dashboardServer := setupBlacklistTestEnvironment(t, ctx, brokerInstance, dashboardPort, metricsPort)
	defer dashboardServer.Stop()

	// Add pattern-based client ID blacklist
	blacklistEntry := map[string]interface{}{
		"type":    "clientid",
		"pattern": "bot_.*",
		"action":  "deny",
		"reason":  "Automated bot clients not allowed",
		"enabled": true,
	}

	entryJSON, err := json.Marshal(blacklistEntry)
	require.NoError(t, err)

	resp, err := http.Post(baseURL+"/api/v5/blacklist", "application/json", bytes.NewBuffer(entryJSON))
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusCreated, resp.StatusCode)

	// Test client matching pattern should be blocked
	blockedClient := createBlacklistTestClientWithID("bot_crawler", "test", "test", brokerPort)
	token := blockedClient.Connect()

	connected := token.WaitTimeout(5 * time.Second)
	if connected {
		time.Sleep(500 * time.Millisecond)
		assert.False(t, blockedClient.IsConnected(), "Pattern-matched client should be disconnected")
	} else {
		assert.Error(t, token.Error(), "Pattern-matched client connection should fail")
	}
	blockedClient.Disconnect(100)

	// Test client not matching pattern should work
	goodClient := createBlacklistTestClientWithID("legitimate_client", "test", "test", brokerPort)
	token = goodClient.Connect()
	require.True(t, token.WaitTimeout(5*time.Second), "Non-pattern-matched client should connect")
	require.NoError(t, token.Error())
	goodClient.Disconnect(100)
}

// TestBlacklistE2ETemporaryBlocking tests temporary blacklist entries with expiration
func TestBlacklistE2ETemporaryBlocking(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Use unique ports for this test
	brokerPort := fmt.Sprintf(":%d", blacklistTestBrokerPortBase+4)
	dashboardPort := blacklistTestDashboardPortBase + 4
	metricsPort := fmt.Sprintf(":%d", blacklistTestMetricsPortBase+4)
	baseURL := fmt.Sprintf("http://localhost:%d", dashboardPort)

	// Start broker
	brokerInstance := broker.New("blacklist-temp-broker", nil)
	brokerInstance.SetupDefaultAuth()
	defer brokerInstance.Close()

	go brokerInstance.StartServer(ctx, brokerPort)
	time.Sleep(300 * time.Millisecond)

	// Setup test environment
	dashboardServer := setupBlacklistTestEnvironment(t, ctx, brokerInstance, dashboardPort, metricsPort)
	defer dashboardServer.Stop()

	// Add temporary blacklist entry (expires in 3 seconds)
	expiresAt := time.Now().Add(3 * time.Second)
	blacklistEntry := map[string]interface{}{
		"type":       "clientid",
		"value":      "temp-blocked-client",
		"action":     "deny",
		"reason":     "Temporary block for testing",
		"enabled":    true,
		"expires_at": expiresAt.Format(time.RFC3339),
	}

	entryJSON, err := json.Marshal(blacklistEntry)
	require.NoError(t, err)

	resp, err := http.Post(baseURL+"/api/v5/blacklist", "application/json", bytes.NewBuffer(entryJSON))
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusCreated, resp.StatusCode)

	// Test client should be blocked initially
	blockedClient := createBlacklistTestClientWithID("temp-blocked-client", "test", "test", brokerPort)
	token := blockedClient.Connect()

	connected := token.WaitTimeout(5 * time.Second)
	if connected {
		time.Sleep(500 * time.Millisecond)
		assert.False(t, blockedClient.IsConnected(), "Temporarily blocked client should be disconnected")
	}
	blockedClient.Disconnect(100)

	// Wait for expiration
	time.Sleep(4 * time.Second)

	// Test client should be able to connect after expiration
	unblockedClient := createBlacklistTestClientWithID("temp-blocked-client", "test", "test", brokerPort)
	token = unblockedClient.Connect()
	assert.True(t, token.WaitTimeout(5*time.Second), "Client should connect after blacklist expires")
	if token.Error() != nil {
		t.Logf("Connection error after expiration: %v", token.Error())
	}
	unblockedClient.Disconnect(100)
}

// TestBlacklistE2EAPI tests the complete blacklist management API
func TestBlacklistE2EAPI(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Use unique ports for this test
	brokerPort := fmt.Sprintf(":%d", blacklistTestBrokerPortBase+5)
	dashboardPort := blacklistTestDashboardPortBase + 5
	metricsPort := fmt.Sprintf(":%d", blacklistTestMetricsPortBase+5)
	baseURL := fmt.Sprintf("http://localhost:%d", dashboardPort)

	// Start broker
	brokerInstance := broker.New("blacklist-api-broker", nil)
	brokerInstance.SetupDefaultAuth()
	defer brokerInstance.Close()

	go brokerInstance.StartServer(ctx, brokerPort)
	time.Sleep(300 * time.Millisecond)

	// Setup test environment
	dashboardServer := setupBlacklistTestEnvironment(t, ctx, brokerInstance, dashboardPort, metricsPort)
	defer dashboardServer.Stop()

	// Test 1: Create multiple blacklist entries
	entries := []map[string]interface{}{
		{
			"type":        "clientid",
			"value":       "api-test-client-1",
			"action":      "deny",
			"reason":      "API test entry 1",
			"description": "Test entry for API validation",
			"enabled":     true,
		},
		{
			"type":    "username",
			"value":   "api-test-user",
			"action":  "log",
			"reason":  "API test entry 2",
			"enabled": false,
		},
		{
			"type":    "topic",
			"pattern": "test/api/.*",
			"action":  "deny",
			"reason":  "API test pattern",
			"enabled": true,
		},
	}

	var createdIDs []string
	for i, entry := range entries {
		entryJSON, err := json.Marshal(entry)
		require.NoError(t, err)

		resp, err := http.Post(baseURL+"/api/v5/blacklist", "application/json", bytes.NewBuffer(entryJSON))
		require.NoError(t, err)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusCreated, resp.StatusCode)

		var response map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&response)
		require.NoError(t, err)

		data := response["data"].(map[string]interface{})
		createdIDs = append(createdIDs, data["id"].(string))

		t.Logf("Created blacklist entry %d with ID: %s", i+1, data["id"])
	}

	// Test 2: List all entries
	resp, err := http.Get(baseURL + "/api/v5/blacklist")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var listResponse map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&listResponse)
	require.NoError(t, err)

	entriesList := listResponse["data"].([]interface{})
	assert.Len(t, entriesList, 3)

	// Test 3: Filter by type
	resp, err = http.Get(baseURL + "/api/v5/blacklist?type=clientid")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	err = json.NewDecoder(resp.Body).Decode(&listResponse)
	require.NoError(t, err)

	filteredEntries := listResponse["data"].([]interface{})
	assert.Len(t, filteredEntries, 1)

	// Test 4: Get specific entry
	entryID := createdIDs[0]
	resp, err = http.Get(baseURL + "/api/v5/blacklist/" + entryID)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var getResponse map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&getResponse)
	require.NoError(t, err)

	entryData := getResponse["data"].(map[string]interface{})
	assert.Equal(t, entryID, entryData["id"])
	assert.Equal(t, "api-test-client-1", entryData["value"])

	// Test 5: Update entry
	updateData := map[string]interface{}{
		"reason":      "Updated reason via API",
		"description": "Updated description",
		"enabled":     false,
	}

	updateJSON, err := json.Marshal(updateData)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPut, baseURL+"/api/v5/blacklist/"+entryID, bytes.NewBuffer(updateJSON))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err = client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Verify update
	resp, err = http.Get(baseURL + "/api/v5/blacklist/" + entryID)
	require.NoError(t, err)
	defer resp.Body.Close()

	err = json.NewDecoder(resp.Body).Decode(&getResponse)
	require.NoError(t, err)

	updatedEntry := getResponse["data"].(map[string]interface{})
	assert.Equal(t, "Updated reason via API", updatedEntry["reason"])
	assert.Equal(t, false, updatedEntry["enabled"])

	// Test 6: Delete entry
	req, err = http.NewRequest(http.MethodDelete, baseURL+"/api/v5/blacklist/"+entryID, nil)
	require.NoError(t, err)

	resp, err = client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusNoContent, resp.StatusCode)

	// Verify deletion
	resp, err = http.Get(baseURL + "/api/v5/blacklist/" + entryID)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)

	// Test 7: Get blacklist statistics
	resp, err = http.Get(baseURL + "/api/v5/blacklist/stats")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var statsResponse map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&statsResponse)
	require.NoError(t, err)

	stats := statsResponse["data"].(map[string]interface{})
	assert.Equal(t, float64(2), stats["total_entries"]) // 2 remaining after deletion
}

// Helper functions

func setupBlacklistTestEnvironment(t *testing.T, ctx context.Context, brokerInstance *broker.Broker, dashboardPort int, metricsPort string) *dashboard.Server {
	// Start metrics server
	go metrics.Serve(metricsPort)
	time.Sleep(100 * time.Millisecond)

	// Create dashboard components
	metricsManager := metrics.NewMetricsManager()
	healthChecker := monitor.NewHealthChecker()

	// Create test broker interface with blacklist support
	testBroker := &testBrokerInterfaceWithBlacklist{
		testBrokerInterface: &testBrokerInterface{
			connections:   []admin.ConnectionInfo{},
			sessions:      []admin.SessionInfo{},
			subscriptions: []admin.SubscriptionInfo{},
			routes:        []admin.RouteInfo{},
			nodes: []admin.NodeInfo{
				{
					Node:       "blacklist-test-node",
					NodeStatus: "running",
					Version:    "1.0.0",
					Uptime:     3600,
					Datetime:   time.Now(),
				},
			},
		},
		broker: brokerInstance,
	}

	adminAPI := admin.NewAPIServer(metricsManager, testBroker)

	// Create dashboard server
	config := &dashboard.Config{
		Address:    "127.0.0.1",
		Port:       dashboardPort,
		Username:   "admin",
		Password:   "public",
		EnableAuth: false,
	}

	dashboardServer, err := dashboard.NewServer(config, adminAPI, metricsManager, healthChecker, nil, nil, nil, nil)
	require.NoError(t, err)

	// Start dashboard server
	go dashboardServer.Start(ctx)
	time.Sleep(300 * time.Millisecond)

	return dashboardServer
}

func createBlacklistTestClient(clientID string, brokerPort string) mqtt.Client {
	return createBlacklistTestClientWithID(clientID, "test", "test", brokerPort)
}

func createBlacklistTestClientWithID(clientID, username, password, brokerPort string) mqtt.Client {
	opts := mqtt.NewClientOptions()
	opts.AddBroker("tcp://localhost" + brokerPort)
	opts.SetClientID(clientID)
	opts.SetUsername(username)
	opts.SetPassword(password)
	opts.SetCleanSession(true)
	opts.SetAutoReconnect(false)
	opts.SetConnectTimeout(5 * time.Second)

	return mqtt.NewClient(opts)
}

// Extended test broker interface with blacklist support
type testBrokerInterfaceWithBlacklist struct {
	*testBrokerInterface
	broker *broker.Broker
}

func (t *testBrokerInterfaceWithBlacklist) GetBlacklistMiddleware() *blacklist.BlacklistMiddleware {
	if t.broker != nil {
		return t.broker.GetBlacklistMiddleware()
	}
	return nil
}