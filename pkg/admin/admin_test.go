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

package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/turtacn/emqx-go/pkg/metrics"
)

// mockBroker implements BrokerInterface for testing
type mockBroker struct {
	connections   []ConnectionInfo
	sessions      []SessionInfo
	subscriptions []SubscriptionInfo
	routes        []RouteInfo
	nodes         []NodeInfo
	nodeInfo      NodeInfo
}

func newMockBroker() *mockBroker {
	return &mockBroker{
		connections: []ConnectionInfo{
			{
				ClientID:    "client1",
				Username:    "user1",
				PeerHost:    "127.0.0.1",
				SockPort:    1883,
				Protocol:    "MQTT",
				ConnectedAt: time.Now(),
				KeepAlive:   60,
				CleanStart:  true,
				ProtoVer:    5,
				Node:        "node1",
			},
			{
				ClientID:    "client2",
				Username:    "user2",
				PeerHost:    "127.0.0.1",
				SockPort:    1883,
				Protocol:    "MQTT",
				ConnectedAt: time.Now(),
				KeepAlive:   120,
				CleanStart:  false,
				ProtoVer:    4,
				Node:        "node1",
			},
		},
		sessions: []SessionInfo{
			{
				ClientID:  "client1",
				Username:  "user1",
				CreatedAt: time.Now(),
				Node:      "node1",
			},
			{
				ClientID:  "client2",
				Username:  "user2",
				CreatedAt: time.Now(),
				Node:      "node1",
			},
		},
		subscriptions: []SubscriptionInfo{
			{
				ClientID: "client1",
				Topic:    "test/topic1",
				QoS:      1,
				Node:     "node1",
			},
			{
				ClientID: "client2",
				Topic:    "test/topic2",
				QoS:      2,
				Node:     "node1",
			},
		},
		routes: []RouteInfo{
			{
				Topic: "test/topic1",
				Nodes: []string{"node1"},
			},
			{
				Topic: "test/topic2",
				Nodes: []string{"node1", "node2"},
			},
		},
		nodes: []NodeInfo{
			{
				Node:       "node1",
				NodeStatus: "running",
				Version:    "1.0.0",
				Uptime:     3600,
				Datetime:   time.Now(),
			},
		},
		nodeInfo: NodeInfo{
			Node:       "node1",
			NodeStatus: "running",
			Version:    "1.0.0",
			Uptime:     3600,
			Datetime:   time.Now(),
		},
	}
}

func (m *mockBroker) GetConnections() []ConnectionInfo {
	return m.connections
}

func (m *mockBroker) GetSessions() []SessionInfo {
	return m.sessions
}

func (m *mockBroker) GetSubscriptions() []SubscriptionInfo {
	return m.subscriptions
}

func (m *mockBroker) GetRoutes() []RouteInfo {
	return m.routes
}

func (m *mockBroker) DisconnectClient(clientID string) error {
	// Remove connection from mock data
	for i, conn := range m.connections {
		if conn.ClientID == clientID {
			m.connections = append(m.connections[:i], m.connections[i+1:]...)
			break
		}
	}
	return nil
}

func (m *mockBroker) KickoutSession(clientID string) error {
	// Remove session from mock data
	for i, session := range m.sessions {
		if session.ClientID == clientID {
			m.sessions = append(m.sessions[:i], m.sessions[i+1:]...)
			break
		}
	}
	return nil
}

func (m *mockBroker) GetNodeInfo() NodeInfo {
	return m.nodeInfo
}

func (m *mockBroker) GetClusterNodes() []NodeInfo {
	return m.nodes
}

func TestNewAPIServer(t *testing.T) {
	metricsManager := metrics.NewMetricsManager()
	broker := newMockBroker()

	server := NewAPIServer(metricsManager, broker)

	assert.NotNil(t, server)
	assert.Equal(t, metricsManager, server.metricsManager)
	assert.Equal(t, broker, server.broker)
}

func TestAPIServerStats(t *testing.T) {
	metricsManager := metrics.NewMetricsManager()
	broker := newMockBroker()
	server := NewAPIServer(metricsManager, broker)

	// Add some test metrics
	metricsManager.IncConnections()
	metricsManager.IncSessions(true)
	metricsManager.IncSubscriptions(false)

	req, err := http.NewRequest("GET", "/api/v5/stats", nil)
	require.NoError(t, err)

	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(server.handleStats)
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "application/json", rr.Header().Get("Content-Type"))

	var response APIResponse
	err = json.Unmarshal(rr.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, 0, response.Code)
	assert.NotNil(t, response.Data)
}

func TestAPIServerConnections(t *testing.T) {
	metricsManager := metrics.NewMetricsManager()
	broker := newMockBroker()
	server := NewAPIServer(metricsManager, broker)

	req, err := http.NewRequest("GET", "/api/v5/connections", nil)
	require.NoError(t, err)

	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(server.handleConnections)
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)

	var response APIResponse
	err = json.Unmarshal(rr.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, 0, response.Code)

	// Verify response structure
	data, ok := response.Data.(map[string]interface{})
	require.True(t, ok)

	assert.Contains(t, data, "data")
	assert.Contains(t, data, "meta")

	// Verify pagination metadata
	meta, ok := data["meta"].(map[string]interface{})
	require.True(t, ok)

	assert.Equal(t, float64(1), meta["page"])
	assert.Equal(t, float64(20), meta["limit"])
	assert.Equal(t, float64(2), meta["count"])
	assert.Equal(t, float64(2), meta["total"])
}

func TestAPIServerConnectionsPagination(t *testing.T) {
	metricsManager := metrics.NewMetricsManager()
	broker := newMockBroker()
	server := NewAPIServer(metricsManager, broker)

	req, err := http.NewRequest("GET", "/api/v5/connections?page=1&limit=1", nil)
	require.NoError(t, err)

	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(server.handleConnections)
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)

	var response APIResponse
	err = json.Unmarshal(rr.Body.Bytes(), &response)
	require.NoError(t, err)

	data, ok := response.Data.(map[string]interface{})
	require.True(t, ok)

	meta, ok := data["meta"].(map[string]interface{})
	require.True(t, ok)

	assert.Equal(t, float64(1), meta["page"])
	assert.Equal(t, float64(1), meta["limit"])
	assert.Equal(t, float64(1), meta["count"])
	assert.Equal(t, float64(2), meta["total"])
}

func TestAPIServerConnectionByID(t *testing.T) {
	metricsManager := metrics.NewMetricsManager()
	broker := newMockBroker()
	server := NewAPIServer(metricsManager, broker)

	// Test GET connection by ID
	req, err := http.NewRequest("GET", "/api/v5/connections/client1", nil)
	require.NoError(t, err)

	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(server.handleConnectionByID)
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)

	var response APIResponse
	err = json.Unmarshal(rr.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, 0, response.Code)

	// Verify connection data
	data, ok := response.Data.(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "client1", data["clientid"])
}

func TestAPIServerConnectionByIDNotFound(t *testing.T) {
	metricsManager := metrics.NewMetricsManager()
	broker := newMockBroker()
	server := NewAPIServer(metricsManager, broker)

	req, err := http.NewRequest("GET", "/api/v5/connections/nonexistent", nil)
	require.NoError(t, err)

	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(server.handleConnectionByID)
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusNotFound, rr.Code)
}

func TestAPIServerDisconnectClient(t *testing.T) {
	metricsManager := metrics.NewMetricsManager()
	broker := newMockBroker()
	server := NewAPIServer(metricsManager, broker)

	// Test DELETE connection (disconnect client)
	req, err := http.NewRequest("DELETE", "/api/v5/connections/client1", nil)
	require.NoError(t, err)

	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(server.handleConnectionByID)
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)

	var response APIResponse
	err = json.Unmarshal(rr.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, 0, response.Code)

	// Verify client was disconnected
	assert.Equal(t, 1, len(broker.GetConnections()))
}

func TestAPIServerSessions(t *testing.T) {
	metricsManager := metrics.NewMetricsManager()
	broker := newMockBroker()
	server := NewAPIServer(metricsManager, broker)

	req, err := http.NewRequest("GET", "/api/v5/sessions", nil)
	require.NoError(t, err)

	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(server.handleSessions)
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)

	var response APIResponse
	err = json.Unmarshal(rr.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, 0, response.Code)

	data, ok := response.Data.(map[string]interface{})
	require.True(t, ok)
	assert.Contains(t, data, "data")
	assert.Contains(t, data, "meta")
}

func TestAPIServerSessionByID(t *testing.T) {
	metricsManager := metrics.NewMetricsManager()
	broker := newMockBroker()
	server := NewAPIServer(metricsManager, broker)

	req, err := http.NewRequest("GET", "/api/v5/sessions/client1", nil)
	require.NoError(t, err)

	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(server.handleSessionByID)
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)

	var response APIResponse
	err = json.Unmarshal(rr.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, 0, response.Code)
}

func TestAPIServerKickoutSession(t *testing.T) {
	metricsManager := metrics.NewMetricsManager()
	broker := newMockBroker()
	server := NewAPIServer(metricsManager, broker)

	req, err := http.NewRequest("DELETE", "/api/v5/sessions/client1", nil)
	require.NoError(t, err)

	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(server.handleSessionByID)
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)

	// Verify session was removed
	assert.Equal(t, 1, len(broker.GetSessions()))
}

func TestAPIServerSubscriptions(t *testing.T) {
	metricsManager := metrics.NewMetricsManager()
	broker := newMockBroker()
	server := NewAPIServer(metricsManager, broker)

	req, err := http.NewRequest("GET", "/api/v5/subscriptions", nil)
	require.NoError(t, err)

	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(server.handleSubscriptions)
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)

	var response APIResponse
	err = json.Unmarshal(rr.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, 0, response.Code)
}

func TestAPIServerSubscriptionByClientID(t *testing.T) {
	metricsManager := metrics.NewMetricsManager()
	broker := newMockBroker()
	server := NewAPIServer(metricsManager, broker)

	req, err := http.NewRequest("GET", "/api/v5/subscriptions/client1", nil)
	require.NoError(t, err)

	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(server.handleSubscriptionByID)
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)

	var response APIResponse
	err = json.Unmarshal(rr.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, 0, response.Code)

	// Verify subscriptions data
	data, ok := response.Data.([]interface{})
	require.True(t, ok)
	assert.Equal(t, 1, len(data))
}

func TestAPIServerRoutes(t *testing.T) {
	metricsManager := metrics.NewMetricsManager()
	broker := newMockBroker()
	server := NewAPIServer(metricsManager, broker)

	req, err := http.NewRequest("GET", "/api/v5/routes", nil)
	require.NoError(t, err)

	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(server.handleRoutes)
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)

	var response APIResponse
	err = json.Unmarshal(rr.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, 0, response.Code)
}

func TestAPIServerRouteByTopic(t *testing.T) {
	metricsManager := metrics.NewMetricsManager()
	broker := newMockBroker()
	server := NewAPIServer(metricsManager, broker)

	req, err := http.NewRequest("GET", "/api/v5/routes/test/topic1", nil)
	require.NoError(t, err)

	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(server.handleRouteByTopic)
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)

	var response APIResponse
	err = json.Unmarshal(rr.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, 0, response.Code)
}

func TestAPIServerNodes(t *testing.T) {
	metricsManager := metrics.NewMetricsManager()
	broker := newMockBroker()
	server := NewAPIServer(metricsManager, broker)

	req, err := http.NewRequest("GET", "/api/v5/nodes", nil)
	require.NoError(t, err)

	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(server.handleNodes)
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)

	var response APIResponse
	err = json.Unmarshal(rr.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, 0, response.Code)
}

func TestAPIServerNodeByID(t *testing.T) {
	metricsManager := metrics.NewMetricsManager()
	broker := newMockBroker()
	server := NewAPIServer(metricsManager, broker)

	req, err := http.NewRequest("GET", "/api/v5/nodes/node1", nil)
	require.NoError(t, err)

	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(server.handleNodeByID)
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)

	var response APIResponse
	err = json.Unmarshal(rr.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, 0, response.Code)
}

func TestAPIServerStatus(t *testing.T) {
	metricsManager := metrics.NewMetricsManager()
	broker := newMockBroker()
	server := NewAPIServer(metricsManager, broker)

	req, err := http.NewRequest("GET", "/api/v5/status", nil)
	require.NoError(t, err)

	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(server.handleStatus)
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)

	var response APIResponse
	err = json.Unmarshal(rr.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, 0, response.Code)

	data, ok := response.Data.(map[string]interface{})
	require.True(t, ok)
	assert.Contains(t, data, "node")
	assert.Contains(t, data, "connections")
	assert.Contains(t, data, "sessions")
	assert.Contains(t, data, "subscriptions")
}

func TestAPIServerHealth(t *testing.T) {
	metricsManager := metrics.NewMetricsManager()
	broker := newMockBroker()
	server := NewAPIServer(metricsManager, broker)

	req, err := http.NewRequest("GET", "/health", nil)
	require.NoError(t, err)

	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(server.handleHealth)
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)

	var response APIResponse
	err = json.Unmarshal(rr.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, 0, response.Code)

	data, ok := response.Data.(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "ok", data["status"])
	assert.Contains(t, data, "time")
}

func TestAPIServerMethodNotAllowed(t *testing.T) {
	metricsManager := metrics.NewMetricsManager()
	broker := newMockBroker()
	server := NewAPIServer(metricsManager, broker)

	req, err := http.NewRequest("POST", "/api/v5/stats", strings.NewReader("{}"))
	require.NoError(t, err)

	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(server.handleStats)
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
}

func TestExtractIDFromPath(t *testing.T) {
	server := &APIServer{}

	tests := []struct {
		path     string
		prefix   string
		expected string
	}{
		{"/api/v5/connections/client1", "/api/v5/connections/", "client1"},
		{"/api/v5/sessions/session1", "/api/v5/sessions/", "session1"},
		{"/api/v5/nodes/node1", "/api/v5/nodes/", "node1"},
		{"/api/v5/connections/", "/api/v5/connections/", ""},
		{"/api/v5/other", "/api/v5/connections/", ""},
	}

	for _, test := range tests {
		result := server.extractIDFromPath(test.path, test.prefix)
		assert.Equal(t, test.expected, result)
	}
}

func TestGetPagination(t *testing.T) {
	server := &APIServer{}

	tests := []struct {
		url           string
		expectedPage  int
		expectedLimit int
	}{
		{"http://example.com/api", 1, 20},
		{"http://example.com/api?page=2", 2, 20},
		{"http://example.com/api?limit=50", 1, 50},
		{"http://example.com/api?page=3&limit=10", 3, 10},
		{"http://example.com/api?page=0", 1, 20},     // Invalid page defaults to 1
		{"http://example.com/api?limit=0", 1, 20},    // Invalid limit defaults to 20
		{"http://example.com/api?limit=2000", 1, 20}, // Limit too high defaults to 20
	}

	for _, test := range tests {
		req, err := http.NewRequest("GET", test.url, nil)
		require.NoError(t, err)

		page, limit := server.getPagination(req)
		assert.Equal(t, test.expectedPage, page)
		assert.Equal(t, test.expectedLimit, limit)
	}
}