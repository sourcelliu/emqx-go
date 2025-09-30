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

package dashboard

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/turtacn/emqx-go/pkg/admin"
	"github.com/turtacn/emqx-go/pkg/metrics"
	"github.com/turtacn/emqx-go/pkg/monitor"
)

// mockBrokerInterface for testing
type mockBrokerInterface struct{}

func (m *mockBrokerInterface) GetConnections() []admin.ConnectionInfo        { return []admin.ConnectionInfo{} }
func (m *mockBrokerInterface) GetSessions() []admin.SessionInfo              { return []admin.SessionInfo{} }
func (m *mockBrokerInterface) GetSubscriptions() []admin.SubscriptionInfo    { return []admin.SubscriptionInfo{} }
func (m *mockBrokerInterface) GetRoutes() []admin.RouteInfo                  { return []admin.RouteInfo{} }
func (m *mockBrokerInterface) DisconnectClient(clientID string) error        { return nil }
func (m *mockBrokerInterface) KickoutSession(clientID string) error          { return nil }
func (m *mockBrokerInterface) GetNodeInfo() admin.NodeInfo                   { return admin.NodeInfo{} }
func (m *mockBrokerInterface) GetClusterNodes() []admin.NodeInfo             { return []admin.NodeInfo{} }

func createTestServer(t *testing.T) *Server {
	config := &Config{
		Address:    "127.0.0.1",
		Port:       18083,
		Username:   "admin",
		Password:   "public",
		EnableAuth: false, // Disable auth for easier testing
	}

	metricsManager := metrics.NewMetricsManager()
	healthChecker := monitor.NewHealthChecker()
	mockBroker := &mockBrokerInterface{}
	adminAPI := admin.NewAPIServer(metricsManager, mockBroker)

	server, err := NewServer(config, adminAPI, metricsManager, healthChecker)
	require.NoError(t, err)
	require.NotNil(t, server)

	return server
}

func TestNewServer(t *testing.T) {
	server := createTestServer(t)
	assert.NotNil(t, server.httpServer)
	assert.NotNil(t, server.mux)
	assert.NotNil(t, server.templates)
	assert.Equal(t, "127.0.0.1:18083", server.GetListenAddress())
}

func TestServerDefaultConfig(t *testing.T) {
	config := DefaultConfig()
	assert.Equal(t, "0.0.0.0", config.Address)
	assert.Equal(t, 18083, config.Port)
	assert.Equal(t, "admin", config.Username)
	assert.Equal(t, "public", config.Password)
	assert.True(t, config.EnableAuth)
}

func TestServerValidateCredentials(t *testing.T) {
	server := createTestServer(t)

	// Test valid credentials
	assert.True(t, server.validateCredentials("admin", "public"))

	// Test invalid credentials
	assert.False(t, server.validateCredentials("admin", "wrong"))
	assert.False(t, server.validateCredentials("wrong", "public"))
	assert.False(t, server.validateCredentials("", ""))
}

func TestLoginPageHandler(t *testing.T) {
	server := createTestServer(t)

	// Test GET request
	req, err := http.NewRequest("GET", "/login", nil)
	require.NoError(t, err)

	rr := httptest.NewRecorder()
	server.handleLogin(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Contains(t, rr.Body.String(), "EMQX Go Dashboard")
	assert.Contains(t, rr.Header().Get("Content-Type"), "text/html")
}

func TestLoginPostHandler(t *testing.T) {
	server := createTestServer(t)

	// Test valid login
	form := url.Values{}
	form.Add("username", "admin")
	form.Add("password", "public")

	req, err := http.NewRequest("POST", "/login", strings.NewReader(form.Encode()))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	rr := httptest.NewRecorder()
	server.handleLogin(rr, req)

	assert.Equal(t, http.StatusFound, rr.Code)
	assert.Equal(t, "/dashboard", rr.Header().Get("Location"))

	// Check for session cookie
	cookies := rr.Result().Cookies()
	found := false
	for _, cookie := range cookies {
		if cookie.Name == "dashboard_session" {
			found = true
			break
		}
	}
	assert.True(t, found, "Session cookie should be set")
}

func TestLoginPostHandlerInvalidCredentials(t *testing.T) {
	server := createTestServer(t)

	// Test invalid login
	form := url.Values{}
	form.Add("username", "admin")
	form.Add("password", "wrong")

	req, err := http.NewRequest("POST", "/login", strings.NewReader(form.Encode()))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	rr := httptest.NewRecorder()
	server.handleLogin(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Contains(t, rr.Body.String(), "Invalid username or password")
}

func TestDashboardHandler(t *testing.T) {
	server := createTestServer(t)

	req, err := http.NewRequest("GET", "/dashboard", nil)
	require.NoError(t, err)

	rr := httptest.NewRecorder()
	server.handleDashboard(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Contains(t, rr.Header().Get("Content-Type"), "text/html")
}

func TestConnectionsHandler(t *testing.T) {
	server := createTestServer(t)

	req, err := http.NewRequest("GET", "/connections", nil)
	require.NoError(t, err)

	rr := httptest.NewRecorder()
	server.handleConnections(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Contains(t, rr.Header().Get("Content-Type"), "text/html")
}

func TestAPIStatsHandler(t *testing.T) {
	server := createTestServer(t)

	req, err := http.NewRequest("GET", "/api/dashboard/stats", nil)
	require.NoError(t, err)

	rr := httptest.NewRecorder()
	server.handleAPIStats(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "application/json", rr.Header().Get("Content-Type"))

	var stats metrics.Statistics
	err = json.Unmarshal(rr.Body.Bytes(), &stats)
	require.NoError(t, err)

	// Verify stats structure
	assert.GreaterOrEqual(t, stats.Connections.Count, int64(0))
	assert.GreaterOrEqual(t, stats.Sessions.Count, int64(0))
	assert.GreaterOrEqual(t, stats.Subscriptions.Count, int64(0))
}

func TestAPIConnectionsHandler(t *testing.T) {
	server := createTestServer(t)

	req, err := http.NewRequest("GET", "/api/dashboard/connections", nil)
	require.NoError(t, err)

	rr := httptest.NewRecorder()
	server.handleAPIConnections(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "application/json", rr.Header().Get("Content-Type"))

	var response map[string]interface{}
	err = json.Unmarshal(rr.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Contains(t, response, "connections")
	assert.Contains(t, response, "count")

	connections, ok := response["connections"].([]interface{})
	require.True(t, ok)
	assert.GreaterOrEqual(t, len(connections), 0)
}

func TestAPISessionsHandler(t *testing.T) {
	server := createTestServer(t)

	req, err := http.NewRequest("GET", "/api/dashboard/sessions", nil)
	require.NoError(t, err)

	rr := httptest.NewRecorder()
	server.handleAPISessions(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "application/json", rr.Header().Get("Content-Type"))

	var response map[string]interface{}
	err = json.Unmarshal(rr.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Contains(t, response, "sessions")
	assert.Contains(t, response, "count")
}

func TestAPISubscriptionsHandler(t *testing.T) {
	server := createTestServer(t)

	req, err := http.NewRequest("GET", "/api/dashboard/subscriptions", nil)
	require.NoError(t, err)

	rr := httptest.NewRecorder()
	server.handleAPISubscriptions(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "application/json", rr.Header().Get("Content-Type"))

	var response map[string]interface{}
	err = json.Unmarshal(rr.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Contains(t, response, "subscriptions")
	assert.Contains(t, response, "count")
}

func TestAPIHealthHandler(t *testing.T) {
	server := createTestServer(t)

	req, err := http.NewRequest("GET", "/api/dashboard/health", nil)
	require.NoError(t, err)

	rr := httptest.NewRecorder()
	server.handleAPIHealth(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "application/json", rr.Header().Get("Content-Type"))

	var health monitor.HealthStatus
	err = json.Unmarshal(rr.Body.Bytes(), &health)
	require.NoError(t, err)

	assert.NotEmpty(t, health.Status)
	assert.NotNil(t, health.SystemInfo)
}

func TestMethodNotAllowed(t *testing.T) {
	server := createTestServer(t)

	// Test POST to dashboard endpoint
	req, err := http.NewRequest("POST", "/dashboard", nil)
	require.NoError(t, err)

	rr := httptest.NewRecorder()
	server.handleDashboard(rr, req)

	assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)

	// Test POST to API stats endpoint
	req, err = http.NewRequest("POST", "/api/dashboard/stats", nil)
	require.NoError(t, err)

	rr = httptest.NewRecorder()
	server.handleAPIStats(rr, req)

	assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
}

func TestLogoutHandler(t *testing.T) {
	server := createTestServer(t)

	req, err := http.NewRequest("GET", "/logout", nil)
	require.NoError(t, err)

	rr := httptest.NewRecorder()
	server.handleLogout(rr, req)

	assert.Equal(t, http.StatusFound, rr.Code)
	assert.Equal(t, "/login", rr.Header().Get("Location"))

	// Check that session cookie is cleared
	cookies := rr.Result().Cookies()
	found := false
	for _, cookie := range cookies {
		if cookie.Name == "dashboard_session" && cookie.Value == "" {
			found = true
			break
		}
	}
	assert.True(t, found, "Session cookie should be cleared")
}

func TestAuthMiddleware(t *testing.T) {
	// Create server with auth enabled
	config := &Config{
		Address:    "127.0.0.1",
		Port:       18083,
		Username:   "admin",
		Password:   "public",
		EnableAuth: true,
	}

	metricsManager := metrics.NewMetricsManager()
	healthChecker := monitor.NewHealthChecker()
	mockBroker := &mockBrokerInterface{}
	adminAPI := admin.NewAPIServer(metricsManager, mockBroker)

	server, err := NewServer(config, adminAPI, metricsManager, healthChecker)
	require.NoError(t, err)

	// Create a test handler
	testHandler := server.authMiddleware(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("success"))
	})

	// Test without authentication - should redirect to login
	req, err := http.NewRequest("GET", "/dashboard", nil)
	require.NoError(t, err)

	rr := httptest.NewRecorder()
	testHandler(rr, req)

	assert.Equal(t, http.StatusFound, rr.Code)
	assert.Equal(t, "/login", rr.Header().Get("Location"))

	// Test with valid session cookie
	req, err = http.NewRequest("GET", "/dashboard", nil)
	require.NoError(t, err)
	req.AddCookie(&http.Cookie{
		Name:  "dashboard_session",
		Value: "valid_session_token",
	})

	rr = httptest.NewRecorder()
	testHandler(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "success", rr.Body.String())
}

func TestAuthMiddlewareAPIEndpoint(t *testing.T) {
	// Create server with auth enabled
	config := &Config{
		Address:    "127.0.0.1",
		Port:       18083,
		Username:   "admin",
		Password:   "public",
		EnableAuth: true,
	}

	metricsManager := metrics.NewMetricsManager()
	healthChecker := monitor.NewHealthChecker()
	mockBroker := &mockBrokerInterface{}
	adminAPI := admin.NewAPIServer(metricsManager, mockBroker)

	server, err := NewServer(config, adminAPI, metricsManager, healthChecker)
	require.NoError(t, err)

	// Create a test handler
	testHandler := server.authMiddleware(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("success"))
	})

	// Test API endpoint without auth - should return 401
	req, err := http.NewRequest("GET", "/api/dashboard/stats", nil)
	require.NoError(t, err)

	rr := httptest.NewRecorder()
	testHandler(rr, req)

	assert.Equal(t, http.StatusUnauthorized, rr.Code)
}

func TestGetDashboardData(t *testing.T) {
	server := createTestServer(t)

	data := server.getDashboardData()

	assert.Contains(t, data, "Title")
	assert.Contains(t, data, "Stats")
	assert.Contains(t, data, "Health")
	assert.Contains(t, data, "CurrentTime")

	assert.Equal(t, "Dashboard - EMQX Go", data["Title"])
}

func TestUpdateConfig(t *testing.T) {
	server := createTestServer(t)

	newConfig := &Config{
		Address:    "0.0.0.0",
		Port:       8080,
		Username:   "newuser",
		Password:   "newpass",
		EnableAuth: false,
	}

	server.UpdateConfig(newConfig)

	assert.Equal(t, newConfig, server.config)
	assert.Equal(t, "newuser", server.config.Username)
	assert.Equal(t, "newpass", server.config.Password)
	assert.Equal(t, 8080, server.config.Port)
}