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

package monitor

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/turtacn/emqx-go/pkg/metrics"
)

func TestNewHealthChecker(t *testing.T) {
	hc := NewHealthChecker()

	assert.NotNil(t, hc)
	assert.True(t, hc.healthy)
	assert.NotEmpty(t, hc.checks)
	assert.Contains(t, hc.checks, "memory")
	assert.Contains(t, hc.checks, "goroutines")
}

func TestHealthCheckerRegisterCheck(t *testing.T) {
	hc := NewHealthChecker()

	checkCalled := false
	hc.RegisterCheck("test", func() error {
		checkCalled = true
		return nil
	}, false)

	assert.Contains(t, hc.checks, "test")
	assert.True(t, hc.checks["test"].Enabled)
	assert.False(t, hc.checks["test"].Critical)

	// Run checks to verify the function is called
	hc.RunChecks()
	assert.True(t, checkCalled)
}

func TestHealthCheckerUnregisterCheck(t *testing.T) {
	hc := NewHealthChecker()

	hc.RegisterCheck("test", func() error { return nil }, false)
	assert.Contains(t, hc.checks, "test")

	hc.UnregisterCheck("test")
	assert.NotContains(t, hc.checks, "test")
}

func TestHealthCheckerEnableDisableCheck(t *testing.T) {
	hc := NewHealthChecker()

	hc.RegisterCheck("test", func() error { return nil }, false)
	assert.True(t, hc.checks["test"].Enabled)

	hc.DisableCheck("test")
	assert.False(t, hc.checks["test"].Enabled)

	hc.EnableCheck("test")
	assert.True(t, hc.checks["test"].Enabled)
}

func TestHealthCheckerRunChecks(t *testing.T) {
	hc := NewHealthChecker()

	// Add a successful check
	hc.RegisterCheck("success", func() error {
		return nil
	}, false)

	// Add a failing non-critical check
	hc.RegisterCheck("fail", func() error {
		return assert.AnError
	}, false)

	// Add a failing critical check
	hc.RegisterCheck("critical_fail", func() error {
		return assert.AnError
	}, true)

	status := hc.RunChecks()

	assert.Equal(t, "unhealthy", status.Status)
	assert.NotZero(t, status.Timestamp)
	assert.Contains(t, status.Checks, "success")
	assert.Contains(t, status.Checks, "fail")
	assert.Contains(t, status.Checks, "critical_fail")

	assert.Equal(t, "passed", status.Checks["success"].Status)
	assert.Equal(t, "failed", status.Checks["fail"].Status)
	assert.Equal(t, "failed", status.Checks["critical_fail"].Status)
	assert.True(t, status.Checks["critical_fail"].Critical)

	assert.False(t, hc.IsHealthy())
}

func TestHealthCheckerRunChecksDisabled(t *testing.T) {
	hc := NewHealthChecker()

	hc.RegisterCheck("disabled", func() error {
		t.Fatal("Disabled check should not be called")
		return nil
	}, false)

	hc.DisableCheck("disabled")

	status := hc.RunChecks()
	assert.NotContains(t, status.Checks, "disabled")
}

func TestHealthCheckerGetStatus(t *testing.T) {
	hc := NewHealthChecker()

	hc.RegisterCheck("test", func() error {
		return nil
	}, false)

	// Run checks first
	hc.RunChecks()

	// Get status without running checks again
	status := hc.GetStatus()

	assert.Equal(t, "healthy", status.Status)
	assert.Contains(t, status.Checks, "test")
	assert.Equal(t, "passed", status.Checks["test"].Status)
}

func TestHealthCheckerIsHealthy(t *testing.T) {
	hc := NewHealthChecker()

	// Initially healthy
	assert.True(t, hc.IsHealthy())

	// Add critical failing check
	hc.RegisterCheck("critical", func() error {
		return assert.AnError
	}, true)

	hc.RunChecks()
	assert.False(t, hc.IsHealthy())
}

func TestHealthServerBasicHealth(t *testing.T) {
	hc := NewHealthChecker()
	metricsManager := metrics.NewMetricsManager()
	server := NewHealthServer(hc, metricsManager)

	req, err := http.NewRequest("GET", "/health", nil)
	require.NoError(t, err)

	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(server.handleHealth)
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "application/json", rr.Header().Get("Content-Type"))

	var response map[string]string
	err = json.Unmarshal(rr.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, "ok", response["status"])
	assert.Contains(t, response, "time")
}

func TestHealthServerBasicHealthUnhealthy(t *testing.T) {
	hc := NewHealthChecker()
	metricsManager := metrics.NewMetricsManager()
	server := NewHealthServer(hc, metricsManager)

	// Add failing critical check
	hc.RegisterCheck("critical", func() error {
		return assert.AnError
	}, true)

	hc.RunChecks() // Make it unhealthy

	req, err := http.NewRequest("GET", "/health", nil)
	require.NoError(t, err)

	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(server.handleHealth)
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusServiceUnavailable, rr.Code)

	var response map[string]string
	err = json.Unmarshal(rr.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, "unhealthy", response["status"])
}

func TestHealthServerLiveness(t *testing.T) {
	hc := NewHealthChecker()
	metricsManager := metrics.NewMetricsManager()
	server := NewHealthServer(hc, metricsManager)

	req, err := http.NewRequest("GET", "/health/live", nil)
	require.NoError(t, err)

	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(server.handleLiveness)
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "OK", rr.Body.String())
}

func TestHealthServerReadiness(t *testing.T) {
	hc := NewHealthChecker()
	metricsManager := metrics.NewMetricsManager()
	server := NewHealthServer(hc, metricsManager)

	req, err := http.NewRequest("GET", "/health/ready", nil)
	require.NoError(t, err)

	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(server.handleReadiness)
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "OK", rr.Body.String())
}

func TestHealthServerReadinessUnhealthy(t *testing.T) {
	hc := NewHealthChecker()
	metricsManager := metrics.NewMetricsManager()
	server := NewHealthServer(hc, metricsManager)

	// Add failing critical check
	hc.RegisterCheck("critical", func() error {
		return assert.AnError
	}, true)

	hc.RunChecks() // Make it unhealthy

	req, err := http.NewRequest("GET", "/health/ready", nil)
	require.NoError(t, err)

	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(server.handleReadiness)
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusServiceUnavailable, rr.Code)
	assert.Equal(t, "Service Unavailable", rr.Body.String())
}

func TestHealthServerDetailedHealth(t *testing.T) {
	hc := NewHealthChecker()
	metricsManager := metrics.NewMetricsManager()
	server := NewHealthServer(hc, metricsManager)

	hc.RegisterCheck("test", func() error {
		return nil
	}, false)

	req, err := http.NewRequest("GET", "/health/detailed", nil)
	require.NoError(t, err)

	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(server.handleDetailedHealth)
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)

	var status HealthStatus
	err = json.Unmarshal(rr.Body.Bytes(), &status)
	require.NoError(t, err)

	assert.Equal(t, "healthy", status.Status)
	assert.Contains(t, status.Checks, "test")
	assert.NotNil(t, status.SystemInfo)
	assert.NotEmpty(t, status.Version)
}

func TestHealthServerAPIHealth(t *testing.T) {
	hc := NewHealthChecker()
	metricsManager := metrics.NewMetricsManager()
	server := NewHealthServer(hc, metricsManager)

	req, err := http.NewRequest("GET", "/api/v5/health", nil)
	require.NoError(t, err)

	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(server.handleAPIHealth)
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)

	var response map[string]interface{}
	err = json.Unmarshal(rr.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, float64(0), response["code"])
	assert.Contains(t, response, "data")

	data, ok := response["data"].(map[string]interface{})
	require.True(t, ok)
	assert.Contains(t, data, "health")
	assert.Contains(t, data, "statistics")
}

func TestHealthServerMethodNotAllowed(t *testing.T) {
	hc := NewHealthChecker()
	metricsManager := metrics.NewMetricsManager()
	server := NewHealthServer(hc, metricsManager)

	req, err := http.NewRequest("POST", "/health", nil)
	require.NoError(t, err)

	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(server.handleHealth)
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
}

func TestSetVersionAndNodeID(t *testing.T) {
	originalVersion := getVersion()
	originalNodeID := getNodeID()

	defer func() {
		SetVersion(originalVersion)
		SetNodeID(originalNodeID)
	}()

	SetVersion("2.0.0")
	SetNodeID("test-node")

	assert.Equal(t, "2.0.0", getVersion())
	assert.Equal(t, "test-node", getNodeID())
}

func TestHealthStatusSystemInfo(t *testing.T) {
	hc := NewHealthChecker()

	status := hc.RunChecks()

	assert.NotNil(t, status.SystemInfo)
	assert.Greater(t, status.SystemInfo.Memory.Alloc, uint64(0))
	assert.Greater(t, status.SystemInfo.Goroutines, 0)
	assert.NotEmpty(t, status.SystemInfo.OSInfo.OS)
	assert.NotEmpty(t, status.SystemInfo.OSInfo.Arch)
	assert.Greater(t, status.SystemInfo.OSInfo.NumCPU, 0)
}

func TestHealthCheckerSlowCheck(t *testing.T) {
	hc := NewHealthChecker()

	hc.RegisterCheck("slow", func() error {
		time.Sleep(10 * time.Millisecond) // Simulate slow check
		return nil
	}, false)

	start := time.Now()
	status := hc.RunChecks()
	duration := time.Since(start)

	assert.Greater(t, duration, 10*time.Millisecond)
	assert.Equal(t, "healthy", status.Status)
	assert.Equal(t, "passed", status.Checks["slow"].Status)
}

func TestHealthCheckerConcurrentAccess(t *testing.T) {
	hc := NewHealthChecker()

	hc.RegisterCheck("concurrent", func() error {
		return nil
	}, false)

	// Run checks concurrently
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func() {
			hc.RunChecks()
			hc.IsHealthy()
			hc.GetStatus()
			done <- true
		}()
	}

	// Wait for all goroutines to complete
	for i := 0; i < 10; i++ {
		<-done
	}

	// Should not panic and should be healthy
	assert.True(t, hc.IsHealthy())
}