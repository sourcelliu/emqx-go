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

// Package dashboard provides a web-based management interface for EMQX-go.
// It implements a dashboard similar to EMQX's web interface with real-time
// monitoring, connection management, and system administration features.
package dashboard

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/turtacn/emqx-go/pkg/admin"
	"github.com/turtacn/emqx-go/pkg/metrics"
	"github.com/turtacn/emqx-go/pkg/monitor"
)

//go:embed web/templates/* web/static/*
var embeddedFiles embed.FS

// Server represents the dashboard web server
type Server struct {
	httpServer     *http.Server
	adminAPI       *admin.APIServer
	metricsManager *metrics.MetricsManager
	healthChecker  *monitor.HealthChecker
	templates      *template.Template
	mux            *http.ServeMux
	config         *Config
}

// Config contains dashboard server configuration
type Config struct {
	Address     string `json:"address"`     // Server listen address
	Port        int    `json:"port"`        // Server port
	Username    string `json:"username"`    // Dashboard username
	Password    string `json:"password"`    // Dashboard password
	EnableAuth  bool   `json:"enable_auth"` // Enable authentication
	StaticDir   string `json:"static_dir"`  // Static files directory (optional)
	TemplateDir string `json:"template_dir"` // Template directory (optional)
}

// DefaultConfig returns a default dashboard configuration
func DefaultConfig() *Config {
	return &Config{
		Address:    "0.0.0.0",
		Port:       18083,
		Username:   "admin",
		Password:   "public",
		EnableAuth: true,
	}
}

// NewServer creates a new dashboard server
func NewServer(config *Config, adminAPI *admin.APIServer, metricsManager *metrics.MetricsManager, healthChecker *monitor.HealthChecker) (*Server, error) {
	if config == nil {
		config = DefaultConfig()
	}

	mux := http.NewServeMux()

	server := &Server{
		adminAPI:       adminAPI,
		metricsManager: metricsManager,
		healthChecker:  healthChecker,
		mux:            mux,
		config:         config,
	}

	// Load templates
	if err := server.loadTemplates(); err != nil {
		return nil, fmt.Errorf("failed to load templates: %v", err)
	}

	// Setup routes
	server.setupRoutes()

	// Create HTTP server
	addr := fmt.Sprintf("%s:%d", config.Address, config.Port)
	server.httpServer = &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	return server, nil
}

// loadTemplates loads HTML templates
func (s *Server) loadTemplates() error {
	var err error

	if s.config.TemplateDir != "" {
		// Load from external directory if specified
		s.templates, err = template.ParseGlob(s.config.TemplateDir + "/*.html")
	} else {
		// Load from embedded files
		templateFS, fsErr := fs.Sub(embeddedFiles, "web/templates")
		if fsErr != nil {
			return fsErr
		}
		s.templates, err = template.ParseFS(templateFS, "*.html")
	}

	if err != nil {
		return fmt.Errorf("failed to parse templates: %v", err)
	}

	return nil
}

// setupRoutes configures HTTP routes
func (s *Server) setupRoutes() {
	// Static files
	s.setupStaticRoutes()

	// Dashboard pages
	s.mux.HandleFunc("/", s.authMiddleware(s.handleDashboard))
	s.mux.HandleFunc("/dashboard", s.authMiddleware(s.handleDashboard))
	s.mux.HandleFunc("/connections", s.authMiddleware(s.handleConnections))
	s.mux.HandleFunc("/sessions", s.authMiddleware(s.handleSessions))
	s.mux.HandleFunc("/subscriptions", s.authMiddleware(s.handleSubscriptions))
	s.mux.HandleFunc("/monitoring", s.authMiddleware(s.handleMonitoring))

	// Authentication
	s.mux.HandleFunc("/login", s.handleLogin)
	s.mux.HandleFunc("/logout", s.handleLogout)

	// API endpoints for AJAX requests
	s.mux.HandleFunc("/api/dashboard/stats", s.authMiddleware(s.handleAPIStats))
	s.mux.HandleFunc("/api/dashboard/connections", s.authMiddleware(s.handleAPIConnections))
	s.mux.HandleFunc("/api/dashboard/sessions", s.authMiddleware(s.handleAPISessions))
	s.mux.HandleFunc("/api/dashboard/subscriptions", s.authMiddleware(s.handleAPISubscriptions))
	s.mux.HandleFunc("/api/dashboard/health", s.authMiddleware(s.handleAPIHealth))

	// Proxy admin API endpoints
	s.mux.Handle("/api/v5/", s.authMiddleware(s.proxyAdminAPI))
}

// setupStaticRoutes configures static file serving
func (s *Server) setupStaticRoutes() {
	if s.config.StaticDir != "" {
		// Serve from external directory if specified
		fs := http.FileServer(http.Dir(s.config.StaticDir))
		s.mux.Handle("/static/", http.StripPrefix("/static/", fs))
	} else {
		// Serve from embedded files
		staticFS, err := fs.Sub(embeddedFiles, "web/static")
		if err != nil {
			log.Printf("Warning: Could not load embedded static files: %v", err)
			return
		}
		staticHandler := http.FileServer(http.FS(staticFS))
		s.mux.Handle("/static/", http.StripPrefix("/static/", staticHandler))
	}
}

// Start starts the dashboard server
func (s *Server) Start(ctx context.Context) error {
	log.Printf("Starting dashboard server on %s", s.httpServer.Addr)

	// Start server in a goroutine
	go func() {
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("Dashboard server error: %v", err)
		}
	}()

	// Wait for context cancellation
	<-ctx.Done()

	// Graceful shutdown
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	return s.httpServer.Shutdown(shutdownCtx)
}

// Stop stops the dashboard server
func (s *Server) Stop() error {
	if s.httpServer != nil {
		return s.httpServer.Close()
	}
	return nil
}

// authMiddleware provides basic authentication middleware
func (s *Server) authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.config.EnableAuth {
			next(w, r)
			return
		}

		// Check for session cookie
		cookie, err := r.Cookie("dashboard_session")
		if err == nil && s.validateSession(cookie.Value) {
			next(w, r)
			return
		}

		// Check for basic auth
		username, password, ok := r.BasicAuth()
		if ok && s.validateCredentials(username, password) {
			// Set session cookie
			s.setSessionCookie(w)
			next(w, r)
			return
		}

		// Redirect to login for HTML requests, return 401 for API requests
		if strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("WWW-Authenticate", `Basic realm="Dashboard"`)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		http.Redirect(w, r, "/login", http.StatusFound)
	}
}

// validateCredentials validates username and password
func (s *Server) validateCredentials(username, password string) bool {
	return username == s.config.Username && password == s.config.Password
}

// validateSession validates session token (simplified implementation)
func (s *Server) validateSession(token string) bool {
	// In a real implementation, this would validate against a session store
	// For now, we'll use a simple check
	return token == "valid_session_token"
}

// setSessionCookie sets a session cookie
func (s *Server) setSessionCookie(w http.ResponseWriter) {
	cookie := &http.Cookie{
		Name:     "dashboard_session",
		Value:    "valid_session_token",
		Path:     "/",
		HttpOnly: true,
		Secure:   false, // Set to true in production with HTTPS
		SameSite: http.SameSiteStrictMode,
		Expires:  time.Now().Add(24 * time.Hour),
	}
	http.SetCookie(w, cookie)
}

// Dashboard page handlers

// handleDashboard serves the main dashboard page
func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	data := s.getDashboardData()
	s.renderTemplate(w, "dashboard.html", data)
}

// handleConnections serves the connections management page
func (s *Server) handleConnections(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	data := s.getConnectionsData()
	s.renderTemplate(w, "connections.html", data)
}

// handleSessions serves the sessions management page
func (s *Server) handleSessions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	data := s.getSessionsData()
	s.renderTemplate(w, "sessions.html", data)
}

// handleSubscriptions serves the subscriptions management page
func (s *Server) handleSubscriptions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	data := s.getSubscriptionsData()
	s.renderTemplate(w, "subscriptions.html", data)
}

// handleMonitoring serves the monitoring page
func (s *Server) handleMonitoring(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	data := s.getMonitoringData()
	s.renderTemplate(w, "monitoring.html", data)
}

// handleLogin serves the login page and processes login requests
func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.renderTemplate(w, "login.html", map[string]interface{}{
			"Title": "Login - EMQX Dashboard",
		})
	case http.MethodPost:
		r.ParseForm()
		username := r.FormValue("username")
		password := r.FormValue("password")

		if s.validateCredentials(username, password) {
			s.setSessionCookie(w)
			http.Redirect(w, r, "/dashboard", http.StatusFound)
		} else {
			s.renderTemplate(w, "login.html", map[string]interface{}{
				"Title": "Login - EMQX Dashboard",
				"Error": "Invalid username or password",
			})
		}
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleLogout processes logout requests
func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	// Clear session cookie
	cookie := &http.Cookie{
		Name:     "dashboard_session",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Expires:  time.Now().Add(-1 * time.Hour),
	}
	http.SetCookie(w, cookie)

	http.Redirect(w, r, "/login", http.StatusFound)
}

// API handlers for AJAX requests

// handleAPIStats returns statistics data as JSON
func (s *Server) handleAPIStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	stats := s.metricsManager.GetStatistics()
	s.writeJSON(w, stats)
}

// handleAPIConnections returns connections data as JSON
func (s *Server) handleAPIConnections(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get connections from admin API if available
	if s.adminAPI != nil {
		// Use internal broker interface to get connections
		// For now, return mock data - in a real implementation this would
		// call the actual broker interface
		connections := []map[string]interface{}{
			{
				"clientid":     "demo_client_1",
				"username":     "demo_user",
				"peerhost":     "127.0.0.1",
				"sockport":     1883,
				"connected":    true,
				"connected_at": time.Now().Add(-5 * time.Minute).Format(time.RFC3339),
				"protocol":     "MQTT",
				"proto_ver":    5,
				"keepalive":    60,
				"clean_start":  true,
			},
			{
				"clientid":     "demo_client_2",
				"username":     "test_user",
				"peerhost":     "192.168.1.100",
				"sockport":     1883,
				"connected":    true,
				"connected_at": time.Now().Add(-2 * time.Minute).Format(time.RFC3339),
				"protocol":     "MQTT",
				"proto_ver":    4,
				"keepalive":    120,
				"clean_start":  false,
			},
		}

		data := map[string]interface{}{
			"connections": connections,
			"count":       len(connections),
		}
		s.writeJSON(w, data)
		return
	}

	// Fallback to empty data
	data := map[string]interface{}{
		"connections": []interface{}{},
		"count":       0,
	}

	s.writeJSON(w, data)
}

// handleAPISessions returns sessions data as JSON
func (s *Server) handleAPISessions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Mock session data for demonstration
	sessions := []map[string]interface{}{
		{
			"clientid":         "demo_client_1",
			"username":         "demo_user",
			"clean_start":      true,
			"created_at":       time.Now().Add(-5 * time.Minute).Format(time.RFC3339),
			"subscriptions_cnt": 2,
		},
		{
			"clientid":         "demo_client_2",
			"username":         "test_user",
			"clean_start":      false,
			"created_at":       time.Now().Add(-2 * time.Minute).Format(time.RFC3339),
			"subscriptions_cnt": 5,
		},
	}

	data := map[string]interface{}{
		"sessions": sessions,
		"count":    len(sessions),
	}

	s.writeJSON(w, data)
}

// handleAPISubscriptions returns subscriptions data as JSON
func (s *Server) handleAPISubscriptions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Mock subscription data for demonstration
	subscriptions := []map[string]interface{}{
		{
			"clientid": "demo_client_1",
			"topic":    "devices/+/status",
			"qos":      1,
			"node":     "emqx-go-node",
		},
		{
			"clientid": "demo_client_1",
			"topic":    "sensors/temperature",
			"qos":      0,
			"node":     "emqx-go-node",
		},
		{
			"clientid": "demo_client_2",
			"topic":    "devices/sensor1/data",
			"qos":      2,
			"node":     "emqx-go-node",
		},
		{
			"clientid": "demo_client_2",
			"topic":    "alerts/+",
			"qos":      1,
			"node":     "emqx-go-node",
		},
	}

	data := map[string]interface{}{
		"subscriptions": subscriptions,
		"count":         len(subscriptions),
	}

	s.writeJSON(w, data)
}

// handleAPIHealth returns health status as JSON
func (s *Server) handleAPIHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	health := s.healthChecker.GetStatus()
	s.writeJSON(w, health)
}

// proxyAdminAPI proxies requests to the admin API
func (s *Server) proxyAdminAPI(w http.ResponseWriter, r *http.Request) {
	// Strip /api/v5 prefix and forward to admin API
	// For now, return a simple response
	// In a full implementation, this would proxy to the actual admin API
	response := map[string]interface{}{
		"code":    0,
		"message": "success",
		"data":    nil,
	}

	s.writeJSON(w, response)
}

// Helper methods

// getDashboardData prepares data for the dashboard page
func (s *Server) getDashboardData() map[string]interface{} {
	stats := s.metricsManager.GetStatistics()
	health := s.healthChecker.GetStatus()

	return map[string]interface{}{
		"Title":       "Dashboard - EMQX Go",
		"Stats":       stats,
		"Health":      health,
		"CurrentTime": time.Now().Format("2006-01-02 15:04:05"),
	}
}

// getConnectionsData prepares data for connections page
func (s *Server) getConnectionsData() map[string]interface{} {
	stats := s.metricsManager.GetStatistics()

	return map[string]interface{}{
		"Title":           "Connections - EMQX Go",
		"ConnectionCount": stats.Connections.Count,
		"Connections":     []interface{}{}, // Placeholder
	}
}

// getSessionsData prepares data for sessions page
func (s *Server) getSessionsData() map[string]interface{} {
	stats := s.metricsManager.GetStatistics()

	return map[string]interface{}{
		"Title":        "Sessions - EMQX Go",
		"SessionCount": stats.Sessions.Count,
		"Sessions":     []interface{}{}, // Placeholder
	}
}

// getSubscriptionsData prepares data for subscriptions page
func (s *Server) getSubscriptionsData() map[string]interface{} {
	stats := s.metricsManager.GetStatistics()

	return map[string]interface{}{
		"Title":              "Subscriptions - EMQX Go",
		"SubscriptionCount":  stats.Subscriptions.Count,
		"Subscriptions":      []interface{}{}, // Placeholder
	}
}

// getMonitoringData prepares data for monitoring page
func (s *Server) getMonitoringData() map[string]interface{} {
	stats := s.metricsManager.GetStatistics()
	health := s.healthChecker.GetStatus()

	return map[string]interface{}{
		"Title":  "Monitoring - EMQX Go",
		"Stats":  stats,
		"Health": health,
	}
}

// renderTemplate renders an HTML template with data
func (s *Server) renderTemplate(w http.ResponseWriter, templateName string, data interface{}) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	if err := s.templates.ExecuteTemplate(w, templateName, data); err != nil {
		log.Printf("Template execution error: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
}

// writeJSON writes JSON response
func (s *Server) writeJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("JSON encoding error: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
}

// GetListenAddress returns the server listen address
func (s *Server) GetListenAddress() string {
	if s.httpServer != nil {
		return s.httpServer.Addr
	}
	return fmt.Sprintf("%s:%d", s.config.Address, s.config.Port)
}

// UpdateConfig updates server configuration (requires restart)
func (s *Server) UpdateConfig(config *Config) {
	s.config = config
}