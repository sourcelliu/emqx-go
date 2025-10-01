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
	"github.com/turtacn/emqx-go/pkg/auth/x509"
	"github.com/turtacn/emqx-go/pkg/connector"
	"github.com/turtacn/emqx-go/pkg/integration"
	"github.com/turtacn/emqx-go/pkg/metrics"
	"github.com/turtacn/emqx-go/pkg/monitor"
	"github.com/turtacn/emqx-go/pkg/rules"
	tlspkg "github.com/turtacn/emqx-go/pkg/tls"
)

//go:embed web/templates/* web/static/*
var embeddedFiles embed.FS

// Server represents the dashboard web server
type Server struct {
	httpServer         *http.Server
	adminAPI           *admin.APIServer
	metricsManager     *metrics.MetricsManager
	healthChecker      *monitor.HealthChecker
	certificateManager *tlspkg.CertificateManager
	connectorManager   *connector.ConnectorManager
	ruleEngine         *rules.RuleEngine
	integrationEngine  *integration.DataIntegrationEngine
	templates          *template.Template
	mux                *http.ServeMux
	config             *Config
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
func NewServer(config *Config, adminAPI *admin.APIServer, metricsManager *metrics.MetricsManager, healthChecker *monitor.HealthChecker, certManager *tlspkg.CertificateManager, connectorManager *connector.ConnectorManager, ruleEngine *rules.RuleEngine, integrationEngine *integration.DataIntegrationEngine) (*Server, error) {
	if config == nil {
		config = DefaultConfig()
	}

	mux := http.NewServeMux()

	server := &Server{
		adminAPI:           adminAPI,
		metricsManager:     metricsManager,
		healthChecker:      healthChecker,
		certificateManager: certManager,
		connectorManager:   connectorManager,
		ruleEngine:         ruleEngine,
		integrationEngine:  integrationEngine,
		mux:                mux,
		config:             config,
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
	s.mux.HandleFunc("/blacklist", s.authMiddleware(s.handleBlacklist))
	s.mux.HandleFunc("/certificates", s.authMiddleware(s.handleCertificates))
	s.mux.HandleFunc("/tls-config", s.authMiddleware(s.handleTLSConfig))
	s.mux.HandleFunc("/connectors", s.authMiddleware(s.handleConnectors))
	s.mux.HandleFunc("/rules", s.authMiddleware(s.handleRules))
	s.mux.HandleFunc("/integration", s.authMiddleware(s.handleIntegration))

	// Authentication
	s.mux.HandleFunc("/login", s.handleLogin)
	s.mux.HandleFunc("/logout", s.handleLogout)

	// API endpoints for AJAX requests
	s.mux.HandleFunc("/api/dashboard/stats", s.authMiddleware(s.handleAPIStats))
	s.mux.HandleFunc("/api/dashboard/connections", s.authMiddleware(s.handleAPIConnections))
	s.mux.HandleFunc("/api/dashboard/sessions", s.authMiddleware(s.handleAPISessions))
	s.mux.HandleFunc("/api/dashboard/subscriptions", s.authMiddleware(s.handleAPISubscriptions))
	s.mux.HandleFunc("/api/dashboard/health", s.authMiddleware(s.handleAPIHealth))

	// Blacklist management API endpoints
	s.mux.HandleFunc("/api/blacklist", s.authMiddleware(s.handleAPIBlacklist))
	s.mux.HandleFunc("/api/blacklist/", s.authMiddleware(s.handleAPIBlacklistByID))
	s.mux.HandleFunc("/api/blacklist/stats", s.authMiddleware(s.handleAPIBlacklistStats))

	// Certificate management API endpoints
	s.mux.HandleFunc("/api/certificates", s.authMiddleware(s.handleAPICertificates))
	s.mux.HandleFunc("/api/certificates/", s.authMiddleware(s.handleAPICertificateByID))
	s.mux.HandleFunc("/api/tls-config", s.authMiddleware(s.handleAPITLSConfig))
	s.mux.HandleFunc("/api/tls-config/", s.authMiddleware(s.handleAPITLSConfigByID))
	s.mux.HandleFunc("/api/x509-auth", s.authMiddleware(s.handleAPIX509Auth))

	// Connector management API endpoints
	s.mux.HandleFunc("/api/connectors", s.authMiddleware(s.handleAPIConnectors))
	s.mux.HandleFunc("/api/connectors/", s.authMiddleware(s.handleAPIConnectorByID))
	s.mux.HandleFunc("/api/connector-types", s.authMiddleware(s.handleAPIConnectorTypes))

	// Rule engine management API endpoints
	s.mux.HandleFunc("/api/rules", s.authMiddleware(s.handleAPIRules))
	s.mux.HandleFunc("/api/rules/", s.authMiddleware(s.handleAPIRuleByID))
	s.mux.HandleFunc("/api/action-types", s.authMiddleware(s.handleAPIActionTypes))
	s.mux.HandleFunc("/api/action-schemas/", s.authMiddleware(s.handleAPIActionSchema))

	// Data integration management API endpoints
	s.mux.HandleFunc("/api/integration/bridges", s.authMiddleware(s.handleAPIBridges))
	s.mux.HandleFunc("/api/integration/bridges/", s.authMiddleware(s.handleAPIBridgeByID))
	s.mux.HandleFunc("/api/integration/connectors", s.authMiddleware(s.handleAPIIntegrationConnectors))
	s.mux.HandleFunc("/api/integration/metrics", s.authMiddleware(s.handleAPIIntegrationMetrics))

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

// handleConnectors serves the connector management page
func (s *Server) handleConnectors(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	data := s.getConnectorsData()
	s.renderTemplate(w, "connectors.html", data)
}

// handleCertificates serves the certificate management page
func (s *Server) handleCertificates(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	data := s.getCertificatesData()
	s.renderTemplate(w, "certificates.html", data)
}

// handleTLSConfig serves the TLS configuration page
func (s *Server) handleTLSConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	data := s.getTLSConfigData()
	s.renderTemplate(w, "tls-config.html", data)
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

// Certificate management API handlers

// handleAPICertificates handles certificate CRUD operations
func (s *Server) handleAPICertificates(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		// List all certificates
		certs := s.certificateManager.ListCertificates()
		data := map[string]interface{}{
			"certificates": certs,
			"count":        len(certs),
		}
		s.writeJSON(w, data)

	case http.MethodPost:
		// Add new certificate
		var cert tlspkg.Certificate
		if err := json.NewDecoder(r.Body).Decode(&cert); err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}

		if err := s.certificateManager.AddCertificate(&cert); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		s.writeJSON(w, map[string]interface{}{
			"message": "Certificate added successfully",
			"id":      cert.ID,
		})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleAPICertificateByID handles operations on specific certificates
func (s *Server) handleAPICertificateByID(w http.ResponseWriter, r *http.Request) {
	// Extract certificate ID from URL path
	path := strings.TrimPrefix(r.URL.Path, "/api/certificates/")
	if path == "" {
		http.Error(w, "Certificate ID required", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		// Get specific certificate
		cert, err := s.certificateManager.GetCertificate(path)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		s.writeJSON(w, cert)

	case http.MethodDelete:
		// Delete certificate
		if err := s.certificateManager.DeleteCertificate(path); err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		s.writeJSON(w, map[string]interface{}{
			"message": "Certificate deleted successfully",
		})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleAPITLSConfig handles TLS configuration operations
func (s *Server) handleAPITLSConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		// Add new TLS configuration
		var config tlspkg.TLSConfig
		if err := json.NewDecoder(r.Body).Decode(&config); err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}

		// Generate a simple ID for the config
		configID := fmt.Sprintf("tls-config-%d", time.Now().Unix())
		s.certificateManager.SetTLSConfig(configID, &config)

		s.writeJSON(w, map[string]interface{}{
			"message":   "TLS configuration added successfully",
			"config_id": configID,
		})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleAPITLSConfigByID handles operations on specific TLS configurations
func (s *Server) handleAPITLSConfigByID(w http.ResponseWriter, r *http.Request) {
	// Extract config ID from URL path
	path := strings.TrimPrefix(r.URL.Path, "/api/tls-config/")
	if path == "" {
		http.Error(w, "TLS config ID required", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		// Get TLS configuration
		tlsConfig, err := s.certificateManager.GetTLSConfig(path)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}

		// Convert to our config format for response
		config := &tlspkg.TLSConfig{
			Enabled:         true,
			ServerName:      tlsConfig.ServerName,
			ClientCertAuth:  tlsConfig.ClientAuth != 0,
		}
		s.writeJSON(w, config)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleAPIX509Auth handles X.509 authentication configuration
func (s *Server) handleAPIX509Auth(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		// Return default X.509 authentication configuration
		config := x509.DefaultX509Config()
		s.writeJSON(w, config)

	case http.MethodPost:
		// Update X.509 authentication configuration
		var config x509.X509Config
		if err := json.NewDecoder(r.Body).Decode(&config); err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}

		// In a real implementation, this would update the broker's X.509 authenticator
		// For now, just return success
		s.writeJSON(w, map[string]interface{}{
			"message": "X.509 authentication configuration updated successfully",
			"config":  config,
		})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// Connector management API handlers

// handleAPIConnectors handles connector CRUD operations
func (s *Server) handleAPIConnectors(w http.ResponseWriter, r *http.Request) {
	if s.connectorManager == nil {
		http.Error(w, "Connector manager not available", http.StatusServiceUnavailable)
		return
	}

	switch r.Method {
	case http.MethodGet:
		// List all connectors
		connectors := s.connectorManager.ListConnectors()
		connectorsData := make([]map[string]interface{}, 0)

		for id, conn := range connectors {
			config := conn.GetConfig()
			status := conn.Status()
			metrics := conn.GetMetrics()

			connectorsData = append(connectorsData, map[string]interface{}{
				"id":          id,
				"name":        config.Name,
				"type":        string(config.Type),
				"enabled":     config.Enabled,
				"state":       string(status.State),
				"created_at":  config.CreatedAt.Format(time.RFC3339),
				"updated_at":  config.UpdatedAt.Format(time.RFC3339),
				"description": config.Description,
				"metrics": map[string]interface{}{
					"messages_sent":     metrics.MessagesSent,
					"messages_received": metrics.MessagesReceived,
					"messages_dropped":  metrics.MessagesDropped,
					"error_count":       metrics.ErrorCount,
					"success_count":     metrics.SuccessCount,
				},
				"health_status": map[string]interface{}{
					"healthy":         status.HealthStatus.Healthy,
					"last_check_time": status.HealthStatus.LastCheckTime.Format(time.RFC3339),
					"message":         status.HealthStatus.Message,
				},
			})
		}

		data := map[string]interface{}{
			"connectors": connectorsData,
			"count":      len(connectorsData),
		}
		s.writeJSON(w, data)

	case http.MethodPost:
		// Create new connector
		var config connector.ConnectorConfig
		if err := json.NewDecoder(r.Body).Decode(&config); err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}

		if err := s.connectorManager.CreateConnector(config); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		s.writeJSON(w, map[string]interface{}{
			"message": "Connector created successfully",
			"id":      config.ID,
		})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleAPIConnectorByID handles operations on specific connectors
func (s *Server) handleAPIConnectorByID(w http.ResponseWriter, r *http.Request) {
	if s.connectorManager == nil {
		http.Error(w, "Connector manager not available", http.StatusServiceUnavailable)
		return
	}

	// Extract connector ID from URL path
	path := strings.TrimPrefix(r.URL.Path, "/api/connectors/")
	if path == "" {
		http.Error(w, "Connector ID required", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		// Get specific connector
		info, err := s.connectorManager.GetConnectorInfo(path)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		s.writeJSON(w, info)

	case http.MethodPut:
		// Update connector configuration
		var config connector.ConnectorConfig
		if err := json.NewDecoder(r.Body).Decode(&config); err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}

		if err := s.connectorManager.UpdateConnector(path, config); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		s.writeJSON(w, map[string]interface{}{
			"message": "Connector updated successfully",
		})

	case http.MethodDelete:
		// Delete connector
		if err := s.connectorManager.DeleteConnector(path); err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		s.writeJSON(w, map[string]interface{}{
			"message": "Connector deleted successfully",
		})

	case http.MethodPost:
		// Handle connector actions (start/stop/restart)
		action := r.URL.Query().Get("action")
		var err error

		switch action {
		case "start":
			err = s.connectorManager.StartConnector(path)
		case "stop":
			err = s.connectorManager.StopConnector(path)
		case "restart":
			err = s.connectorManager.RestartConnector(path)
		default:
			http.Error(w, "Invalid action", http.StatusBadRequest)
			return
		}

		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		s.writeJSON(w, map[string]interface{}{
			"message": fmt.Sprintf("Connector %s %sed successfully", path, action),
		})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleAPIConnectorTypes returns available connector types
func (s *Server) handleAPIConnectorTypes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.connectorManager == nil {
		http.Error(w, "Connector manager not available", http.StatusServiceUnavailable)
		return
	}

	types := s.connectorManager.GetConnectorTypes()
	typesData := make([]map[string]interface{}, 0)

	for _, connType := range types {
		schema, err := s.connectorManager.GetConnectorSchema(connType)
		if err != nil {
			continue
		}

		typesData = append(typesData, map[string]interface{}{
			"type":        string(connType),
			"name":        strings.Title(string(connType)),
			"description": getConnectorTypeDescription(connType),
			"schema":      schema,
		})
	}

	data := map[string]interface{}{
		"types": typesData,
		"count": len(typesData),
	}

	s.writeJSON(w, data)
}

// getConnectorTypeDescription returns a description for each connector type
func getConnectorTypeDescription(connType connector.ConnectorType) string {
	switch connType {
	case connector.ConnectorTypeHTTP:
		return "HTTP/HTTPS webhook connector for sending messages to external HTTP endpoints"
	case connector.ConnectorTypeMySQL:
		return "MySQL database connector for storing messages in MySQL database"
	case connector.ConnectorTypePostgreSQL:
		return "PostgreSQL database connector for storing messages in PostgreSQL database"
	case connector.ConnectorTypeRedis:
		return "Redis connector for publishing messages to Redis or storing in Redis data structures"
	case connector.ConnectorTypeKafka:
		return "Apache Kafka connector for publishing messages to Kafka topics"
	default:
		return "External system connector"
	}
}

// Helper methods for connector management

// getConnectorsData prepares data for connectors page
func (s *Server) getConnectorsData() map[string]interface{} {
	var connectors []map[string]interface{}
	var count int

	if s.connectorManager != nil {
		connectorMap := s.connectorManager.ListConnectors()
		connectors = make([]map[string]interface{}, 0, len(connectorMap))

		for id, conn := range connectorMap {
			config := conn.GetConfig()
			status := conn.Status()

			connectors = append(connectors, map[string]interface{}{
				"id":          id,
				"name":        config.Name,
				"type":        string(config.Type),
				"enabled":     config.Enabled,
				"state":       string(status.State),
				"created_at":  config.CreatedAt.Format("2006-01-02 15:04:05"),
				"description": config.Description,
			})
		}
		count = len(connectors)
	}

	return map[string]interface{}{
		"Title":      "Connectors - EMQX Go",
		"Connectors": connectors,
		"Count":      count,
		"Types":      getAvailableConnectorTypes(),
	}
}

// getAvailableConnectorTypes returns available connector types for the UI
func getAvailableConnectorTypes() []map[string]interface{} {
	return []map[string]interface{}{
		{
			"value":       "http",
			"label":       "HTTP",
			"description": "HTTP/HTTPS webhook connector",
		},
		{
			"value":       "mysql",
			"label":       "MySQL",
			"description": "MySQL database connector",
		},
		{
			"value":       "postgresql",
			"label":       "PostgreSQL",
			"description": "PostgreSQL database connector",
		},
		{
			"value":       "redis",
			"label":       "Redis",
			"description": "Redis connector",
		},
		{
			"value":       "kafka",
			"label":       "Kafka",
			"description": "Apache Kafka connector",
		},
	}
}

// Helper methods for certificate management

// getCertificatesData prepares data for certificates page
func (s *Server) getCertificatesData() map[string]interface{} {
	certs := s.certificateManager.ListCertificates()

	return map[string]interface{}{
		"Title":        "Certificates - EMQX Go",
		"Certificates": certs,
		"Count":        len(certs),
	}
}

// getTLSConfigData prepares data for TLS configuration page
func (s *Server) getTLSConfigData() map[string]interface{} {
	return map[string]interface{}{
		"Title": "TLS Configuration - EMQX Go",
		"VerifyModes": []map[string]interface{}{
			{"value": "none", "label": "No Verification"},
			{"value": "verify_peer", "label": "Verify Peer"},
			{"value": "verify_peer_fail_if_no_peer_cert", "label": "Verify Peer (Fail if No Cert)"},
		},
		"CertTypes": []map[string]interface{}{
			{"value": "server", "label": "Server Certificate"},
			{"value": "ca", "label": "CA Certificate"},
			{"value": "client", "label": "Client Certificate"},
		},
		"IdentitySources": []map[string]interface{}{
			{"value": "subject_dn", "label": "Subject DN"},
			{"value": "subject_cn", "label": "Subject Common Name"},
			{"value": "san", "label": "Subject Alternative Names"},
			{"value": "serial", "label": "Serial Number"},
			{"value": "fingerprint", "label": "Fingerprint"},
		},
	}
}

// Rule Engine Management Handlers

// handleRules renders the rules management page
func (s *Server) handleRules(w http.ResponseWriter, r *http.Request) {
	data := s.getRulesData()
	if err := s.templates.ExecuteTemplate(w, "rules.html", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

// handleAPIRules handles rule management API requests
func (s *Server) handleAPIRules(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleAPIGetRules(w, r)
	case http.MethodPost:
		s.handleAPICreateRule(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleAPIRuleByID handles individual rule API requests
func (s *Server) handleAPIRuleByID(w http.ResponseWriter, r *http.Request) {
	// Extract rule ID from URL
	pathParts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/rules/"), "/")
	if len(pathParts) == 0 || pathParts[0] == "" {
		http.Error(w, "Rule ID is required", http.StatusBadRequest)
		return
	}

	ruleID := pathParts[0]

	// Handle enable/disable actions
	if len(pathParts) > 1 {
		action := pathParts[1]
		switch action {
		case "enable":
			s.handleAPIEnableRule(w, r, ruleID)
		case "disable":
			s.handleAPIDisableRule(w, r, ruleID)
		default:
			http.Error(w, "Invalid action", http.StatusBadRequest)
		}
		return
	}

	// Handle CRUD operations
	switch r.Method {
	case http.MethodGet:
		s.handleAPIGetRule(w, r, ruleID)
	case http.MethodPut:
		s.handleAPIUpdateRule(w, r, ruleID)
	case http.MethodDelete:
		s.handleAPIDeleteRule(w, r, ruleID)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleAPIGetRules returns all rules
func (s *Server) handleAPIGetRules(w http.ResponseWriter, r *http.Request) {
	rules := s.ruleEngine.ListRules()

	rulesList := make([]map[string]interface{}, 0)
	for _, rule := range rules {
		rulesList = append(rulesList, map[string]interface{}{
			"id":          rule.ID,
			"name":        rule.Name,
			"description": rule.Description,
			"sql":         rule.SQL,
			"actions":     rule.Actions,
			"status":      rule.Status,
			"priority":    rule.Priority,
			"created_at":  rule.CreatedAt,
			"updated_at":  rule.UpdatedAt,
			"metrics":     rule.Metrics,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(rulesList)
}

// handleAPIGetRule returns a specific rule
func (s *Server) handleAPIGetRule(w http.ResponseWriter, r *http.Request, ruleID string) {
	rule, err := s.ruleEngine.GetRule(ruleID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(rule)
}

// handleAPICreateRule creates a new rule
func (s *Server) handleAPICreateRule(w http.ResponseWriter, r *http.Request) {
	var rule rules.Rule
	if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	if err := s.ruleEngine.CreateRule(rule); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{"message": "Rule created successfully"})
}

// handleAPIUpdateRule updates an existing rule
func (s *Server) handleAPIUpdateRule(w http.ResponseWriter, r *http.Request, ruleID string) {
	var rule rules.Rule
	if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	if err := s.ruleEngine.UpdateRule(ruleID, rule); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"message": "Rule updated successfully"})
}

// handleAPIDeleteRule deletes a rule
func (s *Server) handleAPIDeleteRule(w http.ResponseWriter, r *http.Request, ruleID string) {
	if err := s.ruleEngine.DeleteRule(ruleID); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"message": "Rule deleted successfully"})
}

// handleAPIEnableRule enables a rule
func (s *Server) handleAPIEnableRule(w http.ResponseWriter, r *http.Request, ruleID string) {
	if err := s.ruleEngine.EnableRule(ruleID); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"message": "Rule enabled successfully"})
}

// handleAPIDisableRule disables a rule
func (s *Server) handleAPIDisableRule(w http.ResponseWriter, r *http.Request, ruleID string) {
	if err := s.ruleEngine.DisableRule(ruleID); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"message": "Rule disabled successfully"})
}

// handleAPIActionTypes returns available action types
func (s *Server) handleAPIActionTypes(w http.ResponseWriter, r *http.Request) {
	actionTypes := s.ruleEngine.GetActionTypes()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(actionTypes)
}

// handleAPIActionSchema returns the schema for a specific action type
func (s *Server) handleAPIActionSchema(w http.ResponseWriter, r *http.Request) {
	// Extract action type from URL
	actionType := strings.TrimPrefix(r.URL.Path, "/api/action-schemas/")
	if actionType == "" {
		http.Error(w, "Action type is required", http.StatusBadRequest)
		return
	}

	schema, err := s.ruleEngine.GetActionSchema(rules.ActionType(actionType))
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(schema)
}

// getRulesData prepares data for rules page
func (s *Server) getRulesData() map[string]interface{} {
	rules := s.ruleEngine.ListRules()

	rulesList := make([]map[string]interface{}, 0)
	for _, rule := range rules {
		rulesList = append(rulesList, map[string]interface{}{
			"ID":          rule.ID,
			"Name":        rule.Name,
			"Description": rule.Description,
			"SQL":         rule.SQL,
			"Actions":     rule.Actions,
			"Status":      rule.Status,
			"Priority":    rule.Priority,
			"CreatedAt":   rule.CreatedAt,
			"UpdatedAt":   rule.UpdatedAt,
			"Metrics":     rule.Metrics,
		})
	}

	return map[string]interface{}{
		"Title": "Rules - EMQX Go",
		"Rules": rulesList,
		"Count": len(rulesList),
	}
}

// Data Integration Management Handlers

// handleIntegration serves the data integration management page
func (s *Server) handleIntegration(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	data := s.getIntegrationData()
	s.renderTemplate(w, "integration.html", data)
}

// handleAPIBridges handles bridge CRUD operations
func (s *Server) handleAPIBridges(w http.ResponseWriter, r *http.Request) {
	if s.integrationEngine == nil {
		http.Error(w, "Integration engine not available", http.StatusServiceUnavailable)
		return
	}

	switch r.Method {
	case http.MethodGet:
		// List all bridges
		bridges := s.integrationEngine.ListBridges()
		s.writeJSON(w, bridges)

	case http.MethodPost:
		// Create new bridge
		var bridge integration.Bridge
		if err := json.NewDecoder(r.Body).Decode(&bridge); err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}

		if err := s.integrationEngine.CreateBridge(bridge); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		s.writeJSON(w, map[string]interface{}{
			"message": "Bridge created successfully",
			"id":      bridge.ID,
		})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleAPIBridgeByID handles operations on specific bridges
func (s *Server) handleAPIBridgeByID(w http.ResponseWriter, r *http.Request) {
	if s.integrationEngine == nil {
		http.Error(w, "Integration engine not available", http.StatusServiceUnavailable)
		return
	}

	// Extract bridge ID from URL path
	path := strings.TrimPrefix(r.URL.Path, "/api/integration/bridges/")
	parts := strings.Split(path, "/")
	if len(parts) == 0 || parts[0] == "" {
		http.Error(w, "Bridge ID required", http.StatusBadRequest)
		return
	}

	bridgeID := parts[0]

	// Handle enable/disable actions
	if len(parts) > 1 {
		action := parts[1]
		switch action {
		case "enable":
			if err := s.integrationEngine.EnableBridge(bridgeID); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			s.writeJSON(w, map[string]interface{}{
				"message": "Bridge enabled successfully",
			})
		case "disable":
			if err := s.integrationEngine.DisableBridge(bridgeID); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			s.writeJSON(w, map[string]interface{}{
				"message": "Bridge disabled successfully",
			})
		default:
			http.Error(w, "Invalid action", http.StatusBadRequest)
		}
		return
	}

	// Handle CRUD operations
	switch r.Method {
	case http.MethodGet:
		// Get specific bridge
		bridge, err := s.integrationEngine.GetBridge(bridgeID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		s.writeJSON(w, bridge)

	case http.MethodPut:
		// Update bridge
		var bridge integration.Bridge
		if err := json.NewDecoder(r.Body).Decode(&bridge); err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}

		if err := s.integrationEngine.UpdateBridge(bridgeID, bridge); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		s.writeJSON(w, map[string]interface{}{
			"message": "Bridge updated successfully",
		})

	case http.MethodDelete:
		// Delete bridge
		if err := s.integrationEngine.DeleteBridge(bridgeID); err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		s.writeJSON(w, map[string]interface{}{
			"message": "Bridge deleted successfully",
		})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleAPIIntegrationConnectors returns integration connectors
func (s *Server) handleAPIIntegrationConnectors(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.integrationEngine == nil {
		http.Error(w, "Integration engine not available", http.StatusServiceUnavailable)
		return
	}

	connectors := s.integrationEngine.ListConnectors()
	connectorsData := make([]map[string]interface{}, 0)

	for _, connector := range connectors {
		metrics := connector.GetMetrics()
		connectorsData = append(connectorsData, map[string]interface{}{
			"id":        connector.ID(),
			"type":      connector.Type(),
			"connected": connector.IsConnected(),
			"metrics":   metrics,
		})
	}

	s.writeJSON(w, connectorsData)
}

// handleAPIIntegrationMetrics returns integration metrics
func (s *Server) handleAPIIntegrationMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.integrationEngine == nil {
		http.Error(w, "Integration engine not available", http.StatusServiceUnavailable)
		return
	}

	metrics := s.integrationEngine.GetMetrics()
	s.writeJSON(w, metrics)
}

// getIntegrationData prepares data for integration page
func (s *Server) getIntegrationData() map[string]interface{} {
	var bridges []integration.Bridge
	var metrics *integration.IntegrationMetrics

	if s.integrationEngine != nil {
		bridges = s.integrationEngine.ListBridges()
		metrics = s.integrationEngine.GetMetrics()
	}

	return map[string]interface{}{
		"Title":   "Data Integration - EMQX Go",
		"Bridges": bridges,
		"Count":   len(bridges),
		"Metrics": metrics,
	}
}

// Blacklist Management Handlers

// handleBlacklist serves the blacklist management page
func (s *Server) handleBlacklist(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	data := s.getBlacklistData()
	s.renderTemplate(w, "blacklist.html", data)
}

// handleAPIBlacklist handles blacklist CRUD operations
func (s *Server) handleAPIBlacklist(w http.ResponseWriter, r *http.Request) {
	// For demo purposes, provide mock blacklist data
	// In a full implementation, this would integrate with the actual blacklist middleware

	switch r.Method {
	case http.MethodGet:
		// Handle listing with optional type filter
		typeFilter := r.URL.Query().Get("type")

		// Return mock blacklist entries
		entries := []map[string]interface{}{
			{
				"id":          "demo-blocked-client",
				"type":        "clientid",
				"value":       "malicious-client",
				"action":      "deny",
				"reason":      "Suspicious behavior detected",
				"enabled":     true,
				"created_at":  time.Now().Add(-1 * time.Hour).Format(time.RFC3339),
				"updated_at":  time.Now().Add(-1 * time.Hour).Format(time.RFC3339),
			},
			{
				"id":          "demo-blocked-user",
				"type":        "username",
				"value":       "baduser",
				"action":      "deny",
				"reason":      "Account compromised",
				"enabled":     true,
				"created_at":  time.Now().Add(-2 * time.Hour).Format(time.RFC3339),
				"updated_at":  time.Now().Add(-2 * time.Hour).Format(time.RFC3339),
			},
		}

		// Filter by type if specified
		if typeFilter != "" {
			filtered := make([]map[string]interface{}, 0)
			for _, entry := range entries {
				if entry["type"] == typeFilter {
					filtered = append(filtered, entry)
				}
			}
			entries = filtered
		}

		data := map[string]interface{}{
			"data":  entries,
			"count": len(entries),
		}
		s.writeJSON(w, data)

	case http.MethodPost:
		// Create new blacklist entry
		var entry map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&entry); err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}

		// Validate required fields
		if entry["type"] == nil || entry["action"] == nil {
			http.Error(w, "Type and action are required", http.StatusBadRequest)
			return
		}

		// Generate ID and timestamps
		entryID := fmt.Sprintf("blacklist-entry-%d", time.Now().UnixNano())
		now := time.Now().Format(time.RFC3339)

		entry["id"] = entryID
		entry["created_at"] = now
		entry["updated_at"] = now

		// In a full implementation, this would call the blacklist manager
		response := map[string]interface{}{
			"code":    0,
			"message": "Blacklist entry created successfully",
			"data":    entry,
		}

		w.WriteHeader(http.StatusCreated)
		s.writeJSON(w, response)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleAPIBlacklistByID handles operations on specific blacklist entries
func (s *Server) handleAPIBlacklistByID(w http.ResponseWriter, r *http.Request) {
	// Extract entry ID from URL path
	path := strings.TrimPrefix(r.URL.Path, "/api/blacklist/")
	if path == "" {
		http.Error(w, "Blacklist entry ID required", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		// Get specific blacklist entry
		// For demo purposes, return a mock entry
		entry := map[string]interface{}{
			"id":          path,
			"type":        "clientid",
			"value":       "demo-client",
			"action":      "deny",
			"reason":      "Demo entry",
			"enabled":     true,
			"created_at":  time.Now().Add(-1 * time.Hour).Format(time.RFC3339),
			"updated_at":  time.Now().Add(-1 * time.Hour).Format(time.RFC3339),
		}

		response := map[string]interface{}{
			"code":    0,
			"message": "success",
			"data":    entry,
		}
		s.writeJSON(w, response)

	case http.MethodPut:
		// Update blacklist entry
		var updates map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&updates); err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}

		// Add updated timestamp
		updates["updated_at"] = time.Now().Format(time.RFC3339)

		// In a full implementation, this would call the blacklist manager
		response := map[string]interface{}{
			"code":    0,
			"message": "Blacklist entry updated successfully",
			"data":    updates,
		}
		s.writeJSON(w, response)

	case http.MethodDelete:
		// Delete blacklist entry
		// In a full implementation, this would call the blacklist manager
		response := map[string]interface{}{
			"code":    0,
			"message": "Blacklist entry deleted successfully",
		}
		w.WriteHeader(http.StatusNoContent)
		s.writeJSON(w, response)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleAPIBlacklistStats returns blacklist statistics
func (s *Server) handleAPIBlacklistStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// For demo purposes, return mock statistics
	stats := map[string]interface{}{
		"total_entries": 2,
		"entries_by_type": map[string]interface{}{
			"clientid": 1,
			"username": 1,
			"ip":       0,
			"topic":    0,
		},
		"recent_blocks": []map[string]interface{}{
			{
				"timestamp":   time.Now().Add(-5 * time.Minute).Format(time.RFC3339),
				"client_id":   "malicious-client",
				"reason":      "Blocked by clientid blacklist",
				"entry_type":  "clientid",
			},
		},
	}

	response := map[string]interface{}{
		"code":    0,
		"message": "success",
		"data":    stats,
	}
	s.writeJSON(w, response)
}

// getBlacklistData prepares data for blacklist page
func (s *Server) getBlacklistData() map[string]interface{} {
	// Mock data for demonstration
	entries := []map[string]interface{}{
		{
			"ID":          "demo-blocked-client",
			"Type":        "clientid",
			"Value":       "malicious-client",
			"Action":      "deny",
			"Reason":      "Suspicious behavior detected",
			"Enabled":     true,
			"CreatedAt":   time.Now().Add(-1 * time.Hour).Format("2006-01-02 15:04:05"),
			"UpdatedAt":   time.Now().Add(-1 * time.Hour).Format("2006-01-02 15:04:05"),
		},
		{
			"ID":          "demo-blocked-user",
			"Type":        "username",
			"Value":       "baduser",
			"Action":      "deny",
			"Reason":      "Account compromised",
			"Enabled":     true,
			"CreatedAt":   time.Now().Add(-2 * time.Hour).Format("2006-01-02 15:04:05"),
			"UpdatedAt":   time.Now().Add(-2 * time.Hour).Format("2006-01-02 15:04:05"),
		},
	}

	stats := map[string]interface{}{
		"TotalEntries": 2,
		"EntriesByType": map[string]interface{}{
			"clientid": 1,
			"username": 1,
			"ip":       0,
			"topic":    0,
		},
	}

	return map[string]interface{}{
		"Title":           "Blacklist - EMQX Go",
		"BlacklistEntries": entries,
		"Count":           len(entries),
		"Stats":           stats,
		"Types": []map[string]interface{}{
			{"value": "clientid", "label": "Client ID"},
			{"value": "username", "label": "Username"},
			{"value": "ip", "label": "IP Address"},
			{"value": "topic", "label": "Topic"},
		},
		"Actions": []map[string]interface{}{
			{"value": "deny", "label": "Deny"},
			{"value": "log", "label": "Log Only"},
		},
	}
}