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

// Package monitor provides health checking and system monitoring capabilities
// for the EMQX-go broker, including memory usage, CPU metrics, and service health.
package monitor

import (
	"encoding/json"
	"fmt"
	"net/http"
	"runtime"
	"sync"
	"time"

	"github.com/turtacn/emqx-go/pkg/metrics"
)

// HealthChecker provides health checking functionality
type HealthChecker struct {
	mu sync.RWMutex

	// Health status
	healthy   bool
	lastCheck time.Time
	errors    []string

	// System metrics
	memStats       runtime.MemStats
	goroutineCount int
	cpuUsage       float64

	// Service checks
	checks map[string]HealthCheck
}

// HealthCheck represents a health check function
type HealthCheck struct {
	Name        string
	CheckFunc   func() error
	Critical    bool
	LastChecked time.Time
	LastError   error
	Enabled     bool
}

// HealthStatus represents the overall health status
type HealthStatus struct {
	Status      string                 `json:"status"`
	Timestamp   time.Time              `json:"timestamp"`
	Uptime      int64                  `json:"uptime"`
	Version     string                 `json:"version"`
	Node        string                 `json:"node"`
	Checks      map[string]CheckResult `json:"checks"`
	SystemInfo  SystemInfo             `json:"system_info"`
}

// CheckResult represents the result of a health check
type CheckResult struct {
	Status      string    `json:"status"`
	LastChecked time.Time `json:"last_checked"`
	Message     string    `json:"message,omitempty"`
	Critical    bool      `json:"critical"`
}

// SystemInfo contains system-level information
type SystemInfo struct {
	Memory       MemoryInfo `json:"memory"`
	Goroutines   int        `json:"goroutines"`
	CPUUsage     float64    `json:"cpu_usage"`
	OSInfo       OSInfo     `json:"os_info"`
}

// MemoryInfo contains memory usage information
type MemoryInfo struct {
	Alloc      uint64  `json:"alloc"`
	TotalAlloc uint64  `json:"total_alloc"`
	Sys        uint64  `json:"sys"`
	NumGC      uint32  `json:"num_gc"`
	GCPause    float64 `json:"gc_pause_ms"`
}

// OSInfo contains operating system information
type OSInfo struct {
	OS       string `json:"os"`
	Arch     string `json:"arch"`
	Version  string `json:"version"`
	NumCPU   int    `json:"num_cpu"`
	Compiler string `json:"compiler"`
}

// NewHealthChecker creates a new health checker instance
func NewHealthChecker() *HealthChecker {
	hc := &HealthChecker{
		healthy:   true,
		lastCheck: time.Now(),
		checks:    make(map[string]HealthCheck),
	}

	// Register default system checks
	hc.RegisterCheck("memory", func() error {
		var m runtime.MemStats
		runtime.GC()
		runtime.ReadMemStats(&m)

		// Check if memory usage is too high (>90% of system memory)
		if m.Sys > 1024*1024*1024*8 { // 8GB threshold
			return fmt.Errorf("high memory usage: %d bytes", m.Sys)
		}
		return nil
	}, false)

	hc.RegisterCheck("goroutines", func() error {
		count := runtime.NumGoroutine()
		if count > 10000 { // Too many goroutines
			return fmt.Errorf("high goroutine count: %d", count)
		}
		return nil
	}, false)

	return hc
}

// RegisterCheck registers a new health check
func (hc *HealthChecker) RegisterCheck(name string, checkFunc func() error, critical bool) {
	hc.mu.Lock()
	defer hc.mu.Unlock()

	hc.checks[name] = HealthCheck{
		Name:      name,
		CheckFunc: checkFunc,
		Critical:  critical,
		Enabled:   true,
	}
}

// UnregisterCheck removes a health check
func (hc *HealthChecker) UnregisterCheck(name string) {
	hc.mu.Lock()
	defer hc.mu.Unlock()

	delete(hc.checks, name)
}

// EnableCheck enables a health check
func (hc *HealthChecker) EnableCheck(name string) {
	hc.mu.Lock()
	defer hc.mu.Unlock()

	if check, exists := hc.checks[name]; exists {
		check.Enabled = true
		hc.checks[name] = check
	}
}

// DisableCheck disables a health check
func (hc *HealthChecker) DisableCheck(name string) {
	hc.mu.Lock()
	defer hc.mu.Unlock()

	if check, exists := hc.checks[name]; exists {
		check.Enabled = false
		hc.checks[name] = check
	}
}

// RunChecks executes all registered health checks
func (hc *HealthChecker) RunChecks() HealthStatus {
	hc.mu.Lock()
	defer hc.mu.Unlock()

	now := time.Now()
	hc.lastCheck = now

	// Update system metrics
	hc.updateSystemMetrics()

	// Run all enabled checks
	checkResults := make(map[string]CheckResult)
	overallHealthy := true
	var criticalErrors []string

	for name, check := range hc.checks {
		if !check.Enabled {
			continue
		}

		var err error
		var status string
		var message string

		// Run the check
		startTime := time.Now()
		err = check.CheckFunc()
		duration := time.Since(startTime)

		if err != nil {
			status = "failed"
			message = err.Error()
			check.LastError = err

			if check.Critical {
				criticalErrors = append(criticalErrors, fmt.Sprintf("%s: %s", name, err.Error()))
				overallHealthy = false
			}
		} else {
			status = "passed"
			check.LastError = nil
		}

		check.LastChecked = now
		hc.checks[name] = check

		checkResults[name] = CheckResult{
			Status:      status,
			LastChecked: now,
			Message:     message,
			Critical:    check.Critical,
		}

		// Log slow checks
		if duration > time.Second {
			fmt.Printf("Warning: Health check '%s' took %v\n", name, duration)
		}
	}

	hc.healthy = overallHealthy
	if len(criticalErrors) > 0 {
		hc.errors = criticalErrors
	} else {
		hc.errors = nil
	}

	return HealthStatus{
		Status:     hc.getOverallStatus(),
		Timestamp:  now,
		Uptime:     int64(time.Since(startTime).Seconds()),
		Version:    getVersion(),
		Node:       getNodeID(),
		Checks:     checkResults,
		SystemInfo: hc.getSystemInfo(),
	}
}

// GetStatus returns the current health status without running checks
func (hc *HealthChecker) GetStatus() HealthStatus {
	hc.mu.RLock()
	defer hc.mu.RUnlock()

	checkResults := make(map[string]CheckResult)
	for name, check := range hc.checks {
		if !check.Enabled {
			continue
		}

		status := "unknown"
		message := ""
		if !check.LastChecked.IsZero() {
			if check.LastError != nil {
				status = "failed"
				message = check.LastError.Error()
			} else {
				status = "passed"
			}
		}

		checkResults[name] = CheckResult{
			Status:      status,
			LastChecked: check.LastChecked,
			Message:     message,
			Critical:    check.Critical,
		}
	}

	return HealthStatus{
		Status:     hc.getOverallStatus(),
		Timestamp:  hc.lastCheck,
		Uptime:     int64(time.Since(startTime).Seconds()),
		Version:    getVersion(),
		Node:       getNodeID(),
		Checks:     checkResults,
		SystemInfo: hc.getSystemInfo(),
	}
}

// IsHealthy returns true if the system is healthy
func (hc *HealthChecker) IsHealthy() bool {
	hc.mu.RLock()
	defer hc.mu.RUnlock()
	return hc.healthy
}

// updateSystemMetrics updates system-level metrics
func (hc *HealthChecker) updateSystemMetrics() {
	runtime.ReadMemStats(&hc.memStats)
	hc.goroutineCount = runtime.NumGoroutine()

	// CPU usage would require more sophisticated monitoring
	// For now, we'll use a placeholder
	hc.cpuUsage = 0.0 // TODO: Implement actual CPU usage monitoring
}

// getSystemInfo returns current system information
func (hc *HealthChecker) getSystemInfo() SystemInfo {
	var gcPause float64
	if hc.memStats.NumGC > 0 {
		gcPause = float64(hc.memStats.PauseNs[(hc.memStats.NumGC+255)%256]) / 1000000.0
	}

	return SystemInfo{
		Memory: MemoryInfo{
			Alloc:      hc.memStats.Alloc,
			TotalAlloc: hc.memStats.TotalAlloc,
			Sys:        hc.memStats.Sys,
			NumGC:      hc.memStats.NumGC,
			GCPause:    gcPause,
		},
		Goroutines: hc.goroutineCount,
		CPUUsage:   hc.cpuUsage,
		OSInfo: OSInfo{
			OS:       runtime.GOOS,
			Arch:     runtime.GOARCH,
			Version:  runtime.Version(),
			NumCPU:   runtime.NumCPU(),
			Compiler: runtime.Compiler,
		},
	}
}

// getOverallStatus returns the overall health status string
func (hc *HealthChecker) getOverallStatus() string {
	if hc.healthy {
		return "healthy"
	}
	return "unhealthy"
}

// HealthServer provides HTTP endpoints for health checking
type HealthServer struct {
	checker        *HealthChecker
	metricsManager *metrics.MetricsManager
}

// NewHealthServer creates a new health server instance
func NewHealthServer(checker *HealthChecker, metricsManager *metrics.MetricsManager) *HealthServer {
	return &HealthServer{
		checker:        checker,
		metricsManager: metricsManager,
	}
}

// RegisterRoutes registers health check routes
func (hs *HealthServer) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/health", hs.handleHealth)
	mux.HandleFunc("/health/live", hs.handleLiveness)
	mux.HandleFunc("/health/ready", hs.handleReadiness)
	mux.HandleFunc("/health/detailed", hs.handleDetailedHealth)
	mux.HandleFunc("/api/v5/health", hs.handleAPIHealth)
}

// handleHealth handles basic health check endpoint
func (hs *HealthServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if hs.checker.IsHealthy() {
		hs.writeJSON(w, http.StatusOK, map[string]string{
			"status": "ok",
			"time":   time.Now().Format(time.RFC3339),
		})
	} else {
		hs.writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"status": "unhealthy",
			"time":   time.Now().Format(time.RFC3339),
		})
	}
}

// handleLiveness handles Kubernetes liveness probe endpoint
func (hs *HealthServer) handleLiveness(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Liveness probe - check if the application is running
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

// handleReadiness handles Kubernetes readiness probe endpoint
func (hs *HealthServer) handleReadiness(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Readiness probe - check if the application is ready to serve traffic
	if hs.checker.IsHealthy() {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	} else {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte("Service Unavailable"))
	}
}

// handleDetailedHealth handles detailed health status endpoint
func (hs *HealthServer) handleDetailedHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Run health checks and return detailed status
	status := hs.checker.RunChecks()

	statusCode := http.StatusOK
	if status.Status != "healthy" {
		statusCode = http.StatusServiceUnavailable
	}

	hs.writeJSON(w, statusCode, status)
}

// handleAPIHealth handles EMQX-compatible health API endpoint
func (hs *HealthServer) handleAPIHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	status := hs.checker.GetStatus()
	stats := hs.metricsManager.GetStatistics()

	response := map[string]interface{}{
		"code":   0,
		"status": status.Status,
		"data": map[string]interface{}{
			"health":     status,
			"statistics": stats,
		},
	}

	hs.writeJSON(w, http.StatusOK, response)
}

// writeJSON writes JSON response
func (hs *HealthServer) writeJSON(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		http.Error(w, "Failed to encode JSON", http.StatusInternalServerError)
	}
}

// Global variables for version and node info
var (
	startTime = time.Now()
	version   = "1.0.0" // This should be set during build
	nodeID    = "emqx-go-node"
)

func getVersion() string {
	return version
}

func getNodeID() string {
	return nodeID
}

// SetVersion sets the application version
func SetVersion(v string) {
	version = v
}

// SetNodeID sets the node ID
func SetNodeID(id string) {
	nodeID = id
}

// StartPeriodicHealthChecks starts periodic health checks in a background goroutine
func StartPeriodicHealthChecks(checker *HealthChecker, interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				status := checker.RunChecks()
				if status.Status != "healthy" {
					fmt.Printf("Health check failed: %+v\n", status)
				}
			}
		}
	}()
}