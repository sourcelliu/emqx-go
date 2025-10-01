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
	"fmt"
	"sync"
	"time"
)

// DataIntegrationEngine manages data integration flows
type DataIntegrationEngine struct {
	bridges     map[string]Bridge
	connectors  map[string]Connector
	processors  map[string]DataProcessor
	metrics     *IntegrationMetrics
	mu          sync.RWMutex
	ctx         context.Context
	cancel      context.CancelFunc
}

// Message represents a message in the data integration pipeline
type Message struct {
	ID            string                 `json:"id"`
	Topic         string                 `json:"topic"`
	Payload       []byte                 `json:"payload"`
	QoS           int                    `json:"qos"`
	Headers       map[string]string      `json:"headers"`
	Metadata      map[string]interface{} `json:"metadata"`
	Timestamp     time.Time              `json:"timestamp"`
	SourceType    string                 `json:"source_type"`    // mqtt, http, kafka, etc.
	SourceID      string                 `json:"source_id"`      // client_id, connector_id, etc.
	DestinationType string               `json:"destination_type,omitempty"`
	DestinationID   string               `json:"destination_id,omitempty"`
}

// Bridge represents a data bridge configuration
type Bridge struct {
	ID                string                 `json:"id"`
	Name              string                 `json:"name"`
	Description       string                 `json:"description"`
	Type              string                 `json:"type"`              // kafka, http, database, etc.
	Direction         string                 `json:"direction"`         // ingress, egress, bidirectional
	SourceConnector   string                 `json:"source_connector"`
	TargetConnector   string                 `json:"target_connector"`
	DataProcessor     string                 `json:"data_processor,omitempty"`
	Configuration     map[string]interface{} `json:"configuration"`
	Status            string                 `json:"status"`            // enabled, disabled, error
	Metrics           BridgeMetrics          `json:"metrics"`
	CreatedAt         time.Time              `json:"created_at"`
	UpdatedAt         time.Time              `json:"updated_at"`
}

// Connector interface defines the contract for all connectors
type Connector interface {
	ID() string
	Type() string
	Connect(ctx context.Context) error
	Disconnect(ctx context.Context) error
	Send(ctx context.Context, msg *Message) error
	Receive(ctx context.Context) (<-chan *Message, error)
	IsConnected() bool
	GetMetrics() ConnectorMetrics
	GetConfiguration() map[string]interface{}
	SetConfiguration(config map[string]interface{}) error
}

// DataProcessor interface for data transformation
type DataProcessor interface {
	ID() string
	Name() string
	Process(ctx context.Context, msg *Message) (*Message, error)
	GetConfiguration() map[string]interface{}
	SetConfiguration(config map[string]interface{}) error
}

// BridgeMetrics tracks bridge performance
type BridgeMetrics struct {
	MessagesProcessed int64     `json:"messages_processed"`
	MessagesSucceeded int64     `json:"messages_succeeded"`
	MessagesFailed    int64     `json:"messages_failed"`
	BytesProcessed    int64     `json:"bytes_processed"`
	AverageLatency    float64   `json:"average_latency_ms"`
	LastActivity      time.Time `json:"last_activity"`
	ErrorRate         float64   `json:"error_rate"`
}

// ConnectorMetrics tracks connector performance
type ConnectorMetrics struct {
	ConnectionStatus  string    `json:"connection_status"`
	MessagesReceived  int64     `json:"messages_received"`
	MessagesSent      int64     `json:"messages_sent"`
	ConnectionErrors  int64     `json:"connection_errors"`
	LastConnected     time.Time `json:"last_connected"`
	LastDisconnected  time.Time `json:"last_disconnected"`
	Throughput        float64   `json:"throughput_msg_per_sec"`
}

// IntegrationMetrics tracks overall system metrics
type IntegrationMetrics struct {
	TotalBridges      int                         `json:"total_bridges"`
	ActiveBridges     int                         `json:"active_bridges"`
	TotalConnectors   int                         `json:"total_connectors"`
	ConnectedSources  int                         `json:"connected_sources"`
	TotalMessages     int64                       `json:"total_messages"`
	BridgeMetrics     map[string]BridgeMetrics    `json:"bridge_metrics"`
	ConnectorMetrics  map[string]ConnectorMetrics `json:"connector_metrics"`
}

// NewDataIntegrationEngine creates a new data integration engine
func NewDataIntegrationEngine() *DataIntegrationEngine {
	ctx, cancel := context.WithCancel(context.Background())

	return &DataIntegrationEngine{
		bridges:    make(map[string]Bridge),
		connectors: make(map[string]Connector),
		processors: make(map[string]DataProcessor),
		metrics: &IntegrationMetrics{
			BridgeMetrics:    make(map[string]BridgeMetrics),
			ConnectorMetrics: make(map[string]ConnectorMetrics),
		},
		ctx:    ctx,
		cancel: cancel,
	}
}

// CreateBridge creates a new data bridge
func (die *DataIntegrationEngine) CreateBridge(bridge Bridge) error {
	die.mu.Lock()
	defer die.mu.Unlock()

	if _, exists := die.bridges[bridge.ID]; exists {
		return fmt.Errorf("bridge with ID %s already exists", bridge.ID)
	}

	// Validate bridge configuration
	if err := die.validateBridge(bridge); err != nil {
		return fmt.Errorf("invalid bridge configuration: %w", err)
	}

	bridge.CreatedAt = time.Now()
	bridge.UpdatedAt = time.Now()
	bridge.Status = "disabled"
	bridge.Metrics = BridgeMetrics{}

	die.bridges[bridge.ID] = bridge
	die.metrics.TotalBridges++

	return nil
}

// GetBridge retrieves a bridge by ID
func (die *DataIntegrationEngine) GetBridge(bridgeID string) (Bridge, error) {
	die.mu.RLock()
	defer die.mu.RUnlock()

	bridge, exists := die.bridges[bridgeID]
	if !exists {
		return Bridge{}, fmt.Errorf("bridge %s not found", bridgeID)
	}

	return bridge, nil
}

// ListBridges returns all bridges
func (die *DataIntegrationEngine) ListBridges() []Bridge {
	die.mu.RLock()
	defer die.mu.RUnlock()

	bridges := make([]Bridge, 0, len(die.bridges))
	for _, bridge := range die.bridges {
		bridges = append(bridges, bridge)
	}

	return bridges
}

// UpdateBridge updates an existing bridge
func (die *DataIntegrationEngine) UpdateBridge(bridgeID string, updates Bridge) error {
	die.mu.Lock()
	defer die.mu.Unlock()

	existing, exists := die.bridges[bridgeID]
	if !exists {
		return fmt.Errorf("bridge %s not found", bridgeID)
	}

	// Preserve certain fields
	updates.ID = existing.ID
	updates.CreatedAt = existing.CreatedAt
	updates.UpdatedAt = time.Now()
	updates.Metrics = existing.Metrics

	// Validate updated configuration
	if err := die.validateBridge(updates); err != nil {
		return fmt.Errorf("invalid bridge configuration: %w", err)
	}

	die.bridges[bridgeID] = updates
	return nil
}

// DeleteBridge deletes a bridge
func (die *DataIntegrationEngine) DeleteBridge(bridgeID string) error {
	die.mu.Lock()
	defer die.mu.Unlock()

	if _, exists := die.bridges[bridgeID]; !exists {
		return fmt.Errorf("bridge %s not found", bridgeID)
	}

	// Stop bridge if running
	if err := die.stopBridge(bridgeID); err != nil {
		return fmt.Errorf("failed to stop bridge: %w", err)
	}

	delete(die.bridges, bridgeID)
	delete(die.metrics.BridgeMetrics, bridgeID)
	die.metrics.TotalBridges--

	return nil
}

// EnableBridge enables a bridge
func (die *DataIntegrationEngine) EnableBridge(bridgeID string) error {
	die.mu.Lock()
	defer die.mu.Unlock()

	bridge, exists := die.bridges[bridgeID]
	if !exists {
		return fmt.Errorf("bridge %s not found", bridgeID)
	}

	if bridge.Status == "enabled" {
		return nil
	}

	// Start bridge
	if err := die.startBridge(bridgeID); err != nil {
		return fmt.Errorf("failed to start bridge: %w", err)
	}

	bridge.Status = "enabled"
	bridge.UpdatedAt = time.Now()
	die.bridges[bridgeID] = bridge
	die.metrics.ActiveBridges++

	return nil
}

// DisableBridge disables a bridge
func (die *DataIntegrationEngine) DisableBridge(bridgeID string) error {
	die.mu.Lock()
	defer die.mu.Unlock()

	bridge, exists := die.bridges[bridgeID]
	if !exists {
		return fmt.Errorf("bridge %s not found", bridgeID)
	}

	if bridge.Status == "disabled" {
		return nil
	}

	// Stop bridge
	if err := die.stopBridge(bridgeID); err != nil {
		return fmt.Errorf("failed to stop bridge: %w", err)
	}

	bridge.Status = "disabled"
	bridge.UpdatedAt = time.Now()
	die.bridges[bridgeID] = bridge
	die.metrics.ActiveBridges--

	return nil
}

// RegisterConnector registers a new connector
func (die *DataIntegrationEngine) RegisterConnector(connector Connector) error {
	die.mu.Lock()
	defer die.mu.Unlock()

	if _, exists := die.connectors[connector.ID()]; exists {
		return fmt.Errorf("connector with ID %s already exists", connector.ID())
	}

	die.connectors[connector.ID()] = connector
	die.metrics.TotalConnectors++

	return nil
}

// GetConnector retrieves a connector by ID
func (die *DataIntegrationEngine) GetConnector(connectorID string) (Connector, error) {
	die.mu.RLock()
	defer die.mu.RUnlock()

	connector, exists := die.connectors[connectorID]
	if !exists {
		return nil, fmt.Errorf("connector %s not found", connectorID)
	}

	return connector, nil
}

// ListConnectors returns all connectors
func (die *DataIntegrationEngine) ListConnectors() []Connector {
	die.mu.RLock()
	defer die.mu.RUnlock()

	connectors := make([]Connector, 0, len(die.connectors))
	for _, connector := range die.connectors {
		connectors = append(connectors, connector)
	}

	return connectors
}

// RegisterDataProcessor registers a data processor
func (die *DataIntegrationEngine) RegisterDataProcessor(processor DataProcessor) error {
	die.mu.Lock()
	defer die.mu.Unlock()

	if _, exists := die.processors[processor.ID()]; exists {
		return fmt.Errorf("data processor with ID %s already exists", processor.ID())
	}

	die.processors[processor.ID()] = processor
	return nil
}

// ProcessMessage processes a message through the integration pipeline
func (die *DataIntegrationEngine) ProcessMessage(ctx context.Context, msg *Message) error {
	die.mu.RLock()
	defer die.mu.RUnlock()

	die.metrics.TotalMessages++

	// Find applicable bridges
	for _, bridge := range die.bridges {
		if bridge.Status != "enabled" {
			continue
		}

		// Check if this bridge should process this message
		if die.shouldProcessMessage(bridge, msg) {
			go die.processThroughBridge(ctx, bridge, msg)
		}
	}

	return nil
}

// GetMetrics returns integration metrics
func (die *DataIntegrationEngine) GetMetrics() *IntegrationMetrics {
	die.mu.RLock()
	defer die.mu.RUnlock()

	// Update real-time metrics
	die.updateMetrics()

	return die.metrics
}

// Close shuts down the data integration engine
func (die *DataIntegrationEngine) Close() error {
	die.mu.Lock()
	defer die.mu.Unlock()

	die.cancel()

	// Disconnect all connectors
	for _, connector := range die.connectors {
		if connector.IsConnected() {
			connector.Disconnect(die.ctx)
		}
	}

	return nil
}

// validateBridge validates bridge configuration
func (die *DataIntegrationEngine) validateBridge(bridge Bridge) error {
	if bridge.ID == "" {
		return fmt.Errorf("bridge ID is required")
	}

	if bridge.Name == "" {
		return fmt.Errorf("bridge name is required")
	}

	if bridge.Type == "" {
		return fmt.Errorf("bridge type is required")
	}

	if bridge.Direction == "" {
		return fmt.Errorf("bridge direction is required")
	}

	validDirections := map[string]bool{
		"ingress":       true,
		"egress":        true,
		"bidirectional": true,
	}

	if !validDirections[bridge.Direction] {
		return fmt.Errorf("invalid bridge direction: %s", bridge.Direction)
	}

	return nil
}

// startBridge starts a bridge
func (die *DataIntegrationEngine) startBridge(bridgeID string) error {
	bridge := die.bridges[bridgeID]

	// Connect source connector if needed
	if bridge.SourceConnector != "" {
		if connector, exists := die.connectors[bridge.SourceConnector]; exists {
			if !connector.IsConnected() {
				if err := connector.Connect(die.ctx); err != nil {
					return fmt.Errorf("failed to connect source connector: %w", err)
				}
			}
		}
	}

	// Connect target connector if needed
	if bridge.TargetConnector != "" {
		if connector, exists := die.connectors[bridge.TargetConnector]; exists {
			if !connector.IsConnected() {
				if err := connector.Connect(die.ctx); err != nil {
					return fmt.Errorf("failed to connect target connector: %w", err)
				}
			}
		}
	}

	return nil
}

// stopBridge stops a bridge
func (die *DataIntegrationEngine) stopBridge(bridgeID string) error {
	// Implementation would handle stopping the bridge processing
	// For now, we'll just mark it as stopped
	return nil
}

// shouldProcessMessage determines if a bridge should process a message
func (die *DataIntegrationEngine) shouldProcessMessage(bridge Bridge, msg *Message) bool {
	// Simple topic-based routing for now
	// This could be enhanced with more sophisticated routing logic
	if topicFilter, ok := bridge.Configuration["topic_filter"].(string); ok {
		return die.matchTopic(topicFilter, msg.Topic)
	}

	return true
}

// matchTopic performs simple topic matching
func (die *DataIntegrationEngine) matchTopic(filter, topic string) bool {
	// Simple wildcard matching
	if filter == "#" || filter == topic {
		return true
	}

	// More sophisticated topic matching could be implemented here
	return false
}

// processThroughBridge processes a message through a specific bridge
func (die *DataIntegrationEngine) processThroughBridge(ctx context.Context, bridge Bridge, msg *Message) {
	startTime := time.Now()

	// Apply data processor if configured
	processedMsg := msg
	if bridge.DataProcessor != "" {
		if processor, exists := die.processors[bridge.DataProcessor]; exists {
			var err error
			processedMsg, err = processor.Process(ctx, msg)
			if err != nil {
				die.updateBridgeMetrics(bridge.ID, false, 0, time.Since(startTime))
				return
			}
		}
	}

	// Send to target connector
	if bridge.TargetConnector != "" {
		if connector, exists := die.connectors[bridge.TargetConnector]; exists {
			if err := connector.Send(ctx, processedMsg); err != nil {
				die.updateBridgeMetrics(bridge.ID, false, len(processedMsg.Payload), time.Since(startTime))
				return
			}
		}
	}

	die.updateBridgeMetrics(bridge.ID, true, len(processedMsg.Payload), time.Since(startTime))
}

// updateBridgeMetrics updates bridge metrics
func (die *DataIntegrationEngine) updateBridgeMetrics(bridgeID string, success bool, bytes int, latency time.Duration) {
	die.mu.Lock()
	defer die.mu.Unlock()

	metrics := die.metrics.BridgeMetrics[bridgeID]
	metrics.MessagesProcessed++
	metrics.BytesProcessed += int64(bytes)
	metrics.LastActivity = time.Now()

	if success {
		metrics.MessagesSucceeded++
	} else {
		metrics.MessagesFailed++
	}

	// Update average latency
	if metrics.MessagesProcessed == 1 {
		metrics.AverageLatency = float64(latency.Nanoseconds()) / 1e6
	} else {
		metrics.AverageLatency = (metrics.AverageLatency + float64(latency.Nanoseconds())/1e6) / 2
	}

	// Update error rate
	if metrics.MessagesProcessed > 0 {
		metrics.ErrorRate = float64(metrics.MessagesFailed) / float64(metrics.MessagesProcessed) * 100
	}

	die.metrics.BridgeMetrics[bridgeID] = metrics
}

// updateMetrics updates overall metrics
func (die *DataIntegrationEngine) updateMetrics() {
	activeCount := 0
	connectedCount := 0

	for _, bridge := range die.bridges {
		if bridge.Status == "enabled" {
			activeCount++
		}
	}

	for _, connector := range die.connectors {
		if connector.IsConnected() {
			connectedCount++
		}
	}

	die.metrics.ActiveBridges = activeCount
	die.metrics.ConnectedSources = connectedCount
}