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

// Server implements the gRPC ClusterService server.
type Server struct {
	clusterpb.UnimplementedClusterServiceServer
	NodeID  string
	manager *Manager
}

// NewServer creates a new cluster server.
func NewServer(nodeID string, manager *Manager) *Server {
	return &Server{
		NodeID:  nodeID,
		manager: manager,
	}
}

// Join handles a request from another node to join the cluster.
func (s *Server) Join(ctx context.Context, req *clusterpb.JoinRequest) (*clusterpb.JoinResponse, error) {
	log.Printf("Received Join request from node %s at %s", req.Node.NodeId, req.Node.Address)
	// In a real implementation, we would add the node to our peer list.
	return &clusterpb.JoinResponse{
		Success:      true,
		Message:      "Welcome to the cluster!",
		ClusterNodes: []*clusterpb.NodeInfo{{NodeId: s.NodeID}},
	}, nil
}

// BatchUpdateRoutes handles a request to update routing information.
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

// ForwardPublish handles a request to forward a published message to local subscribers.
func (s *Server) ForwardPublish(ctx context.Context, req *clusterpb.PublishForward) (*clusterpb.ForwardAck, error) {
	log.Printf("Received ForwardPublish request for topic '%s' from node %s", req.Topic, req.FromNode)
	if s.manager.LocalPublishFunc != nil {
		s.manager.LocalPublishFunc(req.Topic, req.Payload)
	}
	return &clusterpb.ForwardAck{Success: true}, nil
}