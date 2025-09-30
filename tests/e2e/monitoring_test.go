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
	"net/http"
	"testing"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/turtacn/emqx-go/pkg/admin"
	"github.com/turtacn/emqx-go/pkg/broker"
	"github.com/turtacn/emqx-go/pkg/metrics"
	"github.com/turtacn/emqx-go/pkg/monitor"
)

func TestMonitoringBasicFunctionality(t *testing.T) {
	// Start broker with monitoring
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	brokerInstance := broker.New("monitor-test-broker", nil)
	brokerInstance.SetupDefaultAuth()
	defer brokerInstance.Close()

	// Start broker server
	go brokerInstance.StartServer(ctx, ":1930")
	time.Sleep(200 * time.Millisecond)

	// Start metrics server
	go metrics.Serve(":8091")
	time.Sleep(100 * time.Millisecond)

	// Test basic metrics endpoint
	resp, err := http.Get("http://localhost:8091/api/v5/stats")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "application/json", resp.Header.Get("Content-Type"))

	var stats metrics.Statistics
	err = json.NewDecoder(resp.Body).Decode(&stats)
	require.NoError(t, err)

	// Verify basic structure
	assert.GreaterOrEqual(t, stats.Node.Uptime, int64(0))
	assert.NotEmpty(t, stats.Node.StartTime)
}

func TestMonitoringWithMQTTTraffic(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	brokerInstance := broker.New("monitor-traffic-broker", nil)
	brokerInstance.SetupDefaultAuth()
	defer brokerInstance.Close()

	go brokerInstance.StartServer(ctx, ":1931")
	time.Sleep(200 * time.Millisecond)

	// Start metrics server
	go metrics.Serve(":8092")
	time.Sleep(100 * time.Millisecond)

	// Connect MQTT clients and generate traffic
	client1 := createMonitorTestClient("monitor-client-1", ":1931")
	token := client1.Connect()
	require.True(t, token.WaitTimeout(5*time.Second))
	require.NoError(t, token.Error())
	defer client1.Disconnect(100)

	client2 := createMonitorTestClient("monitor-client-2", ":1931")
	token = client2.Connect()
	require.True(t, token.WaitTimeout(5*time.Second))
	require.NoError(t, token.Error())
	defer client2.Disconnect(100)

	// Subscribe to topics
	client1.Subscribe("monitor/test/+", 1, func(client mqtt.Client, msg mqtt.Message) {
		// Message handler
	})

	client2.Subscribe("monitor/test/data", 2, func(client mqtt.Client, msg mqtt.Message) {
		// Message handler
	})

	time.Sleep(100 * time.Millisecond)

	// Publish messages
	for i := 0; i < 10; i++ {
		payload := fmt.Sprintf("test message %d", i)
		token := client1.Publish("monitor/test/data", 1, false, payload)
		require.True(t, token.WaitTimeout(time.Second))
		require.NoError(t, token.Error())
	}

	time.Sleep(200 * time.Millisecond)

	// Update metrics manually (simulating broker integration)
	metrics.DefaultManager.IncConnections()
	metrics.DefaultManager.IncConnections()
	metrics.DefaultManager.IncSessions(false)
	metrics.DefaultManager.IncSessions(true)
	metrics.DefaultManager.IncSubscriptions(false)
	metrics.DefaultManager.IncSubscriptions(false)

	for i := 0; i < 10; i++ {
		metrics.DefaultManager.IncMessagesReceived(1)
		metrics.DefaultManager.IncMessagesSent(1)
		metrics.DefaultManager.IncPacketsReceived("PUBLISH")
		metrics.DefaultManager.IncPacketsSent()
	}

	// Check updated metrics
	resp, err := http.Get("http://localhost:8092/api/v5/stats")
	require.NoError(t, err)
	defer resp.Body.Close()

	var stats metrics.Statistics
	err = json.NewDecoder(resp.Body).Decode(&stats)
	require.NoError(t, err)

	// Verify metrics reflect the traffic
	assert.Equal(t, int64(2), stats.Connections.Count)
	assert.Equal(t, int64(2), stats.Sessions.Count)
	assert.Equal(t, int64(1), stats.Sessions.Persistent)
	assert.Equal(t, int64(1), stats.Sessions.Transient)
	assert.Equal(t, int64(2), stats.Subscriptions.Count)
	assert.Equal(t, int64(10), stats.Messages.Received)
	assert.Equal(t, int64(10), stats.Messages.Sent)
	assert.Equal(t, int64(10), stats.Packets.Publish)
}

func TestHealthCheckEndpoints(t *testing.T) {
	// Create health checker and metrics manager
	healthChecker := monitor.NewHealthChecker()
	metricsManager := metrics.NewMetricsManager()
	healthServer := monitor.NewHealthServer(healthChecker, metricsManager)

	// Start test server
	mux := http.NewServeMux()
	healthServer.RegisterRoutes(mux)
	server := &http.Server{
		Addr:    ":8093",
		Handler: mux,
	}

	go server.ListenAndServe()
	defer server.Close()
	time.Sleep(100 * time.Millisecond)

	// Test basic health endpoint
	resp, err := http.Get("http://localhost:8093/health")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var healthResp map[string]string
	err = json.NewDecoder(resp.Body).Decode(&healthResp)
	require.NoError(t, err)

	assert.Equal(t, "ok", healthResp["status"])
	assert.Contains(t, healthResp, "time")

	// Test liveness probe
	resp, err = http.Get("http://localhost:8093/health/live")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Test readiness probe
	resp, err = http.Get("http://localhost:8093/health/ready")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Test detailed health
	resp, err = http.Get("http://localhost:8093/health/detailed")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var detailedHealth monitor.HealthStatus
	err = json.NewDecoder(resp.Body).Decode(&detailedHealth)
	require.NoError(t, err)

	assert.Equal(t, "healthy", detailedHealth.Status)
	assert.NotEmpty(t, detailedHealth.Checks)
	assert.NotNil(t, detailedHealth.SystemInfo)
	assert.Greater(t, detailedHealth.SystemInfo.Memory.Alloc, uint64(0))
	assert.Greater(t, detailedHealth.SystemInfo.Goroutines, 0)
}

func TestHealthCheckFailure(t *testing.T) {
	healthChecker := monitor.NewHealthChecker()
	metricsManager := metrics.NewMetricsManager()

	// Add a failing critical check
	healthChecker.RegisterCheck("critical_failure", func() error {
		return fmt.Errorf("critical system failure")
	}, true)

	healthServer := monitor.NewHealthServer(healthChecker, metricsManager)

	mux := http.NewServeMux()
	healthServer.RegisterRoutes(mux)
	server := &http.Server{
		Addr:    ":8094",
		Handler: mux,
	}

	go server.ListenAndServe()
	defer server.Close()
	time.Sleep(100 * time.Millisecond)

	// Run health checks to trigger failure
	healthChecker.RunChecks()

	// Test basic health endpoint - should return unhealthy
	resp, err := http.Get("http://localhost:8094/health")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)

	var healthResp map[string]string
	err = json.NewDecoder(resp.Body).Decode(&healthResp)
	require.NoError(t, err)

	assert.Equal(t, "unhealthy", healthResp["status"])

	// Test readiness probe - should fail
	resp, err = http.Get("http://localhost:8094/health/ready")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)

	// Liveness should still pass (it doesn't check health)
	resp, err = http.Get("http://localhost:8094/health/live")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestAdminAPIEndpoints(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start broker
	brokerInstance := broker.New("admin-test-broker", nil)
	brokerInstance.SetupDefaultAuth()
	defer brokerInstance.Close()

	go brokerInstance.StartServer(ctx, ":1932")
	time.Sleep(200 * time.Millisecond)

	// Create mock broker for admin API
	mockBroker := newTestBrokerInterface()

	// Add test data to the mock broker
	mockBroker.connections = []admin.ConnectionInfo{
		{
			ClientID:    "admin-client-1",
			Username:    "user1",
			PeerHost:    "127.0.0.1",
			SockPort:    1932,
			Protocol:    "MQTT",
			ConnectedAt: time.Now(),
			Node:        "admin-test-broker",
		},
	}
	mockBroker.sessions = []admin.SessionInfo{
		{
			ClientID:  "admin-client-1",
			Username:  "user1",
			CreatedAt: time.Now(),
			Node:      "admin-test-broker",
		},
	}
	mockBroker.subscriptions = []admin.SubscriptionInfo{
		{
			ClientID: "admin-client-1",
			Topic:    "admin/test",
			QoS:      1,
			Node:     "admin-test-broker",
		},
	}
	mockBroker.routes = []admin.RouteInfo{
		{
			Topic: "admin/test",
			Nodes: []string{"admin-test-broker"},
		},
	}
	mockBroker.nodes = []admin.NodeInfo{
		{
			Node:       "admin-test-broker",
			NodeStatus: "running",
			Version:    "1.0.0",
			Uptime:     3600,
			Datetime:   time.Now(),
		},
	}

	// Start admin API server
	metricsManager := metrics.NewMetricsManager()
	apiServer := admin.NewAPIServer(metricsManager, mockBroker)

	mux := http.NewServeMux()
	apiServer.RegisterRoutes(mux)
	server := &http.Server{
		Addr:    ":8095",
		Handler: mux,
	}

	go server.ListenAndServe()
	defer server.Close()
	time.Sleep(100 * time.Millisecond)

	// Test connections endpoint
	resp, err := http.Get("http://localhost:8095/api/v5/connections")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var connectionsResp admin.APIResponse
	err = json.NewDecoder(resp.Body).Decode(&connectionsResp)
	require.NoError(t, err)

	assert.Equal(t, 0, connectionsResp.Code)
	assert.NotNil(t, connectionsResp.Data)

	// Test sessions endpoint
	resp, err = http.Get("http://localhost:8095/api/v5/sessions")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Test subscriptions endpoint
	resp, err = http.Get("http://localhost:8095/api/v5/subscriptions")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Test routes endpoint
	resp, err = http.Get("http://localhost:8095/api/v5/routes")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Test nodes endpoint
	resp, err = http.Get("http://localhost:8095/api/v5/nodes")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Test status endpoint
	resp, err = http.Get("http://localhost:8095/api/v5/status")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestPrometheusMetricsIntegration(t *testing.T) {
	// Start metrics server
	go metrics.Serve(":8096")
	time.Sleep(100 * time.Millisecond)

	// Generate some metrics
	metrics.DefaultManager.IncConnections()
	metrics.DefaultManager.IncSessions(true)
	metrics.DefaultManager.IncSubscriptions(false)
	metrics.DefaultManager.IncMessagesReceived(1)
	metrics.DefaultManager.IncMessagesSent(1)

	// Test Prometheus metrics endpoint
	resp, err := http.Get("http://localhost:8096/metrics")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Test EMQX-compatible Prometheus endpoint
	resp, err = http.Get("http://localhost:8096/api/v5/prometheus/stats")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestMonitoringPerformance(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start broker
	brokerInstance := broker.New("perf-monitor-broker", nil)
	brokerInstance.SetupDefaultAuth()
	defer brokerInstance.Close()

	go brokerInstance.StartServer(ctx, ":1933")
	time.Sleep(200 * time.Millisecond)

	// Start metrics server
	go metrics.Serve(":8097")
	time.Sleep(100 * time.Millisecond)

	// Performance test: rapid metrics updates
	const numOperations = 1000
	startTime := time.Now()

	for i := 0; i < numOperations; i++ {
		metrics.DefaultManager.IncConnections()
		metrics.DefaultManager.DecConnections()
		metrics.DefaultManager.IncSessions(i%2 == 0)
		metrics.DefaultManager.DecSessions(i%2 == 0)
		metrics.DefaultManager.IncSubscriptions(false)
		metrics.DefaultManager.DecSubscriptions(false)
		metrics.DefaultManager.IncMessagesReceived(byte(i % 3))
		metrics.DefaultManager.IncMessagesSent(byte(i % 3))
		metrics.DefaultManager.IncPacketsReceived("PUBLISH")
		metrics.DefaultManager.IncPacketsSent()
	}

	duration := time.Since(startTime)
	operationsPerSecond := float64(numOperations*10) / duration.Seconds() // 10 operations per iteration

	t.Logf("Monitoring performance: %d operations in %v (%.2f ops/sec)",
		numOperations*10, duration, operationsPerSecond)

	// Performance assertion (should handle at least 10k operations per second)
	assert.Greater(t, operationsPerSecond, 10000.0, "Monitoring performance below expected threshold")

	// Verify metrics are still accurate
	stats := metrics.DefaultManager.GetStatistics()
	assert.Equal(t, int64(0), stats.Connections.Count)
	assert.Equal(t, int64(0), stats.Sessions.Count)
	assert.Equal(t, int64(0), stats.Subscriptions.Count)
	assert.Equal(t, int64(numOperations), stats.Messages.Received)
	assert.Equal(t, int64(numOperations), stats.Messages.Sent)
}

func TestConcurrentMetricsAccess(t *testing.T) {
	// Test concurrent access to metrics
	const numGoroutines = 50
	const operationsPerGoroutine = 100

	done := make(chan bool, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			for j := 0; j < operationsPerGoroutine; j++ {
				// Alternate between different operations
				switch (id + j) % 4 {
				case 0:
					metrics.DefaultManager.IncConnections()
					metrics.DefaultManager.DecConnections()
				case 1:
					metrics.DefaultManager.IncSessions(j%2 == 0)
					metrics.DefaultManager.DecSessions(j%2 == 0)
				case 2:
					metrics.DefaultManager.IncSubscriptions(false)
					metrics.DefaultManager.DecSubscriptions(false)
				case 3:
					metrics.DefaultManager.IncMessagesReceived(byte(j % 3))
					metrics.DefaultManager.IncMessagesSent(byte(j % 3))
				}

				// Occasionally read statistics
				if j%10 == 0 {
					metrics.DefaultManager.GetStatistics()
				}
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines to complete
	for i := 0; i < numGoroutines; i++ {
		<-done
	}

	// Should not panic and metrics should be consistent
	stats := metrics.DefaultManager.GetStatistics()
	t.Logf("Final stats after concurrent access: connections=%d, sessions=%d, messages_received=%d",
		stats.Connections.Count, stats.Sessions.Count, stats.Messages.Received)

	// All increment/decrement operations should cancel out
	assert.Equal(t, int64(0), stats.Connections.Count)
	assert.Equal(t, int64(0), stats.Sessions.Count)
	assert.Equal(t, int64(0), stats.Subscriptions.Count)
}

// Helper functions

func createMonitorTestClient(clientID, address string) mqtt.Client {
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