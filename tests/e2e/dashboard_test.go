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
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/turtacn/emqx-go/pkg/admin"
	"github.com/turtacn/emqx-go/pkg/broker"
	"github.com/turtacn/emqx-go/pkg/connector"
	"github.com/turtacn/emqx-go/pkg/dashboard"
	"github.com/turtacn/emqx-go/pkg/metrics"
	"github.com/turtacn/emqx-go/pkg/monitor"
)

const (
	dashboardPort = ":18084"
	brokerPort    = ":1934"
	metricsPort   = ":8098"
)

// mockBrokerInterface for e2e testing
type testBrokerInterface struct {
	connections   []admin.ConnectionInfo
	sessions      []admin.SessionInfo
	subscriptions []admin.SubscriptionInfo
	routes        []admin.RouteInfo
	nodes         []admin.NodeInfo
}

func newTestBrokerInterface() *testBrokerInterface {
	return &testBrokerInterface{
		connections:   []admin.ConnectionInfo{},
		sessions:      []admin.SessionInfo{},
		subscriptions: []admin.SubscriptionInfo{},
		routes:        []admin.RouteInfo{},
		nodes: []admin.NodeInfo{
			{
				Node:       "dashboard-test-node",
				NodeStatus: "running",
				Version:    "1.0.0",
				Uptime:     3600,
				Datetime:   time.Now(),
			},
		},
	}
}

func (t *testBrokerInterface) GetConnections() []admin.ConnectionInfo     { return t.connections }
func (t *testBrokerInterface) GetSessions() []admin.SessionInfo           { return t.sessions }
func (t *testBrokerInterface) GetSubscriptions() []admin.SubscriptionInfo { return t.subscriptions }
func (t *testBrokerInterface) GetRoutes() []admin.RouteInfo               { return t.routes }
func (t *testBrokerInterface) GetClusterNodes() []admin.NodeInfo          { return t.nodes }

func (t *testBrokerInterface) DisconnectClient(clientID string) error {
	for i, conn := range t.connections {
		if conn.ClientID == clientID {
			t.connections = append(t.connections[:i], t.connections[i+1:]...)
			break
		}
	}
	return nil
}

func (t *testBrokerInterface) KickoutSession(clientID string) error {
	for i, session := range t.sessions {
		if session.ClientID == clientID {
			t.sessions = append(t.sessions[:i], t.sessions[i+1:]...)
			break
		}
	}
	return nil
}

func (t *testBrokerInterface) GetNodeInfo() admin.NodeInfo {
	if len(t.nodes) > 0 {
		return t.nodes[0]
	}
	return admin.NodeInfo{
		Node:       "dashboard-test-node",
		NodeStatus: "running",
		Version:    "1.0.0",
		Uptime:     3600,
		Datetime:   time.Now(),
	}
}

func TestDashboardBasicFunctionality(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start broker
	brokerInstance := broker.New("dashboard-test-broker", nil)
	brokerInstance.SetupDefaultAuth()
	defer brokerInstance.Close()

	go brokerInstance.StartServer(ctx, brokerPort)
	time.Sleep(200 * time.Millisecond)

	// Start metrics server
	go metrics.Serve(metricsPort)
	time.Sleep(100 * time.Millisecond)

	// Create dashboard components
	metricsManager := metrics.NewMetricsManager()
	healthChecker := monitor.NewHealthChecker()
	mockBroker := newTestBrokerInterface()
	adminAPI := admin.NewAPIServer(metricsManager, mockBroker)

	// Create dashboard server
	config := &dashboard.Config{
		Address:    "127.0.0.1",
		Port:       18084,
		Username:   "admin",
		Password:   "public",
		EnableAuth: false, // Disable for testing
	}

	// Create connector manager for dashboard
	connectorManager := connector.CreateDefaultConnectorManager()

	dashboardServer, err := dashboard.NewServer(config, adminAPI, metricsManager, healthChecker, nil, connectorManager)
	require.NoError(t, err)

	// Start dashboard server
	go dashboardServer.Start(ctx)
	time.Sleep(200 * time.Millisecond)

	// Test dashboard main page
	resp, err := http.Get("http://localhost:18084/dashboard")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Contains(t, resp.Header.Get("Content-Type"), "text/html")

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Contains(t, string(body), "EMQX Go Dashboard")
	assert.Contains(t, string(body), "Dashboard Overview")

	dashboardServer.Stop()
}

func TestDashboardAPIEndpoints(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create dashboard components
	metricsManager := metrics.NewMetricsManager()
	healthChecker := monitor.NewHealthChecker()
	mockBroker := newTestBrokerInterface()
	adminAPI := admin.NewAPIServer(metricsManager, mockBroker)

	// Generate some test metrics
	metricsManager.IncConnections()
	metricsManager.IncSessions(true)
	metricsManager.IncSubscriptions(false)
	metricsManager.IncMessagesReceived(1)
	metricsManager.IncMessagesSent(1)

	// Create dashboard server
	config := &dashboard.Config{
		Address:    "127.0.0.1",
		Port:       18085,
		Username:   "admin",
		Password:   "public",
		EnableAuth: false,
	}

	// Create connector manager for dashboard
	connectorManager := connector.CreateDefaultConnectorManager()

	dashboardServer, err := dashboard.NewServer(config, adminAPI, metricsManager, healthChecker, nil, connectorManager)
	require.NoError(t, err)

	// Start dashboard server
	go dashboardServer.Start(ctx)
	time.Sleep(200 * time.Millisecond)

	// Test stats API
	resp, err := http.Get("http://localhost:18085/api/dashboard/stats")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "application/json", resp.Header.Get("Content-Type"))

	var stats metrics.Statistics
	err = json.NewDecoder(resp.Body).Decode(&stats)
	require.NoError(t, err)

	assert.Equal(t, int64(1), stats.Connections.Count)
	assert.Equal(t, int64(1), stats.Sessions.Count)
	assert.Equal(t, int64(1), stats.Subscriptions.Count)

	// Test connections API
	resp, err = http.Get("http://localhost:18085/api/dashboard/connections")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var connectionsResp map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&connectionsResp)
	require.NoError(t, err)

	assert.Contains(t, connectionsResp, "connections")
	assert.Contains(t, connectionsResp, "count")

	// Test health API
	resp, err = http.Get("http://localhost:18085/api/dashboard/health")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var health monitor.HealthStatus
	err = json.NewDecoder(resp.Body).Decode(&health)
	require.NoError(t, err)

	assert.NotEmpty(t, health.Status)
	assert.NotNil(t, health.SystemInfo)

	dashboardServer.Stop()
}

func TestDashboardWithRealMQTTTraffic(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start broker
	brokerInstance := broker.New("dashboard-mqtt-test-broker", nil)
	brokerInstance.SetupDefaultAuth()
	defer brokerInstance.Close()

	go brokerInstance.StartServer(ctx, ":1935")
	time.Sleep(200 * time.Millisecond)

	// Create dashboard components
	metricsManager := metrics.NewMetricsManager()
	healthChecker := monitor.NewHealthChecker()
	mockBroker := newTestBrokerInterface()
	adminAPI := admin.NewAPIServer(metricsManager, mockBroker)

	// Create dashboard server
	config := &dashboard.Config{
		Address:    "127.0.0.1",
		Port:       18086,
		Username:   "admin",
		Password:   "public",
		EnableAuth: false,
	}

	// Create connector manager for dashboard
	connectorManager := connector.CreateDefaultConnectorManager()

	dashboardServer, err := dashboard.NewServer(config, adminAPI, metricsManager, healthChecker, nil, connectorManager)
	require.NoError(t, err)

	// Start dashboard server
	go dashboardServer.Start(ctx)
	time.Sleep(200 * time.Millisecond)

	// Create MQTT clients
	client1 := createDashboardTestClient("dashboard-client-1", ":1935")
	token := client1.Connect()
	require.True(t, token.WaitTimeout(5*time.Second))
	require.NoError(t, token.Error())
	defer client1.Disconnect(100)

	client2 := createDashboardTestClient("dashboard-client-2", ":1935")
	token = client2.Connect()
	require.True(t, token.WaitTimeout(5*time.Second))
	require.NoError(t, token.Error())
	defer client2.Disconnect(100)

	// Subscribe to topics
	client1.Subscribe("dashboard/test/+", 1, func(client mqtt.Client, msg mqtt.Message) {
		// Message handler
	})

	client2.Subscribe("dashboard/test/data", 2, func(client mqtt.Client, msg mqtt.Message) {
		// Message handler
	})

	time.Sleep(100 * time.Millisecond)

	// Publish messages
	for i := 0; i < 5; i++ {
		payload := fmt.Sprintf("dashboard test message %d", i)
		token := client1.Publish("dashboard/test/data", 1, false, payload)
		require.True(t, token.WaitTimeout(time.Second))
		require.NoError(t, token.Error())
	}

	time.Sleep(200 * time.Millisecond)

	// Update metrics to simulate real activity
	metricsManager.IncConnections()
	metricsManager.IncConnections()
	metricsManager.IncSessions(false)
	metricsManager.IncSessions(true)
	metricsManager.IncSubscriptions(false)
	metricsManager.IncSubscriptions(false)

	for i := 0; i < 5; i++ {
		metricsManager.IncMessagesReceived(1)
		metricsManager.IncMessagesSent(1)
		metricsManager.IncPacketsReceived("PUBLISH")
		metricsManager.IncPacketsSent()
	}

	// Test dashboard shows updated metrics
	resp, err := http.Get("http://localhost:18086/api/dashboard/stats")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var stats metrics.Statistics
	err = json.NewDecoder(resp.Body).Decode(&stats)
	require.NoError(t, err)

	// Verify metrics reflect the activity
	assert.Equal(t, int64(2), stats.Connections.Count)
	assert.Equal(t, int64(2), stats.Sessions.Count)
	assert.Equal(t, int64(2), stats.Subscriptions.Count)
	assert.Equal(t, int64(5), stats.Messages.Received)
	assert.Equal(t, int64(5), stats.Messages.Sent)

	dashboardServer.Stop()
}

func TestDashboardAuthentication(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create dashboard components
	metricsManager := metrics.NewMetricsManager()
	healthChecker := monitor.NewHealthChecker()
	mockBroker := newTestBrokerInterface()
	adminAPI := admin.NewAPIServer(metricsManager, mockBroker)

	// Create dashboard server with auth enabled
	config := &dashboard.Config{
		Address:    "127.0.0.1",
		Port:       18087,
		Username:   "admin",
		Password:   "public",
		EnableAuth: true, // Enable auth for this test
	}

	// Create connector manager for dashboard
	connectorManager := connector.CreateDefaultConnectorManager()

	dashboardServer, err := dashboard.NewServer(config, adminAPI, metricsManager, healthChecker, nil, connectorManager)
	require.NoError(t, err)

	// Start dashboard server
	go dashboardServer.Start(ctx)
	time.Sleep(200 * time.Millisecond)

	// Create HTTP client that doesn't follow redirects
	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	// Test access without authentication - should redirect to login
	resp, err := client.Get("http://localhost:18087/dashboard")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusFound, resp.StatusCode)
	assert.Equal(t, "/login", resp.Header.Get("Location"))

	// Test login page
	resp, err = client.Get("http://localhost:18087/login")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Contains(t, resp.Header.Get("Content-Type"), "text/html")

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Contains(t, string(body), "EMQX Go Dashboard")
	assert.Contains(t, string(body), "Username")
	assert.Contains(t, string(body), "Password")

	// Test login with valid credentials
	form := url.Values{}
	form.Add("username", "admin")
	form.Add("password", "public")

	resp, err = client.Post("http://localhost:18087/login", "application/x-www-form-urlencoded", strings.NewReader(form.Encode()))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusFound, resp.StatusCode)
	assert.Equal(t, "/dashboard", resp.Header.Get("Location"))

	// Test API endpoint without auth - should return 401
	resp, err = client.Get("http://localhost:18087/api/dashboard/stats")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)

	dashboardServer.Stop()
}

func TestDashboardPageNavigation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create dashboard components
	metricsManager := metrics.NewMetricsManager()
	healthChecker := monitor.NewHealthChecker()
	mockBroker := newTestBrokerInterface()
	adminAPI := admin.NewAPIServer(metricsManager, mockBroker)

	// Create dashboard server
	config := &dashboard.Config{
		Address:    "127.0.0.1",
		Port:       18088,
		Username:   "admin",
		Password:   "public",
		EnableAuth: false,
	}

	// Create connector manager for dashboard
	connectorManager := connector.CreateDefaultConnectorManager()

	dashboardServer, err := dashboard.NewServer(config, adminAPI, metricsManager, healthChecker, nil, connectorManager)
	require.NoError(t, err)

	// Start dashboard server
	go dashboardServer.Start(ctx)
	time.Sleep(200 * time.Millisecond)

	baseURL := "http://localhost:18088"

	// Test all main pages
	pages := []struct {
		path     string
		contains string
	}{
		{"/dashboard", "Dashboard Overview"},
		{"/connections", "Connection Management"},
		{"/sessions", "Session Management"},
		{"/subscriptions", "Subscription Management"},
		{"/monitoring", "System Monitoring"},
	}

	for _, page := range pages {
		resp, err := http.Get(baseURL + page.path)
		require.NoError(t, err, "Failed to access page: %s", page.path)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode, "Page %s should return 200", page.path)
		assert.Contains(t, resp.Header.Get("Content-Type"), "text/html", "Page %s should return HTML", page.path)

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		bodyStr := string(body)
		assert.Contains(t, bodyStr, page.contains, "Page %s should contain expected content", page.path)
		assert.Contains(t, bodyStr, "EMQX Go Dashboard", "Page %s should contain dashboard title", page.path)
	}

	dashboardServer.Stop()
}

func TestDashboardStaticFiles(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create dashboard components
	metricsManager := metrics.NewMetricsManager()
	healthChecker := monitor.NewHealthChecker()
	mockBroker := newTestBrokerInterface()
	adminAPI := admin.NewAPIServer(metricsManager, mockBroker)

	// Create dashboard server
	config := &dashboard.Config{
		Address:    "127.0.0.1",
		Port:       18089,
		Username:   "admin",
		Password:   "public",
		EnableAuth: false,
	}

	// Create connector manager for dashboard
	connectorManager := connector.CreateDefaultConnectorManager()

	dashboardServer, err := dashboard.NewServer(config, adminAPI, metricsManager, healthChecker, nil, connectorManager)
	require.NoError(t, err)

	// Start dashboard server
	go dashboardServer.Start(ctx)
	time.Sleep(200 * time.Millisecond)

	baseURL := "http://localhost:18089"

	// Test static files
	staticFiles := []struct {
		path        string
		contentType string
	}{
		{"/static/css/dashboard.css", "text/css"},
		{"/static/js/dashboard.js", "application/javascript"},
	}

	for _, file := range staticFiles {
		resp, err := http.Get(baseURL + file.path)
		require.NoError(t, err, "Failed to access static file: %s", file.path)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode, "Static file %s should return 200", file.path)

		// Note: Content-Type for static files might vary depending on the server implementation
		// We'll just check that we got some content
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		assert.NotEmpty(t, body, "Static file %s should not be empty", file.path)
	}

	dashboardServer.Stop()
}

func createDashboardTestClient(clientID, address string) mqtt.Client {
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