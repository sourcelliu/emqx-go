package cluster

import (
	"context"
	"log"
	"sync"
	"time"

	pb "github.com/turtacn/emqx-go/pkg/proto/cluster"
)

// Manager handles the state of the cluster, including peer connections and routing tables.
type Manager struct {
	NodeID       string
	NodeAddress  string // The gRPC address of this node
	peers        map[string]*Client
	remoteRoutes map[string][]string // Map of Topic to list of NodeIDs
	mu           sync.RWMutex
}

// NewManager creates a new cluster manager.
func NewManager(nodeID, nodeAddress string) *Manager {
	return &Manager{
		NodeID:       nodeID,
		NodeAddress:  nodeAddress,
		peers:        make(map[string]*Client),
		remoteRoutes: make(map[string][]string),
	}
}

// AddPeer connects to a new peer and attempts to join the cluster.
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

	joinReq := &pb.JoinRequest{
		Node: &pb.NodeInfo{
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

// BroadcastRouteUpdate sends a route update to all connected peers.
func (m *Manager) BroadcastRouteUpdate(routes []*pb.Route) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	req := &pb.BatchUpdateRoutesRequest{
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

// AddRemoteRoute is called when a route update is received from a peer.
// In a real implementation, the gRPC server would call this.
func (m *Manager) AddRemoteRoute(topic, nodeID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	log.Printf("Adding remote route: Topic=%s, Node=%s", topic, nodeID)
	m.remoteRoutes[topic] = append(m.remoteRoutes[topic], nodeID)
}

// GetRemoteSubscribers returns the node IDs for a given topic.
func (m *Manager) GetRemoteSubscribers(topic string) []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.remoteRoutes[topic]
}