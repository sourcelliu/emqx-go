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
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/turtacn/emqx-go/pkg/discovery"
	clusterpb "github.com/turtacn/emqx-go/pkg/proto/cluster"
)

// BrokerIntegration provides the integration layer between Mria cluster and the MQTT broker
type BrokerIntegration struct {
	nodeID   string
	nodeRole NodeRole

	// Mria components
	mriaManager        *MriaManager
	replicationManager *ReplicationManager
	clusterServer      *MriaClusterServer

	// Broker callbacks
	onMessagePublish  func(topic string, payload []byte, qos byte, retain bool, userProperties map[string][]byte, responseTopic string, correlationData []byte)
	onClientConnect   func(clientID string, cleanSession bool)
	onClientDisconnect func(clientID string)
	onSubscribe       func(clientID, topicFilter string, qos int)
	onUnsubscribe     func(clientID, topicFilter string)

	// Internal state
	mu             sync.RWMutex
	localSessions  map[string]*SessionInfo
	localRoutes    map[string][]string // topic -> client IDs
	isInitialized  bool
}

// SessionInfo represents local session information
type SessionInfo struct {
	ClientID     string
	Connected    bool
	CleanSession bool
	ConnectedAt  time.Time
	LastSeen     time.Time
}

// NewBrokerIntegration creates a new broker integration
func NewBrokerIntegration(nodeID string, nodeRole NodeRole, discovery discovery.Discovery) *BrokerIntegration {
	// Create Mria configuration
	config := DefaultMriaConfig(nodeID, ":0") // Will be set properly later
	config.NodeRole = nodeRole

	// Create Mria manager
	mriaManager := NewMriaManager(config, discovery)

	// Create replication manager
	replicationManager := NewReplicationManager(nodeID, nodeRole, mriaManager)

	// Create cluster server
	clusterServer := NewMriaClusterServer(nodeID, nodeRole, "", mriaManager, replicationManager)

	bi := &BrokerIntegration{
		nodeID:             nodeID,
		nodeRole:           nodeRole,
		mriaManager:        mriaManager,
		replicationManager: replicationManager,
		clusterServer:      clusterServer,
		localSessions:      make(map[string]*SessionInfo),
		localRoutes:        make(map[string][]string),
	}

	// Set up callbacks
	bi.setupCallbacks()

	return bi
}

// Initialize starts the Mria cluster integration
func (bi *BrokerIntegration) Initialize(ctx context.Context, listenAddr string) error {
	bi.mu.Lock()
	if bi.isInitialized {
		bi.mu.Unlock()
		return fmt.Errorf("broker integration already initialized")
	}
	bi.mu.Unlock()

	// Update listen address
	bi.mriaManager.config.ListenAddress = listenAddr
	bi.clusterServer.listenAddr = listenAddr

	log.Printf("[Mria] Initializing broker integration for node %s (role: %s) on %s",
		bi.nodeID, bi.nodeRole, listenAddr)

	// Start cluster server
	if err := bi.clusterServer.Start(ctx); err != nil {
		return fmt.Errorf("failed to start cluster server: %w", err)
	}

	// Start replication manager
	if err := bi.replicationManager.Start(ctx); err != nil {
		return fmt.Errorf("failed to start replication manager: %w", err)
	}

	// Start Mria manager
	if err := bi.mriaManager.Start(ctx); err != nil {
		return fmt.Errorf("failed to start Mria manager: %w", err)
	}

	bi.mu.Lock()
	bi.isInitialized = true
	bi.mu.Unlock()

	log.Printf("[Mria] Broker integration initialized successfully")
	return nil
}

// Shutdown stops the Mria cluster integration
func (bi *BrokerIntegration) Shutdown() error {
	bi.mu.Lock()
	if !bi.isInitialized {
		bi.mu.Unlock()
		return nil
	}
	bi.isInitialized = false
	bi.mu.Unlock()

	log.Printf("[Mria] Shutting down broker integration")

	// Stop components in reverse order
	if err := bi.mriaManager.Stop(); err != nil {
		log.Printf("[Mria] Error stopping Mria manager: %v", err)
	}

	if err := bi.replicationManager.Stop(); err != nil {
		log.Printf("[Mria] Error stopping replication manager: %v", err)
	}

	if err := bi.clusterServer.Stop(); err != nil {
		log.Printf("[Mria] Error stopping cluster server: %v", err)
	}

	return nil
}

// SetBrokerCallbacks sets the broker callback functions
func (bi *BrokerIntegration) SetBrokerCallbacks(
	onMessagePublish func(topic string, payload []byte, qos byte, retain bool, userProperties map[string][]byte, responseTopic string, correlationData []byte),
	onClientConnect func(clientID string, cleanSession bool),
	onClientDisconnect func(clientID string),
	onSubscribe func(clientID, topicFilter string, qos int),
	onUnsubscribe func(clientID, topicFilter string),
) {
	bi.onMessagePublish = onMessagePublish
	bi.onClientConnect = onClientConnect
	bi.onClientDisconnect = onClientDisconnect
	bi.onSubscribe = onSubscribe
	bi.onUnsubscribe = onUnsubscribe
}

// HandleClientConnect handles client connection events
func (bi *BrokerIntegration) HandleClientConnect(clientID string, cleanSession bool) {
	log.Printf("[Mria] Client connected: %s (clean_session: %t)", clientID, cleanSession)

	// Update local session info
	bi.mu.Lock()
	bi.localSessions[clientID] = &SessionInfo{
		ClientID:     clientID,
		Connected:    true,
		CleanSession: cleanSession,
		ConnectedAt:  time.Now(),
		LastSeen:     time.Now(),
	}
	bi.mu.Unlock()

	// Add to replication if this is a core node
	expiryTime := time.Now().Add(24 * time.Hour) // Default 24 hour expiry
	if cleanSession {
		expiryTime = time.Now() // Immediate expiry for clean sessions
	}

	err := bi.replicationManager.AddSession(clientID, true, cleanSession, expiryTime)
	if err != nil {
		log.Printf("[Mria] Failed to replicate session: %v", err)
	}

	// Trigger callback
	if bi.onClientConnect != nil {
		bi.onClientConnect(clientID, cleanSession)
	}
}

// HandleClientDisconnect handles client disconnection events
func (bi *BrokerIntegration) HandleClientDisconnect(clientID string) {
	log.Printf("[Mria] Client disconnected: %s", clientID)

	// Update local session info
	bi.mu.Lock()
	if session, exists := bi.localSessions[clientID]; exists {
		session.Connected = false
		session.LastSeen = time.Now()

		// Remove from local sessions if clean session
		if session.CleanSession {
			delete(bi.localSessions, clientID)
		}
	}
	bi.mu.Unlock()

	// Update replication
	sessionInfo, exists := bi.localSessions[clientID]
	if exists && sessionInfo.CleanSession {
		// Remove from replication for clean sessions
		err := bi.replicationManager.RemoveSession(clientID)
		if err != nil {
			log.Printf("[Mria] Failed to remove session from replication: %v", err)
		}
	} else {
		// Update session status for persistent sessions
		expiryTime := time.Now().Add(24 * time.Hour)
		err := bi.replicationManager.AddSession(clientID, false, false, expiryTime)
		if err != nil {
			log.Printf("[Mria] Failed to update session in replication: %v", err)
		}
	}

	// Trigger callback
	if bi.onClientDisconnect != nil {
		bi.onClientDisconnect(clientID)
	}
}

// HandleSubscribe handles subscription events
func (bi *BrokerIntegration) HandleSubscribe(clientID, topicFilter string, qos int) {
	log.Printf("[Mria] Client subscribed: %s to %s (QoS %d)", clientID, topicFilter, qos)

	// Update local routes
	bi.mu.Lock()
	if clients, exists := bi.localRoutes[topicFilter]; exists {
		// Check if client already exists
		found := false
		for _, existingClient := range clients {
			if existingClient == clientID {
				found = true
				break
			}
		}
		if !found {
			bi.localRoutes[topicFilter] = append(clients, clientID)
		}
	} else {
		bi.localRoutes[topicFilter] = []string{clientID}
	}
	bi.mu.Unlock()

	// Add to replication
	err := bi.replicationManager.AddSubscription(clientID, topicFilter, qos)
	if err != nil {
		log.Printf("[Mria] Failed to replicate subscription: %v", err)
	}

	// Update cluster routes
	bi.updateClusterRoute(topicFilter)

	// Trigger callback
	if bi.onSubscribe != nil {
		bi.onSubscribe(clientID, topicFilter, qos)
	}
}

// HandleUnsubscribe handles unsubscription events
func (bi *BrokerIntegration) HandleUnsubscribe(clientID, topicFilter string) {
	log.Printf("[Mria] Client unsubscribed: %s from %s", clientID, topicFilter)

	// Update local routes
	bi.mu.Lock()
	if clients, exists := bi.localRoutes[topicFilter]; exists {
		newClients := make([]string, 0, len(clients))
		for _, existingClient := range clients {
			if existingClient != clientID {
				newClients = append(newClients, existingClient)
			}
		}
		if len(newClients) == 0 {
			delete(bi.localRoutes, topicFilter)
		} else {
			bi.localRoutes[topicFilter] = newClients
		}
	}
	bi.mu.Unlock()

	// Remove from replication
	err := bi.replicationManager.RemoveSubscription(clientID, topicFilter)
	if err != nil {
		log.Printf("[Mria] Failed to remove subscription from replication: %v", err)
	}

	// Update cluster routes
	bi.updateClusterRoute(topicFilter)

	// Trigger callback
	if bi.onUnsubscribe != nil {
		bi.onUnsubscribe(clientID, topicFilter)
	}
}

// HandleMessagePublish handles message publication events
func (bi *BrokerIntegration) HandleMessagePublish(topic string, payload []byte, qos byte, retain bool, userProperties map[string][]byte, responseTopic string, correlationData []byte) {
	log.Printf("[Mria] Message published: topic=%s, qos=%d, retain=%t, payload_size=%d",
		topic, qos, retain, len(payload))

	// For replicant nodes, forward to core nodes if needed
	if bi.nodeRole == ReplicantNode {
		// Check if we have local subscribers
		hasLocalSubscribers := bi.hasLocalSubscribers(topic)

		if !hasLocalSubscribers {
			// Forward to core nodes for routing
			err := bi.mriaManager.ForwardMessageToCore(topic, payload)
			if err != nil {
				log.Printf("[Mria] Failed to forward message to core: %v", err)
			}
		}
	}

	// Handle local delivery
	if bi.onMessagePublish != nil {
		bi.onMessagePublish(topic, payload, qos, retain, userProperties, responseTopic, correlationData)
	}

	// For core nodes, handle cluster-wide routing
	if bi.nodeRole == CoreNode {
		bi.handleClusterRouting(topic, payload, qos, retain, userProperties, responseTopic, correlationData)
	}
}

// GetNodeRole returns the current node role
func (bi *BrokerIntegration) GetNodeRole() NodeRole {
	return bi.nodeRole
}

// IsLeader returns true if this node is the cluster leader
func (bi *BrokerIntegration) IsLeader() bool {
	return bi.mriaManager.IsLeader()
}

// GetClusterState returns the current cluster state
func (bi *BrokerIntegration) GetClusterState() *ClusterState {
	return bi.mriaManager.GetClusterState()
}

// GetClusterStats returns cluster statistics
func (bi *BrokerIntegration) GetClusterStats() map[string]interface{} {
	stats := make(map[string]interface{})

	clusterState := bi.GetClusterState()
	stats["cluster_id"] = clusterState.ClusterID
	stats["leader"] = clusterState.Leader
	stats["core_nodes"] = len(clusterState.CoreNodes)
	stats["replicant_nodes"] = len(clusterState.ReplicantNodes)
	stats["is_leader"] = bi.IsLeader()
	stats["node_role"] = string(bi.nodeRole)

	bi.mu.RLock()
	stats["local_sessions"] = len(bi.localSessions)
	stats["local_routes"] = len(bi.localRoutes)
	bi.mu.RUnlock()

	stats["all_subscriptions"] = len(bi.replicationManager.GetAllSubscriptions())
	stats["all_sessions"] = len(bi.replicationManager.GetAllSessions())
	stats["all_routes"] = len(bi.replicationManager.GetAllRoutes())

	return stats
}

// setupCallbacks sets up internal callbacks for Mria components
func (bi *BrokerIntegration) setupCallbacks() {
	// Mria manager callbacks
	bi.mriaManager.SetCallbacks(
		bi.handleNodeJoin,
		bi.handleNodeLeave,
		bi.handleBecomeLeader,
		bi.handleLoseLeadership,
	)

	// Replication manager callbacks
	bi.replicationManager.SetCallbacks(
		bi.handleSubscriptionChange,
		bi.handleSessionChange,
		bi.handleRouteChange,
	)
}

// handleNodeJoin handles when a new node joins the cluster
func (bi *BrokerIntegration) handleNodeJoin(nodeInfo *clusterpb.NodeInfo) {
	log.Printf("[Mria] Node joined cluster: %s at %s", nodeInfo.NodeId, nodeInfo.Address)

	// If this is a core node and the joining node is a replicant,
	// send current state to the new replicant
	if bi.nodeRole == CoreNode && nodeInfo.NodeId != bi.nodeID {
		go bi.syncStateToNode(nodeInfo.NodeId)
	}
}

// handleNodeLeave handles when a node leaves the cluster
func (bi *BrokerIntegration) handleNodeLeave(nodeID string) {
	log.Printf("[Mria] Node left cluster: %s", nodeID)

	// Clean up any sessions or subscriptions from the departed node
	bi.cleanupNodeData(nodeID)
}

// handleBecomeLeader handles becoming the cluster leader
func (bi *BrokerIntegration) handleBecomeLeader() {
	log.Printf("[Mria] Node %s became cluster leader", bi.nodeID)

	// Perform leader-specific initialization
	bi.performLeaderInitialization()
}

// handleLoseLeadership handles losing cluster leadership
func (bi *BrokerIntegration) handleLoseLeadership() {
	log.Printf("[Mria] Node %s lost cluster leadership", bi.nodeID)

	// Stop leader-specific operations
	bi.stopLeaderOperations()
}

// handleSubscriptionChange handles subscription changes from replication
func (bi *BrokerIntegration) handleSubscriptionChange(sub *SubscriptionData, operation string) {
	log.Printf("[Mria] Subscription change: %s %s (op: %s)",
		sub.ClientID, sub.TopicFilter, operation)

	// Update local routing if needed
	if sub.NodeID != bi.nodeID {
		bi.updateRemoteRoute(sub.TopicFilter, sub.NodeID, operation != "DELETE")
	}
}

// handleSessionChange handles session changes from replication
func (bi *BrokerIntegration) handleSessionChange(session *SessionData, operation string) {
	log.Printf("[Mria] Session change: %s (op: %s)", session.ClientID, operation)

	// Handle session state changes as needed
}

// handleRouteChange handles route changes from replication
func (bi *BrokerIntegration) handleRouteChange(route *RouteData, operation string) {
	log.Printf("[Mria] Route change: %s -> %v (op: %s)",
		route.TopicFilter, route.NodeIDs, operation)

	// Update cluster routing information
}

// updateClusterRoute updates the cluster-wide routing information for a topic
func (bi *BrokerIntegration) updateClusterRoute(topicFilter string) {
	bi.mu.RLock()
	hasLocalSubscribers := len(bi.localRoutes[topicFilter]) > 0
	bi.mu.RUnlock()

	// Get all nodes that have subscribers for this topic
	nodeIDs := []string{}
	if hasLocalSubscribers {
		nodeIDs = append(nodeIDs, bi.nodeID)
	}

	// Add other nodes from replication state
	if route, exists := bi.replicationManager.GetRoute(topicFilter); exists {
		for _, nodeID := range route.NodeIDs {
			if nodeID != bi.nodeID {
				nodeIDs = append(nodeIDs, nodeID)
			}
		}
	}

	// Update the route
	err := bi.replicationManager.UpdateRoute(topicFilter, nodeIDs)
	if err != nil {
		log.Printf("[Mria] Failed to update cluster route: %v", err)
	}
}

// updateRemoteRoute updates routing information for remote nodes
func (bi *BrokerIntegration) updateRemoteRoute(topicFilter, nodeID string, add bool) {
	route, exists := bi.replicationManager.GetRoute(topicFilter)
	if !exists {
		if add {
			err := bi.replicationManager.UpdateRoute(topicFilter, []string{nodeID})
			if err != nil {
				log.Printf("[Mria] Failed to create remote route: %v", err)
			}
		}
		return
	}

	nodeIDs := make([]string, 0, len(route.NodeIDs))
	found := false

	for _, existingNodeID := range route.NodeIDs {
		if existingNodeID == nodeID {
			found = true
			if add {
				nodeIDs = append(nodeIDs, existingNodeID)
			}
		} else {
			nodeIDs = append(nodeIDs, existingNodeID)
		}
	}

	if add && !found {
		nodeIDs = append(nodeIDs, nodeID)
	}

	err := bi.replicationManager.UpdateRoute(topicFilter, nodeIDs)
	if err != nil {
		log.Printf("[Mria] Failed to update remote route: %v", err)
	}
}

// hasLocalSubscribers checks if there are local subscribers for a topic
func (bi *BrokerIntegration) hasLocalSubscribers(topic string) bool {
	bi.mu.RLock()
	defer bi.mu.RUnlock()

	// Simple topic matching (could be enhanced with wildcard matching)
	_, exists := bi.localRoutes[topic]
	return exists
}

// handleClusterRouting handles cluster-wide message routing for core nodes
func (bi *BrokerIntegration) handleClusterRouting(topic string, payload []byte, qos byte, retain bool, userProperties map[string][]byte, responseTopic string, correlationData []byte) {
	// Get routing information
	route, exists := bi.replicationManager.GetRoute(topic)
	if !exists {
		return // No remote subscribers
	}

	// Forward to nodes that have subscribers
	for _, nodeID := range route.NodeIDs {
		if nodeID != bi.nodeID {
			bi.mriaManager.clusterManager.ForwardPublish(topic, payload, nodeID)
		}
	}
}

// syncStateToNode syncs current state to a new node
func (bi *BrokerIntegration) syncStateToNode(nodeID string) {
	log.Printf("[Mria] Syncing state to new node: %s", nodeID)

	// This would implement full state synchronization
	// Including subscriptions, sessions, and routes
}

// cleanupNodeData cleans up data from a departed node
func (bi *BrokerIntegration) cleanupNodeData(nodeID string) {
	// Remove routes pointing to the departed node
	allRoutes := bi.replicationManager.GetAllRoutes()
	for topicFilter, route := range allRoutes {
		newNodeIDs := make([]string, 0, len(route.NodeIDs))
		for _, existingNodeID := range route.NodeIDs {
			if existingNodeID != nodeID {
				newNodeIDs = append(newNodeIDs, existingNodeID)
			}
		}

		if len(newNodeIDs) != len(route.NodeIDs) {
			err := bi.replicationManager.UpdateRoute(topicFilter, newNodeIDs)
			if err != nil {
				log.Printf("[Mria] Failed to cleanup route for departed node: %v", err)
			}
		}
	}

	// Remove sessions from the departed node
	allSessions := bi.replicationManager.GetAllSessions()
	for clientID, session := range allSessions {
		if session.NodeID == nodeID {
			err := bi.replicationManager.RemoveSession(clientID)
			if err != nil {
				log.Printf("[Mria] Failed to cleanup session for departed node: %v", err)
			}
		}
	}

	// Remove subscriptions from the departed node
	allSubscriptions := bi.replicationManager.GetAllSubscriptions()
	for _, subscription := range allSubscriptions {
		if subscription.NodeID == nodeID {
			err := bi.replicationManager.RemoveSubscription(subscription.ClientID, subscription.TopicFilter)
			if err != nil {
				log.Printf("[Mria] Failed to cleanup subscription for departed node: %v", err)
			}
		}
	}
}

// performLeaderInitialization performs leader-specific initialization
func (bi *BrokerIntegration) performLeaderInitialization() {
	// Start leader-specific operations
	log.Printf("[Mria] Performing leader initialization")

	// This could include:
	// - Starting periodic cluster maintenance
	// - Coordinating cluster-wide operations
	// - Managing cluster configuration
}

// stopLeaderOperations stops leader-specific operations
func (bi *BrokerIntegration) stopLeaderOperations() {
	// Stop leader-specific operations
	log.Printf("[Mria] Stopping leader operations")

	// This could include:
	// - Stopping periodic maintenance
	// - Transferring coordination to new leader
}

// IsInitialized returns true if the integration has been initialized
func (bi *BrokerIntegration) IsInitialized() bool {
	bi.mu.RLock()
	defer bi.mu.RUnlock()
	return bi.isInitialized
}