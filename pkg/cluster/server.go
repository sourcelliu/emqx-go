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

// Server implements the gRPC handlers for the ClusterService. It receives
// requests from peer nodes and uses the cluster Manager to update the local
// cluster state, such as adding new peers or updating routing information.
type Server struct {
	clusterpb.UnimplementedClusterServiceServer
	// NodeID is the unique identifier of the local node.
	NodeID  string
	manager *Manager
}

// NewServer creates and initializes a new Server instance.
//
// - nodeID: The unique identifier of the local node.
// - manager: A pointer to the cluster Manager that the server will interact with.
func NewServer(nodeID string, manager *Manager) *Server {
	return &Server{
		NodeID:  nodeID,
		manager: manager,
	}
}

// Join is the gRPC handler for a JoinRequest from a peer. It processes the
// request and, if successful, allows the peer to join the cluster.
//
// - ctx: The context for the gRPC call.
// - req: The join request from the peer.
//
// Returns a response indicating success or failure.
func (s *Server) Join(ctx context.Context, req *clusterpb.JoinRequest) (*clusterpb.JoinResponse, error) {
	log.Printf("Received Join request from node %s at %s", req.Node.NodeId, req.Node.Address)

	// Add the joining peer to our peer list
	go s.manager.AddPeer(ctx, req.Node.NodeId, req.Node.Address)

	// In a real implementation, we would add the node to our peer list.
	return &clusterpb.JoinResponse{
		Success:      true,
		ClusterId:    "cluster-1",  // Add a cluster ID
		Message:      "Welcome to the cluster!",
		ClusterNodes: []*clusterpb.NodeInfo{{NodeId: s.NodeID}},
	}, nil
}

// BatchUpdateRoutes is the gRPC handler for a request to add new routing
// entries. It iterates through the received routes and adds them to the local
// routing table via the cluster manager.
//
// - ctx: The context for the gRPC call.
// - req: The request containing the new routes.
//
// Returns a response indicating the number of routes updated.
func (s *Server) BatchUpdateRoutes(ctx context.Context, req *clusterpb.BatchUpdateRoutesRequest) (*clusterpb.BatchUpdateRoutesResponse, error) {
	log.Printf("Received BatchUpdateRoutes request from node %s", req.FromNode)
	for _, route := range req.Routes {
		for _, nodeID := range route.NodeIds {
			s.manager.AddRemoteRoute(route.Topic, nodeID)
		}
	}
	return &clusterpb.BatchUpdateRoutesResponse{
		Success:      true,
		UpdatedCount: int32(len(req.Routes)),
	}, nil
}

// ForwardPublish is the gRPC handler for a forwarded PUBLISH message from a
// peer. It receives the message and uses the manager's local publish callback
// to deliver it to subscribers on the local node.
//
// - ctx: The context for the gRPC call.
// - req: The forwarded message.
//
// Returns an acknowledgment of receipt.
func (s *Server) ForwardPublish(ctx context.Context, req *clusterpb.PublishForward) (*clusterpb.ForwardAck, error) {
	log.Printf("Received ForwardPublish request for topic '%s' from node %s", req.Topic, req.FromNode)
	if s.manager.LocalPublishFunc != nil {
		s.manager.LocalPublishFunc(req.Topic, req.Payload)
	}
	return &clusterpb.ForwardAck{Success: true}, nil
}