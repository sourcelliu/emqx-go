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

// Package cluster provides the functionality for creating a distributed cluster
// of EMQX-Go nodes. It handles peer discovery, inter-node communication via
// gRPC, and routing of messages between nodes.
package cluster

import (
	"context"
	"log"
	"sync"
	"time"

	clusterpb "github.com/turtacn/emqx-go/pkg/proto/cluster"
)

// Manager is responsible for managing the state of the cluster from the
// perspective of a single node. It maintains connections to peer nodes, manages
// the distributed routing table, and provides methods for broadcasting and
// forwarding messages to other nodes in the cluster.
type Manager struct {
	NodeID           string
	NodeAddress      string
	peers            map[string]*Client
	remoteRoutes     map[string][]string // Map of Topic to list of NodeIDs
	mu               sync.RWMutex
	LocalPublishFunc func(topic string, payload []byte)
}

// NewManager creates and initializes a new Manager instance.
//
// - nodeID: A unique identifier for the local node.
// - nodeAddress: The gRPC address of the local node.
// - localPublishFunc: A callback function to publish messages to local subscribers.
func NewManager(nodeID, nodeAddress string, localPublishFunc func(topic string, payload []byte)) *Manager {
	return &Manager{
		NodeID:           nodeID,
		NodeAddress:      nodeAddress,
		peers:            make(map[string]*Client),
		remoteRoutes:     make(map[string][]string),
		LocalPublishFunc: localPublishFunc,
	}
}

// AddPeer establishes a connection to a peer node and sends a join request.
// If the connection and join are successful, the peer is added to the manager's
// pool of connected peers.
//
// - ctx: A context for managing the connection and join request.
// - peerID: The unique identifier of the peer node to connect to.
// - address: The network address of the peer's gRPC server.
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

// BroadcastRouteUpdate sends information about new topic subscriptions on the
// local node to all other connected peers in the cluster. This ensures that
// other nodes are aware of which topics this node is subscribed to.
//
// - routes: A slice of Route objects, each representing a new subscription.
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

// AddRemoteRoute adds a new entry to the distributed routing table. It is
// called by the gRPC server when a route update is received from a peer,
// indicating that a client on another node has subscribed to a topic.
//
// - topic: The topic that has a new remote subscriber.
// - nodeID: The ID of the node where the subscriber is located.
func (m *Manager) AddRemoteRoute(topic, nodeID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	log.Printf("Adding remote route: Topic=%s, Node=%s", topic, nodeID)
	// Avoid duplicate entries
	for _, existingNodeID := range m.remoteRoutes[topic] {
		if existingNodeID == nodeID {
			return
		}
	}
	m.remoteRoutes[topic] = append(m.remoteRoutes[topic], nodeID)
}

// GetRemoteSubscribers returns a list of node IDs that have subscribers for a
// given topic. This is used to determine which peers a message should be
// forwarded to.
//
// - topic: The topic to look up in the routing table.
//
// Returns a slice of node IDs.
func (m *Manager) GetRemoteSubscribers(topic string) []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.remoteRoutes[topic]
}

// ForwardPublish sends a PUBLISH message to a specific peer node. This is used
// when a message published to the local node needs to be delivered to clients
// connected to other nodes in the cluster.
//
// - topic: The topic of the message.
// - payload: The message content.
// - nodeID: The ID of the destination node.
func (m *Manager) ForwardPublish(topic string, payload []byte, nodeID string) {
	m.mu.RLock()
	peer, ok := m.peers[nodeID]
	m.mu.RUnlock()

	if !ok {
		log.Printf("Cannot forward publish: no peer client for node %s", nodeID)
		return
	}

	req := &clusterpb.PublishForward{
		Topic:    topic,
		Payload:  payload,
		FromNode: m.NodeID,
	}

	go func() {
		if _, err := peer.ForwardPublish(context.Background(), req); err != nil {
			log.Printf("Failed to forward publish to node %s: %v", nodeID, err)
		}
	}()
}