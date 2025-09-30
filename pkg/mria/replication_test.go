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
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewReplicationState(t *testing.T) {
	state := NewReplicationState()

	assert.NotNil(t, state)
	assert.NotNil(t, state.subscriptions)
	assert.NotNil(t, state.sessions)
	assert.NotNil(t, state.routes)
	assert.NotNil(t, state.replicationLogs)
	assert.Equal(t, int64(0), state.lastLogID)
	assert.Equal(t, int64(0), state.version)
	assert.Equal(t, 0, len(state.subscriptions))
	assert.Equal(t, 0, len(state.sessions))
	assert.Equal(t, 0, len(state.routes))
}

func TestNewReplicationManager(t *testing.T) {
	manager := NewReplicationManager("test-node", CoreNode, nil)

	assert.NotNil(t, manager)
	assert.Equal(t, "test-node", manager.nodeID)
	assert.Equal(t, CoreNode, manager.nodeRole)
	assert.NotNil(t, manager.state)
	assert.NotNil(t, manager.replicationCh)
	assert.NotNil(t, manager.stopCh)
}

func TestReplicationManagerStartStop(t *testing.T) {
	manager := NewReplicationManager("test-node", CoreNode, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Test start
	err := manager.Start(ctx)
	assert.NoError(t, err)

	// Test stop
	err = manager.Stop()
	assert.NoError(t, err)
}

func TestReplicationManagerAddSubscription(t *testing.T) {
	manager := NewReplicationManager("core-1", CoreNode, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := manager.Start(ctx)
	require.NoError(t, err)
	defer manager.Stop()

	// Test adding subscription
	err = manager.AddSubscription("client-1", "test/topic", 1)
	assert.NoError(t, err)

	// Verify subscription was added
	subscriptions := manager.GetSubscriptions("client-1")
	assert.Len(t, subscriptions, 1)
	assert.Equal(t, "client-1", subscriptions[0].ClientID)
	assert.Equal(t, "test/topic", subscriptions[0].TopicFilter)
	assert.Equal(t, 1, subscriptions[0].QoS)
	assert.Equal(t, "core-1", subscriptions[0].NodeID)

	// Test all subscriptions
	allSubs := manager.GetAllSubscriptions()
	assert.Len(t, allSubs, 1)
	key := "client-1:test/topic"
	assert.Contains(t, allSubs, key)
}

func TestReplicationManagerRemoveSubscription(t *testing.T) {
	manager := NewReplicationManager("core-1", CoreNode, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := manager.Start(ctx)
	require.NoError(t, err)
	defer manager.Stop()

	// Add a subscription first
	err = manager.AddSubscription("client-1", "test/topic", 1)
	require.NoError(t, err)

	// Verify it was added
	subscriptions := manager.GetSubscriptions("client-1")
	assert.Len(t, subscriptions, 1)

	// Remove the subscription
	err = manager.RemoveSubscription("client-1", "test/topic")
	assert.NoError(t, err)

	// Verify it was removed
	subscriptions = manager.GetSubscriptions("client-1")
	assert.Len(t, subscriptions, 0)

	// Test removing non-existent subscription
	err = manager.RemoveSubscription("client-1", "non/existent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "subscription not found")
}

func TestReplicationManagerAddSession(t *testing.T) {
	manager := NewReplicationManager("core-1", CoreNode, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := manager.Start(ctx)
	require.NoError(t, err)
	defer manager.Stop()

	// Test adding session
	expiryTime := time.Now().Add(1 * time.Hour)
	err = manager.AddSession("client-1", true, false, expiryTime)
	assert.NoError(t, err)

	// Verify session was added
	session, exists := manager.GetSession("client-1")
	assert.True(t, exists)
	assert.Equal(t, "client-1", session.ClientID)
	assert.Equal(t, "core-1", session.NodeID)
	assert.True(t, session.Connected)
	assert.False(t, session.CleanSession)
	assert.Equal(t, expiryTime.Unix(), session.ExpiryTime.Unix())

	// Test all sessions
	allSessions := manager.GetAllSessions()
	assert.Len(t, allSessions, 1)
	assert.Contains(t, allSessions, "client-1")
}

func TestReplicationManagerRemoveSession(t *testing.T) {
	manager := NewReplicationManager("core-1", CoreNode, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := manager.Start(ctx)
	require.NoError(t, err)
	defer manager.Stop()

	// Add a session first
	expiryTime := time.Now().Add(1 * time.Hour)
	err = manager.AddSession("client-1", true, false, expiryTime)
	require.NoError(t, err)

	// Verify it was added
	_, exists := manager.GetSession("client-1")
	assert.True(t, exists)

	// Remove the session
	err = manager.RemoveSession("client-1")
	assert.NoError(t, err)

	// Verify it was removed
	_, exists = manager.GetSession("client-1")
	assert.False(t, exists)

	// Test removing non-existent session
	err = manager.RemoveSession("non-existent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "session not found")
}

func TestReplicationManagerUpdateRoute(t *testing.T) {
	manager := NewReplicationManager("core-1", CoreNode, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := manager.Start(ctx)
	require.NoError(t, err)
	defer manager.Stop()

	// Test updating route
	nodeIDs := []string{"node-1", "node-2"}
	err = manager.UpdateRoute("test/topic", nodeIDs)
	assert.NoError(t, err)

	// Verify route was updated
	route, exists := manager.GetRoute("test/topic")
	assert.True(t, exists)
	assert.Equal(t, "test/topic", route.TopicFilter)
	assert.Equal(t, nodeIDs, route.NodeIDs)

	// Test all routes
	allRoutes := manager.GetAllRoutes()
	assert.Len(t, allRoutes, 1)
	assert.Contains(t, allRoutes, "test/topic")
}

func TestReplicationManagerCallbacks(t *testing.T) {
	manager := NewReplicationManager("core-1", CoreNode, nil)

	var subscriptionChangeCalled bool
	var sessionChangeCalled bool
	var routeChangeCalled bool

	// Set callbacks
	manager.SetCallbacks(
		func(*SubscriptionData, string) { subscriptionChangeCalled = true },
		func(*SessionData, string) { sessionChangeCalled = true },
		func(*RouteData, string) { routeChangeCalled = true },
	)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := manager.Start(ctx)
	require.NoError(t, err)
	defer manager.Stop()

	// Add data to trigger callbacks
	err = manager.AddSubscription("client-1", "test/topic", 1)
	require.NoError(t, err)

	expiryTime := time.Now().Add(1 * time.Hour)
	err = manager.AddSession("client-1", true, false, expiryTime)
	require.NoError(t, err)

	err = manager.UpdateRoute("test/topic", []string{"node-1"})
	require.NoError(t, err)

	// Allow some time for async operations
	time.Sleep(100 * time.Millisecond)

	assert.True(t, subscriptionChangeCalled)
	assert.True(t, sessionChangeCalled)
	assert.True(t, routeChangeCalled)
}

func TestReplicationLogGeneration(t *testing.T) {
	manager := NewReplicationManager("core-1", CoreNode, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := manager.Start(ctx)
	require.NoError(t, err)
	defer manager.Stop()

	// Add subscription to generate log
	err = manager.AddSubscription("client-1", "test/topic", 1)
	require.NoError(t, err)

	// Allow time for async processing
	time.Sleep(100 * time.Millisecond)

	// Check that log was generated
	assert.Greater(t, manager.state.lastLogID, int64(0))
	assert.Greater(t, manager.state.version, int64(0))
	assert.Greater(t, len(manager.state.replicationLogs), 0)

	// Check log content
	log := manager.state.replicationLogs[0]
	assert.Equal(t, "core-1", log.NodeID)
	assert.Equal(t, "INSERT", log.Operation)
	assert.Equal(t, "subscriptions", log.Table)
	assert.NotEmpty(t, log.ID)
	assert.Greater(t, log.Timestamp, int64(0))
}

func TestApplyReplicationLog(t *testing.T) {
	// Create a replicant node
	manager := NewReplicationManager("replicant-1", ReplicantNode, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := manager.Start(ctx)
	require.NoError(t, err)
	defer manager.Stop()

	// Create a replication log for subscription
	subscription := &SubscriptionData{
		ClientID:    "client-1",
		TopicFilter: "test/topic",
		QoS:         1,
		NodeID:      "core-1",
	}

	logEntry := ReplicationLog{
		ID:        "test-log-1",
		Timestamp: time.Now().Unix(),
		NodeID:    "core-1",
		Operation: "INSERT",
		Table:     "subscriptions",
		Data:      subscription,
		Version:   1,
	}

	logBytes, err := json.Marshal(logEntry)
	require.NoError(t, err)

	// Apply the log
	err = manager.ApplyReplicationLog(logBytes)
	assert.NoError(t, err)

	// Verify the subscription was applied
	subs := manager.GetSubscriptions("client-1")
	assert.Len(t, subs, 1)
	assert.Equal(t, "client-1", subs[0].ClientID)
	assert.Equal(t, "test/topic", subs[0].TopicFilter)
	assert.Equal(t, 1, subs[0].QoS)
	assert.Equal(t, "core-1", subs[0].NodeID)

	// Test invalid log
	err = manager.ApplyReplicationLog([]byte("invalid json"))
	assert.Error(t, err)
}

func TestApplySessionReplicationLog(t *testing.T) {
	manager := NewReplicationManager("replicant-1", ReplicantNode, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := manager.Start(ctx)
	require.NoError(t, err)
	defer manager.Stop()

	// Create a session replication log
	expiryTime := time.Now().Add(1 * time.Hour)
	sessionData := &SessionData{
		ClientID:     "client-1",
		NodeID:       "core-1",
		Connected:    true,
		CleanSession: false,
		ExpiryTime:   expiryTime,
	}

	logEntry := ReplicationLog{
		ID:        "test-session-log-1",
		Timestamp: time.Now().Unix(),
		NodeID:    "core-1",
		Operation: "INSERT",
		Table:     "sessions",
		Data:      sessionData,
		Version:   1,
	}

	logBytes, err := json.Marshal(logEntry)
	require.NoError(t, err)

	// Apply the log
	err = manager.ApplyReplicationLog(logBytes)
	assert.NoError(t, err)

	// Verify the session was applied
	session, exists := manager.GetSession("client-1")
	assert.True(t, exists)
	assert.Equal(t, "client-1", session.ClientID)
	assert.Equal(t, "core-1", session.NodeID)
	assert.True(t, session.Connected)
	assert.False(t, session.CleanSession)
}

func TestApplyRouteReplicationLog(t *testing.T) {
	manager := NewReplicationManager("replicant-1", ReplicantNode, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := manager.Start(ctx)
	require.NoError(t, err)
	defer manager.Stop()

	// Create a route replication log
	routeData := &RouteData{
		TopicFilter: "test/topic",
		NodeIDs:     []string{"node-1", "node-2"},
	}

	logEntry := ReplicationLog{
		ID:        "test-route-log-1",
		Timestamp: time.Now().Unix(),
		NodeID:    "core-1",
		Operation: "UPDATE",
		Table:     "routes",
		Data:      routeData,
		Version:   1,
	}

	logBytes, err := json.Marshal(logEntry)
	require.NoError(t, err)

	// Apply the log
	err = manager.ApplyReplicationLog(logBytes)
	assert.NoError(t, err)

	// Verify the route was applied
	route, exists := manager.GetRoute("test/topic")
	assert.True(t, exists)
	assert.Equal(t, "test/topic", route.TopicFilter)
	assert.Equal(t, []string{"node-1", "node-2"}, route.NodeIDs)
}

func TestDeleteReplicationLog(t *testing.T) {
	manager := NewReplicationManager("replicant-1", ReplicantNode, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := manager.Start(ctx)
	require.NoError(t, err)
	defer manager.Stop()

	// First add a subscription
	subscription := &SubscriptionData{
		ClientID:    "client-1",
		TopicFilter: "test/topic",
		QoS:         1,
		NodeID:      "core-1",
	}

	insertLog := ReplicationLog{
		ID:        "insert-log-1",
		Timestamp: time.Now().Unix(),
		NodeID:    "core-1",
		Operation: "INSERT",
		Table:     "subscriptions",
		Data:      subscription,
		Version:   1,
	}

	logBytes, err := json.Marshal(insertLog)
	require.NoError(t, err)

	err = manager.ApplyReplicationLog(logBytes)
	require.NoError(t, err)

	// Verify subscription exists
	subs := manager.GetSubscriptions("client-1")
	assert.Len(t, subs, 1)

	// Now delete it
	deleteLog := ReplicationLog{
		ID:        "delete-log-1",
		Timestamp: time.Now().Unix(),
		NodeID:    "core-1",
		Operation: "DELETE",
		Table:     "subscriptions",
		Data:      subscription,
		Version:   2,
	}

	deleteLogBytes, err := json.Marshal(deleteLog)
	require.NoError(t, err)

	err = manager.ApplyReplicationLog(deleteLogBytes)
	assert.NoError(t, err)

	// Verify subscription was deleted
	subs = manager.GetSubscriptions("client-1")
	assert.Len(t, subs, 0)
}

func TestInvalidReplicationTable(t *testing.T) {
	manager := NewReplicationManager("replicant-1", ReplicantNode, nil)

	logEntry := ReplicationLog{
		ID:        "invalid-log-1",
		Timestamp: time.Now().Unix(),
		NodeID:    "core-1",
		Operation: "INSERT",
		Table:     "invalid_table",
		Data:      "some data",
		Version:   1,
	}

	logBytes, err := json.Marshal(logEntry)
	require.NoError(t, err)

	err = manager.ApplyReplicationLog(logBytes)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown table")
}

func TestReplicationManagerConcurrency(t *testing.T) {
	manager := NewReplicationManager("core-1", CoreNode, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	err := manager.Start(ctx)
	require.NoError(t, err)
	defer manager.Stop()

	// Test concurrent subscription operations
	done := make(chan bool, 2)

	go func() {
		for i := 0; i < 50; i++ {
			clientID := fmt.Sprintf("client-%d", i)
			err := manager.AddSubscription(clientID, "test/topic", 1)
			assert.NoError(t, err)
			time.Sleep(time.Millisecond)
		}
		done <- true
	}()

	go func() {
		for i := 0; i < 50; i++ {
			clientID := fmt.Sprintf("client-session-%d", i)
			expiryTime := time.Now().Add(1 * time.Hour)
			err := manager.AddSession(clientID, true, false, expiryTime)
			assert.NoError(t, err)
			time.Sleep(time.Millisecond)
		}
		done <- true
	}()

	// Wait for both goroutines to complete
	<-done
	<-done

	// Verify final state
	allSubs := manager.GetAllSubscriptions()
	allSessions := manager.GetAllSessions()
	assert.Equal(t, 50, len(allSubs))
	assert.Equal(t, 50, len(allSessions))
}