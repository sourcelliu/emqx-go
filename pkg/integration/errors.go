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

import "errors"

// Common errors for data integration
var (
	// Bridge errors
	ErrBridgeNotFound      = errors.New("bridge not found")
	ErrBridgeAlreadyExists = errors.New("bridge already exists")
	ErrBridgeInvalidConfig = errors.New("invalid bridge configuration")
	ErrBridgeNotEnabled    = errors.New("bridge not enabled")
	ErrBridgeStartFailed   = errors.New("failed to start bridge")
	ErrBridgeStopFailed    = errors.New("failed to stop bridge")

	// Connector errors
	ErrConnectorNotFound      = errors.New("connector not found")
	ErrConnectorAlreadyExists = errors.New("connector already exists")
	ErrConnectorNotConnected  = errors.New("connector not connected")
	ErrConnectorConnectionFailed = errors.New("connector connection failed")
	ErrConnectorSendFailed    = errors.New("failed to send message via connector")
	ErrConnectorReceiveFailed = errors.New("failed to receive message from connector")

	// Processor errors
	ErrProcessorNotFound      = errors.New("data processor not found")
	ErrProcessorAlreadyExists = errors.New("data processor already exists")
	ErrProcessorFailed        = errors.New("data processing failed")
	ErrInvalidTransformation  = errors.New("invalid data transformation")

	// Message errors
	ErrInvalidMessage      = errors.New("invalid message format")
	ErrMessageTooLarge     = errors.New("message too large")
	ErrMessageProcessingFailed = errors.New("message processing failed")

	// Configuration errors
	ErrInvalidConfiguration = errors.New("invalid configuration")
	ErrMissingRequiredField = errors.New("missing required field")
	ErrInvalidFieldValue    = errors.New("invalid field value")

	// Kafka specific errors
	ErrKafkaConnectionFailed = errors.New("Kafka connection failed")
	ErrKafkaProducerFailed   = errors.New("Kafka producer failed")
	ErrKafkaConsumerFailed   = errors.New("Kafka consumer failed")
	ErrKafkaTopicNotFound    = errors.New("Kafka topic not found")
	ErrKafkaAuthFailed       = errors.New("Kafka authentication failed")
	ErrKafkaSSLFailed        = errors.New("Kafka SSL configuration failed")

	// Integration engine errors
	ErrEngineNotRunning     = errors.New("integration engine not running")
	ErrEngineAlreadyRunning = errors.New("integration engine already running")
	ErrEngineShutdownFailed = errors.New("integration engine shutdown failed")
)