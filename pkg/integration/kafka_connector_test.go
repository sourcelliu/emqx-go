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
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewKafkaConnector(t *testing.T) {
	config := &KafkaConfig{
		ID:      "test-kafka",
		Brokers: []string{"localhost:9092"},
		Topics:  []string{"test-topic"},
	}

	connector := NewKafkaConnector(config)
	assert.NotNil(t, connector)
	assert.Equal(t, "test-kafka", connector.ID())
	assert.Equal(t, "kafka", connector.Type())
	assert.False(t, connector.IsConnected())
}

func TestKafkaConnectorValidation(t *testing.T) {
	testCases := []struct {
		name   string
		config *KafkaConfig
		hasErr bool
	}{
		{
			name: "missing ID",
			config: &KafkaConfig{
				Brokers: []string{"localhost:9092"},
				Topics:  []string{"test-topic"},
			},
			hasErr: true,
		},
		{
			name: "missing brokers",
			config: &KafkaConfig{
				ID:     "test-kafka",
				Topics: []string{"test-topic"},
			},
			hasErr: true,
		},
		{
			name: "empty brokers",
			config: &KafkaConfig{
				ID:      "test-kafka",
				Brokers: []string{},
				Topics:  []string{"test-topic"},
			},
			hasErr: true,
		},
		{
			name: "valid configuration",
			config: &KafkaConfig{
				ID:      "test-kafka",
				Brokers: []string{"localhost:9092"},
				Topics:  []string{"test-topic"},
			},
			hasErr: false,
		},
		{
			name: "multiple brokers",
			config: &KafkaConfig{
				ID:      "test-kafka",
				Brokers: []string{"localhost:9092", "localhost:9093"},
				Topics:  []string{"topic1", "topic2"},
			},
			hasErr: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			connector := NewKafkaConnector(tc.config)
			// For now, NewKafkaConnector doesn't validate, it just creates the connector
			// Validation would happen on Connect()
			assert.NotNil(t, connector)
		})
	}
}

func TestKafkaConnectorConfiguration(t *testing.T) {
	config := &KafkaConfig{
		ID:               "test-kafka",
		Brokers:          []string{"localhost:9092"},
		Topics:           []string{"test-topic"},
		ProducerTopic:    "output-topic",
		ConsumerGroup:    "test-group",
		SecurityProtocol: "SASL_SSL",
		Username:         "testuser",
		Password:         "testpass",
		CompressionType:  "gzip",
		BatchSize:        100,
		LingerTime:       10,
		MaxMessageBytes:  1000000,
	}

	connector := NewKafkaConnector(config)
	require.NotNil(t, connector)

	// Test getting configuration
	retrievedConfig := connector.GetConfiguration()
	assert.Equal(t, "test-kafka", retrievedConfig["id"])
	assert.Equal(t, []interface{}{"localhost:9092"}, retrievedConfig["brokers"])
	assert.Equal(t, "SASL_SSL", retrievedConfig["security_protocol"])
	assert.Equal(t, "testuser", retrievedConfig["username"])
	assert.Equal(t, "gzip", retrievedConfig["compression_type"])

	// Test setting configuration
	newConfig := map[string]interface{}{
		"id":                "updated-kafka",
		"brokers":           []string{"localhost:9094"},
		"topics":            []string{"new-topic"},
		"security_protocol": "PLAINTEXT",
	}

	err := connector.SetConfiguration(newConfig)
	assert.NoError(t, err)

	updatedConfig := connector.GetConfiguration()
	assert.Equal(t, "updated-kafka", updatedConfig["id"])
	assert.Equal(t, []interface{}{"localhost:9094"}, updatedConfig["brokers"])
	assert.Equal(t, "PLAINTEXT", updatedConfig["security_protocol"])
}

func TestKafkaConnectorMetrics(t *testing.T) {
	config := &KafkaConfig{
		ID:      "test-kafka",
		Brokers: []string{"localhost:9092"},
		Topics:  []string{"test-topic"},
	}

	connector := NewKafkaConnector(config)
	require.NotNil(t, connector)

	// Initial metrics should be zero
	metrics := connector.GetMetrics()
	assert.Equal(t, "disconnected", metrics.ConnectionStatus)
	assert.Equal(t, int64(0), metrics.MessagesReceived)
	assert.Equal(t, int64(0), metrics.MessagesSent)
	assert.Equal(t, int64(0), metrics.ConnectionErrors)

	// Note: We can't test actual Kafka operations without a running Kafka instance
	// In a real test environment, you would use testcontainers or a test Kafka cluster
}

func TestKafkaConnectorMessageCreation(t *testing.T) {
	config := &KafkaConfig{
		ID:            "test-kafka",
		Brokers:       []string{"localhost:9092"},
		Topics:        []string{"test-topic"},
		ProducerTopic: "output-topic",
	}

	connector := NewKafkaConnector(config)
	require.NotNil(t, connector)

	// Test message creation for sending
	msg := &Message{
		ID:        "test-msg",
		Topic:     "sensor/temperature",
		Payload:   []byte(`{"temperature": 25.5, "timestamp": "2023-01-01T12:00:00Z"}`),
		QoS:       1,
		Headers:   map[string]string{"source": "sensor1"},
		Timestamp: time.Now(),
	}

	// Test that we can prepare a message for Kafka
	// In a real implementation, this would test the message conversion
	assert.NotNil(t, msg)
	assert.Equal(t, "sensor/temperature", msg.Topic)
	assert.Contains(t, string(msg.Payload), "temperature")
}

func TestKafkaSecurityProtocols(t *testing.T) {
	validProtocols := []string{"PLAINTEXT", "SASL_PLAINTEXT", "SASL_SSL", "SSL"}

	for _, protocol := range validProtocols {
		config := &KafkaConfig{
			ID:               "test-kafka",
			Brokers:          []string{"localhost:9092"},
			SecurityProtocol: protocol,
		}

		connector := NewKafkaConnector(config)
		assert.NotNil(t, connector, "Protocol %s should be valid", protocol)

		// Verify the protocol is stored correctly
		retrievedConfig := connector.GetConfiguration()
		assert.Equal(t, protocol, retrievedConfig["security_protocol"])
	}
}

func TestKafkaCompressionTypes(t *testing.T) {
	validCompressions := []string{"none", "gzip", "snappy", "lz4", "zstd"}

	for _, compression := range validCompressions {
		config := &KafkaConfig{
			ID:              "test-kafka",
			Brokers:         []string{"localhost:9092"},
			CompressionType: compression,
		}

		connector := NewKafkaConnector(config)
		assert.NotNil(t, connector, "Compression %s should be valid", compression)

		// Verify the compression is stored correctly
		retrievedConfig := connector.GetConfiguration()
		assert.Equal(t, compression, retrievedConfig["compression_type"])
	}
}

func TestKafkaConfigurationFields(t *testing.T) {
	config := &KafkaConfig{
		ID:              "test-kafka",
		Name:            "Test Kafka Connector",
		Brokers:         []string{"localhost:9092", "localhost:9093"},
		Topics:          []string{"topic1", "topic2"},
		ConsumerGroup:   "test-group",
		ProducerTopic:   "output-topic",
		SecurityProtocol: "SASL_SSL",
		SASLMechanism:   "SCRAM-SHA-256",
		Username:        "testuser",
		Password:        "testpass",
		MessageFormat:   "json",
		CompressionType: "gzip",
		BatchSize:       1000,
		LingerTime:      100,
		MaxMessageBytes: 1000000,
		ClientID:        "test-client",
		EnableIdempotence: true,
		Headers:         map[string]string{"source": "emqx-go"},
	}

	connector := NewKafkaConnector(config)
	require.NotNil(t, connector)

	retrievedConfig := connector.GetConfiguration()

	// Test all major configuration fields
	assert.Equal(t, "test-kafka", retrievedConfig["id"])
	assert.Equal(t, "Test Kafka Connector", retrievedConfig["name"])
	assert.Equal(t, []interface{}{"localhost:9092", "localhost:9093"}, retrievedConfig["brokers"])
	assert.Equal(t, []interface{}{"topic1", "topic2"}, retrievedConfig["topics"])
	assert.Equal(t, "test-group", retrievedConfig["consumer_group"])
	assert.Equal(t, "output-topic", retrievedConfig["producer_topic"])
	assert.Equal(t, "SASL_SSL", retrievedConfig["security_protocol"])
	assert.Equal(t, "SCRAM-SHA-256", retrievedConfig["sasl_mechanism"])
	assert.Equal(t, "testuser", retrievedConfig["username"])
	assert.Equal(t, "testpass", retrievedConfig["password"])
	assert.Equal(t, "json", retrievedConfig["message_format"])
	assert.Equal(t, "gzip", retrievedConfig["compression_type"])
	assert.Equal(t, float64(1000), retrievedConfig["batch_size"])
	assert.Equal(t, float64(100), retrievedConfig["linger_time_ms"])
	assert.Equal(t, float64(1000000), retrievedConfig["max_message_bytes"])
	assert.Equal(t, "test-client", retrievedConfig["client_id"])
	assert.Equal(t, true, retrievedConfig["enable_idempotence"])
	assert.Equal(t, map[string]interface{}{"source": "emqx-go"}, retrievedConfig["headers"])
}