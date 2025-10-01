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

	"github.com/segmentio/kafka-go"
)

// KafkaConnectorConfig holds Kafka-specific configuration
type KafkaConnectorConfig struct {
	Brokers          []string      `json:"brokers" yaml:"brokers"`
	Topic            string        `json:"topic" yaml:"topic"`
	Partition        int           `json:"partition" yaml:"partition"`
	RequiredAcks     int           `json:"required_acks" yaml:"required_acks"`
	BatchSize        int           `json:"batch_size" yaml:"batch_size"`
	BatchTimeout     time.Duration `json:"batch_timeout" yaml:"batch_timeout"`
	ReadTimeout      time.Duration `json:"read_timeout" yaml:"read_timeout"`
	WriteTimeout     time.Duration `json:"write_timeout" yaml:"write_timeout"`
	CompressionType  string        `json:"compression_type" yaml:"compression_type"`
	MaxMessageBytes  int           `json:"max_message_bytes" yaml:"max_message_bytes"`
	UseSSL           bool          `json:"use_ssl" yaml:"use_ssl"`
	SASLMechanism    string        `json:"sasl_mechanism" yaml:"sasl_mechanism"`
	SASLUsername     string        `json:"sasl_username" yaml:"sasl_username"`
	SASLPassword     string        `json:"sasl_password" yaml:"sasl_password"`
	TopicTemplate    string        `json:"topic_template" yaml:"topic_template"`
	KeyTemplate      string        `json:"key_template" yaml:"key_template"`
	HeaderTemplate   map[string]string `json:"header_template" yaml:"header_template"`
}

// KafkaConnector implements a Kafka connector
type KafkaConnector struct {
	*BaseConnector
	kafkaConfig KafkaConnectorConfig
	writer      *kafka.Writer
}

// KafkaConnectorFactory creates Kafka connectors
type KafkaConnectorFactory struct{}

// Type returns the connector type
func (f *KafkaConnectorFactory) Type() ConnectorType {
	return ConnectorTypeKafka
}

// Create creates a new Kafka connector
func (f *KafkaConnectorFactory) Create(config ConnectorConfig) (Connector, error) {
	kafkaConfig, err := f.parseKafkaConfig(config.Parameters)
	if err != nil {
		return nil, fmt.Errorf("invalid Kafka configuration: %w", err)
	}

	baseConnector := NewBaseConnector(config)

	connector := &KafkaConnector{
		BaseConnector: baseConnector,
		kafkaConfig:   kafkaConfig,
	}

	// Initialize Kafka writer
	connector.initWriter()

	return connector, nil
}

// ValidateConfig validates the Kafka connector configuration
func (f *KafkaConnectorFactory) ValidateConfig(config ConnectorConfig) error {
	_, err := f.parseKafkaConfig(config.Parameters)
	return err
}

// GetDefaultConfig returns default Kafka connector configuration
func (f *KafkaConnectorFactory) GetDefaultConfig() ConnectorConfig {
	return ConnectorConfig{
		Type:        ConnectorTypeKafka,
		Enabled:     false,
		HealthCheck: DefaultHealthCheckConfig(),
		Retry:       DefaultRetryConfig(),
		Parameters: map[string]interface{}{
			"brokers":             []string{"localhost:9092"},
			"topic":               "mqtt-messages",
			"partition":           0,
			"required_acks":       1,
			"batch_size":          100,
			"batch_timeout":       "1s",
			"read_timeout":        "10s",
			"write_timeout":       "10s",
			"compression_type":    "none",
			"max_message_bytes":   1000000,
			"use_ssl":             false,
			"sasl_mechanism":      "",
			"sasl_username":       "",
			"sasl_password":       "",
			"topic_template":      "",
			"key_template":        "",
			"header_template":     map[string]string{},
		},
	}
}

// GetConfigSchema returns the configuration schema
func (f *KafkaConnectorFactory) GetConfigSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"brokers": map[string]interface{}{
				"type":        "array",
				"description": "Kafka broker addresses",
				"items": map[string]interface{}{
					"type": "string",
				},
				"default": []string{"localhost:9092"},
			},
			"topic": map[string]interface{}{
				"type":        "string",
				"description": "Kafka topic name",
				"default":     "mqtt-messages",
			},
			"partition": map[string]interface{}{
				"type":        "integer",
				"description": "Target partition (use -1 for automatic)",
				"default":     0,
			},
			"required_acks": map[string]interface{}{
				"type":        "integer",
				"description": "Required acknowledgments (0=none, 1=leader, -1=all)",
				"enum":        []int{-1, 0, 1},
				"default":     1,
			},
			"compression_type": map[string]interface{}{
				"type":        "string",
				"description": "Compression algorithm",
				"enum":        []string{"none", "gzip", "snappy", "lz4", "zstd"},
				"default":     "none",
			},
			"use_ssl": map[string]interface{}{
				"type":        "boolean",
				"description": "Enable SSL/TLS encryption",
				"default":     false,
			},
			"sasl_mechanism": map[string]interface{}{
				"type":        "string",
				"description": "SASL authentication mechanism",
				"enum":        []string{"", "PLAIN", "SCRAM-SHA-256", "SCRAM-SHA-512"},
			},
			"topic_template": map[string]interface{}{
				"type":        "string",
				"description": "Topic template with placeholders",
			},
			"key_template": map[string]interface{}{
				"type":        "string",
				"description": "Message key template",
			},
		},
		"required": []string{"brokers", "topic"},
	}
}

// parseKafkaConfig parses Kafka-specific configuration from parameters
func (f *KafkaConnectorFactory) parseKafkaConfig(params map[string]interface{}) (KafkaConnectorConfig, error) {
	config := KafkaConnectorConfig{
		Partition:        0,
		RequiredAcks:     1,
		BatchSize:        100,
		BatchTimeout:     1 * time.Second,
		ReadTimeout:      10 * time.Second,
		WriteTimeout:     10 * time.Second,
		CompressionType:  "none",
		MaxMessageBytes:  1000000,
		HeaderTemplate:   make(map[string]string),
	}

	// Parse brokers
	if brokers, ok := params["brokers"]; ok {
		switch v := brokers.(type) {
		case []string:
			config.Brokers = v
		case []interface{}:
			config.Brokers = make([]string, len(v))
			for i, broker := range v {
				if str, ok := broker.(string); ok {
					config.Brokers[i] = str
				}
			}
		case string:
			config.Brokers = strings.Split(v, ",")
		}
	}

	if len(config.Brokers) == 0 {
		return config, fmt.Errorf("brokers are required")
	}

	// Parse topic
	if topic, ok := params["topic"].(string); ok {
		config.Topic = topic
	} else {
		return config, fmt.Errorf("topic is required")
	}

	// Parse partition
	if partition, ok := params["partition"]; ok {
		if val, err := convertToInt(partition); err == nil {
			config.Partition = val
		}
	}

	// Parse required acks
	if acks, ok := params["required_acks"]; ok {
		if val, err := convertToInt(acks); err == nil {
			config.RequiredAcks = val
		}
	}

	// Parse batch settings
	if batchSize, ok := params["batch_size"]; ok {
		if val, err := convertToInt(batchSize); err == nil {
			config.BatchSize = val
		}
	}

	if batchTimeout, ok := params["batch_timeout"].(string); ok {
		if timeout, err := time.ParseDuration(batchTimeout); err == nil {
			config.BatchTimeout = timeout
		}
	}

	// Parse timeouts
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

	// Parse compression
	if compression, ok := params["compression_type"].(string); ok {
		config.CompressionType = compression
	}

	// Parse max message bytes
	if maxBytes, ok := params["max_message_bytes"]; ok {
		if val, err := convertToInt(maxBytes); err == nil {
			config.MaxMessageBytes = val
		}
	}

	// Parse SSL settings
	if useSSL, ok := params["use_ssl"].(bool); ok {
		config.UseSSL = useSSL
	}

	// Parse SASL settings
	if mechanism, ok := params["sasl_mechanism"].(string); ok {
		config.SASLMechanism = mechanism
	}

	if username, ok := params["sasl_username"].(string); ok {
		config.SASLUsername = username
	}

	if password, ok := params["sasl_password"].(string); ok {
		config.SASLPassword = password
	}

	// Parse templates
	if topicTemplate, ok := params["topic_template"].(string); ok {
		config.TopicTemplate = topicTemplate
	}

	if keyTemplate, ok := params["key_template"].(string); ok {
		config.KeyTemplate = keyTemplate
	}

	if headerTemplate, ok := params["header_template"].(map[string]interface{}); ok {
		config.HeaderTemplate = make(map[string]string)
		for k, v := range headerTemplate {
			if str, ok := v.(string); ok {
				config.HeaderTemplate[k] = str
			}
		}
	}

	return config, nil
}

// initWriter initializes the Kafka writer
func (kc *KafkaConnector) initWriter() {
	writer := &kafka.Writer{
		Addr:         kafka.TCP(kc.kafkaConfig.Brokers...),
		Topic:        kc.kafkaConfig.Topic,
		Balancer:     &kafka.LeastBytes{},
		RequiredAcks: kafka.RequiredAcks(kc.kafkaConfig.RequiredAcks),
		BatchSize:    kc.kafkaConfig.BatchSize,
		BatchTimeout: kc.kafkaConfig.BatchTimeout,
		ReadTimeout:  kc.kafkaConfig.ReadTimeout,
		WriteTimeout: kc.kafkaConfig.WriteTimeout,
	}

	// Configure compression
	switch kc.kafkaConfig.CompressionType {
	case "gzip":
		writer.Compression = kafka.Gzip
	case "snappy":
		writer.Compression = kafka.Snappy
	case "lz4":
		writer.Compression = kafka.Lz4
	case "zstd":
		writer.Compression = kafka.Zstd
	default:
		// Default to no compression
	}

	kc.writer = writer
}

// Start starts the Kafka connector
func (kc *KafkaConnector) Start(ctx context.Context) error {
	if kc.IsRunning() {
		return ErrConnectorAlreadyRunning
	}

	kc.setState(StateStarting)

	// Perform initial health check
	if err := kc.HealthCheck(ctx); err != nil {
		kc.setError(err)
		kc.setState(StateFailed)
		return err
	}

	now := time.Now()
	kc.startTime = &now
	kc.setState(StateRunning)

	// Start health checking
	kc.startHealthCheck(kc.HealthCheck)

	return nil
}

// Stop stops the Kafka connector
func (kc *KafkaConnector) Stop(ctx context.Context) error {
	if !kc.IsRunning() {
		return ErrConnectorNotRunning
	}

	kc.setState(StateStopping)

	// Close Kafka writer
	if kc.writer != nil {
		if err := kc.writer.Close(); err != nil {
			kc.setError(err)
		}
	}

	kc.setState(StateStopped)
	return nil
}

// Restart restarts the Kafka connector
func (kc *KafkaConnector) Restart(ctx context.Context) error {
	if err := kc.Stop(ctx); err != nil {
		return err
	}
	return kc.Start(ctx)
}

// HealthCheck performs a health check
func (kc *KafkaConnector) HealthCheck(ctx context.Context) error {
	if kc.writer == nil {
		return fmt.Errorf("Kafka writer is nil")
	}

	// Create a temporary connection to test connectivity
	conn, err := kafka.DialContext(ctx, "tcp", kc.kafkaConfig.Brokers[0])
	if err != nil {
		return fmt.Errorf("failed to connect to Kafka broker: %w", err)
	}
	defer conn.Close()

	// Test if topic exists
	partitions, err := conn.ReadPartitions(kc.kafkaConfig.Topic)
	if err != nil {
		return fmt.Errorf("failed to read topic partitions: %w", err)
	}

	if len(partitions) == 0 {
		return fmt.Errorf("topic %s has no partitions", kc.kafkaConfig.Topic)
	}

	return nil
}

// Send sends a message through the Kafka connector
func (kc *KafkaConnector) Send(ctx context.Context, message *Message) (*MessageResult, error) {
	if !kc.IsRunning() {
		return nil, ErrConnectorNotRunning
	}

	start := time.Now()

	result := &MessageResult{
		MessageID: message.ID,
		Timestamp: start,
	}

	// Prepare Kafka message
	kafkaMsg := kafka.Message{
		Value: message.Payload,
	}

	// Set topic if template is provided
	if kc.kafkaConfig.TopicTemplate != "" {
		kafkaMsg.Topic = kc.formatTemplate(kc.kafkaConfig.TopicTemplate, message)
	}

	// Set key if template is provided
	if kc.kafkaConfig.KeyTemplate != "" {
		kafkaMsg.Key = []byte(kc.formatTemplate(kc.kafkaConfig.KeyTemplate, message))
	}

	// Set headers
	for k, v := range kc.kafkaConfig.HeaderTemplate {
		kafkaMsg.Headers = append(kafkaMsg.Headers, kafka.Header{
			Key:   k,
			Value: []byte(kc.formatTemplate(v, message)),
		})
	}

	// Add message headers
	for k, v := range message.Headers {
		kafkaMsg.Headers = append(kafkaMsg.Headers, kafka.Header{
			Key:   k,
			Value: []byte(v),
		})
	}

	// Send message
	err := kc.writer.WriteMessages(ctx, kafkaMsg)
	if err != nil {
		result.Error = err.Error()
		kc.setError(err)
		kc.recordDroppedMessage()
		return result, err
	}

	// Calculate latency
	latency := time.Since(start)
	result.Latency = latency
	result.Success = true

	kc.recordSuccess(latency)
	kc.recordSentMessage()

	return result, nil
}

// SendBatch sends multiple messages
func (kc *KafkaConnector) SendBatch(ctx context.Context, messages []*Message) ([]*MessageResult, error) {
	results := make([]*MessageResult, len(messages))

	if len(messages) == 0 {
		return results, nil
	}

	start := time.Now()

	// Prepare Kafka messages
	kafkaMessages := make([]kafka.Message, len(messages))
	for i, message := range messages {
		kafkaMsg := kafka.Message{
			Value: message.Payload,
		}

		// Set topic if template is provided
		if kc.kafkaConfig.TopicTemplate != "" {
			kafkaMsg.Topic = kc.formatTemplate(kc.kafkaConfig.TopicTemplate, message)
		}

		// Set key if template is provided
		if kc.kafkaConfig.KeyTemplate != "" {
			kafkaMsg.Key = []byte(kc.formatTemplate(kc.kafkaConfig.KeyTemplate, message))
		}

		// Set headers
		for k, v := range kc.kafkaConfig.HeaderTemplate {
			kafkaMsg.Headers = append(kafkaMsg.Headers, kafka.Header{
				Key:   k,
				Value: []byte(kc.formatTemplate(v, message)),
			})
		}

		// Add message headers
		for k, v := range message.Headers {
			kafkaMsg.Headers = append(kafkaMsg.Headers, kafka.Header{
				Key:   k,
				Value: []byte(v),
			})
		}

		kafkaMessages[i] = kafkaMsg
	}

	// Send batch
	err := kc.writer.WriteMessages(ctx, kafkaMessages...)

	latency := time.Since(start)

	// Create results
	for i, message := range messages {
		result := &MessageResult{
			MessageID: message.ID,
			Timestamp: time.Now(),
			Latency:   latency,
		}

		if err != nil {
			result.Error = err.Error()
			kc.setError(err)
			kc.recordDroppedMessage()
		} else {
			result.Success = true
			kc.recordSuccess(latency)
			kc.recordSentMessage()
		}

		results[i] = result
	}

	return results, nil
}

// formatTemplate formats a template string with message data
func (kc *KafkaConnector) formatTemplate(template string, message *Message) string {
	result := template
	result = strings.ReplaceAll(result, "{topic}", message.Topic)
	result = strings.ReplaceAll(result, "{message_id}", message.ID)
	result = strings.ReplaceAll(result, "{timestamp}", message.Timestamp.Format(time.RFC3339))

	// Replace header placeholders
	for k, v := range message.Headers {
		placeholder := fmt.Sprintf("{header.%s}", k)
		result = strings.ReplaceAll(result, placeholder, v)
	}

	return result
}

// Close closes the Kafka connector
func (kc *KafkaConnector) Close() error {
	if kc.writer != nil {
		kc.writer.Close()
	}

	return kc.BaseConnector.Close()
}