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

	_ "github.com/go-sql-driver/mysql" // MySQL driver
)

// MySQLConnectorConfig holds MySQL-specific configuration
type MySQLConnectorConfig struct {
	Host            string        `json:"host" yaml:"host"`
	Port            int           `json:"port" yaml:"port"`
	Username        string        `json:"username" yaml:"username"`
	Password        string        `json:"password" yaml:"password"`
	Database        string        `json:"database" yaml:"database"`
	Table           string        `json:"table" yaml:"table"`
	MaxOpenConns    int           `json:"max_open_conns" yaml:"max_open_conns"`
	MaxIdleConns    int           `json:"max_idle_conns" yaml:"max_idle_conns"`
	ConnMaxLifetime time.Duration `json:"conn_max_lifetime" yaml:"conn_max_lifetime"`
	ConnMaxIdleTime time.Duration `json:"conn_max_idle_time" yaml:"conn_max_idle_time"`
	SSLMode         string        `json:"ssl_mode" yaml:"ssl_mode"`
	Charset         string        `json:"charset" yaml:"charset"`
	Query           string        `json:"query" yaml:"query"`
	BatchSize       int           `json:"batch_size" yaml:"batch_size"`
	Timeout         time.Duration `json:"timeout" yaml:"timeout"`
}

// MySQLConnector implements a MySQL database connector
type MySQLConnector struct {
	*BaseConnector
	mysqlConfig MySQLConnectorConfig
	db          *sql.DB
}

// MySQLConnectorFactory creates MySQL connectors
type MySQLConnectorFactory struct{}

// Type returns the connector type
func (f *MySQLConnectorFactory) Type() ConnectorType {
	return ConnectorTypeMySQL
}

// Create creates a new MySQL connector
func (f *MySQLConnectorFactory) Create(config ConnectorConfig) (Connector, error) {
	mysqlConfig, err := f.parseMySQLConfig(config.Parameters)
	if err != nil {
		return nil, fmt.Errorf("invalid MySQL configuration: %w", err)
	}

	baseConnector := NewBaseConnector(config)

	connector := &MySQLConnector{
		BaseConnector: baseConnector,
		mysqlConfig:   mysqlConfig,
	}

	// Initialize database connection
	if err := connector.initConnection(); err != nil {
		return nil, fmt.Errorf("failed to initialize MySQL connection: %w", err)
	}

	return connector, nil
}

// ValidateConfig validates the MySQL connector configuration
func (f *MySQLConnectorFactory) ValidateConfig(config ConnectorConfig) error {
	_, err := f.parseMySQLConfig(config.Parameters)
	return err
}

// GetDefaultConfig returns default MySQL connector configuration
func (f *MySQLConnectorFactory) GetDefaultConfig() ConnectorConfig {
	return ConnectorConfig{
		Type:        ConnectorTypeMySQL,
		Enabled:     false,
		HealthCheck: DefaultHealthCheckConfig(),
		Retry:       DefaultRetryConfig(),
		Parameters: map[string]interface{}{
			"host":               "localhost",
			"port":               3306,
			"username":           "root",
			"password":           "",
			"database":           "emqx",
			"table":              "mqtt_messages",
			"max_open_conns":     10,
			"max_idle_conns":     5,
			"conn_max_lifetime":  "1h",
			"conn_max_idle_time": "10m",
			"ssl_mode":           "disable",
			"charset":            "utf8mb4",
			"query":              "INSERT INTO mqtt_messages (topic, payload, qos, timestamp) VALUES (?, ?, ?, ?)",
			"batch_size":         1000,
			"timeout":            "30s",
		},
	}
}

// GetConfigSchema returns the configuration schema
func (f *MySQLConnectorFactory) GetConfigSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"host": map[string]interface{}{
				"type":        "string",
				"description": "MySQL server host",
				"default":     "localhost",
			},
			"port": map[string]interface{}{
				"type":        "integer",
				"description": "MySQL server port",
				"default":     3306,
				"minimum":     1,
				"maximum":     65535,
			},
			"username": map[string]interface{}{
				"type":        "string",
				"description": "MySQL username",
			},
			"password": map[string]interface{}{
				"type":        "string",
				"description": "MySQL password",
			},
			"database": map[string]interface{}{
				"type":        "string",
				"description": "MySQL database name",
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

// parseMySQLConfig parses MySQL-specific configuration from parameters
func (f *MySQLConnectorFactory) parseMySQLConfig(params map[string]interface{}) (MySQLConnectorConfig, error) {
	config := MySQLConnectorConfig{
		Port:            3306,
		MaxOpenConns:    10,
		MaxIdleConns:    5,
		ConnMaxLifetime: 1 * time.Hour,
		ConnMaxIdleTime: 10 * time.Minute,
		SSLMode:         "disable",
		Charset:         "utf8mb4",
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

	// Parse charset
	if charset, ok := params["charset"].(string); ok {
		config.Charset = charset
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

// initConnection initializes the MySQL database connection
func (mc *MySQLConnector) initConnection() error {
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=%s&parseTime=true",
		mc.mysqlConfig.Username,
		mc.mysqlConfig.Password,
		mc.mysqlConfig.Host,
		mc.mysqlConfig.Port,
		mc.mysqlConfig.Database,
		mc.mysqlConfig.Charset,
	)

	// Add SSL mode if specified
	if mc.mysqlConfig.SSLMode != "" {
		dsn += "&tls=" + mc.mysqlConfig.SSLMode
	}

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return fmt.Errorf("failed to open MySQL connection: %w", err)
	}

	// Configure connection pool
	db.SetMaxOpenConns(mc.mysqlConfig.MaxOpenConns)
	db.SetMaxIdleConns(mc.mysqlConfig.MaxIdleConns)
	db.SetConnMaxLifetime(mc.mysqlConfig.ConnMaxLifetime)
	db.SetConnMaxIdleTime(mc.mysqlConfig.ConnMaxIdleTime)

	mc.db = db
	return nil
}

// Start starts the MySQL connector
func (mc *MySQLConnector) Start(ctx context.Context) error {
	if mc.IsRunning() {
		return ErrConnectorAlreadyRunning
	}

	mc.setState(StateStarting)

	// Perform initial health check
	if err := mc.HealthCheck(ctx); err != nil {
		mc.setError(err)
		mc.setState(StateFailed)
		return err
	}

	now := time.Now()
	mc.startTime = &now
	mc.setState(StateRunning)

	// Start health checking
	mc.startHealthCheck(mc.HealthCheck)

	return nil
}

// Stop stops the MySQL connector
func (mc *MySQLConnector) Stop(ctx context.Context) error {
	if !mc.IsRunning() {
		return ErrConnectorNotRunning
	}

	mc.setState(StateStopping)

	// Close database connection
	if mc.db != nil {
		if err := mc.db.Close(); err != nil {
			mc.setError(err)
		}
	}

	mc.setState(StateStopped)
	return nil
}

// Restart restarts the MySQL connector
func (mc *MySQLConnector) Restart(ctx context.Context) error {
	if err := mc.Stop(ctx); err != nil {
		return err
	}
	return mc.Start(ctx)
}

// HealthCheck performs a health check
func (mc *MySQLConnector) HealthCheck(ctx context.Context) error {
	if mc.db == nil {
		return fmt.Errorf("database connection is nil")
	}

	ctx, cancel := context.WithTimeout(ctx, mc.mysqlConfig.Timeout)
	defer cancel()

	if err := mc.db.PingContext(ctx); err != nil {
		return fmt.Errorf("database ping failed: %w", err)
	}

	return nil
}

// Send sends a message through the MySQL connector
func (mc *MySQLConnector) Send(ctx context.Context, message *Message) (*MessageResult, error) {
	if !mc.IsRunning() {
		return nil, ErrConnectorNotRunning
	}

	start := time.Now()

	result := &MessageResult{
		MessageID: message.ID,
		Timestamp: start,
	}

	ctx, cancel := context.WithTimeout(ctx, mc.mysqlConfig.Timeout)
	defer cancel()

	// Parse payload as JSON if possible
	var payloadData interface{}
	if err := json.Unmarshal(message.Payload, &payloadData); err != nil {
		// If not JSON, use raw string
		payloadData = string(message.Payload)
	}

	// Execute query
	if mc.mysqlConfig.Query != "" {
		// Use custom query
		_, err := mc.db.ExecContext(ctx, mc.mysqlConfig.Query, message.Topic, payloadData, 0, message.Timestamp)
		if err != nil {
			result.Error = err.Error()
			mc.setError(err)
			mc.recordDroppedMessage()
			return result, err
		}
	} else {
		// Use default insert
		query := fmt.Sprintf("INSERT INTO %s (topic, payload, timestamp) VALUES (?, ?, ?)", mc.mysqlConfig.Table)
		_, err := mc.db.ExecContext(ctx, query, message.Topic, payloadData, message.Timestamp)
		if err != nil {
			result.Error = err.Error()
			mc.setError(err)
			mc.recordDroppedMessage()
			return result, err
		}
	}

	// Calculate latency
	latency := time.Since(start)
	result.Latency = latency
	result.Success = true

	mc.recordSuccess(latency)
	mc.recordSentMessage()

	return result, nil
}

// SendBatch sends multiple messages
func (mc *MySQLConnector) SendBatch(ctx context.Context, messages []*Message) ([]*MessageResult, error) {
	results := make([]*MessageResult, len(messages))

	// Process messages in batches
	batchSize := mc.mysqlConfig.BatchSize
	if batchSize <= 0 {
		batchSize = 1000
	}

	for i := 0; i < len(messages); i += batchSize {
		end := i + batchSize
		if end > len(messages) {
			end = len(messages)
		}

		batch := messages[i:end]
		batchResults, err := mc.processBatch(ctx, batch)
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
func (mc *MySQLConnector) processBatch(ctx context.Context, messages []*Message) ([]*MessageResult, error) {
	results := make([]*MessageResult, len(messages))

	if len(messages) == 0 {
		return results, nil
	}

	start := time.Now()

	ctx, cancel := context.WithTimeout(ctx, mc.mysqlConfig.Timeout)
	defer cancel()

	// Begin transaction
	tx, err := mc.db.BeginTx(ctx, nil)
	if err != nil {
		for i := range results {
			results[i] = &MessageResult{
				MessageID: messages[i].ID,
				Error:     err.Error(),
				Timestamp: start,
			}
		}
		mc.setError(err)
		return results, err
	}
	defer tx.Rollback()

	// Prepare statement
	var stmt *sql.Stmt
	if mc.mysqlConfig.Query != "" {
		stmt, err = tx.PrepareContext(ctx, mc.mysqlConfig.Query)
	} else {
		query := fmt.Sprintf("INSERT INTO %s (topic, payload, timestamp) VALUES (?, ?, ?)", mc.mysqlConfig.Table)
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
		mc.setError(err)
		return results, err
	}
	defer stmt.Close()

	// Execute batch
	for i, message := range messages {
		var payloadData interface{}
		if err := json.Unmarshal(message.Payload, &payloadData); err != nil {
			payloadData = string(message.Payload)
		}

		if mc.mysqlConfig.Query != "" {
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
			mc.setError(err)
			mc.recordDroppedMessage()
		} else {
			latency := time.Since(start)
			results[i] = &MessageResult{
				MessageID: message.ID,
				Success:   true,
				Latency:   latency,
				Timestamp: time.Now(),
			}
			mc.recordSuccess(latency)
			mc.recordSentMessage()
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
		mc.setError(err)
		return results, err
	}

	return results, nil
}

// Close closes the MySQL connector
func (mc *MySQLConnector) Close() error {
	if mc.db != nil {
		mc.db.Close()
	}

	return mc.BaseConnector.Close()
}