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
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewDataIntegrationEngine(t *testing.T) {
	engine := NewDataIntegrationEngine()
	assert.NotNil(t, engine)
	assert.NotNil(t, engine.bridges)
	assert.NotNil(t, engine.connectors)
	assert.NotNil(t, engine.processors)
	assert.NotNil(t, engine.metrics)
	assert.Equal(t, 0, len(engine.bridges))
	assert.Equal(t, 0, len(engine.connectors))
	assert.Equal(t, 0, len(engine.processors))
}

func TestCreateBridge(t *testing.T) {
	engine := NewDataIntegrationEngine()

	bridge := Bridge{
		ID:          "test-bridge-1",
		Name:        "Test Bridge",
		Description: "Test bridge for unit testing",
		Type:        "kafka",
		Direction:   "egress",
		Configuration: map[string]interface{}{
			"brokers": []string{"localhost:9092"},
			"topic":   "test-topic",
		},
	}

	err := engine.CreateBridge(bridge)
	assert.NoError(t, err)

	// Verify bridge was created
	bridges := engine.ListBridges()
	assert.Equal(t, 1, len(bridges))
	assert.Equal(t, "test-bridge-1", bridges[0].ID)
	assert.Equal(t, "Test Bridge", bridges[0].Name)
	assert.Equal(t, "disabled", bridges[0].Status)

	// Verify metrics were updated
	metrics := engine.GetMetrics()
	assert.Equal(t, 1, metrics.TotalBridges)
	assert.Equal(t, 0, metrics.ActiveBridges)
}

func TestCreateBridgeValidation(t *testing.T) {
	engine := NewDataIntegrationEngine()

	testCases := []struct {
		name   string
		bridge Bridge
		hasErr bool
	}{
		{
			name: "missing ID",
			bridge: Bridge{
				Name:      "Test Bridge",
				Type:      "kafka",
				Direction: "egress",
			},
			hasErr: true,
		},
		{
			name: "missing name",
			bridge: Bridge{
				ID:        "test-bridge",
				Type:      "kafka",
				Direction: "egress",
			},
			hasErr: true,
		},
		{
			name: "missing type",
			bridge: Bridge{
				ID:        "test-bridge",
				Name:      "Test Bridge",
				Direction: "egress",
			},
			hasErr: true,
		},
		{
			name: "invalid direction",
			bridge: Bridge{
				ID:        "test-bridge",
				Name:      "Test Bridge",
				Type:      "kafka",
				Direction: "invalid",
			},
			hasErr: true,
		},
		{
			name: "valid bridge",
			bridge: Bridge{
				ID:        "test-bridge",
				Name:      "Test Bridge",
				Type:      "kafka",
				Direction: "egress",
			},
			hasErr: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := engine.CreateBridge(tc.bridge)
			if tc.hasErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestCreateDuplicateBridge(t *testing.T) {
	engine := NewDataIntegrationEngine()

	bridge := Bridge{
		ID:        "test-bridge",
		Name:      "Test Bridge",
		Type:      "kafka",
		Direction: "egress",
	}

	err := engine.CreateBridge(bridge)
	assert.NoError(t, err)

	// Try to create the same bridge again
	err = engine.CreateBridge(bridge)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")
}

func TestGetBridge(t *testing.T) {
	engine := NewDataIntegrationEngine()

	bridge := Bridge{
		ID:        "test-bridge",
		Name:      "Test Bridge",
		Type:      "kafka",
		Direction: "egress",
	}

	err := engine.CreateBridge(bridge)
	require.NoError(t, err)

	// Get existing bridge
	retrieved, err := engine.GetBridge("test-bridge")
	assert.NoError(t, err)
	assert.Equal(t, "test-bridge", retrieved.ID)
	assert.Equal(t, "Test Bridge", retrieved.Name)

	// Get non-existent bridge
	_, err = engine.GetBridge("non-existent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestUpdateBridge(t *testing.T) {
	engine := NewDataIntegrationEngine()

	// Create initial bridge
	bridge := Bridge{
		ID:        "test-bridge",
		Name:      "Test Bridge",
		Type:      "kafka",
		Direction: "egress",
	}

	err := engine.CreateBridge(bridge)
	require.NoError(t, err)

	// Update bridge
	updated := Bridge{
		ID:          "test-bridge",
		Name:        "Updated Bridge",
		Description: "Updated description",
		Type:        "kafka",
		Direction:   "bidirectional",
	}

	err = engine.UpdateBridge("test-bridge", updated)
	assert.NoError(t, err)

	// Verify update
	retrieved, err := engine.GetBridge("test-bridge")
	assert.NoError(t, err)
	assert.Equal(t, "Updated Bridge", retrieved.Name)
	assert.Equal(t, "Updated description", retrieved.Description)
	assert.Equal(t, "bidirectional", retrieved.Direction)

	// Update non-existent bridge
	err = engine.UpdateBridge("non-existent", updated)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestDeleteBridge(t *testing.T) {
	engine := NewDataIntegrationEngine()

	// Create bridge
	bridge := Bridge{
		ID:        "test-bridge",
		Name:      "Test Bridge",
		Type:      "kafka",
		Direction: "egress",
	}

	err := engine.CreateBridge(bridge)
	require.NoError(t, err)

	// Verify bridge exists
	bridges := engine.ListBridges()
	assert.Equal(t, 1, len(bridges))

	// Delete bridge
	err = engine.DeleteBridge("test-bridge")
	assert.NoError(t, err)

	// Verify bridge is deleted
	bridges = engine.ListBridges()
	assert.Equal(t, 0, len(bridges))

	metrics := engine.GetMetrics()
	assert.Equal(t, 0, metrics.TotalBridges)

	// Delete non-existent bridge
	err = engine.DeleteBridge("non-existent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestEnableDisableBridge(t *testing.T) {
	engine := NewDataIntegrationEngine()

	// Create bridge
	bridge := Bridge{
		ID:        "test-bridge",
		Name:      "Test Bridge",
		Type:      "kafka",
		Direction: "egress",
	}

	err := engine.CreateBridge(bridge)
	require.NoError(t, err)

	// Bridge should be disabled by default
	retrieved, err := engine.GetBridge("test-bridge")
	assert.NoError(t, err)
	assert.Equal(t, "disabled", retrieved.Status)

	// Enable bridge
	err = engine.EnableBridge("test-bridge")
	assert.NoError(t, err)

	retrieved, err = engine.GetBridge("test-bridge")
	assert.NoError(t, err)
	assert.Equal(t, "enabled", retrieved.Status)

	metrics := engine.GetMetrics()
	assert.Equal(t, 1, metrics.ActiveBridges)

	// Disable bridge
	err = engine.DisableBridge("test-bridge")
	assert.NoError(t, err)

	retrieved, err = engine.GetBridge("test-bridge")
	assert.NoError(t, err)
	assert.Equal(t, "disabled", retrieved.Status)

	metrics = engine.GetMetrics()
	assert.Equal(t, 0, metrics.ActiveBridges)

	// Enable already enabled bridge (should be no-op)
	err = engine.EnableBridge("test-bridge")
	require.NoError(t, err)
	err = engine.EnableBridge("test-bridge")
	assert.NoError(t, err)

	// Disable already disabled bridge (should be no-op)
	err = engine.DisableBridge("test-bridge")
	require.NoError(t, err)
	err = engine.DisableBridge("test-bridge")
	assert.NoError(t, err)
}

func TestRegisterConnector(t *testing.T) {
	engine := NewDataIntegrationEngine()

	connector := &MockConnector{
		id:   "test-connector",
		ctype: "mock",
	}

	err := engine.RegisterConnector(connector)
	assert.NoError(t, err)

	// Verify connector was registered
	connectors := engine.ListConnectors()
	assert.Equal(t, 1, len(connectors))
	assert.Equal(t, "test-connector", connectors[0].ID())

	metrics := engine.GetMetrics()
	assert.Equal(t, 1, metrics.TotalConnectors)

	// Register duplicate connector
	err = engine.RegisterConnector(connector)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")
}

func TestGetConnector(t *testing.T) {
	engine := NewDataIntegrationEngine()

	connector := &MockConnector{
		id:   "test-connector",
		ctype: "mock",
	}

	err := engine.RegisterConnector(connector)
	require.NoError(t, err)

	// Get existing connector
	retrieved, err := engine.GetConnector("test-connector")
	assert.NoError(t, err)
	assert.Equal(t, "test-connector", retrieved.ID())

	// Get non-existent connector
	_, err = engine.GetConnector("non-existent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestProcessMessage(t *testing.T) {
	engine := NewDataIntegrationEngine()

	// Create and enable a bridge
	bridge := Bridge{
		ID:        "test-bridge",
		Name:      "Test Bridge",
		Type:      "kafka",
		Direction: "egress",
		Configuration: map[string]interface{}{
			"topic_filter": "sensor/temperature",
		},
	}

	err := engine.CreateBridge(bridge)
	require.NoError(t, err)

	err = engine.EnableBridge("test-bridge")
	require.NoError(t, err)

	// Create test message
	msg := &Message{
		ID:        "test-msg",
		Topic:     "sensor/temperature",
		Payload:   []byte(`{"temperature": 25.5}`),
		QoS:       1,
		Timestamp: time.Now(),
	}

	// Process message
	ctx := context.Background()
	err = engine.ProcessMessage(ctx, msg)
	assert.NoError(t, err)

	// Verify metrics
	metrics := engine.GetMetrics()
	assert.Equal(t, int64(1), metrics.TotalMessages)
}

func TestClose(t *testing.T) {
	engine := NewDataIntegrationEngine()

	connector := &MockConnector{
		id:        "test-connector",
		ctype:     "mock",
		connected: true,
	}

	err := engine.RegisterConnector(connector)
	require.NoError(t, err)

	// Close engine
	err = engine.Close()
	assert.NoError(t, err)

	// Verify connector was disconnected
	assert.False(t, connector.connected)
}

// MockConnector for testing
type MockConnector struct {
	id            string
	ctype         string
	connected     bool
	config        map[string]interface{}
	metrics       ConnectorMetrics
	messageChan   chan *Message
}

func (m *MockConnector) ID() string {
	return m.id
}

func (m *MockConnector) Type() string {
	return m.ctype
}

func (m *MockConnector) Connect(ctx context.Context) error {
	m.connected = true
	return nil
}

func (m *MockConnector) Disconnect(ctx context.Context) error {
	m.connected = false
	return nil
}

func (m *MockConnector) Send(ctx context.Context, msg *Message) error {
	if !m.connected {
		return ErrConnectorNotConnected
	}
	m.metrics.MessagesSent++
	return nil
}

func (m *MockConnector) Receive(ctx context.Context) (<-chan *Message, error) {
	if !m.connected {
		return nil, ErrConnectorNotConnected
	}
	if m.messageChan == nil {
		m.messageChan = make(chan *Message, 10)
	}
	return m.messageChan, nil
}

func (m *MockConnector) IsConnected() bool {
	return m.connected
}

func (m *MockConnector) GetMetrics() ConnectorMetrics {
	return m.metrics
}

func (m *MockConnector) GetConfiguration() map[string]interface{} {
	return m.config
}

func (m *MockConnector) SetConfiguration(config map[string]interface{}) error {
	m.config = config
	return nil
}