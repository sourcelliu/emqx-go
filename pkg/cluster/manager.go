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

// package cluster provides the functionality for creating a cluster of EMQX-Go
// nodes. It handles peer discovery, state synchronization, and message routing
// between nodes using gRPC.
package cluster

import (
	"context"
	"log"
	"sync"
	"time"

	clusterpb "github.com/turtacn/emqx-go/pkg/proto/cluster"
)

// Manager is the central component for managing cluster-related activities.
// It maintains the state of the cluster, including the list of connected peers
// and the routing table for message forwarding. It is responsible for initiating
// connections to other nodes and broadcasting state changes.
type Manager struct {
	// NodeID is the unique identifier for this node in the cluster.
	NodeID string
	// NodeAddress is the gRPC address that this node listens on for cluster
	// communication.
	NodeAddress string
	peers        map[string]*Client
	remoteRoutes map[string][]string // Map of Topic to list of NodeIDs
	mu           sync.RWMutex
}

// NewManager creates and returns a new instance of the cluster Manager.
func NewManager(nodeID, nodeAddress string) *Manager {
	return &Manager{
		NodeID:       nodeID,
		NodeAddress:  nodeAddress,
		peers:        make(map[string]*Client),
		remoteRoutes: make(map[string][]string),
	}
}

// AddPeer establishes a connection to a new peer and sends a request to join
// the cluster. If the connection and join request are successful, the peer is
// added to the manager's list of active peers.
func (m *Manager) AddPeer(ctx context.Context, peerID, address string) {
	m.mu.Lock()
	if _, exists := m.peers[peerID]; exists {
		m.mu.Unlock()
		return // Already connected
	}
	m.mu.Unlock()

	log.Printf("Cluster Manager: Attempting to connect to peer %s at %s", peerID, address)
	client := NewClient(m.NodeID)
	if err := client.Connect(ctx, address); err != nil {
		log.Printf("Failed to connect to peer %s: %v", peerID, err)
		return
	}

	joinReq := &clusterpb.JoinRequest{
		Node: &clusterpb.NodeInfo{
			NodeId:  m.NodeID,
			Address: m.NodeAddress,
			Version: "0.1.0-poc",
		},
		Timestamp: time.Now().Unix(),
	}

	resp, err := client.Join(ctx, joinReq)
	if err != nil {
		log.Printf("Failed to join cluster via peer %s: %v", peerID, err)
		client.Close()
		return
	}

	if resp.Success {
		log.Printf("Successfully joined cluster via peer %s. Cluster ID: %s", peerID, resp.ClusterId)
		m.mu.Lock()
		m.peers[peerID] = client
		m.mu.Unlock()
	} else {
		log.Printf("Peer %s rejected join request: %s", peerID, resp.Message)
		client.Close()
	}
}

// BroadcastRouteUpdate sends a route update to all connected peers in the cluster.
// This is used to inform other nodes about new subscriptions on this node.
func (m *Manager) BroadcastRouteUpdate(routes []*clusterpb.Route) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	req := &clusterpb.BatchUpdateRoutesRequest{
		Routes:   routes,
		OpType:   "add",
		FromNode: m.NodeID,
	}

	for id, peer := range m.peers {
		go func(peerID string, c *Client) {
			if _, err := c.BatchUpdateRoutes(context.Background(), req); err != nil {
				log.Printf("Failed to send route update to peer %s: %v", peerID, err)
			}
		}(id, peer)
	}
}

// AddRemoteRoute adds a route for a topic to a remote node. This method is
// typically called by the gRPC server when it receives a route update from a peer.
func (m *Manager) AddRemoteRoute(topic, nodeID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	log.Printf("Adding remote route: Topic=%s, Node=%s", topic, nodeID)
	m.remoteRoutes[topic] = append(m.remoteRoutes[topic], nodeID)
}

// GetRemoteSubscribers returns a list of node IDs that have subscribers for a
// given topic. This is used to determine which nodes to forward a message to.
func (m *Manager) GetRemoteSubscribers(topic string) []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.remoteRoutes[topic]
}