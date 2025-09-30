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
	"log"
	"sync"
	"time"

	clusterpb "github.com/turtacn/emqx-go/pkg/proto/cluster"
)

// ReplicationLog represents a single replication entry
type ReplicationLog struct {
	ID        string      `json:"id"`
	Timestamp int64       `json:"timestamp"`
	NodeID    string      `json:"node_id"`
	Operation string      `json:"operation"` // INSERT, UPDATE, DELETE
	Table     string      `json:"table"`     // subscriptions, sessions, routes, etc.
	Data      interface{} `json:"data"`
	Version   int64       `json:"version"`
}

// SubscriptionData represents subscription information for replication
type SubscriptionData struct {
	ClientID    string `json:"client_id"`
	TopicFilter string `json:"topic_filter"`
	QoS         int    `json:"qos"`
	NodeID      string `json:"node_id"`
}

// SessionData represents session information for replication
type SessionData struct {
	ClientID     string    `json:"client_id"`
	NodeID       string    `json:"node_id"`
	Connected    bool      `json:"connected"`
	CleanSession bool      `json:"clean_session"`
	ExpiryTime   time.Time `json:"expiry_time"`
}

// RouteData represents routing information for replication
type RouteData struct {
	TopicFilter string   `json:"topic_filter"`
	NodeIDs     []string `json:"node_ids"`
}

// ReplicationState manages the state of data replication
type ReplicationState struct {
	mu              sync.RWMutex
	subscriptions   map[string]*SubscriptionData // key: clientID+topicFilter
	sessions        map[string]*SessionData      // key: clientID
	routes          map[string]*RouteData        // key: topicFilter
	replicationLogs []ReplicationLog
	lastLogID       int64
	version         int64
}

// NewReplicationState creates a new replication state manager
func NewReplicationState() *ReplicationState {
	return &ReplicationState{
		subscriptions:   make(map[string]*SubscriptionData),
		sessions:        make(map[string]*SessionData),
		routes:          make(map[string]*RouteData),
		replicationLogs: make([]ReplicationLog, 0),
		lastLogID:       0,
		version:         0,
	}
}

// ReplicationManager handles data replication between core and replicant nodes
type ReplicationManager struct {
	nodeID   string
	nodeRole NodeRole
	state    *ReplicationState
	mria     *MriaManager

	// Replication channels
	replicationCh chan ReplicationLog
	stopCh        chan struct{}

	// Callbacks for data changes
	onSubscriptionChange func(*SubscriptionData, string)
	onSessionChange      func(*SessionData, string)
	onRouteChange        func(*RouteData, string)
}

// NewReplicationManager creates a new replication manager
func NewReplicationManager(nodeID string, nodeRole NodeRole, mria *MriaManager) *ReplicationManager {
	return &ReplicationManager{
		nodeID:        nodeID,
		nodeRole:      nodeRole,
		state:         NewReplicationState(),
		mria:          mria,
		replicationCh: make(chan ReplicationLog, 1000),
		stopCh:        make(chan struct{}),
	}
}

// SetCallbacks sets callback functions for data changes
func (rm *ReplicationManager) SetCallbacks(
	onSubscriptionChange func(*SubscriptionData, string),
	onSessionChange func(*SessionData, string),
	onRouteChange func(*RouteData, string),
) {
	rm.onSubscriptionChange = onSubscriptionChange
	rm.onSessionChange = onSessionChange
	rm.onRouteChange = onRouteChange
}

// Start begins the replication manager
func (rm *ReplicationManager) Start(ctx context.Context) error {
	log.Printf("[Mria] Starting replication manager for node %s (role: %s)",
		rm.nodeID, rm.nodeRole)

	go rm.replicationLoop(ctx)

	// Start periodic sync for replicant nodes
	if rm.nodeRole == ReplicantNode {
		go rm.syncLoop(ctx)
	}

	return nil
}

// Stop shuts down the replication manager
func (rm *ReplicationManager) Stop() error {
	close(rm.stopCh)
	return nil
}

// AddSubscription adds a new subscription and replicates it if this is a core node
func (rm *ReplicationManager) AddSubscription(clientID, topicFilter string, qos int) error {
	key := fmt.Sprintf("%s:%s", clientID, topicFilter)
	subscription := &SubscriptionData{
		ClientID:    clientID,
		TopicFilter: topicFilter,
		QoS:         qos,
		NodeID:      rm.nodeID,
	}

	rm.state.mu.Lock()
	rm.state.subscriptions[key] = subscription
	rm.state.version++
	rm.state.mu.Unlock()

	// Create replication log entry
	if rm.nodeRole == CoreNode {
		logEntry := ReplicationLog{
			ID:        rm.generateLogID(),
			Timestamp: time.Now().Unix(),
			NodeID:    rm.nodeID,
			Operation: "INSERT",
			Table:     "subscriptions",
			Data:      subscription,
			Version:   rm.state.version,
		}

		rm.replicationCh <- logEntry
	}

	// Trigger callback
	if rm.onSubscriptionChange != nil {
		rm.onSubscriptionChange(subscription, "INSERT")
	}

	log.Printf("[Mria] Added subscription: client=%s, topic=%s, qos=%d",
		clientID, topicFilter, qos)
	return nil
}

// RemoveSubscription removes a subscription and replicates the change
func (rm *ReplicationManager) RemoveSubscription(clientID, topicFilter string) error {
	subscriptionKey := fmt.Sprintf("%s:%s", clientID, topicFilter)

	rm.state.mu.Lock()
	subscription, exists := rm.state.subscriptions[subscriptionKey]
	if exists {
		delete(rm.state.subscriptions, subscriptionKey)
		rm.state.version++
	}
	rm.state.mu.Unlock()

	if !exists {
		return fmt.Errorf("subscription not found")
	}

	// Create replication log entry
	if rm.nodeRole == CoreNode {
		logEntry := ReplicationLog{
			ID:        rm.generateLogID(),
			Timestamp: time.Now().Unix(),
			NodeID:    rm.nodeID,
			Operation: "DELETE",
			Table:     "subscriptions",
			Data:      subscription,
			Version:   rm.state.version,
		}

		rm.replicationCh <- logEntry
	}

	// Trigger callback
	if rm.onSubscriptionChange != nil {
		rm.onSubscriptionChange(subscription, "DELETE")
	}

	log.Printf("[Mria] Removed subscription: client=%s, topic=%s",
		clientID, topicFilter)
	return nil
}

// AddSession adds or updates session information
func (rm *ReplicationManager) AddSession(clientID string, connected bool, cleanSession bool, expiryTime time.Time) error {
	session := &SessionData{
		ClientID:     clientID,
		NodeID:       rm.nodeID,
		Connected:    connected,
		CleanSession: cleanSession,
		ExpiryTime:   expiryTime,
	}

	rm.state.mu.Lock()
	rm.state.sessions[clientID] = session
	rm.state.version++
	rm.state.mu.Unlock()

	// Create replication log entry
	if rm.nodeRole == CoreNode {
		operation := "INSERT"
		if _, exists := rm.state.sessions[clientID]; exists {
			operation = "UPDATE"
		}

		logEntry := ReplicationLog{
			ID:        rm.generateLogID(),
			Timestamp: time.Now().Unix(),
			NodeID:    rm.nodeID,
			Operation: operation,
			Table:     "sessions",
			Data:      session,
			Version:   rm.state.version,
		}

		rm.replicationCh <- logEntry
	}

	// Trigger callback
	if rm.onSessionChange != nil {
		rm.onSessionChange(session, "UPSERT")
	}

	log.Printf("[Mria] Added/updated session: client=%s, connected=%t",
		clientID, connected)
	return nil
}

// RemoveSession removes session information
func (rm *ReplicationManager) RemoveSession(clientID string) error {
	rm.state.mu.Lock()
	session, exists := rm.state.sessions[clientID]
	if exists {
		delete(rm.state.sessions, clientID)
		rm.state.version++
	}
	rm.state.mu.Unlock()

	if !exists {
		return fmt.Errorf("session not found")
	}

	// Create replication log entry
	if rm.nodeRole == CoreNode {
		logEntry := ReplicationLog{
			ID:        rm.generateLogID(),
			Timestamp: time.Now().Unix(),
			NodeID:    rm.nodeID,
			Operation: "DELETE",
			Table:     "sessions",
			Data:      session,
			Version:   rm.state.version,
		}

		rm.replicationCh <- logEntry
	}

	// Trigger callback
	if rm.onSessionChange != nil {
		rm.onSessionChange(session, "DELETE")
	}

	log.Printf("[Mria] Removed session: client=%s", clientID)
	return nil
}

// UpdateRoute updates routing information
func (rm *ReplicationManager) UpdateRoute(topicFilter string, nodeIDs []string) error {
	route := &RouteData{
		TopicFilter: topicFilter,
		NodeIDs:     nodeIDs,
	}

	rm.state.mu.Lock()
	rm.state.routes[topicFilter] = route
	rm.state.version++
	rm.state.mu.Unlock()

	// Create replication log entry
	if rm.nodeRole == CoreNode {
		logEntry := ReplicationLog{
			ID:        rm.generateLogID(),
			Timestamp: time.Now().Unix(),
			NodeID:    rm.nodeID,
			Operation: "UPDATE",
			Table:     "routes",
			Data:      route,
			Version:   rm.state.version,
		}

		rm.replicationCh <- logEntry
	}

	// Trigger callback
	if rm.onRouteChange != nil {
		rm.onRouteChange(route, "UPDATE")
	}

	log.Printf("[Mria] Updated route: topic=%s, nodes=%v", topicFilter, nodeIDs)
	return nil
}

// GetSubscriptions returns all subscriptions for a client
func (rm *ReplicationManager) GetSubscriptions(clientID string) []*SubscriptionData {
	rm.state.mu.RLock()
	defer rm.state.mu.RUnlock()

	var subscriptions []*SubscriptionData
	for _, sub := range rm.state.subscriptions {
		if sub.ClientID == clientID {
			subscriptions = append(subscriptions, sub)
		}
	}
	return subscriptions
}

// GetSession returns session information for a client
func (rm *ReplicationManager) GetSession(clientID string) (*SessionData, bool) {
	rm.state.mu.RLock()
	defer rm.state.mu.RUnlock()

	session, exists := rm.state.sessions[clientID]
	return session, exists
}

// GetRoute returns routing information for a topic
func (rm *ReplicationManager) GetRoute(topicFilter string) (*RouteData, bool) {
	rm.state.mu.RLock()
	defer rm.state.mu.RUnlock()

	route, exists := rm.state.routes[topicFilter]
	return route, exists
}

// GetAllSubscriptions returns all subscriptions
func (rm *ReplicationManager) GetAllSubscriptions() map[string]*SubscriptionData {
	rm.state.mu.RLock()
	defer rm.state.mu.RUnlock()

	result := make(map[string]*SubscriptionData)
	for k, v := range rm.state.subscriptions {
		result[k] = v
	}
	return result
}

// GetAllSessions returns all sessions
func (rm *ReplicationManager) GetAllSessions() map[string]*SessionData {
	rm.state.mu.RLock()
	defer rm.state.mu.RUnlock()

	result := make(map[string]*SessionData)
	for k, v := range rm.state.sessions {
		result[k] = v
	}
	return result
}

// GetAllRoutes returns all routes
func (rm *ReplicationManager) GetAllRoutes() map[string]*RouteData {
	rm.state.mu.RLock()
	defer rm.state.mu.RUnlock()

	result := make(map[string]*RouteData)
	for k, v := range rm.state.routes {
		result[k] = v
	}
	return result
}

// replicationLoop handles replication log processing
func (rm *ReplicationManager) replicationLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-rm.stopCh:
			return
		case logEntry := <-rm.replicationCh:
			rm.processReplicationLog(logEntry)
		}
	}
}

// syncLoop periodically syncs data from core nodes (for replicant nodes)
func (rm *ReplicationManager) syncLoop(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second) // Sync every 10 seconds
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-rm.stopCh:
			return
		case <-ticker.C:
			rm.syncFromCore()
		}
	}
}

// processReplicationLog processes a single replication log entry
func (rm *ReplicationManager) processReplicationLog(logEntry ReplicationLog) {
	rm.state.mu.Lock()
	rm.state.replicationLogs = append(rm.state.replicationLogs, logEntry)

	// Keep only the last 1000 log entries
	if len(rm.state.replicationLogs) > 1000 {
		rm.state.replicationLogs = rm.state.replicationLogs[len(rm.state.replicationLogs)-1000:]
	}
	rm.state.mu.Unlock()

	// Replicate to replicant nodes if this is a core node
	if rm.nodeRole == CoreNode {
		rm.replicateToReplicants(logEntry)
	}

	log.Printf("[Mria] Processed replication log: id=%s, operation=%s, table=%s",
		logEntry.ID, logEntry.Operation, logEntry.Table)
}

// replicateToReplicants sends replication data to all replicant nodes
func (rm *ReplicationManager) replicateToReplicants(logEntry ReplicationLog) {
	if rm.mria == nil {
		return
	}

	replicantNodes := rm.mria.GetReplicantNodes()
	if len(replicantNodes) == 0 {
		return
	}

	// Convert log entry to JSON for transmission
	data, err := json.Marshal(logEntry)
	if err != nil {
		log.Printf("[Mria] Failed to marshal replication log: %v", err)
		return
	}

	// Send to all replicant nodes
	for _, node := range replicantNodes {
		// This would use the cluster manager to send the replication data
		rm.mria.clusterManager.ForwardPublish(
			fmt.Sprintf("$mria/replication/%s", node.NodeId),
			data,
			node.NodeId,
		)
	}
}

// syncFromCore syncs data from core nodes (for replicant nodes)
func (rm *ReplicationManager) syncFromCore() {
	if rm.nodeRole != ReplicantNode || rm.mria == nil {
		return
	}

	coreNodes := rm.mria.GetCoreNodes()
	if len(coreNodes) == 0 {
		log.Printf("[Mria] No core nodes available for sync")
		return
	}

	// Choose the leader or first available core node
	var targetCore *clusterpb.NodeInfo
	state := rm.mria.GetClusterState()
	if state.Leader != "" {
		for _, node := range coreNodes {
			if node.NodeId == state.Leader {
				targetCore = node
				break
			}
		}
	}
	if targetCore == nil {
		targetCore = coreNodes[0]
	}

	// Request sync from the target core node
	log.Printf("[Mria] Syncing data from core node %s", targetCore.NodeId)

	// This would use the cluster manager to request sync data
	// Implementation would depend on the specific protocol
}

// generateLogID generates a unique log ID
func (rm *ReplicationManager) generateLogID() string {
	rm.state.mu.Lock()
	rm.state.lastLogID++
	id := rm.state.lastLogID
	rm.state.mu.Unlock()

	return fmt.Sprintf("%s-%d-%d", rm.nodeID, time.Now().UnixNano(), id)
}

// ApplyReplicationLog applies a replication log entry (for replicant nodes)
func (rm *ReplicationManager) ApplyReplicationLog(logData []byte) error {
	var logEntry ReplicationLog
	if err := json.Unmarshal(logData, &logEntry); err != nil {
		return fmt.Errorf("failed to unmarshal replication log: %w", err)
	}

	// Apply the change based on the table and operation
	switch logEntry.Table {
	case "subscriptions":
		return rm.applySubscriptionChange(logEntry)
	case "sessions":
		return rm.applySessionChange(logEntry)
	case "routes":
		return rm.applyRouteChange(logEntry)
	default:
		return fmt.Errorf("unknown table: %s", logEntry.Table)
	}
}

// applySubscriptionChange applies a subscription change
func (rm *ReplicationManager) applySubscriptionChange(logEntry ReplicationLog) error {
	data, err := json.Marshal(logEntry.Data)
	if err != nil {
		return err
	}

	var subscription SubscriptionData
	if err := json.Unmarshal(data, &subscription); err != nil {
		return err
	}

	key := fmt.Sprintf("%s:%s", subscription.ClientID, subscription.TopicFilter)

	rm.state.mu.Lock()
	switch logEntry.Operation {
	case "INSERT", "UPDATE":
		rm.state.subscriptions[key] = &subscription
	case "DELETE":
		delete(rm.state.subscriptions, key)
	}
	rm.state.version = logEntry.Version
	rm.state.mu.Unlock()

	// Trigger callback
	if rm.onSubscriptionChange != nil {
		rm.onSubscriptionChange(&subscription, logEntry.Operation)
	}

	return nil
}

// applySessionChange applies a session change
func (rm *ReplicationManager) applySessionChange(logEntry ReplicationLog) error {
	data, err := json.Marshal(logEntry.Data)
	if err != nil {
		return err
	}

	var session SessionData
	if err := json.Unmarshal(data, &session); err != nil {
		return err
	}

	rm.state.mu.Lock()
	switch logEntry.Operation {
	case "INSERT", "UPDATE":
		rm.state.sessions[session.ClientID] = &session
	case "DELETE":
		delete(rm.state.sessions, session.ClientID)
	}
	rm.state.version = logEntry.Version
	rm.state.mu.Unlock()

	// Trigger callback
	if rm.onSessionChange != nil {
		rm.onSessionChange(&session, logEntry.Operation)
	}

	return nil
}

// applyRouteChange applies a route change
func (rm *ReplicationManager) applyRouteChange(logEntry ReplicationLog) error {
	data, err := json.Marshal(logEntry.Data)
	if err != nil {
		return err
	}

	var route RouteData
	if err := json.Unmarshal(data, &route); err != nil {
		return err
	}

	rm.state.mu.Lock()
	switch logEntry.Operation {
	case "INSERT", "UPDATE":
		rm.state.routes[route.TopicFilter] = &route
	case "DELETE":
		delete(rm.state.routes, route.TopicFilter)
	}
	rm.state.version = logEntry.Version
	rm.state.mu.Unlock()

	// Trigger callback
	if rm.onRouteChange != nil {
		rm.onRouteChange(&route, logEntry.Operation)
	}

	return nil
}