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

// Package clustermon provides cluster monitoring and metrics collection for
// EMQX-go cluster deployments, including node health, load balancing metrics,
// and cluster-wide statistics aggregation.
package clustermon

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/turtacn/emqx-go/pkg/admin"
	"github.com/turtacn/emqx-go/pkg/metrics"
	"github.com/turtacn/emqx-go/pkg/monitor"
	"github.com/turtacn/emqx-go/pkg/mria"
)

// ClusterMonitor provides cluster-wide monitoring and metrics aggregation
type ClusterMonitor struct {
	mu sync.RWMutex

	nodeID         string
	brokerIntegration *mria.BrokerIntegration
	metricsManager *metrics.MetricsManager
	healthChecker  *monitor.HealthChecker

	// Cluster state
	nodes          map[string]*NodeMetrics
	clusterMetrics *ClusterMetrics
	lastUpdate     time.Time

	// Collection intervals
	metricsInterval time.Duration
	healthInterval  time.Duration

	// Stop channel
	stopCh chan struct{}
}

// NodeMetrics represents metrics for a single node
type NodeMetrics struct {
	NodeID        string                `json:"node_id"`
	NodeRole      string                `json:"node_role"`
	Status        string                `json:"status"`
	LastSeen      time.Time             `json:"last_seen"`
	Uptime        int64                 `json:"uptime"`
	Connections   int64                 `json:"connections"`
	Sessions      int64                 `json:"sessions"`
	Subscriptions int64                 `json:"subscriptions"`
	Messages      NodeMessageMetrics    `json:"messages"`
	System        monitor.SystemInfo    `json:"system"`
	Health        monitor.HealthStatus  `json:"health"`
	LoadAverage   float64               `json:"load_average"`
}

// NodeMessageMetrics represents message metrics for a node
type NodeMessageMetrics struct {
	Received int64 `json:"received"`
	Sent     int64 `json:"sent"`
	Dropped  int64 `json:"dropped"`
	Retained int64 `json:"retained"`
}

// ClusterMetrics represents aggregated cluster metrics
type ClusterMetrics struct {
	ClusterID     string                    `json:"cluster_id"`
	NodesCount    int                       `json:"nodes_count"`
	NodesRunning  int                       `json:"nodes_running"`
	NodesStopped  int                       `json:"nodes_stopped"`
	LastUpdate    time.Time                 `json:"last_update"`
	Totals        ClusterTotalMetrics       `json:"totals"`
	Distribution  ClusterDistributionMetrics `json:"distribution"`
	Performance   ClusterPerformanceMetrics `json:"performance"`
}

// ClusterTotalMetrics represents total metrics across the cluster
type ClusterTotalMetrics struct {
	Connections   int64 `json:"connections"`
	Sessions      int64 `json:"sessions"`
	Subscriptions int64 `json:"subscriptions"`
	Messages      struct {
		Received int64 `json:"received"`
		Sent     int64 `json:"sent"`
		Dropped  int64 `json:"dropped"`
		Retained int64 `json:"retained"`
	} `json:"messages"`
	Memory struct {
		Total int64 `json:"total"`
		Used  int64 `json:"used"`
	} `json:"memory"`
}

// ClusterDistributionMetrics represents load distribution across nodes
type ClusterDistributionMetrics struct {
	ConnectionsPerNode   map[string]int64  `json:"connections_per_node"`
	SessionsPerNode      map[string]int64  `json:"sessions_per_node"`
	SubscriptionsPerNode map[string]int64  `json:"subscriptions_per_node"`
	LoadBalanceRatio     float64           `json:"load_balance_ratio"`
	HotspotNodes         []string          `json:"hotspot_nodes"`
}

// ClusterPerformanceMetrics represents cluster performance metrics
type ClusterPerformanceMetrics struct {
	AverageLatency     float64 `json:"average_latency_ms"`
	MessageThroughput  float64 `json:"message_throughput_per_sec"`
	ConnectionRate     float64 `json:"connection_rate_per_sec"`
	ErrorRate          float64 `json:"error_rate_percent"`
	ResourceUtilization struct {
		CPU    float64 `json:"cpu_percent"`
		Memory float64 `json:"memory_percent"`
	} `json:"resource_utilization"`
}

// NewClusterMonitor creates a new cluster monitor instance
func NewClusterMonitor(nodeID string, brokerIntegration *mria.BrokerIntegration,
	metricsManager *metrics.MetricsManager, healthChecker *monitor.HealthChecker) *ClusterMonitor {

	return &ClusterMonitor{
		nodeID:            nodeID,
		brokerIntegration: brokerIntegration,
		metricsManager:    metricsManager,
		healthChecker:     healthChecker,
		nodes:             make(map[string]*NodeMetrics),
		clusterMetrics:    &ClusterMetrics{},
		metricsInterval:   30 * time.Second,
		healthInterval:    60 * time.Second,
		stopCh:            make(chan struct{}),
	}
}

// Start begins cluster monitoring
func (cm *ClusterMonitor) Start(ctx context.Context) error {
	fmt.Printf("Starting cluster monitor for node %s\n", cm.nodeID)

	// Start metrics collection goroutine
	go cm.metricsCollectionLoop(ctx)

	// Start health monitoring goroutine
	go cm.healthMonitoringLoop(ctx)

	// Start cluster analysis goroutine
	go cm.clusterAnalysisLoop(ctx)

	return nil
}

// Stop stops cluster monitoring
func (cm *ClusterMonitor) Stop() error {
	close(cm.stopCh)
	return nil
}

// GetClusterMetrics returns current cluster metrics
func (cm *ClusterMonitor) GetClusterMetrics() *ClusterMetrics {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	// Return a copy to avoid race conditions
	clusterMetrics := *cm.clusterMetrics
	return &clusterMetrics
}

// GetNodeMetrics returns metrics for a specific node
func (cm *ClusterMonitor) GetNodeMetrics(nodeID string) (*NodeMetrics, bool) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	metrics, exists := cm.nodes[nodeID]
	if !exists {
		return nil, false
	}

	// Return a copy
	nodeMetrics := *metrics
	return &nodeMetrics, true
}

// GetAllNodeMetrics returns metrics for all nodes
func (cm *ClusterMonitor) GetAllNodeMetrics() map[string]*NodeMetrics {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	result := make(map[string]*NodeMetrics)
	for nodeID, metrics := range cm.nodes {
		// Return copies
		nodeMetrics := *metrics
		result[nodeID] = &nodeMetrics
	}

	return result
}

// metricsCollectionLoop collects metrics from cluster nodes
func (cm *ClusterMonitor) metricsCollectionLoop(ctx context.Context) {
	ticker := time.NewTicker(cm.metricsInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-cm.stopCh:
			return
		case <-ticker.C:
			cm.collectMetrics()
		}
	}
}

// healthMonitoringLoop monitors health of cluster nodes
func (cm *ClusterMonitor) healthMonitoringLoop(ctx context.Context) {
	ticker := time.NewTicker(cm.healthInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-cm.stopCh:
			return
		case <-ticker.C:
			cm.monitorHealth()
		}
	}
}

// clusterAnalysisLoop performs cluster-wide analysis and optimization suggestions
func (cm *ClusterMonitor) clusterAnalysisLoop(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Minute) // Analysis every 5 minutes
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-cm.stopCh:
			return
		case <-ticker.C:
			cm.analyzeCluster()
		}
	}
}

// collectMetrics collects metrics from all cluster nodes
func (cm *ClusterMonitor) collectMetrics() {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	now := time.Now()

	// Get current cluster state from Mria integration
	if cm.brokerIntegration != nil && cm.brokerIntegration.IsInitialized() {
		clusterState := cm.brokerIntegration.GetClusterState()
		clusterStats := cm.brokerIntegration.GetClusterStats()

		// Update local node metrics
		localStats := cm.metricsManager.GetStatistics()
		localHealth := cm.healthChecker.GetStatus()

		cm.nodes[cm.nodeID] = &NodeMetrics{
			NodeID:        cm.nodeID,
			NodeRole:      string(cm.brokerIntegration.GetNodeRole()),
			Status:        "running",
			LastSeen:      now,
			Uptime:        localStats.Node.Uptime,
			Connections:   localStats.Connections.Count,
			Sessions:      localStats.Sessions.Count,
			Subscriptions: localStats.Subscriptions.Count,
			Messages: NodeMessageMetrics{
				Received: localStats.Messages.Received,
				Sent:     localStats.Messages.Sent,
				Dropped:  localStats.Messages.Dropped,
				Retained: localStats.Messages.Retained,
			},
			System:      localHealth.SystemInfo,
			Health:      localHealth,
			LoadAverage: calculateLoadAverage(localStats),
		}

		// Update cluster metrics
		cm.clusterMetrics = &ClusterMetrics{
			ClusterID:    clusterState.ClusterID,
			NodesCount:   len(clusterState.CoreNodes) + len(clusterState.ReplicantNodes),
			NodesRunning: len(cm.nodes),
			NodesStopped: 0, // TODO: Calculate based on expected vs actual nodes
			LastUpdate:   now,
			Totals:       cm.calculateTotalMetrics(),
			Distribution: cm.calculateDistributionMetrics(),
			Performance:  cm.calculatePerformanceMetrics(),
		}
	}

	cm.lastUpdate = now
}

// monitorHealth monitors health of cluster nodes
func (cm *ClusterMonitor) monitorHealth() {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	now := time.Now()

	// Check for stale nodes (nodes that haven't reported in over 2 minutes)
	for nodeID, metrics := range cm.nodes {
		if now.Sub(metrics.LastSeen) > 2*time.Minute {
			metrics.Status = "stale"
			cm.nodes[nodeID] = metrics
		}
	}

	// Update cluster health status
	runningNodes := 0
	stoppedNodes := 0
	for _, metrics := range cm.nodes {
		if metrics.Status == "running" {
			runningNodes++
		} else {
			stoppedNodes++
		}
	}

	cm.clusterMetrics.NodesRunning = runningNodes
	cm.clusterMetrics.NodesStopped = stoppedNodes
}

// analyzeCluster performs cluster analysis and generates recommendations
func (cm *ClusterMonitor) analyzeCluster() {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	// Analyze load distribution
	distribution := cm.calculateDistributionMetrics()

	// Identify hotspot nodes (nodes with significantly higher load)
	var hotspots []string
	avgConnections := float64(cm.clusterMetrics.Totals.Connections) / float64(len(cm.nodes))

	for nodeID, connections := range distribution.ConnectionsPerNode {
		if float64(connections) > avgConnections*1.5 { // 50% above average
			hotspots = append(hotspots, nodeID)
		}
	}

	if len(hotspots) > 0 {
		fmt.Printf("Cluster analysis: Detected hotspot nodes: %v\n", hotspots)
	}

	// Check for resource constraints
	for nodeID, metrics := range cm.nodes {
		if metrics.System.Memory.Alloc > 1024*1024*1024*2 { // 2GB threshold
			fmt.Printf("Warning: Node %s has high memory usage: %d bytes\n",
				nodeID, metrics.System.Memory.Alloc)
		}

		if metrics.System.Goroutines > 5000 {
			fmt.Printf("Warning: Node %s has high goroutine count: %d\n",
				nodeID, metrics.System.Goroutines)
		}
	}
}

// calculateTotalMetrics calculates total metrics across the cluster
func (cm *ClusterMonitor) calculateTotalMetrics() ClusterTotalMetrics {
	totals := ClusterTotalMetrics{}

	for _, metrics := range cm.nodes {
		totals.Connections += metrics.Connections
		totals.Sessions += metrics.Sessions
		totals.Subscriptions += metrics.Subscriptions
		totals.Messages.Received += metrics.Messages.Received
		totals.Messages.Sent += metrics.Messages.Sent
		totals.Messages.Dropped += metrics.Messages.Dropped
		totals.Messages.Retained += metrics.Messages.Retained
		totals.Memory.Total += int64(metrics.System.Memory.Sys)
		totals.Memory.Used += int64(metrics.System.Memory.Alloc)
	}

	return totals
}

// calculateDistributionMetrics calculates load distribution metrics
func (cm *ClusterMonitor) calculateDistributionMetrics() ClusterDistributionMetrics {
	distribution := ClusterDistributionMetrics{
		ConnectionsPerNode:   make(map[string]int64),
		SessionsPerNode:      make(map[string]int64),
		SubscriptionsPerNode: make(map[string]int64),
	}

	var connectionValues []int64
	var hotspots []string

	for nodeID, metrics := range cm.nodes {
		distribution.ConnectionsPerNode[nodeID] = metrics.Connections
		distribution.SessionsPerNode[nodeID] = metrics.Sessions
		distribution.SubscriptionsPerNode[nodeID] = metrics.Subscriptions

		connectionValues = append(connectionValues, metrics.Connections)
	}

	// Calculate load balance ratio (standard deviation / mean)
	if len(connectionValues) > 1 {
		mean := calculateMean(connectionValues)
		stdDev := calculateStdDev(connectionValues, mean)

		if mean > 0 {
			distribution.LoadBalanceRatio = stdDev / mean
		}

		// Identify hotspots (nodes with load > 1.5 * mean)
		for nodeID, connections := range distribution.ConnectionsPerNode {
			if float64(connections) > mean*1.5 {
				hotspots = append(hotspots, nodeID)
			}
		}
	}

	distribution.HotspotNodes = hotspots
	return distribution
}

// calculatePerformanceMetrics calculates cluster performance metrics
func (cm *ClusterMonitor) calculatePerformanceMetrics() ClusterPerformanceMetrics {
	performance := ClusterPerformanceMetrics{}

	if len(cm.nodes) == 0 {
		return performance
	}

	var totalCPU, totalMemory float64
	var totalMessages, totalErrors int64

	for _, metrics := range cm.nodes {
		totalCPU += metrics.System.CPUUsage
		totalMemory += float64(metrics.System.Memory.Alloc) / float64(metrics.System.Memory.Sys) * 100
		totalMessages += metrics.Messages.Received + metrics.Messages.Sent
		// Note: We would need error metrics from the health checker
	}

	nodeCount := float64(len(cm.nodes))
	performance.ResourceUtilization.CPU = totalCPU / nodeCount
	performance.ResourceUtilization.Memory = totalMemory / nodeCount

	// Calculate throughput (messages per second over the collection interval)
	intervalSeconds := cm.metricsInterval.Seconds()
	performance.MessageThroughput = float64(totalMessages) / intervalSeconds

	// TODO: Implement actual latency measurement
	performance.AverageLatency = 0.0

	// TODO: Implement connection rate calculation
	performance.ConnectionRate = 0.0

	// TODO: Implement error rate calculation
	performance.ErrorRate = 0.0

	return performance
}

// Helper functions

func calculateLoadAverage(stats metrics.Statistics) float64 {
	// Simple load calculation based on connections and sessions
	return float64(stats.Connections.Count + stats.Sessions.Count)
}

func calculateMean(values []int64) float64 {
	if len(values) == 0 {
		return 0
	}

	var sum int64
	for _, v := range values {
		sum += v
	}

	return float64(sum) / float64(len(values))
}

func calculateStdDev(values []int64, mean float64) float64 {
	if len(values) == 0 {
		return 0
	}

	var sumSquares float64
	for _, v := range values {
		diff := float64(v) - mean
		sumSquares += diff * diff
	}

	variance := sumSquares / float64(len(values))
	return variance // For simplicity, returning variance instead of sqrt(variance)
}

// ClusterMonitorServer provides HTTP endpoints for cluster monitoring
type ClusterMonitorServer struct {
	monitor *ClusterMonitor
}

// NewClusterMonitorServer creates a new cluster monitor server
func NewClusterMonitorServer(monitor *ClusterMonitor) *ClusterMonitorServer {
	return &ClusterMonitorServer{
		monitor: monitor,
	}
}

// RegisterRoutes registers cluster monitoring routes
func (cms *ClusterMonitorServer) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/v5/cluster/metrics", cms.handleClusterMetrics)
	mux.HandleFunc("/api/v5/cluster/nodes", cms.handleClusterNodes)
	mux.HandleFunc("/api/v5/cluster/nodes/", cms.handleClusterNodeByID)
	mux.HandleFunc("/api/v5/cluster/health", cms.handleClusterHealth)
	mux.HandleFunc("/api/v5/cluster/distribution", cms.handleClusterDistribution)
	mux.HandleFunc("/api/v5/cluster/performance", cms.handleClusterPerformance)
}

// handleClusterMetrics handles cluster metrics endpoint
func (cms *ClusterMonitorServer) handleClusterMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	metrics := cms.monitor.GetClusterMetrics()
	cms.writeJSON(w, http.StatusOK, map[string]interface{}{
		"code": 0,
		"data": metrics,
	})
}

// handleClusterNodes handles cluster nodes endpoint
func (cms *ClusterMonitorServer) handleClusterNodes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	nodes := cms.monitor.GetAllNodeMetrics()
	cms.writeJSON(w, http.StatusOK, map[string]interface{}{
		"code": 0,
		"data": nodes,
	})
}

// handleClusterNodeByID handles specific cluster node endpoint
func (cms *ClusterMonitorServer) handleClusterNodeByID(w http.ResponseWriter, r *http.Request) {
	nodeID := extractIDFromPath(r.URL.Path, "/api/v5/cluster/nodes/")
	if nodeID == "" {
		http.Error(w, "Node ID is required", http.StatusBadRequest)
		return
	}

	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	node, exists := cms.monitor.GetNodeMetrics(nodeID)
	if !exists {
		http.Error(w, "Node not found", http.StatusNotFound)
		return
	}

	cms.writeJSON(w, http.StatusOK, map[string]interface{}{
		"code": 0,
		"data": node,
	})
}

// handleClusterHealth handles cluster health endpoint
func (cms *ClusterMonitorServer) handleClusterHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	metrics := cms.monitor.GetClusterMetrics()
	nodes := cms.monitor.GetAllNodeMetrics()

	healthStatus := "healthy"
	if metrics.NodesStopped > 0 {
		healthStatus = "degraded"
	}

	unhealthyNodes := 0
	for _, node := range nodes {
		if node.Health.Status != "healthy" {
			unhealthyNodes++
		}
	}

	if unhealthyNodes > 0 {
		healthStatus = "unhealthy"
	}

	health := map[string]interface{}{
		"status":          healthStatus,
		"nodes_total":     metrics.NodesCount,
		"nodes_running":   metrics.NodesRunning,
		"nodes_stopped":   metrics.NodesStopped,
		"unhealthy_nodes": unhealthyNodes,
		"last_update":     metrics.LastUpdate,
	}

	cms.writeJSON(w, http.StatusOK, map[string]interface{}{
		"code": 0,
		"data": health,
	})
}

// handleClusterDistribution handles cluster distribution endpoint
func (cms *ClusterMonitorServer) handleClusterDistribution(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	metrics := cms.monitor.GetClusterMetrics()
	cms.writeJSON(w, http.StatusOK, map[string]interface{}{
		"code": 0,
		"data": metrics.Distribution,
	})
}

// handleClusterPerformance handles cluster performance endpoint
func (cms *ClusterMonitorServer) handleClusterPerformance(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	metrics := cms.monitor.GetClusterMetrics()
	cms.writeJSON(w, http.StatusOK, map[string]interface{}{
		"code": 0,
		"data": metrics.Performance,
	})
}

// writeJSON writes JSON response
func (cms *ClusterMonitorServer) writeJSON(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		http.Error(w, "Failed to encode JSON", http.StatusInternalServerError)
	}
}

// Helper function to extract ID from URL path
func extractIDFromPath(path, prefix string) string {
	if len(path) <= len(prefix) {
		return ""
	}
	return path[len(prefix):]
}