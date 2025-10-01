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
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	_ "github.com/lib/pq" // PostgreSQL driver
)

// PostgreSQLConnectorConfig holds PostgreSQL-specific configuration
type PostgreSQLConnectorConfig struct {
	Host            string        `json:"host" yaml:"host"`
	Port            int           `json:"port" yaml:"port"`
	Username        string        `json:"username" yaml:"username"`
	Password        string        `json:"password" yaml:"password"`
	Database        string        `json:"database" yaml:"database"`
	Schema          string        `json:"schema" yaml:"schema"`
	Table           string        `json:"table" yaml:"table"`
	MaxOpenConns    int           `json:"max_open_conns" yaml:"max_open_conns"`
	MaxIdleConns    int           `json:"max_idle_conns" yaml:"max_idle_conns"`
	ConnMaxLifetime time.Duration `json:"conn_max_lifetime" yaml:"conn_max_lifetime"`
	ConnMaxIdleTime time.Duration `json:"conn_max_idle_time" yaml:"conn_max_idle_time"`
	SSLMode         string        `json:"ssl_mode" yaml:"ssl_mode"`
	Query           string        `json:"query" yaml:"query"`
	BatchSize       int           `json:"batch_size" yaml:"batch_size"`
	Timeout         time.Duration `json:"timeout" yaml:"timeout"`
}

// PostgreSQLConnector implements a PostgreSQL database connector
type PostgreSQLConnector struct {
	*BaseConnector
	pgConfig PostgreSQLConnectorConfig
	db       *sql.DB
}

// PostgreSQLConnectorFactory creates PostgreSQL connectors
type PostgreSQLConnectorFactory struct{}

// Type returns the connector type
func (f *PostgreSQLConnectorFactory) Type() ConnectorType {
	return ConnectorTypePostgreSQL
}

// Create creates a new PostgreSQL connector
func (f *PostgreSQLConnectorFactory) Create(config ConnectorConfig) (Connector, error) {
	pgConfig, err := f.parsePostgreSQLConfig(config.Parameters)
	if err != nil {
		return nil, fmt.Errorf("invalid PostgreSQL configuration: %w", err)
	}

	baseConnector := NewBaseConnector(config)

	connector := &PostgreSQLConnector{
		BaseConnector: baseConnector,
		pgConfig:      pgConfig,
	}

	// Initialize database connection
	if err := connector.initConnection(); err != nil {
		return nil, fmt.Errorf("failed to initialize PostgreSQL connection: %w", err)
	}

	return connector, nil
}

// ValidateConfig validates the PostgreSQL connector configuration
func (f *PostgreSQLConnectorFactory) ValidateConfig(config ConnectorConfig) error {
	_, err := f.parsePostgreSQLConfig(config.Parameters)
	return err
}

// GetDefaultConfig returns default PostgreSQL connector configuration
func (f *PostgreSQLConnectorFactory) GetDefaultConfig() ConnectorConfig {
	return ConnectorConfig{
		Type:        ConnectorTypePostgreSQL,
		Enabled:     false,
		HealthCheck: DefaultHealthCheckConfig(),
		Retry:       DefaultRetryConfig(),
		Parameters: map[string]interface{}{
			"host":               "localhost",
			"port":               5432,
			"username":           "postgres",
			"password":           "",
			"database":           "emqx",
			"schema":             "public",
			"table":              "mqtt_messages",
			"max_open_conns":     10,
			"max_idle_conns":     5,
			"conn_max_lifetime":  "1h",
			"conn_max_idle_time": "10m",
			"ssl_mode":           "disable",
			"query":              "INSERT INTO mqtt_messages (topic, payload, qos, timestamp) VALUES ($1, $2, $3, $4)",
			"batch_size":         1000,
			"timeout":            "30s",
		},
	}
}

// GetConfigSchema returns the configuration schema
func (f *PostgreSQLConnectorFactory) GetConfigSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"host": map[string]interface{}{
				"type":        "string",
				"description": "PostgreSQL server host",
				"default":     "localhost",
			},
			"port": map[string]interface{}{
				"type":        "integer",
				"description": "PostgreSQL server port",
				"default":     5432,
				"minimum":     1,
				"maximum":     65535,
			},
			"username": map[string]interface{}{
				"type":        "string",
				"description": "PostgreSQL username",
			},
			"password": map[string]interface{}{
				"type":        "string",
				"description": "PostgreSQL password",
			},
			"database": map[string]interface{}{
				"type":        "string",
				"description": "PostgreSQL database name",
			},
			"schema": map[string]interface{}{
				"type":        "string",
				"description": "PostgreSQL schema name",
				"default":     "public",
			},
			"table": map[string]interface{}{
				"type":        "string",
				"description": "Target table name",
			},
			"query": map[string]interface{}{
				"type":        "string",
				"description": "SQL query template for inserting messages",
			},
			"ssl_mode": map[string]interface{}{
				"type":        "string",
				"description": "SSL mode for connection",
				"enum":        []string{"disable", "require", "verify-ca", "verify-full"},
				"default":     "disable",
			},
		},
		"required": []string{"host", "username", "database", "table"},
	}
}

// parsePostgreSQLConfig parses PostgreSQL-specific configuration from parameters
func (f *PostgreSQLConnectorFactory) parsePostgreSQLConfig(params map[string]interface{}) (PostgreSQLConnectorConfig, error) {
	config := PostgreSQLConnectorConfig{
		Port:            5432,
		Schema:          "public",
		MaxOpenConns:    10,
		MaxIdleConns:    5,
		ConnMaxLifetime: 1 * time.Hour,
		ConnMaxIdleTime: 10 * time.Minute,
		SSLMode:         "disable",
		BatchSize:       1000,
		Timeout:         30 * time.Second,
	}

	// Parse host
	if host, ok := params["host"].(string); ok {
		config.Host = host
	} else {
		return config, fmt.Errorf("host is required")
	}

	// Parse port
	if port, ok := params["port"]; ok {
		if val, err := convertToInt(port); err == nil {
			config.Port = val
		}
	}

	// Parse username
	if username, ok := params["username"].(string); ok {
		config.Username = username
	} else {
		return config, fmt.Errorf("username is required")
	}

	// Parse password
	if password, ok := params["password"].(string); ok {
		config.Password = password
	}

	// Parse database
	if database, ok := params["database"].(string); ok {
		config.Database = database
	} else {
		return config, fmt.Errorf("database is required")
	}

	// Parse schema
	if schema, ok := params["schema"].(string); ok {
		config.Schema = schema
	}

	// Parse table
	if table, ok := params["table"].(string); ok {
		config.Table = table
	} else {
		return config, fmt.Errorf("table is required")
	}

	// Parse connection pool settings
	if maxOpen, ok := params["max_open_conns"]; ok {
		if val, err := convertToInt(maxOpen); err == nil {
			config.MaxOpenConns = val
		}
	}

	if maxIdle, ok := params["max_idle_conns"]; ok {
		if val, err := convertToInt(maxIdle); err == nil {
			config.MaxIdleConns = val
		}
	}

	// Parse timeouts
	if lifetimeStr, ok := params["conn_max_lifetime"].(string); ok {
		if lifetime, err := time.ParseDuration(lifetimeStr); err == nil {
			config.ConnMaxLifetime = lifetime
		}
	}

	if idleTimeStr, ok := params["conn_max_idle_time"].(string); ok {
		if idleTime, err := time.ParseDuration(idleTimeStr); err == nil {
			config.ConnMaxIdleTime = idleTime
		}
	}

	if timeoutStr, ok := params["timeout"].(string); ok {
		if timeout, err := time.ParseDuration(timeoutStr); err == nil {
			config.Timeout = timeout
		}
	}

	// Parse SSL mode
	if sslMode, ok := params["ssl_mode"].(string); ok {
		config.SSLMode = sslMode
	}

	// Parse query
	if query, ok := params["query"].(string); ok {
		config.Query = query
	}

	// Parse batch size
	if batchSize, ok := params["batch_size"]; ok {
		if val, err := convertToInt(batchSize); err == nil {
			config.BatchSize = val
		}
	}

	return config, nil
}

// initConnection initializes the PostgreSQL database connection
func (pg *PostgreSQLConnector) initConnection() error {
	dsn := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		pg.pgConfig.Host,
		pg.pgConfig.Port,
		pg.pgConfig.Username,
		pg.pgConfig.Password,
		pg.pgConfig.Database,
		pg.pgConfig.SSLMode,
	)

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return fmt.Errorf("failed to open PostgreSQL connection: %w", err)
	}

	// Configure connection pool
	db.SetMaxOpenConns(pg.pgConfig.MaxOpenConns)
	db.SetMaxIdleConns(pg.pgConfig.MaxIdleConns)
	db.SetConnMaxLifetime(pg.pgConfig.ConnMaxLifetime)
	db.SetConnMaxIdleTime(pg.pgConfig.ConnMaxIdleTime)

	pg.db = db
	return nil
}

// Start starts the PostgreSQL connector
func (pg *PostgreSQLConnector) Start(ctx context.Context) error {
	if pg.IsRunning() {
		return ErrConnectorAlreadyRunning
	}

	pg.setState(StateStarting)

	// Perform initial health check
	if err := pg.HealthCheck(ctx); err != nil {
		pg.setError(err)
		pg.setState(StateFailed)
		return err
	}

	now := time.Now()
	pg.startTime = &now
	pg.setState(StateRunning)

	// Start health checking
	pg.startHealthCheck(pg.HealthCheck)

	return nil
}

// Stop stops the PostgreSQL connector
func (pg *PostgreSQLConnector) Stop(ctx context.Context) error {
	if !pg.IsRunning() {
		return ErrConnectorNotRunning
	}

	pg.setState(StateStopping)

	// Close database connection
	if pg.db != nil {
		if err := pg.db.Close(); err != nil {
			pg.setError(err)
		}
	}

	pg.setState(StateStopped)
	return nil
}

// Restart restarts the PostgreSQL connector
func (pg *PostgreSQLConnector) Restart(ctx context.Context) error {
	if err := pg.Stop(ctx); err != nil {
		return err
	}
	return pg.Start(ctx)
}

// HealthCheck performs a health check
func (pg *PostgreSQLConnector) HealthCheck(ctx context.Context) error {
	if pg.db == nil {
		return fmt.Errorf("database connection is nil")
	}

	ctx, cancel := context.WithTimeout(ctx, pg.pgConfig.Timeout)
	defer cancel()

	if err := pg.db.PingContext(ctx); err != nil {
		return fmt.Errorf("database ping failed: %w", err)
	}

	return nil
}

// Send sends a message through the PostgreSQL connector
func (pg *PostgreSQLConnector) Send(ctx context.Context, message *Message) (*MessageResult, error) {
	if !pg.IsRunning() {
		return nil, ErrConnectorNotRunning
	}

	start := time.Now()

	result := &MessageResult{
		MessageID: message.ID,
		Timestamp: start,
	}

	ctx, cancel := context.WithTimeout(ctx, pg.pgConfig.Timeout)
	defer cancel()

	// Parse payload as JSON if possible
	var payloadData interface{}
	if err := json.Unmarshal(message.Payload, &payloadData); err != nil {
		// If not JSON, use raw string
		payloadData = string(message.Payload)
	}

	// Execute query
	if pg.pgConfig.Query != "" {
		// Use custom query
		_, err := pg.db.ExecContext(ctx, pg.pgConfig.Query, message.Topic, payloadData, 0, message.Timestamp)
		if err != nil {
			result.Error = err.Error()
			pg.setError(err)
			pg.recordDroppedMessage()
			return result, err
		}
	} else {
		// Use default insert
		tableName := pg.pgConfig.Table
		if pg.pgConfig.Schema != "" && pg.pgConfig.Schema != "public" {
			tableName = fmt.Sprintf("%s.%s", pg.pgConfig.Schema, pg.pgConfig.Table)
		}
		query := fmt.Sprintf("INSERT INTO %s (topic, payload, timestamp) VALUES ($1, $2, $3)", tableName)
		_, err := pg.db.ExecContext(ctx, query, message.Topic, payloadData, message.Timestamp)
		if err != nil {
			result.Error = err.Error()
			pg.setError(err)
			pg.recordDroppedMessage()
			return result, err
		}
	}

	// Calculate latency
	latency := time.Since(start)
	result.Latency = latency
	result.Success = true

	pg.recordSuccess(latency)
	pg.recordSentMessage()

	return result, nil
}

// SendBatch sends multiple messages
func (pg *PostgreSQLConnector) SendBatch(ctx context.Context, messages []*Message) ([]*MessageResult, error) {
	results := make([]*MessageResult, len(messages))

	// Process messages in batches
	batchSize := pg.pgConfig.BatchSize
	if batchSize <= 0 {
		batchSize = 1000
	}

	for i := 0; i < len(messages); i += batchSize {
		end := i + batchSize
		if end > len(messages) {
			end = len(messages)
		}

		batch := messages[i:end]
		batchResults, err := pg.processBatch(ctx, batch)
		if err != nil {
			// Fill remaining results with errors
			for j := i; j < end; j++ {
				if j-i < len(batchResults) {
					results[j] = batchResults[j-i]
				} else {
					results[j] = &MessageResult{
						MessageID: messages[j].ID,
						Error:     err.Error(),
						Timestamp: time.Now(),
					}
				}
			}
		} else {
			copy(results[i:end], batchResults)
		}
	}

	return results, nil
}

// processBatch processes a batch of messages
func (pg *PostgreSQLConnector) processBatch(ctx context.Context, messages []*Message) ([]*MessageResult, error) {
	results := make([]*MessageResult, len(messages))

	if len(messages) == 0 {
		return results, nil
	}

	start := time.Now()

	ctx, cancel := context.WithTimeout(ctx, pg.pgConfig.Timeout)
	defer cancel()

	// Begin transaction
	tx, err := pg.db.BeginTx(ctx, nil)
	if err != nil {
		for i := range results {
			results[i] = &MessageResult{
				MessageID: messages[i].ID,
				Error:     err.Error(),
				Timestamp: start,
			}
		}
		pg.setError(err)
		return results, err
	}
	defer tx.Rollback()

	// Prepare statement
	var stmt *sql.Stmt
	if pg.pgConfig.Query != "" {
		stmt, err = tx.PrepareContext(ctx, pg.pgConfig.Query)
	} else {
		tableName := pg.pgConfig.Table
		if pg.pgConfig.Schema != "" && pg.pgConfig.Schema != "public" {
			tableName = fmt.Sprintf("%s.%s", pg.pgConfig.Schema, pg.pgConfig.Table)
		}
		query := fmt.Sprintf("INSERT INTO %s (topic, payload, timestamp) VALUES ($1, $2, $3)", tableName)
		stmt, err = tx.PrepareContext(ctx, query)
	}

	if err != nil {
		for i := range results {
			results[i] = &MessageResult{
				MessageID: messages[i].ID,
				Error:     err.Error(),
				Timestamp: start,
			}
		}
		pg.setError(err)
		return results, err
	}
	defer stmt.Close()

	// Execute batch
	for i, message := range messages {
		var payloadData interface{}
		if err := json.Unmarshal(message.Payload, &payloadData); err != nil {
			payloadData = string(message.Payload)
		}

		if pg.pgConfig.Query != "" {
			_, err = stmt.ExecContext(ctx, message.Topic, payloadData, 0, message.Timestamp)
		} else {
			_, err = stmt.ExecContext(ctx, message.Topic, payloadData, message.Timestamp)
		}

		if err != nil {
			results[i] = &MessageResult{
				MessageID: message.ID,
				Error:     err.Error(),
				Timestamp: time.Now(),
			}
			pg.setError(err)
			pg.recordDroppedMessage()
		} else {
			latency := time.Since(start)
			results[i] = &MessageResult{
				MessageID: message.ID,
				Success:   true,
				Latency:   latency,
				Timestamp: time.Now(),
			}
			pg.recordSuccess(latency)
			pg.recordSentMessage()
		}
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		for i := range results {
			if results[i].Success {
				results[i].Success = false
				results[i].Error = err.Error()
			}
		}
		pg.setError(err)
		return results, err
	}

	return results, nil
}

// Close closes the PostgreSQL connector
func (pg *PostgreSQLConnector) Close() error {
	if pg.db != nil {
		pg.db.Close()
	}

	return pg.BaseConnector.Close()
}