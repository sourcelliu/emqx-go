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

package cluster

import (
	"context"
	"log"

	clusterpb "github.com/turtacn/emqx-go/pkg/proto/cluster"
)

// Server implements the gRPC ClusterService server. It is responsible for
// handling incoming RPCs from other nodes in the cluster. This includes
// requests to join or leave the cluster, as well as requests to synchronize
// state such as routes and configurations.
type Server struct {
	clusterpb.UnimplementedClusterServiceServer
	// NodeID is the unique identifier of the node this server is running on.
	NodeID string
}

// NewServer creates and returns a new instance of the cluster Server.
func NewServer(nodeID string) *Server {
	return &Server{NodeID: nodeID}
}

// Join handles a request from another node to join the cluster.
// In a real implementation, this would involve authenticating the new node and
// adding it to the list of cluster members.
func (s *Server) Join(ctx context.Context, req *clusterpb.JoinRequest) (*clusterpb.JoinResponse, error) {
	log.Printf("Received Join request from node %s at %s", req.Node.NodeId, req.Node.Address)
	// In a real implementation, we would return the actual list of nodes.
	return &clusterpb.JoinResponse{
		Success:      true,
		Message:      "Welcome to the cluster!",
		ClusterNodes: []*clusterpb.NodeInfo{{NodeId: s.NodeID}},
	}, nil
}

// Leave handles a request from another node to leave the cluster.
func (s *Server) Leave(ctx context.Context, req *clusterpb.LeaveRequest) (*clusterpb.LeaveResponse, error) {
	log.Printf("Received Leave request from node %s", req.NodeId)
	return &clusterpb.LeaveResponse{Success: true}, nil
}

// SyncNodeStatus handles a request to synchronize node status.
func (s *Server) SyncNodeStatus(ctx context.Context, req *clusterpb.SyncNodeStatusRequest) (*clusterpb.SyncNodeStatusResponse, error) {
	log.Printf("Received SyncNodeStatus request from node %s", req.Node.NodeId)
	return &clusterpb.SyncNodeStatusResponse{Success: true}, nil
}

// SyncSubscriptions handles a request to synchronize subscription information.
func (s *Server) SyncSubscriptions(ctx context.Context, req *clusterpb.SyncSubscriptionsRequest) (*clusterpb.SyncSubscriptionsResponse, error) {
	log.Printf("Received SyncSubscriptions request from node %s with %d subscriptions", req.NodeId, len(req.Subscriptions))
	return &clusterpb.SyncSubscriptionsResponse{Success: true, ReceivedCount: int32(len(req.Subscriptions))}, nil
}

// ForwardPublish handles a request to forward a published message to local subscribers.
func (s *Server) ForwardPublish(ctx context.Context, req *clusterpb.PublishForward) (*clusterpb.ForwardAck, error) {
	log.Printf("Received ForwardPublish request for topic %s from node %s", req.Topic, req.FromNode)
	// In a real implementation, this would trigger a local publish to the topic.
	return &clusterpb.ForwardAck{MessageId: req.MessageId, Success: true}, nil
}

// BatchUpdateRoutes handles a request to update routing information.
func (s *Server) BatchUpdateRoutes(ctx context.Context, req *clusterpb.BatchUpdateRoutesRequest) (*clusterpb.BatchUpdateRoutesResponse, error) {
	log.Printf("Received BatchUpdateRoutes request from node %s", req.FromNode)
	// Here, we would update the local routing table based on the request.
	return &clusterpb.BatchUpdateRoutesResponse{Success: true, UpdatedCount: int32(len(req.Routes))}, nil
}

// SyncConfig handles a request to synchronize configuration.
func (s *Server) SyncConfig(ctx context.Context, req *clusterpb.SyncConfigRequest) (*clusterpb.SyncConfigResponse, error) {
	log.Printf("Received SyncConfig request from node %s", req.NodeId)
	return &clusterpb.SyncConfigResponse{Success: true}, nil
}

// GetClusterStats handles a request for cluster statistics.
func (s *Server) GetClusterStats(ctx context.Context, req *clusterpb.ClusterStatsRequest) (*clusterpb.ClusterStatsResponse, error) {
	log.Printf("Received GetClusterStats request from node %s", req.NodeId)
	return &clusterpb.ClusterStatsResponse{Success: true}, nil
}