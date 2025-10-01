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

package connector

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// HTTPConnectorConfig holds HTTP-specific configuration
type HTTPConnectorConfig struct {
	URL                string            `json:"url" yaml:"url"`
	Method             string            `json:"method" yaml:"method"`
	Headers            map[string]string `json:"headers" yaml:"headers"`
	Timeout            time.Duration     `json:"timeout" yaml:"timeout"`
	MaxIdleConns       int               `json:"max_idle_conns" yaml:"max_idle_conns"`
	MaxConnsPerHost    int               `json:"max_conns_per_host" yaml:"max_conns_per_host"`
	IdleConnTimeout    time.Duration     `json:"idle_conn_timeout" yaml:"idle_conn_timeout"`
	TLSHandshakeTimeout time.Duration    `json:"tls_handshake_timeout" yaml:"tls_handshake_timeout"`
	ExpectContinueTimeout time.Duration  `json:"expect_continue_timeout" yaml:"expect_continue_timeout"`
	InsecureSkipVerify bool              `json:"insecure_skip_verify" yaml:"insecure_skip_verify"`
	EnableKeepAlive    bool              `json:"enable_keepalive" yaml:"enable_keepalive"`
	BasicAuth          *BasicAuth        `json:"basic_auth,omitempty" yaml:"basic_auth,omitempty"`
	BearerToken        string            `json:"bearer_token,omitempty" yaml:"bearer_token,omitempty"`
	RequestTemplate    string            `json:"request_template,omitempty" yaml:"request_template,omitempty"`
	ResponseHandler    string            `json:"response_handler,omitempty" yaml:"response_handler,omitempty"`
}

// BasicAuth represents HTTP basic authentication
type BasicAuth struct {
	Username string `json:"username" yaml:"username"`
	Password string `json:"password" yaml:"password"`
}

// HTTPConnector implements an HTTP/HTTPS connector
type HTTPConnector struct {
	*BaseConnector
	httpConfig HTTPConnectorConfig
	client     *http.Client
}

// HTTPConnectorFactory creates HTTP connectors
type HTTPConnectorFactory struct{}

// Type returns the connector type
func (f *HTTPConnectorFactory) Type() ConnectorType {
	return ConnectorTypeHTTP
}

// Create creates a new HTTP connector
func (f *HTTPConnectorFactory) Create(config ConnectorConfig) (Connector, error) {
	httpConfig, err := f.parseHTTPConfig(config.Parameters)
	if err != nil {
		return nil, fmt.Errorf("invalid HTTP configuration: %w", err)
	}

	baseConnector := NewBaseConnector(config)

	connector := &HTTPConnector{
		BaseConnector: baseConnector,
		httpConfig:    httpConfig,
	}

	// Initialize HTTP client
	connector.initHTTPClient()

	return connector, nil
}

// ValidateConfig validates the HTTP connector configuration
func (f *HTTPConnectorFactory) ValidateConfig(config ConnectorConfig) error {
	_, err := f.parseHTTPConfig(config.Parameters)
	return err
}

// GetDefaultConfig returns default HTTP connector configuration
func (f *HTTPConnectorFactory) GetDefaultConfig() ConnectorConfig {
	return ConnectorConfig{
		Type:        ConnectorTypeHTTP,
		Enabled:     false,
		HealthCheck: DefaultHealthCheckConfig(),
		Retry:       DefaultRetryConfig(),
		Parameters: map[string]interface{}{
			"url":                      "http://localhost:8080/webhook",
			"method":                   "POST",
			"timeout":                  "30s",
			"max_idle_conns":           100,
			"max_conns_per_host":       10,
			"idle_conn_timeout":        "90s",
			"tls_handshake_timeout":    "10s",
			"expect_continue_timeout":  "1s",
			"enable_keepalive":         true,
			"insecure_skip_verify":     false,
			"headers": map[string]string{
				"Content-Type": "application/json",
				"User-Agent":   "emqx-go-connector",
			},
		},
	}
}

// GetConfigSchema returns the configuration schema
func (f *HTTPConnectorFactory) GetConfigSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"url": map[string]interface{}{
				"type":        "string",
				"description": "HTTP/HTTPS endpoint URL",
				"format":      "uri",
			},
			"method": map[string]interface{}{
				"type":        "string",
				"description": "HTTP method",
				"enum":        []string{"GET", "POST", "PUT", "DELETE", "PATCH"},
				"default":     "POST",
			},
			"timeout": map[string]interface{}{
				"type":        "string",
				"description": "Request timeout duration",
				"default":     "30s",
			},
			"headers": map[string]interface{}{
				"type":        "object",
				"description": "HTTP headers to include in requests",
				"additionalProperties": map[string]interface{}{
					"type": "string",
				},
			},
			"basic_auth": map[string]interface{}{
				"type":        "object",
				"description": "HTTP basic authentication",
				"properties": map[string]interface{}{
					"username": map[string]interface{}{
						"type": "string",
					},
					"password": map[string]interface{}{
						"type": "string",
					},
				},
			},
			"bearer_token": map[string]interface{}{
				"type":        "string",
				"description": "Bearer token for authentication",
			},
			"insecure_skip_verify": map[string]interface{}{
				"type":        "boolean",
				"description": "Skip TLS certificate verification",
				"default":     false,
			},
		},
		"required": []string{"url"},
	}
}

// parseHTTPConfig parses HTTP-specific configuration from parameters
func (f *HTTPConnectorFactory) parseHTTPConfig(params map[string]interface{}) (HTTPConnectorConfig, error) {
	config := HTTPConnectorConfig{
		Method:                "POST",
		Timeout:               30 * time.Second,
		MaxIdleConns:          100,
		MaxConnsPerHost:       10,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		EnableKeepAlive:       true,
		Headers:               make(map[string]string),
	}

	// Parse URL
	if urlStr, ok := params["url"].(string); ok {
		if _, err := url.Parse(urlStr); err != nil {
			return config, fmt.Errorf("invalid URL: %w", err)
		}
		config.URL = urlStr
	} else {
		return config, fmt.Errorf("url is required")
	}

	// Parse method
	if method, ok := params["method"].(string); ok {
		config.Method = strings.ToUpper(method)
	}

	// Parse timeout
	if timeoutStr, ok := params["timeout"].(string); ok {
		if timeout, err := time.ParseDuration(timeoutStr); err == nil {
			config.Timeout = timeout
		}
	} else if timeoutFloat, ok := params["timeout"].(float64); ok {
		config.Timeout = time.Duration(timeoutFloat) * time.Second
	}

	// Parse connection settings
	if maxIdle, ok := params["max_idle_conns"]; ok {
		if val, err := convertToInt(maxIdle); err == nil {
			config.MaxIdleConns = val
		}
	}

	if maxConns, ok := params["max_conns_per_host"]; ok {
		if val, err := convertToInt(maxConns); err == nil {
			config.MaxConnsPerHost = val
		}
	}

	// Parse timeouts
	if idleTimeout, ok := params["idle_conn_timeout"].(string); ok {
		if timeout, err := time.ParseDuration(idleTimeout); err == nil {
			config.IdleConnTimeout = timeout
		}
	}

	if tlsTimeout, ok := params["tls_handshake_timeout"].(string); ok {
		if timeout, err := time.ParseDuration(tlsTimeout); err == nil {
			config.TLSHandshakeTimeout = timeout
		}
	}

	if expectTimeout, ok := params["expect_continue_timeout"].(string); ok {
		if timeout, err := time.ParseDuration(expectTimeout); err == nil {
			config.ExpectContinueTimeout = timeout
		}
	}

	// Parse boolean flags
	if insecure, ok := params["insecure_skip_verify"].(bool); ok {
		config.InsecureSkipVerify = insecure
	}

	if keepAlive, ok := params["enable_keepalive"].(bool); ok {
		config.EnableKeepAlive = keepAlive
	}

	// Parse headers
	if headers, ok := params["headers"].(map[string]interface{}); ok {
		for k, v := range headers {
			if str, ok := v.(string); ok {
				config.Headers[k] = str
			}
		}
	}

	// Parse basic auth
	if basicAuth, ok := params["basic_auth"].(map[string]interface{}); ok {
		config.BasicAuth = &BasicAuth{}
		if username, ok := basicAuth["username"].(string); ok {
			config.BasicAuth.Username = username
		}
		if password, ok := basicAuth["password"].(string); ok {
			config.BasicAuth.Password = password
		}
	}

	// Parse bearer token
	if token, ok := params["bearer_token"].(string); ok {
		config.BearerToken = token
	}

	// Parse request template
	if template, ok := params["request_template"].(string); ok {
		config.RequestTemplate = template
	}

	// Parse response handler
	if handler, ok := params["response_handler"].(string); ok {
		config.ResponseHandler = handler
	}

	return config, nil
}

// convertToInt converts various types to int
func convertToInt(val interface{}) (int, error) {
	switch v := val.(type) {
	case int:
		return v, nil
	case float64:
		return int(v), nil
	case string:
		return strconv.Atoi(v)
	default:
		return 0, fmt.Errorf("cannot convert %T to int", val)
	}
}

// initHTTPClient initializes the HTTP client
func (hc *HTTPConnector) initHTTPClient() {
	transport := &http.Transport{
		MaxIdleConns:          hc.httpConfig.MaxIdleConns,
		MaxIdleConnsPerHost:   hc.httpConfig.MaxConnsPerHost,
		IdleConnTimeout:       hc.httpConfig.IdleConnTimeout,
		TLSHandshakeTimeout:   hc.httpConfig.TLSHandshakeTimeout,
		ExpectContinueTimeout: hc.httpConfig.ExpectContinueTimeout,
		DisableKeepAlives:     !hc.httpConfig.EnableKeepAlive,
	}

	if hc.httpConfig.InsecureSkipVerify {
		transport.TLSClientConfig = &tls.Config{
			InsecureSkipVerify: true,
		}
	}

	hc.client = &http.Client{
		Transport: transport,
		Timeout:   hc.httpConfig.Timeout,
	}
}

// Start starts the HTTP connector
func (hc *HTTPConnector) Start(ctx context.Context) error {
	if hc.IsRunning() {
		return ErrConnectorAlreadyRunning
	}

	hc.setState(StateStarting)

	// Perform initial health check
	if err := hc.HealthCheck(ctx); err != nil {
		hc.setError(err)
		hc.setState(StateFailed)
		return err
	}

	now := time.Now()
	hc.startTime = &now
	hc.setState(StateRunning)

	// Start health checking
	hc.startHealthCheck(hc.HealthCheck)

	return nil
}

// Stop stops the HTTP connector
func (hc *HTTPConnector) Stop(ctx context.Context) error {
	if !hc.IsRunning() {
		return ErrConnectorNotRunning
	}

	hc.setState(StateStopping)

	// Close HTTP client transport
	if transport, ok := hc.client.Transport.(*http.Transport); ok {
		transport.CloseIdleConnections()
	}

	hc.setState(StateStopped)
	return nil
}

// Restart restarts the HTTP connector
func (hc *HTTPConnector) Restart(ctx context.Context) error {
	if err := hc.Stop(ctx); err != nil {
		return err
	}
	return hc.Start(ctx)
}

// HealthCheck performs a health check
func (hc *HTTPConnector) HealthCheck(ctx context.Context) error {
	// Create a simple HEAD or GET request to check connectivity
	method := "HEAD"
	if hc.httpConfig.Method == "GET" {
		method = "GET"
	}

	req, err := http.NewRequestWithContext(ctx, method, hc.httpConfig.URL, nil)
	if err != nil {
		return fmt.Errorf("failed to create health check request: %w", err)
	}

	// Add headers
	for k, v := range hc.httpConfig.Headers {
		req.Header.Set(k, v)
	}

	// Add authentication
	hc.addAuthentication(req)

	resp, err := hc.client.Do(req)
	if err != nil {
		return fmt.Errorf("health check request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("health check failed with status: %d", resp.StatusCode)
	}

	return nil
}

// Send sends a message through the HTTP connector
func (hc *HTTPConnector) Send(ctx context.Context, message *Message) (*MessageResult, error) {
	if !hc.IsRunning() {
		return nil, ErrConnectorNotRunning
	}

	start := time.Now()

	result := &MessageResult{
		MessageID: message.ID,
		Timestamp: start,
	}

	// Prepare request body
	var body io.Reader
	if len(message.Payload) > 0 {
		body = bytes.NewReader(message.Payload)
	}

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, hc.httpConfig.Method, hc.httpConfig.URL, body)
	if err != nil {
		result.Error = err.Error()
		hc.setError(err)
		hc.recordDroppedMessage()
		return result, err
	}

	// Add headers
	for k, v := range hc.httpConfig.Headers {
		req.Header.Set(k, v)
	}

	// Add message headers
	for k, v := range message.Headers {
		req.Header.Set(k, v)
	}

	// Add topic as header if present
	if message.Topic != "" {
		req.Header.Set("X-MQTT-Topic", message.Topic)
	}

	// Add authentication
	hc.addAuthentication(req)

	// Send request
	resp, err := hc.client.Do(req)
	if err != nil {
		result.Error = err.Error()
		hc.setError(err)
		hc.recordDroppedMessage()
		return result, err
	}
	defer resp.Body.Close()

	// Calculate latency
	latency := time.Since(start)
	result.Latency = latency

	// Check response status
	if resp.StatusCode >= 400 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		result.Error = fmt.Sprintf("HTTP %d: %s", resp.StatusCode, string(bodyBytes))
		hc.setError(fmt.Errorf("HTTP request failed with status %d", resp.StatusCode))
		hc.recordDroppedMessage()
		return result, fmt.Errorf("HTTP request failed with status %d", resp.StatusCode)
	}

	// Success
	result.Success = true
	hc.recordSuccess(latency)
	hc.recordSentMessage()

	return result, nil
}

// SendBatch sends multiple messages
func (hc *HTTPConnector) SendBatch(ctx context.Context, messages []*Message) ([]*MessageResult, error) {
	results := make([]*MessageResult, len(messages))

	for i, message := range messages {
		result, err := hc.Send(ctx, message)
		results[i] = result
		if err != nil {
			// Continue with other messages even if one fails
			continue
		}
	}

	return results, nil
}

// addAuthentication adds authentication to the request
func (hc *HTTPConnector) addAuthentication(req *http.Request) {
	if hc.httpConfig.BasicAuth != nil {
		req.SetBasicAuth(hc.httpConfig.BasicAuth.Username, hc.httpConfig.BasicAuth.Password)
	}

	if hc.httpConfig.BearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+hc.httpConfig.BearerToken)
	}
}

// Close closes the HTTP connector
func (hc *HTTPConnector) Close() error {
	if transport, ok := hc.client.Transport.(*http.Transport); ok {
		transport.CloseIdleConnections()
	}

	return hc.BaseConnector.Close()
}