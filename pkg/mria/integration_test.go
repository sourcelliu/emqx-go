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

package mria

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	clusterpb "github.com/turtacn/emqx-go/pkg/proto/cluster"
)

func TestNewBrokerIntegration(t *testing.T) {
	integration := NewBrokerIntegration("test-node", CoreNode, nil)

	assert.NotNil(t, integration)
	assert.Equal(t, "test-node", integration.nodeID)
	assert.Equal(t, CoreNode, integration.nodeRole)
	assert.NotNil(t, integration.mriaManager)
	assert.NotNil(t, integration.replicationManager)
	assert.NotNil(t, integration.clusterServer)
	assert.NotNil(t, integration.localSessions)
	assert.NotNil(t, integration.localRoutes)
	assert.False(t, integration.isInitialized)
}

func TestBrokerIntegrationInitializeShutdown(t *testing.T) {
	integration := NewBrokerIntegration("test-node", CoreNode, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Test initialization
	err := integration.Initialize(ctx, "localhost:0")
	assert.NoError(t, err)
	assert.True(t, integration.IsInitialized())

	// Test double initialization
	err = integration.Initialize(ctx, "localhost:0")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already initialized")

	// Test shutdown
	err = integration.Shutdown()
	assert.NoError(t, err)
	assert.False(t, integration.IsInitialized())

	// Test double shutdown (should not error)
	err = integration.Shutdown()
	assert.NoError(t, err)
}

func TestBrokerIntegrationGetMethods(t *testing.T) {
	integration := NewBrokerIntegration("test-node", ReplicantNode, nil)

	// Test GetNodeRole
	assert.Equal(t, ReplicantNode, integration.GetNodeRole())

	// Test GetClusterState
	state := integration.GetClusterState()
	assert.NotNil(t, state)

	// Test IsLeader (should be false for replicant)
	assert.False(t, integration.IsLeader())

	// Test GetClusterStats
	stats := integration.GetClusterStats()
	assert.NotNil(t, stats)
	assert.Contains(t, stats, "node_role")
	assert.Equal(t, string(ReplicantNode), stats["node_role"])
	assert.Contains(t, stats, "is_leader")
	assert.Equal(t, false, stats["is_leader"])
}

func TestBrokerIntegrationCallbacks(t *testing.T) {
	integration := NewBrokerIntegration("test-node", CoreNode, nil)

	var publishCalled bool
	var connectCalled bool
	var disconnectCalled bool
	var subscribeCalled bool
	var unsubscribeCalled bool

	// Set callbacks
	integration.SetBrokerCallbacks(
		func(topic string, payload []byte, qos byte, retain bool, userProperties map[string][]byte, responseTopic string, correlationData []byte) {
			publishCalled = true
		},
		func(clientID string, cleanSession bool) {
			connectCalled = true
		},
		func(clientID string) {
			disconnectCalled = true
		},
		func(clientID, topicFilter string, qos int) {
			subscribeCalled = true
		},
		func(clientID, topicFilter string) {
			unsubscribeCalled = true
		},
	)

	// Test callback assignment
	assert.NotNil(t, integration.onMessagePublish)
	assert.NotNil(t, integration.onClientConnect)
	assert.NotNil(t, integration.onClientDisconnect)
	assert.NotNil(t, integration.onSubscribe)
	assert.NotNil(t, integration.onUnsubscribe)

	// Test callback invocation through handle methods
	integration.HandleClientConnect("client-1", true)
	integration.HandleSubscribe("client-1", "test/topic", 1)
	integration.HandleMessagePublish("test/topic", []byte("payload"), 1, false, nil, "", nil)
	integration.HandleUnsubscribe("client-1", "test/topic")
	integration.HandleClientDisconnect("client-1")

	assert.True(t, connectCalled)
	assert.True(t, subscribeCalled)
	assert.True(t, publishCalled)
	assert.True(t, unsubscribeCalled)
	assert.True(t, disconnectCalled)
}

func TestBrokerIntegrationClientConnect(t *testing.T) {
	integration := NewBrokerIntegration("core-1", CoreNode, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	err := integration.Initialize(ctx, "localhost:0")
	require.NoError(t, err)
	defer integration.Shutdown()

	// Test client connection
	integration.HandleClientConnect("client-1", false)

	// Check local session
	integration.mu.RLock()
	session, exists := integration.localSessions["client-1"]
	integration.mu.RUnlock()

	assert.True(t, exists)
	assert.Equal(t, "client-1", session.ClientID)
	assert.True(t, session.Connected)
	assert.False(t, session.CleanSession)

	// Check replication state
	replicationSession, exists := integration.replicationManager.GetSession("client-1")
	assert.True(t, exists)
	assert.Equal(t, "client-1", replicationSession.ClientID)
	assert.True(t, replicationSession.Connected)
}

func TestBrokerIntegrationClientDisconnect(t *testing.T) {
	integration := NewBrokerIntegration("core-1", CoreNode, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	err := integration.Initialize(ctx, "localhost:0")
	require.NoError(t, err)
	defer integration.Shutdown()

	// Connect client first
	integration.HandleClientConnect("client-1", false)

	// Verify connection
	integration.mu.RLock()
	session, exists := integration.localSessions["client-1"]
	integration.mu.RUnlock()
	assert.True(t, exists)
	assert.True(t, session.Connected)

	// Disconnect client
	integration.HandleClientDisconnect("client-1")

	// Check local session (should still exist for persistent session)
	integration.mu.RLock()
	session, exists = integration.localSessions["client-1"]
	integration.mu.RUnlock()
	assert.True(t, exists)
	assert.False(t, session.Connected)

	// Test clean session disconnect
	integration.HandleClientConnect("client-2", true)
	integration.HandleClientDisconnect("client-2")

	// Clean session should be removed
	integration.mu.RLock()
	_, exists = integration.localSessions["client-2"]
	integration.mu.RUnlock()
	assert.False(t, exists)
}

func TestBrokerIntegrationSubscribe(t *testing.T) {
	integration := NewBrokerIntegration("core-1", CoreNode, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	err := integration.Initialize(ctx, "localhost:0")
	require.NoError(t, err)
	defer integration.Shutdown()

	// Test subscription
	integration.HandleSubscribe("client-1", "test/topic", 1)

	// Check local routes
	integration.mu.RLock()
	clients, exists := integration.localRoutes["test/topic"]
	integration.mu.RUnlock()

	assert.True(t, exists)
	assert.Len(t, clients, 1)
	assert.Equal(t, "client-1", clients[0])

	// Check replication state
	subscriptions := integration.replicationManager.GetSubscriptions("client-1")
	assert.Len(t, subscriptions, 1)
	assert.Equal(t, "client-1", subscriptions[0].ClientID)
	assert.Equal(t, "test/topic", subscriptions[0].TopicFilter)
	assert.Equal(t, 1, subscriptions[0].QoS)

	// Test multiple clients on same topic
	integration.HandleSubscribe("client-2", "test/topic", 2)

	integration.mu.RLock()
	clients, exists = integration.localRoutes["test/topic"]
	integration.mu.RUnlock()

	assert.True(t, exists)
	assert.Len(t, clients, 2)
	assert.Contains(t, clients, "client-1")
	assert.Contains(t, clients, "client-2")
}

func TestBrokerIntegrationUnsubscribe(t *testing.T) {
	integration := NewBrokerIntegration("core-1", CoreNode, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	err := integration.Initialize(ctx, "localhost:0")
	require.NoError(t, err)
	defer integration.Shutdown()

	// Subscribe first
	integration.HandleSubscribe("client-1", "test/topic", 1)
	integration.HandleSubscribe("client-2", "test/topic", 1)

	// Verify subscriptions
	integration.mu.RLock()
	clients, exists := integration.localRoutes["test/topic"]
	integration.mu.RUnlock()
	assert.True(t, exists)
	assert.Len(t, clients, 2)

	// Unsubscribe one client
	integration.HandleUnsubscribe("client-1", "test/topic")

	// Check local routes
	integration.mu.RLock()
	clients, exists = integration.localRoutes["test/topic"]
	integration.mu.RUnlock()

	assert.True(t, exists)
	assert.Len(t, clients, 1)
	assert.Equal(t, "client-2", clients[0])

	// Unsubscribe last client
	integration.HandleUnsubscribe("client-2", "test/topic")

	// Route should be removed
	integration.mu.RLock()
	_, exists = integration.localRoutes["test/topic"]
	integration.mu.RUnlock()
	assert.False(t, exists)

	// Check replication state
	subscriptions := integration.replicationManager.GetSubscriptions("client-1")
	assert.Len(t, subscriptions, 0)
}

func TestBrokerIntegrationMessagePublish(t *testing.T) {
	integration := NewBrokerIntegration("core-1", CoreNode, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	err := integration.Initialize(ctx, "localhost:0")
	require.NoError(t, err)
	defer integration.Shutdown()

	var publishedTopic string
	var publishedPayload []byte

	// Set callback to capture published message
	integration.SetBrokerCallbacks(
		func(topic string, payload []byte, qos byte, retain bool, userProperties map[string][]byte, responseTopic string, correlationData []byte) {
			publishedTopic = topic
			publishedPayload = payload
		},
		nil, nil, nil, nil,
	)

	// Test message publish
	userProps := map[string][]byte{"key": []byte("value")}
	integration.HandleMessagePublish("test/topic", []byte("test payload"), 1, false, userProps, "response/topic", []byte("correlation"))

	assert.Equal(t, "test/topic", publishedTopic)
	assert.Equal(t, []byte("test payload"), publishedPayload)
}

func TestBrokerIntegrationNodeCallbacks(t *testing.T) {
	integration := NewBrokerIntegration("core-1", CoreNode, nil)

	// Test node join callback
	nodeInfo := &clusterpb.NodeInfo{
		NodeId:  "new-node",
		Address: "localhost:8080",
	}

	// This should not panic
	integration.handleNodeJoin(nodeInfo)

	// Test node leave callback
	integration.handleNodeLeave("old-node")

	// Test leadership callbacks
	integration.handleBecomeLeader()
	integration.handleLoseLeadership()

	// No assertions needed - just verify no panics
}

func TestBrokerIntegrationReplicationCallbacks(t *testing.T) {
	integration := NewBrokerIntegration("core-1", CoreNode, nil)

	// Test subscription change callback
	sub := &SubscriptionData{
		ClientID:    "client-1",
		TopicFilter: "test/topic",
		QoS:         1,
		NodeID:      "remote-node",
	}
	integration.handleSubscriptionChange(sub, "INSERT")

	// Test session change callback
	session := &SessionData{
		ClientID:  "client-1",
		NodeID:    "remote-node",
		Connected: true,
	}
	integration.handleSessionChange(session, "UPDATE")

	// Test route change callback
	route := &RouteData{
		TopicFilter: "test/topic",
		NodeIDs:     []string{"node-1", "node-2"},
	}
	integration.handleRouteChange(route, "UPDATE")

	// No assertions needed - just verify no panics
}

func TestBrokerIntegrationLocalSubscriberCheck(t *testing.T) {
	integration := NewBrokerIntegration("core-1", CoreNode, nil)

	// Initially no local subscribers
	assert.False(t, integration.hasLocalSubscribers("test/topic"))

	// Add local route
	integration.mu.Lock()
	integration.localRoutes["test/topic"] = []string{"client-1"}
	integration.mu.Unlock()

	// Now should have local subscribers
	assert.True(t, integration.hasLocalSubscribers("test/topic"))

	// Different topic should not have subscribers
	assert.False(t, integration.hasLocalSubscribers("other/topic"))
}

func TestBrokerIntegrationCleanupNodeData(t *testing.T) {
	integration := NewBrokerIntegration("core-1", CoreNode, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	err := integration.Initialize(ctx, "localhost:0")
	require.NoError(t, err)
	defer integration.Shutdown()

	// Add some test data from a remote node
	departed_nodeID := "departed-node"

	// Add session from departed node
	session := &SessionData{
		ClientID:  "remote-client",
		NodeID:    departed_nodeID,
		Connected: true,
	}
	integration.replicationManager.state.sessions["remote-client"] = session

	// Add subscription from departed node
	sub := &SubscriptionData{
		ClientID:    "remote-client",
		TopicFilter: "remote/topic",
		QoS:         1,
		NodeID:      departed_nodeID,
	}
	key := "remote-client:remote/topic"
	integration.replicationManager.state.subscriptions[key] = sub

	// Add route pointing to departed node
	route := &RouteData{
		TopicFilter: "remote/topic",
		NodeIDs:     []string{"core-1", departed_nodeID},
	}
	integration.replicationManager.state.routes["remote/topic"] = route

	// Verify data exists
	_, exists := integration.replicationManager.GetSession("remote-client")
	assert.True(t, exists)

	subs := integration.replicationManager.GetAllSubscriptions()
	assert.Contains(t, subs, key)

	routeData, exists := integration.replicationManager.GetRoute("remote/topic")
	assert.True(t, exists)
	assert.Contains(t, routeData.NodeIDs, departed_nodeID)

	// Clean up data from departed node
	integration.cleanupNodeData(departed_nodeID)

	// Verify cleanup
	_, exists = integration.replicationManager.GetSession("remote-client")
	assert.False(t, exists)

	subs = integration.replicationManager.GetAllSubscriptions()
	assert.NotContains(t, subs, key)

	routeData, exists = integration.replicationManager.GetRoute("remote/topic")
	assert.True(t, exists)
	assert.NotContains(t, routeData.NodeIDs, departed_nodeID)
	assert.Contains(t, routeData.NodeIDs, "core-1")
}

func TestBrokerIntegrationCoreNodeBehavior(t *testing.T) {
	integration := NewBrokerIntegration("core-1", CoreNode, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	err := integration.Initialize(ctx, "localhost:0")
	require.NoError(t, err)
	defer integration.Shutdown()

	// Core node should handle cluster routing
	var routingCalled bool

	// Mock the cluster routing by overriding the callback
	originalCallback := integration.onMessagePublish
	integration.onMessagePublish = func(topic string, payload []byte, qos byte, retain bool, userProperties map[string][]byte, responseTopic string, correlationData []byte) {
		routingCalled = true
		if originalCallback != nil {
			originalCallback(topic, payload, qos, retain, userProperties, responseTopic, correlationData)
		}
	}

	// Publish a message
	integration.HandleMessagePublish("test/topic", []byte("payload"), 1, false, nil, "", nil)

	assert.True(t, routingCalled)
}

func TestBrokerIntegrationReplicantNodeBehavior(t *testing.T) {
	integration := NewBrokerIntegration("replicant-1", ReplicantNode, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	err := integration.Initialize(ctx, "localhost:0")
	require.NoError(t, err)
	defer integration.Shutdown()

	// Replicant node should forward messages without local subscribers
	// This is tested by checking that ForwardMessageToCore is called
	// Since we don't have core nodes in this test, it should return an error

	// Test message publish on replicant without local subscribers
	integration.HandleMessagePublish("remote/topic", []byte("payload"), 1, false, nil, "", nil)

	// No assertions needed for now as the forwarding logic is internally handled
	// In a real scenario, this would forward to core nodes
}

func TestBrokerIntegrationStats(t *testing.T) {
	integration := NewBrokerIntegration("test-node", CoreNode, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	err := integration.Initialize(ctx, "localhost:0")
	require.NoError(t, err)
	defer integration.Shutdown()

	// Add some test data
	integration.HandleClientConnect("client-1", false)
	integration.HandleSubscribe("client-1", "test/topic", 1)

	// Get stats
	stats := integration.GetClusterStats()

	assert.Contains(t, stats, "cluster_id")
	assert.Contains(t, stats, "leader")
	assert.Contains(t, stats, "core_nodes")
	assert.Contains(t, stats, "replicant_nodes")
	assert.Contains(t, stats, "is_leader")
	assert.Contains(t, stats, "node_role")
	assert.Contains(t, stats, "local_sessions")
	assert.Contains(t, stats, "local_routes")
	assert.Contains(t, stats, "all_subscriptions")
	assert.Contains(t, stats, "all_sessions")
	assert.Contains(t, stats, "all_routes")

	assert.Equal(t, string(CoreNode), stats["node_role"])
	assert.Equal(t, 1, stats["local_sessions"])
	assert.Equal(t, 1, stats["local_routes"])
}