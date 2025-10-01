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

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/IBM/sarama"
)

// KafkaConnector implements the Connector interface for Apache Kafka
type KafkaConnector struct {
	id            string
	config        *KafkaConfig
	producer      sarama.SyncProducer
	consumer      sarama.Consumer
	consumerGroup sarama.ConsumerGroup
	connected     bool
	metrics       ConnectorMetrics
	mu            sync.RWMutex
	receiveChan   chan *Message
	ctx           context.Context
	cancel        context.CancelFunc
}

// KafkaConfig represents Kafka connector configuration
type KafkaConfig struct {
	ID                string            `json:"id"`
	Name              string            `json:"name"`
	Brokers           []string          `json:"brokers"`
	Topics            []string          `json:"topics"`
	ConsumerGroup     string            `json:"consumer_group,omitempty"`
	ProducerTopic     string            `json:"producer_topic,omitempty"`
	SecurityProtocol  string            `json:"security_protocol,omitempty"`  // PLAINTEXT, SASL_PLAINTEXT, SASL_SSL, SSL
	SASLMechanism     string            `json:"sasl_mechanism,omitempty"`     // PLAIN, SCRAM-SHA-256, SCRAM-SHA-512
	Username          string            `json:"username,omitempty"`
	Password          string            `json:"password,omitempty"`
	SSLConfig         *SSLConfig        `json:"ssl_config,omitempty"`
	ProducerConfig    *ProducerConfig   `json:"producer_config,omitempty"`
	ConsumerConfig    *ConsumerConfig   `json:"consumer_config,omitempty"`
	MessageFormat     string            `json:"message_format"`               // json, avro, plain
	CompressionType   string            `json:"compression_type,omitempty"`   // none, gzip, snappy, lz4, zstd
	BatchSize         int               `json:"batch_size,omitempty"`
	LingerTime        int               `json:"linger_time_ms,omitempty"`
	MaxMessageBytes   int               `json:"max_message_bytes,omitempty"`
	ClientID          string            `json:"client_id,omitempty"`
	EnableIdempotence bool              `json:"enable_idempotence"`
	Headers           map[string]string `json:"headers,omitempty"`
}

// SSLConfig represents SSL configuration for Kafka
type SSLConfig struct {
	CertFile   string `json:"cert_file,omitempty"`
	KeyFile    string `json:"key_file,omitempty"`
	CAFile     string `json:"ca_file,omitempty"`
	VerifySSL  bool   `json:"verify_ssl"`
	ServerName string `json:"server_name,omitempty"`
}

// ProducerConfig represents Kafka producer specific configuration
type ProducerConfig struct {
	RequiredAcks      int    `json:"required_acks"`           // 0, 1, -1 (all)
	Timeout           int    `json:"timeout_ms"`
	Retries           int    `json:"retries"`
	RetryBackoff      int    `json:"retry_backoff_ms"`
	Partitioner       string `json:"partitioner"`             // manual, hash, random, round_robin
	PartitionKey      string `json:"partition_key,omitempty"` // field to use for partitioning
	MaxMessageBytes   int    `json:"max_message_bytes"`
	FlushFrequency    int    `json:"flush_frequency_ms"`
}

// ConsumerConfig represents Kafka consumer specific configuration
type ConsumerConfig struct {
	AutoOffsetReset    string `json:"auto_offset_reset"`     // earliest, latest
	EnableAutoCommit   bool   `json:"enable_auto_commit"`
	AutoCommitInterval int    `json:"auto_commit_interval_ms"`
	SessionTimeout     int    `json:"session_timeout_ms"`
	HeartbeatInterval  int    `json:"heartbeat_interval_ms"`
	MaxPollRecords     int    `json:"max_poll_records"`
	FetchMinBytes      int    `json:"fetch_min_bytes"`
	FetchMaxWait       int    `json:"fetch_max_wait_ms"`
}

// KafkaMessage represents a Kafka message
type KafkaMessage struct {
	Topic     string            `json:"topic"`
	Partition int32             `json:"partition"`
	Offset    int64             `json:"offset"`
	Key       []byte            `json:"key,omitempty"`
	Value     []byte            `json:"value"`
	Headers   map[string][]byte `json:"headers,omitempty"`
	Timestamp time.Time         `json:"timestamp"`
}

// NewKafkaConnector creates a new Kafka connector
func NewKafkaConnector(config *KafkaConfig) *KafkaConnector {
	ctx, cancel := context.WithCancel(context.Background())

	return &KafkaConnector{
		id:          config.ID,
		config:      config,
		connected:   false,
		receiveChan: make(chan *Message, 1000), // Buffered channel
		ctx:         ctx,
		cancel:      cancel,
		metrics: ConnectorMetrics{
			ConnectionStatus: "disconnected",
		},
	}
}

// ID returns the connector ID
func (kc *KafkaConnector) ID() string {
	return kc.id
}

// Type returns the connector type
func (kc *KafkaConnector) Type() string {
	return "kafka"
}

// Connect establishes connection to Kafka cluster
func (kc *KafkaConnector) Connect(ctx context.Context) error {
	kc.mu.Lock()
	defer kc.mu.Unlock()

	if kc.connected {
		return nil
	}

	// Create Kafka configuration
	saramaConfig, err := kc.createSaramaConfig()
	if err != nil {
		kc.metrics.ConnectionErrors++
		return fmt.Errorf("failed to create Kafka config: %w", err)
	}

	// Create producer if producer topic is configured
	if kc.config.ProducerTopic != "" {
		producer, err := sarama.NewSyncProducer(kc.config.Brokers, saramaConfig)
		if err != nil {
			kc.metrics.ConnectionErrors++
			return fmt.Errorf("failed to create Kafka producer: %w", err)
		}
		kc.producer = producer
	}

	// Create consumer if topics are configured for consumption
	if len(kc.config.Topics) > 0 {
		if kc.config.ConsumerGroup != "" {
			// Consumer group mode
			consumerGroup, err := sarama.NewConsumerGroup(kc.config.Brokers, kc.config.ConsumerGroup, saramaConfig)
			if err != nil {
				kc.metrics.ConnectionErrors++
				return fmt.Errorf("failed to create Kafka consumer group: %w", err)
			}
			kc.consumerGroup = consumerGroup

			// Start consuming in background
			go kc.consumeMessages(ctx)
		} else {
			// Simple consumer mode
			consumer, err := sarama.NewConsumer(kc.config.Brokers, saramaConfig)
			if err != nil {
				kc.metrics.ConnectionErrors++
				return fmt.Errorf("failed to create Kafka consumer: %w", err)
			}
			kc.consumer = consumer

			// Start consuming in background
			go kc.consumeSimpleMessages(ctx)
		}
	}

	kc.connected = true
	kc.metrics.ConnectionStatus = "connected"
	kc.metrics.LastConnected = time.Now()

	log.Printf("[INFO] Kafka connector %s connected to brokers: %v", kc.id, kc.config.Brokers)
	return nil
}

// Disconnect closes connection to Kafka cluster
func (kc *KafkaConnector) Disconnect(ctx context.Context) error {
	kc.mu.Lock()
	defer kc.mu.Unlock()

	if !kc.connected {
		return nil
	}

	kc.cancel() // Cancel all operations

	// Close producer
	if kc.producer != nil {
		if err := kc.producer.Close(); err != nil {
			log.Printf("[WARN] Error closing Kafka producer: %v", err)
		}
		kc.producer = nil
	}

	// Close consumer
	if kc.consumer != nil {
		if err := kc.consumer.Close(); err != nil {
			log.Printf("[WARN] Error closing Kafka consumer: %v", err)
		}
		kc.consumer = nil
	}

	// Close consumer group
	if kc.consumerGroup != nil {
		if err := kc.consumerGroup.Close(); err != nil {
			log.Printf("[WARN] Error closing Kafka consumer group: %v", err)
		}
		kc.consumerGroup = nil
	}

	kc.connected = false
	kc.metrics.ConnectionStatus = "disconnected"
	kc.metrics.LastDisconnected = time.Now()

	log.Printf("[INFO] Kafka connector %s disconnected", kc.id)
	return nil
}

// Send sends a message to Kafka
func (kc *KafkaConnector) Send(ctx context.Context, msg *Message) error {
	kc.mu.RLock()
	defer kc.mu.RUnlock()

	if !kc.connected || kc.producer == nil {
		return fmt.Errorf("Kafka producer not connected")
	}

	// Convert Message to Kafka message
	kafkaMsg, err := kc.convertToKafkaMessage(msg)
	if err != nil {
		return fmt.Errorf("failed to convert message: %w", err)
	}

	// Send message
	partition, offset, err := kc.producer.SendMessage(kafkaMsg)
	if err != nil {
		return fmt.Errorf("failed to send message to Kafka: %w", err)
	}

	kc.metrics.MessagesSent++
	kc.updateThroughputMetrics()

	log.Printf("[DEBUG] Message sent to Kafka topic %s, partition %d, offset %d",
		kafkaMsg.Topic, partition, offset)

	return nil
}

// Receive returns a channel for receiving messages
func (kc *KafkaConnector) Receive(ctx context.Context) (<-chan *Message, error) {
	if !kc.connected {
		return nil, fmt.Errorf("Kafka connector not connected")
	}

	return kc.receiveChan, nil
}

// IsConnected returns connection status
func (kc *KafkaConnector) IsConnected() bool {
	kc.mu.RLock()
	defer kc.mu.RUnlock()
	return kc.connected
}

// GetMetrics returns connector metrics
func (kc *KafkaConnector) GetMetrics() ConnectorMetrics {
	kc.mu.RLock()
	defer kc.mu.RUnlock()
	return kc.metrics
}

// GetConfiguration returns connector configuration
func (kc *KafkaConnector) GetConfiguration() map[string]interface{} {
	configBytes, _ := json.Marshal(kc.config)
	var configMap map[string]interface{}
	json.Unmarshal(configBytes, &configMap)
	return configMap
}

// SetConfiguration updates connector configuration
func (kc *KafkaConnector) SetConfiguration(config map[string]interface{}) error {
	configBytes, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	var kafkaConfig KafkaConfig
	if err := json.Unmarshal(configBytes, &kafkaConfig); err != nil {
		return fmt.Errorf("failed to unmarshal Kafka config: %w", err)
	}

	kc.mu.Lock()
	defer kc.mu.Unlock()

	kc.config = &kafkaConfig
	return nil
}

// createSaramaConfig creates Sarama configuration from KafkaConfig
func (kc *KafkaConnector) createSaramaConfig() (*sarama.Config, error) {
	config := sarama.NewConfig()

	// Basic configuration
	config.ClientID = kc.config.ClientID
	if config.ClientID == "" {
		config.ClientID = fmt.Sprintf("emqx-go-kafka-%s", kc.id)
	}

	// Producer configuration
	if kc.config.ProducerConfig != nil {
		pc := kc.config.ProducerConfig
		config.Producer.RequiredAcks = sarama.RequiredAcks(pc.RequiredAcks)
		config.Producer.Timeout = time.Duration(pc.Timeout) * time.Millisecond
		config.Producer.Retry.Max = pc.Retries
		config.Producer.Retry.Backoff = time.Duration(pc.RetryBackoff) * time.Millisecond
		config.Producer.MaxMessageBytes = pc.MaxMessageBytes
		config.Producer.Flush.Frequency = time.Duration(pc.FlushFrequency) * time.Millisecond

		// Set partitioner
		switch pc.Partitioner {
		case "hash":
			config.Producer.Partitioner = sarama.NewHashPartitioner
		case "random":
			config.Producer.Partitioner = sarama.NewRandomPartitioner
		case "round_robin":
			config.Producer.Partitioner = sarama.NewRoundRobinPartitioner
		default:
			config.Producer.Partitioner = sarama.NewHashPartitioner
		}
	}

	// Consumer configuration
	if kc.config.ConsumerConfig != nil {
		cc := kc.config.ConsumerConfig

		switch cc.AutoOffsetReset {
		case "earliest":
			config.Consumer.Offsets.Initial = sarama.OffsetOldest
		case "latest":
			config.Consumer.Offsets.Initial = sarama.OffsetNewest
		default:
			config.Consumer.Offsets.Initial = sarama.OffsetNewest
		}

		config.Consumer.Group.Session.Timeout = time.Duration(cc.SessionTimeout) * time.Millisecond
		config.Consumer.Group.Heartbeat.Interval = time.Duration(cc.HeartbeatInterval) * time.Millisecond
		config.Consumer.Fetch.Min = int32(cc.FetchMinBytes)
		config.Consumer.MaxWaitTime = time.Duration(cc.FetchMaxWait) * time.Millisecond
	}

	// Compression
	switch kc.config.CompressionType {
	case "gzip":
		config.Producer.Compression = sarama.CompressionGZIP
	case "snappy":
		config.Producer.Compression = sarama.CompressionSnappy
	case "lz4":
		config.Producer.Compression = sarama.CompressionLZ4
	case "zstd":
		config.Producer.Compression = sarama.CompressionZSTD
	default:
		config.Producer.Compression = sarama.CompressionNone
	}

	// Security configuration
	if kc.config.SecurityProtocol != "" && kc.config.SecurityProtocol != "PLAINTEXT" {
		if err := kc.configureSecurity(config); err != nil {
			return nil, fmt.Errorf("failed to configure security: %w", err)
		}
	}

	// Enable idempotence if configured
	if kc.config.EnableIdempotence {
		config.Producer.Idempotent = true
		config.Producer.RequiredAcks = sarama.WaitForAll
		config.Producer.Retry.Max = 5
		config.Net.MaxOpenRequests = 1
	}

	// Return success
	config.Producer.Return.Successes = true
	config.Producer.Return.Errors = true

	return config, nil
}

// configureSecurity configures security settings
func (kc *KafkaConnector) configureSecurity(config *sarama.Config) error {
	switch kc.config.SecurityProtocol {
	case "SASL_PLAINTEXT", "SASL_SSL":
		config.Net.SASL.Enable = true
		config.Net.SASL.User = kc.config.Username
		config.Net.SASL.Password = kc.config.Password

		switch kc.config.SASLMechanism {
		case "PLAIN":
			config.Net.SASL.Mechanism = sarama.SASLTypePlaintext
		case "SCRAM-SHA-256":
			config.Net.SASL.Mechanism = sarama.SASLTypeSCRAMSHA256
		case "SCRAM-SHA-512":
			config.Net.SASL.Mechanism = sarama.SASLTypeSCRAMSHA512
		default:
			config.Net.SASL.Mechanism = sarama.SASLTypePlaintext
		}
	}

	if kc.config.SecurityProtocol == "SSL" || kc.config.SecurityProtocol == "SASL_SSL" {
		config.Net.TLS.Enable = true
		if kc.config.SSLConfig != nil {
			// TLS configuration would be implemented here
			// For now, we'll use basic TLS
		}
	}

	return nil
}

// convertToKafkaMessage converts Message to Kafka message
func (kc *KafkaConnector) convertToKafkaMessage(msg *Message) (*sarama.ProducerMessage, error) {
	topic := kc.config.ProducerTopic
	if topic == "" {
		topic = msg.Topic
	}

	var value []byte
	var err error

	switch kc.config.MessageFormat {
	case "json":
		value, err = json.Marshal(map[string]interface{}{
			"topic":     msg.Topic,
			"payload":   string(msg.Payload),
			"qos":       msg.QoS,
			"headers":   msg.Headers,
			"metadata":  msg.Metadata,
			"timestamp": msg.Timestamp,
		})
	case "plain":
		value = msg.Payload
	default:
		value = msg.Payload
	}

	if err != nil {
		return nil, fmt.Errorf("failed to format message: %w", err)
	}

	kafkaMsg := &sarama.ProducerMessage{
		Topic: topic,
		Value: sarama.ByteEncoder(value),
	}

	// Add headers
	if len(msg.Headers) > 0 || len(kc.config.Headers) > 0 {
		kafkaMsg.Headers = make([]sarama.RecordHeader, 0)

		// Add configured headers
		for k, v := range kc.config.Headers {
			kafkaMsg.Headers = append(kafkaMsg.Headers, sarama.RecordHeader{
				Key:   []byte(k),
				Value: []byte(v),
			})
		}

		// Add message headers
		for k, v := range msg.Headers {
			kafkaMsg.Headers = append(kafkaMsg.Headers, sarama.RecordHeader{
				Key:   []byte(k),
				Value: []byte(v),
			})
		}
	}

	// Set partition key if configured
	if kc.config.ProducerConfig != nil && kc.config.ProducerConfig.PartitionKey != "" {
		if keyValue, ok := msg.Metadata[kc.config.ProducerConfig.PartitionKey]; ok {
			kafkaMsg.Key = sarama.StringEncoder(fmt.Sprintf("%v", keyValue))
		}
	}

	return kafkaMsg, nil
}

// consumeMessages consumes messages using consumer group
func (kc *KafkaConnector) consumeMessages(ctx context.Context) {
	handler := &consumerGroupHandler{
		connector: kc,
	}

	for {
		select {
		case <-ctx.Done():
			return
		default:
			if err := kc.consumerGroup.Consume(ctx, kc.config.Topics, handler); err != nil {
				log.Printf("[ERROR] Kafka consumer group error: %v", err)
				time.Sleep(time.Second)
			}
		}
	}
}

// consumeSimpleMessages consumes messages using simple consumer
func (kc *KafkaConnector) consumeSimpleMessages(ctx context.Context) {
	for _, topic := range kc.config.Topics {
		partitions, err := kc.consumer.Partitions(topic)
		if err != nil {
			log.Printf("[ERROR] Failed to get partitions for topic %s: %v", topic, err)
			continue
		}

		for _, partition := range partitions {
			go kc.consumePartition(ctx, topic, partition)
		}
	}
}

// consumePartition consumes messages from a specific partition
func (kc *KafkaConnector) consumePartition(ctx context.Context, topic string, partition int32) {
	partitionConsumer, err := kc.consumer.ConsumePartition(topic, partition, sarama.OffsetNewest)
	if err != nil {
		log.Printf("[ERROR] Failed to create partition consumer: %v", err)
		return
	}
	defer partitionConsumer.Close()

	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-partitionConsumer.Messages():
			kc.handleKafkaMessage(msg)
		case err := <-partitionConsumer.Errors():
			log.Printf("[ERROR] Kafka partition consumer error: %v", err)
		}
	}
}

// handleKafkaMessage processes a received Kafka message
func (kc *KafkaConnector) handleKafkaMessage(kafkaMsg *sarama.ConsumerMessage) {
	msg := &Message{
		ID:        fmt.Sprintf("kafka-%d-%d", kafkaMsg.Partition, kafkaMsg.Offset),
		Topic:     kafkaMsg.Topic,
		Payload:   kafkaMsg.Value,
		Headers:   make(map[string]string),
		Metadata: map[string]interface{}{
			"kafka_topic":     kafkaMsg.Topic,
			"kafka_partition": kafkaMsg.Partition,
			"kafka_offset":    kafkaMsg.Offset,
			"kafka_key":       string(kafkaMsg.Key),
		},
		Timestamp:  kafkaMsg.Timestamp,
		SourceType: "kafka",
		SourceID:   kc.id,
	}

	// Convert Kafka headers
	for _, header := range kafkaMsg.Headers {
		msg.Headers[string(header.Key)] = string(header.Value)
	}

	select {
	case kc.receiveChan <- msg:
		kc.metrics.MessagesReceived++
		kc.updateThroughputMetrics()
	default:
		log.Printf("[WARN] Kafka connector receive channel full, dropping message")
	}
}

// updateThroughputMetrics updates throughput metrics
func (kc *KafkaConnector) updateThroughputMetrics() {
	// Simple throughput calculation - could be enhanced
	now := time.Now()
	total := kc.metrics.MessagesReceived + kc.metrics.MessagesSent
	if !kc.metrics.LastConnected.IsZero() {
		duration := now.Sub(kc.metrics.LastConnected).Seconds()
		if duration > 0 {
			kc.metrics.Throughput = float64(total) / duration
		}
	}
}

// consumerGroupHandler implements sarama.ConsumerGroupHandler
type consumerGroupHandler struct {
	connector *KafkaConnector
}

func (h *consumerGroupHandler) Setup(sarama.ConsumerGroupSession) error {
	return nil
}

func (h *consumerGroupHandler) Cleanup(sarama.ConsumerGroupSession) error {
	return nil
}

func (h *consumerGroupHandler) ConsumeClaim(session sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
	for {
		select {
		case msg := <-claim.Messages():
			if msg == nil {
				return nil
			}
			h.connector.handleKafkaMessage(msg)
			session.MarkMessage(msg, "")
		case <-session.Context().Done():
			return nil
		}
	}
}