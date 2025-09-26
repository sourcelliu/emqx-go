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
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// Client provides the functionality to interact with another node in the cluster
// as a gRPC client. It manages the connection and provides methods for sending
// RPCs to the remote node's gRPC server.
type Client struct {
	// NodeID is the identifier of the node this client belongs to.
	NodeID string
	conn   *grpc.ClientConn
	client clusterpb.ClusterServiceClient
}

// NewClient creates and returns a new instance of a cluster Client.
func NewClient(nodeID string) *Client {
	return &Client{NodeID: nodeID}
}

// Connect establishes a gRPC connection to a peer node at the specified address.
// It creates a new gRPC client for the ClusterService.
func (c *Client) Connect(ctx context.Context, targetAddress string) error {
	conn, err := grpc.DialContext(ctx, targetAddress, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithBlock())
	if err != nil {
		return err
	}
	c.conn = conn
	c.client = clusterpb.NewClusterServiceClient(conn)
	return nil
}

// Close terminates the gRPC connection to the peer node.
func (c *Client) Close() {
	if c.conn != nil {
		c.conn.Close()
	}
}

// Join sends a JoinRequest to the peer node to initiate the process of joining
// the cluster.
func (c *Client) Join(ctx context.Context, req *clusterpb.JoinRequest) (*clusterpb.JoinResponse, error) {
	log.Printf("Sending Join request to peer")
	return c.client.Join(ctx, req)
}

// BatchUpdateRoutes sends a batch of route updates to the peer node. This is
// used to synchronize routing information across the cluster.
func (c *Client) BatchUpdateRoutes(ctx context.Context, req *clusterpb.BatchUpdateRoutesRequest) (*clusterpb.BatchUpdateRoutesResponse, error) {
	return c.client.BatchUpdateRoutes(ctx, req)
}