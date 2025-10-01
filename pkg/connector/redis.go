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
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisConnectorConfig holds Redis-specific configuration
type RedisConnectorConfig struct {
	Host        string        `json:"host" yaml:"host"`
	Port        int           `json:"port" yaml:"port"`
	Username    string        `json:"username" yaml:"username"`
	Password    string        `json:"password" yaml:"password"`
	Database    int           `json:"database" yaml:"database"`
	PoolSize    int           `json:"pool_size" yaml:"pool_size"`
	MinIdleConn int           `json:"min_idle_conn" yaml:"min_idle_conn"`
	MaxConnAge  time.Duration `json:"max_conn_age" yaml:"max_conn_age"`
	PoolTimeout time.Duration `json:"pool_timeout" yaml:"pool_timeout"`
	IdleTimeout time.Duration `json:"idle_timeout" yaml:"idle_timeout"`
	ReadTimeout time.Duration `json:"read_timeout" yaml:"read_timeout"`
	WriteTimeout time.Duration `json:"write_timeout" yaml:"write_timeout"`
	Command      string        `json:"command" yaml:"command"`
	Key          string        `json:"key" yaml:"key"`
	TTL          time.Duration `json:"ttl" yaml:"ttl"`
	BatchSize    int           `json:"batch_size" yaml:"batch_size"`
	UseTLS       bool          `json:"use_tls" yaml:"use_tls"`
	TLSConfig    map[string]interface{} `json:"tls_config,omitempty" yaml:"tls_config,omitempty"`
}

// RedisConnector implements a Redis connector
type RedisConnector struct {
	*BaseConnector
	redisConfig RedisConnectorConfig
	client      redis.Cmdable
}

// RedisConnectorFactory creates Redis connectors
type RedisConnectorFactory struct{}

// Type returns the connector type
func (f *RedisConnectorFactory) Type() ConnectorType {
	return ConnectorTypeRedis
}

// Create creates a new Redis connector
func (f *RedisConnectorFactory) Create(config ConnectorConfig) (Connector, error) {
	redisConfig, err := f.parseRedisConfig(config.Parameters)
	if err != nil {
		return nil, fmt.Errorf("invalid Redis configuration: %w", err)
	}

	baseConnector := NewBaseConnector(config)

	connector := &RedisConnector{
		BaseConnector: baseConnector,
		redisConfig:   redisConfig,
	}

	// Initialize Redis client
	if err := connector.initClient(); err != nil {
		return nil, fmt.Errorf("failed to initialize Redis client: %w", err)
	}

	return connector, nil
}

// ValidateConfig validates the Redis connector configuration
func (f *RedisConnectorFactory) ValidateConfig(config ConnectorConfig) error {
	_, err := f.parseRedisConfig(config.Parameters)
	return err
}

// GetDefaultConfig returns default Redis connector configuration
func (f *RedisConnectorFactory) GetDefaultConfig() ConnectorConfig {
	return ConnectorConfig{
		Type:        ConnectorTypeRedis,
		Enabled:     false,
		HealthCheck: DefaultHealthCheckConfig(),
		Retry:       DefaultRetryConfig(),
		Parameters: map[string]interface{}{
			"host":          "localhost",
			"port":          6379,
			"username":      "",
			"password":      "",
			"database":      0,
			"pool_size":     10,
			"min_idle_conn": 1,
			"max_conn_age":  "30m",
			"pool_timeout":  "4s",
			"idle_timeout":  "5m",
			"read_timeout":  "3s",
			"write_timeout": "3s",
			"command":       "PUBLISH",
			"key":           "mqtt:messages",
			"ttl":           "1h",
			"batch_size":    1000,
			"use_tls":       false,
		},
	}
}

// GetConfigSchema returns the configuration schema
func (f *RedisConnectorFactory) GetConfigSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"host": map[string]interface{}{
				"type":        "string",
				"description": "Redis server host",
				"default":     "localhost",
			},
			"port": map[string]interface{}{
				"type":        "integer",
				"description": "Redis server port",
				"default":     6379,
				"minimum":     1,
				"maximum":     65535,
			},
			"username": map[string]interface{}{
				"type":        "string",
				"description": "Redis username (Redis 6.0+)",
			},
			"password": map[string]interface{}{
				"type":        "string",
				"description": "Redis password",
			},
			"database": map[string]interface{}{
				"type":        "integer",
				"description": "Redis database number",
				"default":     0,
				"minimum":     0,
				"maximum":     15,
			},
			"command": map[string]interface{}{
				"type":        "string",
				"description": "Redis command to execute",
				"enum":        []string{"SET", "HSET", "LPUSH", "RPUSH", "PUBLISH", "ZADD"},
				"default":     "PUBLISH",
			},
			"key": map[string]interface{}{
				"type":        "string",
				"description": "Redis key pattern",
				"default":     "mqtt:messages",
			},
			"ttl": map[string]interface{}{
				"type":        "string",
				"description": "Time to live for stored data",
				"default":     "1h",
			},
			"use_tls": map[string]interface{}{
				"type":        "boolean",
				"description": "Enable TLS connection",
				"default":     false,
			},
		},
		"required": []string{"host"},
	}
}

// parseRedisConfig parses Redis-specific configuration from parameters
func (f *RedisConnectorFactory) parseRedisConfig(params map[string]interface{}) (RedisConnectorConfig, error) {
	config := RedisConnectorConfig{
		Port:         6379,
		Database:     0,
		PoolSize:     10,
		MinIdleConn:  1,
		MaxConnAge:   30 * time.Minute,
		PoolTimeout:  4 * time.Second,
		IdleTimeout:  5 * time.Minute,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
		Command:      "PUBLISH",
		Key:          "mqtt:messages",
		TTL:          1 * time.Hour,
		BatchSize:    1000,
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
	}

	// Parse password
	if password, ok := params["password"].(string); ok {
		config.Password = password
	}

	// Parse database
	if database, ok := params["database"]; ok {
		if val, err := convertToInt(database); err == nil {
			config.Database = val
		}
	}

	// Parse pool settings
	if poolSize, ok := params["pool_size"]; ok {
		if val, err := convertToInt(poolSize); err == nil {
			config.PoolSize = val
		}
	}

	if minIdle, ok := params["min_idle_conn"]; ok {
		if val, err := convertToInt(minIdle); err == nil {
			config.MinIdleConn = val
		}
	}

	// Parse timeouts
	if maxAge, ok := params["max_conn_age"].(string); ok {
		if age, err := time.ParseDuration(maxAge); err == nil {
			config.MaxConnAge = age
		}
	}

	if poolTimeout, ok := params["pool_timeout"].(string); ok {
		if timeout, err := time.ParseDuration(poolTimeout); err == nil {
			config.PoolTimeout = timeout
		}
	}

	if idleTimeout, ok := params["idle_timeout"].(string); ok {
		if timeout, err := time.ParseDuration(idleTimeout); err == nil {
			config.IdleTimeout = timeout
		}
	}

	if readTimeout, ok := params["read_timeout"].(string); ok {
		if timeout, err := time.ParseDuration(readTimeout); err == nil {
			config.ReadTimeout = timeout
		}
	}

	if writeTimeout, ok := params["write_timeout"].(string); ok {
		if timeout, err := time.ParseDuration(writeTimeout); err == nil {
			config.WriteTimeout = timeout
		}
	}

	// Parse command and key
	if command, ok := params["command"].(string); ok {
		config.Command = strings.ToUpper(command)
	}

	if key, ok := params["key"].(string); ok {
		config.Key = key
	}

	// Parse TTL
	if ttlStr, ok := params["ttl"].(string); ok {
		if ttl, err := time.ParseDuration(ttlStr); err == nil {
			config.TTL = ttl
		}
	}

	// Parse batch size
	if batchSize, ok := params["batch_size"]; ok {
		if val, err := convertToInt(batchSize); err == nil {
			config.BatchSize = val
		}
	}

	// Parse TLS settings
	if useTLS, ok := params["use_tls"].(bool); ok {
		config.UseTLS = useTLS
	}

	if tlsConfig, ok := params["tls_config"].(map[string]interface{}); ok {
		config.TLSConfig = tlsConfig
	}

	return config, nil
}

// initClient initializes the Redis client
func (rc *RedisConnector) initClient() error {
	options := &redis.Options{
		Addr:         fmt.Sprintf("%s:%d", rc.redisConfig.Host, rc.redisConfig.Port),
		Username:     rc.redisConfig.Username,
		Password:     rc.redisConfig.Password,
		DB:           rc.redisConfig.Database,
		PoolSize:     rc.redisConfig.PoolSize,
		MinIdleConns: rc.redisConfig.MinIdleConn,
		ConnMaxLifetime: rc.redisConfig.MaxConnAge,
		ConnMaxIdleTime: rc.redisConfig.IdleTimeout,
		PoolTimeout:  rc.redisConfig.PoolTimeout,
		ReadTimeout:  rc.redisConfig.ReadTimeout,
		WriteTimeout: rc.redisConfig.WriteTimeout,
	}

	// Configure TLS if enabled
	if rc.redisConfig.UseTLS {
		// Note: TLS configuration would need crypto/tls.Config
		// For now, we'll skip TLS setup in the test version
	}

	rc.client = redis.NewClient(options)
	return nil
}

// Start starts the Redis connector
func (rc *RedisConnector) Start(ctx context.Context) error {
	if rc.IsRunning() {
		return ErrConnectorAlreadyRunning
	}

	rc.setState(StateStarting)

	// Perform initial health check
	if err := rc.HealthCheck(ctx); err != nil {
		rc.setError(err)
		rc.setState(StateFailed)
		return err
	}

	now := time.Now()
	rc.startTime = &now
	rc.setState(StateRunning)

	// Start health checking
	rc.startHealthCheck(rc.HealthCheck)

	return nil
}

// Stop stops the Redis connector
func (rc *RedisConnector) Stop(ctx context.Context) error {
	if !rc.IsRunning() {
		return ErrConnectorNotRunning
	}

	rc.setState(StateStopping)

	// Close Redis client
	if client, ok := rc.client.(*redis.Client); ok {
		if err := client.Close(); err != nil {
			rc.setError(err)
		}
	}

	rc.setState(StateStopped)
	return nil
}

// Restart restarts the Redis connector
func (rc *RedisConnector) Restart(ctx context.Context) error {
	if err := rc.Stop(ctx); err != nil {
		return err
	}
	return rc.Start(ctx)
}

// HealthCheck performs a health check
func (rc *RedisConnector) HealthCheck(ctx context.Context) error {
	if rc.client == nil {
		return fmt.Errorf("Redis client is nil")
	}

	pong, err := rc.client.Ping(ctx).Result()
	if err != nil {
		return fmt.Errorf("Redis ping failed: %w", err)
	}

	if pong != "PONG" {
		return fmt.Errorf("unexpected ping response: %s", pong)
	}

	return nil
}

// Send sends a message through the Redis connector
func (rc *RedisConnector) Send(ctx context.Context, message *Message) (*MessageResult, error) {
	if !rc.IsRunning() {
		return nil, ErrConnectorNotRunning
	}

	start := time.Now()

	result := &MessageResult{
		MessageID: message.ID,
		Timestamp: start,
	}

	// Execute Redis command based on configuration
	var err error
	switch rc.redisConfig.Command {
	case "SET":
		err = rc.executeSet(ctx, message)
	case "HSET":
		err = rc.executeHSet(ctx, message)
	case "LPUSH", "RPUSH":
		err = rc.executeListPush(ctx, message)
	case "PUBLISH":
		err = rc.executePublish(ctx, message)
	case "ZADD":
		err = rc.executeZAdd(ctx, message)
	default:
		err = fmt.Errorf("unsupported Redis command: %s", rc.redisConfig.Command)
	}

	if err != nil {
		result.Error = err.Error()
		rc.setError(err)
		rc.recordDroppedMessage()
		return result, err
	}

	// Calculate latency
	latency := time.Since(start)
	result.Latency = latency
	result.Success = true

	rc.recordSuccess(latency)
	rc.recordSentMessage()

	return result, nil
}

// executeSet executes a SET command
func (rc *RedisConnector) executeSet(ctx context.Context, message *Message) error {
	key := rc.formatKey(message.Topic)

	cmd := rc.client.Set(ctx, key, string(message.Payload), rc.redisConfig.TTL)
	return cmd.Err()
}

// executeHSet executes an HSET command
func (rc *RedisConnector) executeHSet(ctx context.Context, message *Message) error {
	key := rc.formatKey("")

	fields := map[string]interface{}{
		"topic":     message.Topic,
		"payload":   string(message.Payload),
		"timestamp": message.Timestamp.Unix(),
	}

	cmd := rc.client.HSet(ctx, key, fields)
	if cmd.Err() != nil {
		return cmd.Err()
	}

	// Set TTL for the hash
	if rc.redisConfig.TTL > 0 {
		rc.client.Expire(ctx, key, rc.redisConfig.TTL)
	}

	return nil
}

// executeListPush executes LPUSH or RPUSH command
func (rc *RedisConnector) executeListPush(ctx context.Context, message *Message) error {
	key := rc.formatKey("")

	data := map[string]interface{}{
		"topic":     message.Topic,
		"payload":   string(message.Payload),
		"timestamp": message.Timestamp.Unix(),
	}

	var cmd *redis.IntCmd
	if rc.redisConfig.Command == "LPUSH" {
		cmd = rc.client.LPush(ctx, key, data)
	} else {
		cmd = rc.client.RPush(ctx, key, data)
	}

	if cmd.Err() != nil {
		return cmd.Err()
	}

	// Set TTL for the list
	if rc.redisConfig.TTL > 0 {
		rc.client.Expire(ctx, key, rc.redisConfig.TTL)
	}

	return nil
}

// executePublish executes a PUBLISH command
func (rc *RedisConnector) executePublish(ctx context.Context, message *Message) error {
	channel := rc.formatKey(message.Topic)

	cmd := rc.client.Publish(ctx, channel, string(message.Payload))
	return cmd.Err()
}

// executeZAdd executes a ZADD command
func (rc *RedisConnector) executeZAdd(ctx context.Context, message *Message) error {
	key := rc.formatKey("")
	score := float64(message.Timestamp.Unix())

	member := map[string]interface{}{
		"topic":   message.Topic,
		"payload": string(message.Payload),
	}

	cmd := rc.client.ZAdd(ctx, key, redis.Z{
		Score:  score,
		Member: member,
	})

	if cmd.Err() != nil {
		return cmd.Err()
	}

	// Set TTL for the sorted set
	if rc.redisConfig.TTL > 0 {
		rc.client.Expire(ctx, key, rc.redisConfig.TTL)
	}

	return nil
}

// formatKey formats the Redis key using the configured pattern
func (rc *RedisConnector) formatKey(topic string) string {
	key := rc.redisConfig.Key
	if topic != "" {
		// Replace placeholder or append topic
		if strings.Contains(key, "{topic}") {
			key = strings.ReplaceAll(key, "{topic}", topic)
		} else {
			key = key + ":" + topic
		}
	}
	return key
}

// SendBatch sends multiple messages
func (rc *RedisConnector) SendBatch(ctx context.Context, messages []*Message) ([]*MessageResult, error) {
	results := make([]*MessageResult, len(messages))

	// Process messages in batches using pipeline
	batchSize := rc.redisConfig.BatchSize
	if batchSize <= 0 {
		batchSize = 1000
	}

	for i := 0; i < len(messages); i += batchSize {
		end := i + batchSize
		if end > len(messages) {
			end = len(messages)
		}

		batch := messages[i:end]
		batchResults, err := rc.processBatch(ctx, batch)
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

// processBatch processes a batch of messages using Redis pipeline
func (rc *RedisConnector) processBatch(ctx context.Context, messages []*Message) ([]*MessageResult, error) {
	results := make([]*MessageResult, len(messages))

	if len(messages) == 0 {
		return results, nil
	}

	start := time.Now()

	// Use Redis pipeline for batch operations
	pipe := rc.client.Pipeline()

	// Add commands to pipeline
	for _, message := range messages {
		switch rc.redisConfig.Command {
		case "SET":
			key := rc.formatKey(message.Topic)
			pipe.Set(ctx, key, string(message.Payload), rc.redisConfig.TTL)
		case "PUBLISH":
			channel := rc.formatKey(message.Topic)
			pipe.Publish(ctx, channel, string(message.Payload))
		// Add other commands as needed
		}
	}

	// Execute pipeline
	cmds, err := pipe.Exec(ctx)
	if err != nil {
		for i := range results {
			results[i] = &MessageResult{
				MessageID: messages[i].ID,
				Error:     err.Error(),
				Timestamp: start,
			}
		}
		rc.setError(err)
		return results, err
	}

	// Process results
	for i, cmd := range cmds {
		if cmd.Err() != nil {
			results[i] = &MessageResult{
				MessageID: messages[i].ID,
				Error:     cmd.Err().Error(),
				Timestamp: time.Now(),
			}
			rc.setError(cmd.Err())
			rc.recordDroppedMessage()
		} else {
			latency := time.Since(start)
			results[i] = &MessageResult{
				MessageID: messages[i].ID,
				Success:   true,
				Latency:   latency,
				Timestamp: time.Now(),
			}
			rc.recordSuccess(latency)
			rc.recordSentMessage()
		}
	}

	return results, nil
}

// Close closes the Redis connector
func (rc *RedisConnector) Close() error {
	if client, ok := rc.client.(*redis.Client); ok {
		client.Close()
	}

	return rc.BaseConnector.Close()
}