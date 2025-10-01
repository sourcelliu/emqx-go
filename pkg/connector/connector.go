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

// Package connector provides connector management functionality for emqx-go.
// It implements a connector system similar to EMQX for integrating with external systems.
package connector

import (
	"context"
	"encoding/json"
	"sync"
	"time"
)

// ConnectorType represents different types of connectors
type ConnectorType string

const (
	ConnectorTypeHTTP       ConnectorType = "http"
	ConnectorTypeMySQL      ConnectorType = "mysql"
	ConnectorTypePostgreSQL ConnectorType = "postgresql"
	ConnectorTypeMongoDB    ConnectorType = "mongodb"
	ConnectorTypeRedis      ConnectorType = "redis"
	ConnectorTypeKafka      ConnectorType = "kafka"
	ConnectorTypeRabbitMQ   ConnectorType = "rabbitmq"
	ConnectorTypeNATS       ConnectorType = "nats"
	ConnectorTypeWebhook    ConnectorType = "webhook"
)

// ConnectorState represents the current state of a connector
type ConnectorState string

const (
	StateCreated      ConnectorState = "created"
	StateStarting     ConnectorState = "starting"
	StateRunning      ConnectorState = "running"
	StateStopping     ConnectorState = "stopping"
	StateStopped      ConnectorState = "stopped"
	StateFailed       ConnectorState = "failed"
	StateReconnecting ConnectorState = "reconnecting"
)

// ConnectorConfig defines the configuration structure for connectors
type ConnectorConfig struct {
	ID          string                 `json:"id" yaml:"id"`
	Name        string                 `json:"name" yaml:"name"`
	Type        ConnectorType          `json:"type" yaml:"type"`
	Description string                 `json:"description,omitempty" yaml:"description,omitempty"`
	Enabled     bool                   `json:"enabled" yaml:"enabled"`
	Parameters  map[string]interface{} `json:"parameters" yaml:"parameters"`
	HealthCheck HealthCheckConfig      `json:"health_check" yaml:"health_check"`
	Retry       RetryConfig            `json:"retry" yaml:"retry"`
	Pool        PoolConfig             `json:"pool,omitempty" yaml:"pool,omitempty"`
	Tags        []string               `json:"tags,omitempty" yaml:"tags,omitempty"`
	CreatedAt   time.Time              `json:"created_at" yaml:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at" yaml:"updated_at"`
}

// HealthCheckConfig defines health check settings for connectors
type HealthCheckConfig struct {
	Enabled  bool          `json:"enabled" yaml:"enabled"`
	Interval time.Duration `json:"interval" yaml:"interval"`
	Timeout  time.Duration `json:"timeout" yaml:"timeout"`
	MaxFails int           `json:"max_fails" yaml:"max_fails"`
}

// RetryConfig defines retry settings for connectors
type RetryConfig struct {
	MaxAttempts       int           `json:"max_attempts" yaml:"max_attempts"`
	InitialBackoff    time.Duration `json:"initial_backoff" yaml:"initial_backoff"`
	MaxBackoff        time.Duration `json:"max_backoff" yaml:"max_backoff"`
	BackoffMultiplier float64       `json:"backoff_multiplier" yaml:"backoff_multiplier"`
}

// PoolConfig defines connection pool settings
type PoolConfig struct {
	MaxSize     int           `json:"max_size" yaml:"max_size"`
	MinSize     int           `json:"min_size" yaml:"min_size"`
	MaxIdle     time.Duration `json:"max_idle" yaml:"max_idle"`
	MaxLifetime time.Duration `json:"max_lifetime" yaml:"max_lifetime"`
}

// ConnectorStatus represents the current status and metrics of a connector
type ConnectorStatus struct {
	ID             string           `json:"id"`
	State          ConnectorState   `json:"state"`
	LastError      string           `json:"last_error,omitempty"`
	LastErrorTime  *time.Time       `json:"last_error_time,omitempty"`
	StartTime      *time.Time       `json:"start_time,omitempty"`
	UpdateTime     time.Time        `json:"update_time"`
	Metrics        ConnectorMetrics `json:"metrics"`
	HealthStatus   HealthStatus     `json:"health_status"`
	NextRetryTime  *time.Time       `json:"next_retry_time,omitempty"`
	RetryCount     int              `json:"retry_count"`
}

// ConnectorMetrics holds performance metrics for a connector
type ConnectorMetrics struct {
	MessagesSent       int64         `json:"messages_sent"`
	MessagesReceived   int64         `json:"messages_received"`
	MessagesDropped    int64         `json:"messages_dropped"`
	ErrorCount         int64         `json:"error_count"`
	SuccessCount       int64         `json:"success_count"`
	AvgLatency         time.Duration `json:"avg_latency"`
	MinLatency         time.Duration `json:"min_latency"`
	MaxLatency         time.Duration `json:"max_latency"`
	ThroughputPerSec   float64       `json:"throughput_per_sec"`
	ActiveConnections  int           `json:"active_connections"`
	LastMessageTime    *time.Time    `json:"last_message_time,omitempty"`
}

// HealthStatus represents the health status of a connector
type HealthStatus struct {
	Healthy       bool      `json:"healthy"`
	LastCheckTime time.Time `json:"last_check_time"`
	CheckCount    int64     `json:"check_count"`
	FailureCount  int64     `json:"failure_count"`
	Message       string    `json:"message,omitempty"`
}

// ConnectorInfo contains comprehensive information about a connector
type ConnectorInfo struct {
	Config ConnectorConfig `json:"config"`
	Status ConnectorStatus `json:"status"`
}

// Message represents a message to be sent through a connector
type Message struct {
	ID        string                 `json:"id"`
	Topic     string                 `json:"topic,omitempty"`
	Payload   []byte                 `json:"payload"`
	Headers   map[string]string      `json:"headers,omitempty"`
	Timestamp time.Time              `json:"timestamp"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// MessageResult represents the result of sending a message
type MessageResult struct {
	MessageID string    `json:"message_id"`
	Success   bool      `json:"success"`
	Error     string    `json:"error,omitempty"`
	Latency   time.Duration `json:"latency"`
	Timestamp time.Time `json:"timestamp"`
}

// Connector defines the interface that all connectors must implement
type Connector interface {
	// Life cycle management
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	Restart(ctx context.Context) error

	// Status and health
	Status() ConnectorStatus
	HealthCheck(ctx context.Context) error
	IsRunning() bool

	// Data operations
	Send(ctx context.Context, message *Message) (*MessageResult, error)
	SendBatch(ctx context.Context, messages []*Message) ([]*MessageResult, error)

	// Configuration management
	GetConfig() ConnectorConfig
	UpdateConfig(ctx context.Context, config ConnectorConfig) error

	// Metrics and monitoring
	GetMetrics() ConnectorMetrics
	ResetMetrics()

	// Resource management
	Close() error
}

// ConnectorFactory defines the interface for creating connectors
type ConnectorFactory interface {
	Type() ConnectorType
	Create(config ConnectorConfig) (Connector, error)
	ValidateConfig(config ConnectorConfig) error
	GetDefaultConfig() ConnectorConfig
	GetConfigSchema() map[string]interface{}
}

// ConnectorManager manages all connectors in the system
type ConnectorManager struct {
	connectors map[string]Connector
	factories  map[ConnectorType]ConnectorFactory
	mu         sync.RWMutex
	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup
}

// NewConnectorManager creates a new connector manager
func NewConnectorManager() *ConnectorManager {
	ctx, cancel := context.WithCancel(context.Background())
	return &ConnectorManager{
		connectors: make(map[string]Connector),
		factories:  make(map[ConnectorType]ConnectorFactory),
		ctx:        ctx,
		cancel:     cancel,
	}
}

// RegisterFactory registers a connector factory
func (cm *ConnectorManager) RegisterFactory(factory ConnectorFactory) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.factories[factory.Type()] = factory
}

// CreateConnector creates a new connector with the given configuration
func (cm *ConnectorManager) CreateConnector(config ConnectorConfig) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if _, exists := cm.connectors[config.ID]; exists {
		return ErrConnectorExists
	}

	factory, exists := cm.factories[config.Type]
	if !exists {
		return ErrConnectorTypeNotSupported
	}

	if err := factory.ValidateConfig(config); err != nil {
		return err
	}

	connector, err := factory.Create(config)
	if err != nil {
		return err
	}

	cm.connectors[config.ID] = connector

	// Start the connector if it's enabled
	if config.Enabled {
		go func() {
			if err := connector.Start(cm.ctx); err != nil {
				// Log error but don't block creation
				// TODO: Add proper logging
			}
		}()
	}

	return nil
}

// GetConnector retrieves a connector by ID
func (cm *ConnectorManager) GetConnector(id string) (Connector, error) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	connector, exists := cm.connectors[id]
	if !exists {
		return nil, ErrConnectorNotFound
	}

	return connector, nil
}

// ListConnectors returns all connectors
func (cm *ConnectorManager) ListConnectors() map[string]Connector {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	connectors := make(map[string]Connector)
	for id, connector := range cm.connectors {
		connectors[id] = connector
	}

	return connectors
}

// GetConnectorInfo returns comprehensive information about a connector
func (cm *ConnectorManager) GetConnectorInfo(id string) (*ConnectorInfo, error) {
	connector, err := cm.GetConnector(id)
	if err != nil {
		return nil, err
	}

	return &ConnectorInfo{
		Config: connector.GetConfig(),
		Status: connector.Status(),
	}, nil
}

// UpdateConnector updates a connector's configuration
func (cm *ConnectorManager) UpdateConnector(id string, config ConnectorConfig) error {
	connector, err := cm.GetConnector(id)
	if err != nil {
		return err
	}

	factory, exists := cm.factories[config.Type]
	if !exists {
		return ErrConnectorTypeNotSupported
	}

	if err := factory.ValidateConfig(config); err != nil {
		return err
	}

	return connector.UpdateConfig(cm.ctx, config)
}

// DeleteConnector removes a connector
func (cm *ConnectorManager) DeleteConnector(id string) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	connector, exists := cm.connectors[id]
	if !exists {
		return ErrConnectorNotFound
	}

	// Stop connector if it's running (ignore error if already stopped)
	if connector.IsRunning() {
		if err := connector.Stop(cm.ctx); err != nil {
			return err
		}
	}

	if err := connector.Close(); err != nil {
		return err
	}

	delete(cm.connectors, id)
	return nil
}

// StartConnector starts a connector
func (cm *ConnectorManager) StartConnector(id string) error {
	connector, err := cm.GetConnector(id)
	if err != nil {
		return err
	}

	return connector.Start(cm.ctx)
}

// StopConnector stops a connector
func (cm *ConnectorManager) StopConnector(id string) error {
	connector, err := cm.GetConnector(id)
	if err != nil {
		return err
	}

	return connector.Stop(cm.ctx)
}

// RestartConnector restarts a connector
func (cm *ConnectorManager) RestartConnector(id string) error {
	connector, err := cm.GetConnector(id)
	if err != nil {
		return err
	}

	return connector.Restart(cm.ctx)
}

// GetConnectorTypes returns all registered connector types
func (cm *ConnectorManager) GetConnectorTypes() []ConnectorType {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	types := make([]ConnectorType, 0, len(cm.factories))
	for t := range cm.factories {
		types = append(types, t)
	}

	return types
}

// GetConnectorSchema returns the configuration schema for a connector type
func (cm *ConnectorManager) GetConnectorSchema(connectorType ConnectorType) (map[string]interface{}, error) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	factory, exists := cm.factories[connectorType]
	if !exists {
		return nil, ErrConnectorTypeNotSupported
	}

	return factory.GetConfigSchema(), nil
}

// HealthCheckAll performs health checks on all running connectors
func (cm *ConnectorManager) HealthCheckAll(ctx context.Context) map[string]error {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	results := make(map[string]error)
	for id, connector := range cm.connectors {
		if connector.IsRunning() {
			results[id] = connector.HealthCheck(ctx)
		}
	}

	return results
}

// GetAllMetrics returns metrics for all connectors
func (cm *ConnectorManager) GetAllMetrics() map[string]ConnectorMetrics {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	metrics := make(map[string]ConnectorMetrics)
	for id, connector := range cm.connectors {
		metrics[id] = connector.GetMetrics()
	}

	return metrics
}

// Close gracefully shuts down the connector manager
func (cm *ConnectorManager) Close() error {
	cm.cancel()

	cm.mu.Lock()
	defer cm.mu.Unlock()

	for _, connector := range cm.connectors {
		if err := connector.Stop(context.Background()); err != nil {
			// Log error but continue cleanup
		}
		if err := connector.Close(); err != nil {
			// Log error but continue cleanup
		}
	}

	cm.wg.Wait()
	return nil
}

// MarshalJSON implements custom JSON marshaling for ConnectorConfig
func (c ConnectorConfig) MarshalJSON() ([]byte, error) {
	type Alias ConnectorConfig
	return json.Marshal(&struct {
		CreatedAt string `json:"created_at"`
		UpdatedAt string `json:"updated_at"`
		*Alias
	}{
		CreatedAt: c.CreatedAt.Format(time.RFC3339),
		UpdatedAt: c.UpdatedAt.Format(time.RFC3339),
		Alias:     (*Alias)(&c),
	})
}

// UnmarshalJSON implements custom JSON unmarshaling for ConnectorConfig
func (c *ConnectorConfig) UnmarshalJSON(data []byte) error {
	type Alias ConnectorConfig
	aux := &struct {
		CreatedAt string `json:"created_at"`
		UpdatedAt string `json:"updated_at"`
		*Alias
	}{
		Alias: (*Alias)(c),
	}

	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}

	if aux.CreatedAt != "" {
		if t, err := time.Parse(time.RFC3339, aux.CreatedAt); err == nil {
			c.CreatedAt = t
		}
	}

	if aux.UpdatedAt != "" {
		if t, err := time.Parse(time.RFC3339, aux.UpdatedAt); err == nil {
			c.UpdatedAt = t
		}
	}

	return nil
}