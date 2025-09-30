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
	"net"
	"time"

	"github.com/turtacn/emqx-go/pkg/cluster"
	clusterpb "github.com/turtacn/emqx-go/pkg/proto/cluster"
	"google.golang.org/grpc"
)

// MriaClusterServer extends the basic cluster server with Mria-specific functionality
type MriaClusterServer struct {
	clusterpb.UnimplementedClusterServiceServer

	mriaManager        *MriaManager
	replicationManager *ReplicationManager
	baseServer         *cluster.Server
	grpcServer         *grpc.Server
	listener           net.Listener

	// Configuration
	nodeID      string
	nodeRole    NodeRole
	listenAddr  string
}

// NewMriaClusterServer creates a new Mria cluster server
func NewMriaClusterServer(
	nodeID string,
	nodeRole NodeRole,
	listenAddr string,
	mriaManager *MriaManager,
	replicationManager *ReplicationManager,
) *MriaClusterServer {
	return &MriaClusterServer{
		mriaManager:        mriaManager,
		replicationManager: replicationManager,
		nodeID:             nodeID,
		nodeRole:           nodeRole,
		listenAddr:         listenAddr,
	}
}

// Start starts the Mria cluster server
func (s *MriaClusterServer) Start(ctx context.Context) error {
	// Create listener
	listener, err := net.Listen("tcp", s.listenAddr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", s.listenAddr, err)
	}
	s.listener = listener

	// Create gRPC server
	s.grpcServer = grpc.NewServer()
	clusterpb.RegisterClusterServiceServer(s.grpcServer, s)

	log.Printf("[Mria] Starting Mria cluster server on %s", s.listenAddr)

	// Start serving in a goroutine
	go func() {
		if err := s.grpcServer.Serve(listener); err != nil {
			log.Printf("[Mria] Cluster server error: %v", err)
		}
	}()

	return nil
}

// Stop stops the Mria cluster server
func (s *MriaClusterServer) Stop() error {
	if s.grpcServer != nil {
		s.grpcServer.GracefulStop()
	}
	if s.listener != nil {
		s.listener.Close()
	}
	return nil
}

// Join handles node join requests with Mria role awareness
func (s *MriaClusterServer) Join(ctx context.Context, req *clusterpb.JoinRequest) (*clusterpb.JoinResponse, error) {
	log.Printf("[Mria] Join request from node %s at %s", req.Node.NodeId, req.Node.Address)

	// Validate the joining node
	if req.Node.NodeId == s.nodeID {
		return &clusterpb.JoinResponse{
			Success: false,
			Message: "Cannot join self",
		}, nil
	}

	// For Mria, core nodes can accept both core and replicant nodes
	// Replicant nodes typically don't accept join requests directly
	if s.nodeRole == ReplicantNode {
		// Redirect to a core node
		coreNodes := s.mriaManager.GetCoreNodes()
		if len(coreNodes) > 0 {
			coreAddr := coreNodes[0].Address
			return &clusterpb.JoinResponse{
				Success: false,
				Message: fmt.Sprintf("Replicant node - please join via core node at %s", coreAddr),
			}, nil
		}
	}

	// Accept the join request
	clusterState := s.mriaManager.GetClusterState()
	allNodes := make([]*clusterpb.NodeInfo, 0)

	// Add all core nodes
	for _, node := range clusterState.CoreNodes {
		allNodes = append(allNodes, node)
	}

	// Add all replicant nodes
	for _, node := range clusterState.ReplicantNodes {
		allNodes = append(allNodes, node)
	}

	// Update cluster state with the new node
	if req.Node.NodeId != "" {
		// Determine node role based on some criteria (could be sent in request)
		// For now, assume core nodes have "core" in their ID
		newNodeRole := ReplicantNode
		if req.Node.NodeId == "core" || len(req.Node.NodeId) > 4 && req.Node.NodeId[:4] == "core" {
			newNodeRole = CoreNode
		}

		// Add to appropriate node list
		if newNodeRole == CoreNode {
			s.mriaManager.clusterState.CoreNodes[req.Node.NodeId] = req.Node
		} else {
			s.mriaManager.clusterState.ReplicantNodes[req.Node.NodeId] = req.Node
		}

		// Trigger callback
		if s.mriaManager.onNodeJoin != nil {
			s.mriaManager.onNodeJoin(req.Node)
		}
	}

	return &clusterpb.JoinResponse{
		Success:      true,
		Message:      fmt.Sprintf("Successfully joined cluster as %s node", s.nodeRole),
		ClusterNodes: allNodes,
		ClusterId:    clusterState.ClusterID,
	}, nil
}

// Leave handles node leave requests
func (s *MriaClusterServer) Leave(ctx context.Context, req *clusterpb.LeaveRequest) (*clusterpb.LeaveResponse, error) {
	log.Printf("[Mria] Leave request from node %s", req.NodeId)

	// Remove from cluster state
	clusterState := s.mriaManager.GetClusterState()

	delete(clusterState.CoreNodes, req.NodeId)
	delete(clusterState.ReplicantNodes, req.NodeId)

	// Trigger callback
	if s.mriaManager.onNodeLeave != nil {
		s.mriaManager.onNodeLeave(req.NodeId)
	}

	// Get remaining nodes
	remainingNodes := make([]*clusterpb.NodeInfo, 0)
	for _, node := range clusterState.CoreNodes {
		remainingNodes = append(remainingNodes, node)
	}
	for _, node := range clusterState.ReplicantNodes {
		remainingNodes = append(remainingNodes, node)
	}

	return &clusterpb.LeaveResponse{
		Success:        true,
		Message:        "Node removed from cluster",
		RemainingNodes: remainingNodes,
	}, nil
}

// SyncNodeStatus handles node status synchronization
func (s *MriaClusterServer) SyncNodeStatus(ctx context.Context, req *clusterpb.SyncNodeStatusRequest) (*clusterpb.SyncNodeStatusResponse, error) {
	log.Printf("[Mria] Node status sync from %s", req.Node.NodeId)

	// Update the requesting node's status
	clusterState := s.mriaManager.GetClusterState()

	// Update in appropriate list
	if _, exists := clusterState.CoreNodes[req.Node.NodeId]; exists {
		clusterState.CoreNodes[req.Node.NodeId] = req.Node
	} else {
		clusterState.ReplicantNodes[req.Node.NodeId] = req.Node
	}

	// Return all known nodes
	allNodes := make([]*clusterpb.NodeInfo, 0)
	for _, node := range clusterState.CoreNodes {
		allNodes = append(allNodes, node)
	}
	for _, node := range clusterState.ReplicantNodes {
		allNodes = append(allNodes, node)
	}

	return &clusterpb.SyncNodeStatusResponse{
		Success:  true,
		Message:  "Status synchronized",
		AllNodes: allNodes,
	}, nil
}

// SyncSubscriptions handles subscription synchronization
func (s *MriaClusterServer) SyncSubscriptions(ctx context.Context, req *clusterpb.SyncSubscriptionsRequest) (*clusterpb.SyncSubscriptionsResponse, error) {
	log.Printf("[Mria] Subscription sync from node %s (%d subscriptions)",
		req.NodeId, len(req.Subscriptions))

	receivedCount := int32(0)

	// Process each subscription
	for _, sub := range req.Subscriptions {
		err := s.replicationManager.AddSubscription(sub.ClientId, sub.TopicFilter, int(sub.Qos))
		if err != nil {
			log.Printf("[Mria] Failed to add subscription: %v", err)
			continue
		}
		receivedCount++
	}

	return &clusterpb.SyncSubscriptionsResponse{
		Success:       true,
		Message:       fmt.Sprintf("Processed %d subscriptions", receivedCount),
		ReceivedCount: receivedCount,
	}, nil
}

// ForwardPublish handles message forwarding with Mria replication awareness
func (s *MriaClusterServer) ForwardPublish(ctx context.Context, req *clusterpb.PublishForward) (*clusterpb.ForwardAck, error) {
	log.Printf("[Mria] Message forward from node %s: topic=%s, payload_size=%d",
		req.FromNode, req.Topic, len(req.Payload))

	// Check if this is a Mria replication message
	if len(req.Topic) > 6 && req.Topic[:6] == "$mria/" {
		return s.handleMriaMessage(ctx, req)
	}

	// Handle regular message forwarding
	// This would typically involve publishing to local subscribers
	success := true
	message := "Message forwarded successfully"

	// If this is a replicant node, it should handle message delivery
	// If this is a core node, it might also need to replicate to other replicants

	return &clusterpb.ForwardAck{
		MessageId: req.MessageId,
		Success:   success,
		Message:   message,
		ToNode:    s.nodeID,
	}, nil
}

// handleMriaMessage handles Mria-specific messages
func (s *MriaClusterServer) handleMriaMessage(ctx context.Context, req *clusterpb.PublishForward) (*clusterpb.ForwardAck, error) {
	// Parse Mria message topic
	if len(req.Topic) > 16 && req.Topic[:16] == "$mria/replication" {
		// This is a replication message
		return s.handleReplicationMessage(ctx, req)
	}

	return &clusterpb.ForwardAck{
		MessageId: req.MessageId,
		Success:   false,
		Message:   "Unknown Mria message type",
		ToNode:    s.nodeID,
	}, nil
}

// handleReplicationMessage handles replication messages
func (s *MriaClusterServer) handleReplicationMessage(ctx context.Context, req *clusterpb.PublishForward) (*clusterpb.ForwardAck, error) {
	if s.nodeRole != ReplicantNode {
		return &clusterpb.ForwardAck{
			MessageId: req.MessageId,
			Success:   false,
			Message:   "Replication messages are only for replicant nodes",
			ToNode:    s.nodeID,
		}, nil
	}

	// Apply the replication log
	err := s.replicationManager.ApplyReplicationLog(req.Payload)
	if err != nil {
		log.Printf("[Mria] Failed to apply replication log: %v", err)
		return &clusterpb.ForwardAck{
			MessageId: req.MessageId,
			Success:   false,
			Message:   fmt.Sprintf("Failed to apply replication: %v", err),
			ToNode:    s.nodeID,
		}, nil
	}

	return &clusterpb.ForwardAck{
		MessageId: req.MessageId,
		Success:   true,
		Message:   "Replication applied successfully",
		ToNode:    s.nodeID,
	}, nil
}

// BatchUpdateRoutes handles routing table updates
func (s *MriaClusterServer) BatchUpdateRoutes(ctx context.Context, req *clusterpb.BatchUpdateRoutesRequest) (*clusterpb.BatchUpdateRoutesResponse, error) {
	log.Printf("[Mria] Route update from node %s: %d routes", req.FromNode, len(req.Routes))

	updatedCount := int32(0)

	for _, route := range req.Routes {
		switch req.OpType {
		case "add", "update":
			err := s.replicationManager.UpdateRoute(route.Topic, route.NodeIds)
			if err != nil {
				log.Printf("[Mria] Failed to update route: %v", err)
				continue
			}
		case "delete":
			err := s.replicationManager.UpdateRoute(route.Topic, []string{})
			if err != nil {
				log.Printf("[Mria] Failed to delete route: %v", err)
				continue
			}
		}
		updatedCount++
	}

	return &clusterpb.BatchUpdateRoutesResponse{
		Success:      true,
		Message:      fmt.Sprintf("Updated %d routes", updatedCount),
		UpdatedCount: updatedCount,
	}, nil
}

// SyncConfig handles configuration synchronization
func (s *MriaClusterServer) SyncConfig(ctx context.Context, req *clusterpb.SyncConfigRequest) (*clusterpb.SyncConfigResponse, error) {
	log.Printf("[Mria] Config sync from node %s", req.NodeId)

	// For Mria, configuration is typically managed by core nodes
	if s.nodeRole != CoreNode {
		return &clusterpb.SyncConfigResponse{
			Success: false,
			Message: "Configuration sync only available on core nodes",
		}, nil
	}

	// Return current configuration
	configs := make(map[string]string)
	configs["node.role"] = string(s.nodeRole)
	configs["cluster.name"] = s.mriaManager.GetClusterState().ClusterID
	configs["node.id"] = s.nodeID

	return &clusterpb.SyncConfigResponse{
		Success: true,
		Message: "Configuration synchronized",
		Configs: configs,
	}, nil
}

// GetClusterStats returns cluster statistics
func (s *MriaClusterServer) GetClusterStats(ctx context.Context, req *clusterpb.ClusterStatsRequest) (*clusterpb.ClusterStatsResponse, error) {
	log.Printf("[Mria] Cluster stats request")

	clusterState := s.mriaManager.GetClusterState()

	// Create stats for current node
	metrics := make(map[string]int64)
	metrics["core_nodes"] = int64(len(clusterState.CoreNodes))
	metrics["replicant_nodes"] = int64(len(clusterState.ReplicantNodes))
	metrics["subscriptions"] = int64(len(s.replicationManager.GetAllSubscriptions()))
	metrics["sessions"] = int64(len(s.replicationManager.GetAllSessions()))
	metrics["routes"] = int64(len(s.replicationManager.GetAllRoutes()))

	if s.mriaManager.IsLeader() {
		metrics["is_leader"] = 1
	} else {
		metrics["is_leader"] = 0
	}

	nodeStats := &clusterpb.NodeStats{
		NodeId:    s.nodeID,
		Metrics:   metrics,
		Timestamp: time.Now().Unix(),
	}

	return &clusterpb.ClusterStatsResponse{
		Success:   true,
		Message:   "Cluster statistics retrieved",
		NodeStats: []*clusterpb.NodeStats{nodeStats},
		Timestamp: time.Now().Unix(),
	}, nil
}

// GetClusterState returns the current Mria cluster state
func (s *MriaClusterServer) GetClusterState() *ClusterState {
	return s.mriaManager.GetClusterState()
}

// GetReplicationManager returns the replication manager
func (s *MriaClusterServer) GetReplicationManager() *ReplicationManager {
	return s.replicationManager
}

// HandleMriaReplication handles replication-specific operations
func (s *MriaClusterServer) HandleMriaReplication(operation, table string, data interface{}) error {
	// Create replication log entry
	logEntry := ReplicationLog{
		ID:        fmt.Sprintf("%s-%d", s.nodeID, time.Now().UnixNano()),
		Timestamp: time.Now().Unix(),
		NodeID:    s.nodeID,
		Operation: operation,
		Table:     table,
		Data:      data,
		Version:   time.Now().Unix(),
	}

	// Apply locally first
	logBytes, err := json.Marshal(logEntry)
	if err != nil {
		return fmt.Errorf("failed to marshal log entry: %w", err)
	}

	return s.replicationManager.ApplyReplicationLog(logBytes)
}