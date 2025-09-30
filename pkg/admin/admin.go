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

// Package admin provides REST API endpoints for EMQX-go broker administration
// and monitoring. It implements EMQX-compatible management interfaces.
package admin

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/turtacn/emqx-go/pkg/metrics"
)

// APIServer provides REST API endpoints for broker management
type APIServer struct {
	metricsManager *metrics.MetricsManager
	broker         BrokerInterface
}

// BrokerInterface defines the interface for broker operations
type BrokerInterface interface {
	GetConnections() []ConnectionInfo
	GetSessions() []SessionInfo
	GetSubscriptions() []SubscriptionInfo
	GetRoutes() []RouteInfo
	DisconnectClient(clientID string) error
	KickoutSession(clientID string) error
	GetNodeInfo() NodeInfo
	GetClusterNodes() []NodeInfo
}

// ConnectionInfo represents client connection information
type ConnectionInfo struct {
	ClientID         string            `json:"clientid"`
	Username         string            `json:"username"`
	PeerHost         string            `json:"peerhost"`
	SockPort         int               `json:"sockport"`
	Protocol         string            `json:"protocol"`
	ConnectedAt      time.Time         `json:"connected_at"`
	DisconnectedAt   *time.Time        `json:"disconnected_at,omitempty"`
	KeepAlive        int               `json:"keepalive"`
	CleanStart       bool              `json:"clean_start"`
	ExpiryInterval   int               `json:"expiry_interval"`
	ProtoVer         int               `json:"proto_ver"`
	SubscriptionsCount int             `json:"subscriptions_count"`
	MaxSubscriptions int               `json:"max_subscriptions"`
	InflightCount    int               `json:"inflight_count"`
	MaxInflight      int               `json:"max_inflight"`
	MqueueCount      int               `json:"mqueue_count"`
	MaxMqueue        int               `json:"max_mqueue"`
	UserProperties   map[string]string `json:"user_properties,omitempty"`
	Node             string            `json:"node"`
}

// SessionInfo represents session information
type SessionInfo struct {
	ClientID         string     `json:"clientid"`
	Username         string     `json:"username"`
	CreatedAt        time.Time  `json:"created_at"`
	ConnectedAt      *time.Time `json:"connected_at,omitempty"`
	DisconnectedAt   *time.Time `json:"disconnected_at,omitempty"`
	ExpiryInterval   int        `json:"expiry_interval"`
	MaxSubscriptions int        `json:"max_subscriptions"`
	SubscriptionsCount int      `json:"subscriptions_count"`
	MaxInflight      int        `json:"max_inflight"`
	InflightCount    int        `json:"inflight_count"`
	MaxMqueue        int        `json:"max_mqueue"`
	MqueueCount      int        `json:"mqueue_count"`
	MaxAwaitingRel   int        `json:"max_awaiting_rel"`
	AwaitingRelCount int        `json:"awaiting_rel_count"`
	Node             string     `json:"node"`
}

// SubscriptionInfo represents subscription information
type SubscriptionInfo struct {
	ClientID string `json:"clientid"`
	Topic    string `json:"topic"`
	QoS      int    `json:"qos"`
	Node     string `json:"node"`
}

// RouteInfo represents routing information
type RouteInfo struct {
	Topic string   `json:"topic"`
	Nodes []string `json:"nodes"`
}

// NodeInfo represents node information
type NodeInfo struct {
	Node      string    `json:"node"`
	NodeStatus string   `json:"node_status"`
	OtpRelease string   `json:"otp_release"`
	Version   string    `json:"version"`
	Uptime    int64     `json:"uptime"`
	Datetime  time.Time `json:"datetime"`
	SysMon    SysMonInfo `json:"sysMon"`
}

// SysMonInfo represents system monitoring information
type SysMonInfo struct {
	ProcCount   int64   `json:"procCount"`
	ProcessLimit int64  `json:"processLimit"`
	MemoryTotal int64   `json:"memoryTotal"`
	MemoryUsed  int64   `json:"memoryUsed"`
	CPUIdle     float64 `json:"cpuIdle"`
	CPULoad1    float64 `json:"cpuLoad1"`
	CPULoad5    float64 `json:"cpuLoad5"`
	CPULoad15   float64 `json:"cpuLoad15"`
}

// APIResponse represents a standard API response
type APIResponse struct {
	Code    int         `json:"code"`
	Message string      `json:"message,omitempty"`
	Data    interface{} `json:"data,omitempty"`
}

// NewAPIServer creates a new API server instance
func NewAPIServer(metricsManager *metrics.MetricsManager, broker BrokerInterface) *APIServer {
	return &APIServer{
		metricsManager: metricsManager,
		broker:         broker,
	}
}

// RegisterRoutes registers all API routes
func (s *APIServer) RegisterRoutes(mux *http.ServeMux) {
	// Statistics and metrics endpoints
	mux.HandleFunc("/api/v5/stats", s.handleStats)
	mux.HandleFunc("/api/v5/metrics", s.handleMetrics)

	// Connection management endpoints
	mux.HandleFunc("/api/v5/connections", s.handleConnections)
	mux.HandleFunc("/api/v5/connections/", s.handleConnectionByID)

	// Session management endpoints
	mux.HandleFunc("/api/v5/sessions", s.handleSessions)
	mux.HandleFunc("/api/v5/sessions/", s.handleSessionByID)

	// Subscription management endpoints
	mux.HandleFunc("/api/v5/subscriptions", s.handleSubscriptions)
	mux.HandleFunc("/api/v5/subscriptions/", s.handleSubscriptionByID)

	// Route management endpoints
	mux.HandleFunc("/api/v5/routes", s.handleRoutes)
	mux.HandleFunc("/api/v5/routes/", s.handleRouteByTopic)

	// Node and cluster endpoints
	mux.HandleFunc("/api/v5/nodes", s.handleNodes)
	mux.HandleFunc("/api/v5/nodes/", s.handleNodeByID)

	// Health check endpoints
	mux.HandleFunc("/api/v5/status", s.handleStatus)
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/api/v5/prometheus/stats", s.handlePrometheusStats)
}

// handleStats handles /api/v5/stats endpoint
func (s *APIServer) handleStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	stats := s.metricsManager.GetStatistics()
	s.writeSuccess(w, stats)
}

// handleMetrics handles /api/v5/metrics endpoint
func (s *APIServer) handleMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	stats := s.metricsManager.GetStatistics()
	s.writeSuccess(w, stats)
}

// handleConnections handles /api/v5/connections endpoint
func (s *APIServer) handleConnections(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		connections := s.broker.GetConnections()

		// Apply pagination and filtering
		page, limit := s.getPagination(r)
		start := (page - 1) * limit
		end := start + limit

		if start > len(connections) {
			start = len(connections)
		}
		if end > len(connections) {
			end = len(connections)
		}

		result := struct {
			Data  []ConnectionInfo `json:"data"`
			Meta  PaginationMeta   `json:"meta"`
		}{
			Data: connections[start:end],
			Meta: PaginationMeta{
				Page:  page,
				Limit: limit,
				Count: len(connections[start:end]),
				Total: len(connections),
			},
		}

		s.writeSuccess(w, result)

	default:
		s.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

// handleConnectionByID handles /api/v5/connections/{clientid} endpoint
func (s *APIServer) handleConnectionByID(w http.ResponseWriter, r *http.Request) {
	clientID := s.extractIDFromPath(r.URL.Path, "/api/v5/connections/")
	if clientID == "" {
		s.writeError(w, http.StatusBadRequest, "Client ID is required")
		return
	}

	switch r.Method {
	case http.MethodGet:
		connections := s.broker.GetConnections()
		for _, conn := range connections {
			if conn.ClientID == clientID {
				s.writeSuccess(w, conn)
				return
			}
		}
		s.writeError(w, http.StatusNotFound, "Connection not found")

	case http.MethodDelete:
		err := s.broker.DisconnectClient(clientID)
		if err != nil {
			s.writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		s.writeSuccess(w, map[string]string{"result": "disconnected"})

	default:
		s.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

// handleSessions handles /api/v5/sessions endpoint
func (s *APIServer) handleSessions(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		sessions := s.broker.GetSessions()

		// Apply pagination
		page, limit := s.getPagination(r)
		start := (page - 1) * limit
		end := start + limit

		if start > len(sessions) {
			start = len(sessions)
		}
		if end > len(sessions) {
			end = len(sessions)
		}

		result := struct {
			Data  []SessionInfo  `json:"data"`
			Meta  PaginationMeta `json:"meta"`
		}{
			Data: sessions[start:end],
			Meta: PaginationMeta{
				Page:  page,
				Limit: limit,
				Count: len(sessions[start:end]),
				Total: len(sessions),
			},
		}

		s.writeSuccess(w, result)

	default:
		s.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

// handleSessionByID handles /api/v5/sessions/{clientid} endpoint
func (s *APIServer) handleSessionByID(w http.ResponseWriter, r *http.Request) {
	clientID := s.extractIDFromPath(r.URL.Path, "/api/v5/sessions/")
	if clientID == "" {
		s.writeError(w, http.StatusBadRequest, "Client ID is required")
		return
	}

	switch r.Method {
	case http.MethodGet:
		sessions := s.broker.GetSessions()
		for _, session := range sessions {
			if session.ClientID == clientID {
				s.writeSuccess(w, session)
				return
			}
		}
		s.writeError(w, http.StatusNotFound, "Session not found")

	case http.MethodDelete:
		err := s.broker.KickoutSession(clientID)
		if err != nil {
			s.writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		s.writeSuccess(w, map[string]string{"result": "kicked out"})

	default:
		s.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

// handleSubscriptions handles /api/v5/subscriptions endpoint
func (s *APIServer) handleSubscriptions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	subscriptions := s.broker.GetSubscriptions()

	// Apply pagination
	page, limit := s.getPagination(r)
	start := (page - 1) * limit
	end := start + limit

	if start > len(subscriptions) {
		start = len(subscriptions)
	}
	if end > len(subscriptions) {
		end = len(subscriptions)
	}

	result := struct {
		Data  []SubscriptionInfo `json:"data"`
		Meta  PaginationMeta     `json:"meta"`
	}{
		Data: subscriptions[start:end],
		Meta: PaginationMeta{
			Page:  page,
			Limit: limit,
			Count: len(subscriptions[start:end]),
			Total: len(subscriptions),
		},
	}

	s.writeSuccess(w, result)
}

// handleSubscriptionByID handles /api/v5/subscriptions/{clientid} endpoint
func (s *APIServer) handleSubscriptionByID(w http.ResponseWriter, r *http.Request) {
	clientID := s.extractIDFromPath(r.URL.Path, "/api/v5/subscriptions/")
	if clientID == "" {
		s.writeError(w, http.StatusBadRequest, "Client ID is required")
		return
	}

	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	subscriptions := s.broker.GetSubscriptions()
	var clientSubs []SubscriptionInfo
	for _, sub := range subscriptions {
		if sub.ClientID == clientID {
			clientSubs = append(clientSubs, sub)
		}
	}

	s.writeSuccess(w, clientSubs)
}

// handleRoutes handles /api/v5/routes endpoint
func (s *APIServer) handleRoutes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	routes := s.broker.GetRoutes()

	// Apply pagination
	page, limit := s.getPagination(r)
	start := (page - 1) * limit
	end := start + limit

	if start > len(routes) {
		start = len(routes)
	}
	if end > len(routes) {
		end = len(routes)
	}

	result := struct {
		Data  []RouteInfo    `json:"data"`
		Meta  PaginationMeta `json:"meta"`
	}{
		Data: routes[start:end],
		Meta: PaginationMeta{
			Page:  page,
			Limit: limit,
			Count: len(routes[start:end]),
			Total: len(routes),
		},
	}

	s.writeSuccess(w, result)
}

// handleRouteByTopic handles /api/v5/routes/{topic} endpoint
func (s *APIServer) handleRouteByTopic(w http.ResponseWriter, r *http.Request) {
	topic := s.extractIDFromPath(r.URL.Path, "/api/v5/routes/")
	if topic == "" {
		s.writeError(w, http.StatusBadRequest, "Topic is required")
		return
	}

	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	routes := s.broker.GetRoutes()
	for _, route := range routes {
		if route.Topic == topic {
			s.writeSuccess(w, route)
			return
		}
	}
	s.writeError(w, http.StatusNotFound, "Route not found")
}

// handleNodes handles /api/v5/nodes endpoint
func (s *APIServer) handleNodes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	nodes := s.broker.GetClusterNodes()
	s.writeSuccess(w, nodes)
}

// handleNodeByID handles /api/v5/nodes/{node} endpoint
func (s *APIServer) handleNodeByID(w http.ResponseWriter, r *http.Request) {
	nodeID := s.extractIDFromPath(r.URL.Path, "/api/v5/nodes/")
	if nodeID == "" {
		s.writeError(w, http.StatusBadRequest, "Node ID is required")
		return
	}

	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	nodes := s.broker.GetClusterNodes()
	for _, node := range nodes {
		if node.Node == nodeID {
			s.writeSuccess(w, node)
			return
		}
	}
	s.writeError(w, http.StatusNotFound, "Node not found")
}

// handleStatus handles /api/v5/status endpoint
func (s *APIServer) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	nodeInfo := s.broker.GetNodeInfo()
	stats := s.metricsManager.GetStatistics()

	status := map[string]interface{}{
		"node":        nodeInfo,
		"connections": stats.Connections,
		"sessions":    stats.Sessions,
		"subscriptions": stats.Subscriptions,
	}

	s.writeSuccess(w, status)
}

// handleHealth handles /health endpoint
func (s *APIServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	health := map[string]string{
		"status": "ok",
		"time":   time.Now().Format(time.RFC3339),
	}

	s.writeSuccess(w, health)
}

// handlePrometheusStats handles /api/v5/prometheus/stats endpoint
func (s *APIServer) handlePrometheusStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	// Update Prometheus metrics and return in Prometheus format
	s.metricsManager.UpdatePrometheusMetrics()

	// For now, return JSON format. In a real implementation,
	// this would return Prometheus exposition format
	stats := s.metricsManager.GetStatistics()
	s.writeSuccess(w, stats)
}

// PaginationMeta represents pagination metadata
type PaginationMeta struct {
	Page  int `json:"page"`
	Limit int `json:"limit"`
	Count int `json:"count"`
	Total int `json:"total"`
}

// Helper methods

func (s *APIServer) writeSuccess(w http.ResponseWriter, data interface{}) {
	response := APIResponse{
		Code: 0,
		Data: data,
	}
	s.writeJSON(w, http.StatusOK, response)
}

func (s *APIServer) writeError(w http.ResponseWriter, statusCode int, message string) {
	response := APIResponse{
		Code:    statusCode,
		Message: message,
	}
	s.writeJSON(w, statusCode, response)
}

func (s *APIServer) writeJSON(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		http.Error(w, "Failed to encode JSON", http.StatusInternalServerError)
	}
}

func (s *APIServer) extractIDFromPath(path, prefix string) string {
	if !strings.HasPrefix(path, prefix) {
		return ""
	}
	return strings.TrimPrefix(path, prefix)
}

func (s *APIServer) getPagination(r *http.Request) (page int, limit int) {
	page = 1
	limit = 20

	if pageStr := r.URL.Query().Get("page"); pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
			page = p
		}
	}

	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 1000 {
			limit = l
		}
	}

	return page, limit
}

// Serve starts the admin API server
func Serve(addr string, metricsManager *metrics.MetricsManager, broker BrokerInterface) error {
	server := NewAPIServer(metricsManager, broker)
	mux := http.NewServeMux()

	server.RegisterRoutes(mux)

	fmt.Printf("Admin API server listening on %s\n", addr)
	fmt.Printf("Available endpoints:\n")
	fmt.Printf("  - /api/v5/stats (broker statistics)\n")
	fmt.Printf("  - /api/v5/connections (connection management)\n")
	fmt.Printf("  - /api/v5/sessions (session management)\n")
	fmt.Printf("  - /api/v5/subscriptions (subscription management)\n")
	fmt.Printf("  - /api/v5/routes (routing information)\n")
	fmt.Printf("  - /api/v5/nodes (cluster nodes)\n")
	fmt.Printf("  - /api/v5/status (broker status)\n")
	fmt.Printf("  - /health (health check)\n")

	return http.ListenAndServe(addr, mux)
}