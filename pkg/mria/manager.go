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

// Package mria provides the EMQX-style Mria cluster architecture implementation
// for emqx-go. It implements the Core+Replicant pattern where Core nodes handle
// data persistence and transactions, while Replicant nodes handle connections
// and message routing.
package mria

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/turtacn/emqx-go/pkg/cluster"
	"github.com/turtacn/emqx-go/pkg/discovery"
	clusterpb "github.com/turtacn/emqx-go/pkg/proto/cluster"
)

// NodeRole defines the role of a node in the Mria cluster
type NodeRole string

const (
	// CoreNode handles data persistence, transactions, and cluster coordination
	CoreNode NodeRole = "core"
	// ReplicantNode handles client connections and message routing
	ReplicantNode NodeRole = "replicant"
)

// NodeStatus represents the current status of a node
type NodeStatus int32

const (
	NodeStatusStarting NodeStatus = 1
	NodeStatusRunning  NodeStatus = 2
	NodeStatusStopping NodeStatus = 3
	NodeStatusOffline  NodeStatus = 4
)

// MriaConfig contains configuration for the Mria cluster
type MriaConfig struct {
	NodeID                string
	NodeRole              NodeRole
	ListenAddress         string
	CoreNodes             []string // List of known core node addresses
	HeartbeatInterval     time.Duration
	ElectionTimeout       time.Duration
	MaxReplicantNodes     int
	AutoDiscoveryEnabled  bool
	ClusterSecret         string
}

// DefaultMriaConfig returns a default configuration for Mria cluster
func DefaultMriaConfig(nodeID, listenAddress string) *MriaConfig {
	return &MriaConfig{
		NodeID:                nodeID,
		NodeRole:              CoreNode, // Default to Core for small clusters
		ListenAddress:         listenAddress,
		HeartbeatInterval:     5 * time.Second,
		ElectionTimeout:       10 * time.Second,
		MaxReplicantNodes:     100,
		AutoDiscoveryEnabled:  true,
		ClusterSecret:         "",
	}
}

// ClusterState represents the overall state of the cluster
type ClusterState struct {
	ClusterID     string
	Leader        string
	CoreNodes     map[string]*clusterpb.NodeInfo
	ReplicantNodes map[string]*clusterpb.NodeInfo
	LastUpdated   time.Time
}

// MriaManager implements the Mria cluster architecture
type MriaManager struct {
	config        *MriaConfig
	nodeInfo      *clusterpb.NodeInfo
	clusterState  *ClusterState

	// Cluster management
	clusterManager *cluster.Manager
	discovery      discovery.Discovery

	// Node connections
	coreConnections      map[string]*cluster.Client
	replicantConnections map[string]*cluster.Client

	// Synchronization
	mu               sync.RWMutex
	stopCh           chan struct{}
	isLeader         bool
	leaderElectionCh chan bool

	// Callback functions
	onNodeJoin  func(nodeInfo *clusterpb.NodeInfo)
	onNodeLeave func(nodeID string)
	onBecomeLeader func()
	onLoseLeadership func()
}

// NewMriaManager creates a new Mria cluster manager
func NewMriaManager(config *MriaConfig, discovery discovery.Discovery) *MriaManager {
	nodeInfo := &clusterpb.NodeInfo{
		NodeId:   config.NodeID,
		Address:  config.ListenAddress,
		Version:  "1.0.0-mria",
		Status:   int32(NodeStatusStarting),
		LastSeen: time.Now().Unix(),
	}

	clusterState := &ClusterState{
		ClusterID:      fmt.Sprintf("emqx-cluster-%d", time.Now().Unix()),
		CoreNodes:      make(map[string]*clusterpb.NodeInfo),
		ReplicantNodes: make(map[string]*clusterpb.NodeInfo),
		LastUpdated:    time.Now(),
	}

	// Create local publish function for cluster manager
	localPublishFunc := func(topic string, payload []byte) {
		log.Printf("[Mria] Local publish: topic=%s, payload_size=%d", topic, len(payload))
		// This will be connected to the broker's routing system
	}

	clusterManager := cluster.NewManager(config.NodeID, config.ListenAddress, localPublishFunc)

	return &MriaManager{
		config:               config,
		nodeInfo:             nodeInfo,
		clusterState:         clusterState,
		clusterManager:       clusterManager,
		discovery:            discovery,
		coreConnections:      make(map[string]*cluster.Client),
		replicantConnections: make(map[string]*cluster.Client),
		stopCh:               make(chan struct{}),
		leaderElectionCh:     make(chan bool, 1),
	}
}

// SetCallbacks sets callback functions for cluster events
func (m *MriaManager) SetCallbacks(
	onNodeJoin func(*clusterpb.NodeInfo),
	onNodeLeave func(string),
	onBecomeLeader func(),
	onLoseLeadership func(),
) {
	m.onNodeJoin = onNodeJoin
	m.onNodeLeave = onNodeLeave
	m.onBecomeLeader = onBecomeLeader
	m.onLoseLeadership = onLoseLeadership
}

// Start initializes and starts the Mria cluster manager
func (m *MriaManager) Start(ctx context.Context) error {
	log.Printf("[Mria] Starting Mria cluster manager for node %s (role: %s)",
		m.config.NodeID, m.config.NodeRole)

	m.mu.Lock()
	m.nodeInfo.Status = int32(NodeStatusRunning)
	m.nodeInfo.Uptime = time.Now().Unix()

	// Add self to cluster state
	if m.config.NodeRole == CoreNode {
		m.clusterState.CoreNodes[m.config.NodeID] = m.nodeInfo
	} else {
		m.clusterState.ReplicantNodes[m.config.NodeID] = m.nodeInfo
	}
	m.mu.Unlock()

	// Start cluster discovery and connection process
	if err := m.startDiscovery(ctx); err != nil {
		return fmt.Errorf("failed to start discovery: %w", err)
	}

	// Start heartbeat and cluster maintenance
	go m.heartbeatLoop(ctx)
	go m.clusterMaintenanceLoop(ctx)

	// Start leader election for core nodes
	if m.config.NodeRole == CoreNode {
		go m.leaderElectionLoop(ctx)
	}

	log.Printf("[Mria] Mria cluster manager started successfully")
	return nil
}

// Stop gracefully shuts down the Mria cluster manager
func (m *MriaManager) Stop() error {
	log.Printf("[Mria] Stopping Mria cluster manager")

	m.mu.Lock()
	m.nodeInfo.Status = int32(NodeStatusStopping)
	close(m.stopCh)
	m.mu.Unlock()

	// Close all connections
	for _, client := range m.coreConnections {
		client.Close()
	}
	for _, client := range m.replicantConnections {
		client.Close()
	}

	log.Printf("[Mria] Mria cluster manager stopped")
	return nil
}

// GetNodeRole returns the role of the current node
func (m *MriaManager) GetNodeRole() NodeRole {
	return m.config.NodeRole
}

// IsLeader returns true if the current node is the cluster leader
func (m *MriaManager) IsLeader() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.isLeader && m.config.NodeRole == CoreNode
}

// GetClusterState returns the current cluster state
func (m *MriaManager) GetClusterState() *ClusterState {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Return a copy to avoid concurrent modification
	state := &ClusterState{
		ClusterID:      m.clusterState.ClusterID,
		Leader:         m.clusterState.Leader,
		CoreNodes:      make(map[string]*clusterpb.NodeInfo),
		ReplicantNodes: make(map[string]*clusterpb.NodeInfo),
		LastUpdated:    m.clusterState.LastUpdated,
	}

	for k, v := range m.clusterState.CoreNodes {
		state.CoreNodes[k] = v
	}
	for k, v := range m.clusterState.ReplicantNodes {
		state.ReplicantNodes[k] = v
	}

	return state
}

// GetCoreNodes returns a list of all core nodes in the cluster
func (m *MriaManager) GetCoreNodes() []*clusterpb.NodeInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	nodes := make([]*clusterpb.NodeInfo, 0, len(m.clusterState.CoreNodes))
	for _, node := range m.clusterState.CoreNodes {
		nodes = append(nodes, node)
	}
	return nodes
}

// GetReplicantNodes returns a list of all replicant nodes in the cluster
func (m *MriaManager) GetReplicantNodes() []*clusterpb.NodeInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	nodes := make([]*clusterpb.NodeInfo, 0, len(m.clusterState.ReplicantNodes))
	for _, node := range m.clusterState.ReplicantNodes {
		nodes = append(nodes, node)
	}
	return nodes
}

// ForwardMessageToCore forwards a message to core nodes for processing
func (m *MriaManager) ForwardMessageToCore(topic string, payload []byte) error {
	if m.config.NodeRole == CoreNode {
		// If this is a core node, process locally
		return nil
	}

	// Forward to a core node
	coreNodes := m.GetCoreNodes()
	if len(coreNodes) == 0 {
		return fmt.Errorf("no core nodes available")
	}

	// Use round-robin or choose the leader
	var targetNode *clusterpb.NodeInfo
	m.mu.RLock()
	if m.clusterState.Leader != "" {
		for _, node := range coreNodes {
			if node.NodeId == m.clusterState.Leader {
				targetNode = node
				break
			}
		}
	}
	if targetNode == nil && len(coreNodes) > 0 {
		targetNode = coreNodes[0] // Fallback to first core node
	}
	m.mu.RUnlock()

	if targetNode == nil {
		return fmt.Errorf("no suitable core node found")
	}

	// Forward the message
	m.clusterManager.ForwardPublish(topic, payload, targetNode.NodeId)
	return nil
}

// ReplicateToReplicants replicates data changes to replicant nodes
func (m *MriaManager) ReplicateToReplicants(data interface{}) error {
	if m.config.NodeRole != CoreNode {
		return fmt.Errorf("only core nodes can replicate data")
	}

	replicantNodes := m.GetReplicantNodes()
	if len(replicantNodes) == 0 {
		return nil // No replicants to update
	}

	// Implement replication logic here
	// This would typically involve sending the data change to all replicant nodes
	log.Printf("[Mria] Replicating data to %d replicant nodes", len(replicantNodes))

	return nil
}

// startDiscovery initiates the node discovery process
func (m *MriaManager) startDiscovery(ctx context.Context) error {
	if !m.config.AutoDiscoveryEnabled {
		return m.connectToKnownNodes(ctx)
	}

	if m.discovery == nil {
		return m.connectToKnownNodes(ctx)
	}

	// Use discovery service to find peers
	peers, err := m.discovery.DiscoverPeers(ctx)
	if err != nil {
		log.Printf("[Mria] Discovery failed, falling back to known nodes: %v", err)
		return m.connectToKnownNodes(ctx)
	}

	log.Printf("[Mria] Discovered %d peers", len(peers))
	for _, peer := range peers {
		if peer.ID != m.config.NodeID {
			go m.connectToPeer(ctx, peer.ID, peer.Address)
		}
	}

	return nil
}

// connectToKnownNodes connects to statically configured core nodes
func (m *MriaManager) connectToKnownNodes(ctx context.Context) error {
	for i, address := range m.config.CoreNodes {
		nodeID := fmt.Sprintf("core-%d", i)
		if nodeID != m.config.NodeID {
			go m.connectToPeer(ctx, nodeID, address)
		}
	}
	return nil
}

// connectToPeer establishes a connection to a peer node
func (m *MriaManager) connectToPeer(ctx context.Context, peerID, address string) {
	log.Printf("[Mria] Attempting to connect to peer %s at %s", peerID, address)
	m.clusterManager.AddPeer(ctx, peerID, address)
}

// heartbeatLoop maintains heartbeat with other nodes
func (m *MriaManager) heartbeatLoop(ctx context.Context) {
	ticker := time.NewTicker(m.config.HeartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-m.stopCh:
			return
		case <-ticker.C:
			m.sendHeartbeat()
		}
	}
}

// clusterMaintenanceLoop performs periodic cluster maintenance
func (m *MriaManager) clusterMaintenanceLoop(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second) // Maintenance every 30 seconds
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-m.stopCh:
			return
		case <-ticker.C:
			m.performMaintenance()
		}
	}
}

// leaderElectionLoop handles leader election for core nodes
func (m *MriaManager) leaderElectionLoop(ctx context.Context) {
	if m.config.NodeRole != CoreNode {
		return
	}

	ticker := time.NewTicker(m.config.ElectionTimeout)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-m.stopCh:
			return
		case <-ticker.C:
			m.performLeaderElection()
		case becomeLeader := <-m.leaderElectionCh:
			m.handleLeadershipChange(becomeLeader)
		}
	}
}

// sendHeartbeat sends heartbeat to maintain node status
func (m *MriaManager) sendHeartbeat() {
	m.mu.Lock()
	m.nodeInfo.LastSeen = time.Now().Unix()
	m.nodeInfo.Uptime = time.Now().Unix() - m.nodeInfo.Uptime
	m.mu.Unlock()

	// Implementation would send heartbeat to peers
	log.Printf("[Mria] Heartbeat sent from node %s", m.config.NodeID)
}

// performMaintenance performs periodic cluster maintenance tasks
func (m *MriaManager) performMaintenance() {
	// Remove offline nodes, update cluster state, etc.
	log.Printf("[Mria] Performing cluster maintenance")

	m.mu.Lock()
	m.clusterState.LastUpdated = time.Now()
	m.mu.Unlock()
}

// performLeaderElection performs leader election among core nodes
func (m *MriaManager) performLeaderElection() {
	if m.config.NodeRole != CoreNode {
		return
	}

	// Simple leader election based on node ID (lexicographically smallest)
	coreNodes := m.GetCoreNodes()
	if len(coreNodes) == 0 {
		return
	}

	var leader string
	for _, node := range coreNodes {
		if node.Status == int32(NodeStatusRunning) {
			if leader == "" || node.NodeId < leader {
				leader = node.NodeId
			}
		}
	}

	m.mu.Lock()
	oldLeader := m.clusterState.Leader
	wasLeader := m.isLeader
	m.clusterState.Leader = leader
	m.isLeader = (leader == m.config.NodeID)
	m.mu.Unlock()

	if oldLeader != leader {
		log.Printf("[Mria] Leader changed from %s to %s", oldLeader, leader)
		if m.isLeader && !wasLeader {
			m.leaderElectionCh <- true
		} else if !m.isLeader && wasLeader {
			m.leaderElectionCh <- false
		}
	}
}

// handleLeadershipChange handles becoming/losing leadership
func (m *MriaManager) handleLeadershipChange(becomeLeader bool) {
	if becomeLeader {
		log.Printf("[Mria] Node %s became cluster leader", m.config.NodeID)
		if m.onBecomeLeader != nil {
			m.onBecomeLeader()
		}
	} else {
		log.Printf("[Mria] Node %s lost cluster leadership", m.config.NodeID)
		if m.onLoseLeadership != nil {
			m.onLoseLeadership()
		}
	}
}