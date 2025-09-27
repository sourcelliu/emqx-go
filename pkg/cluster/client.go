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

// Client manages the gRPC connection to another node in the cluster.
type Client struct {
	NodeID string
	conn   *grpc.ClientConn
	client clusterpb.ClusterServiceClient
}

// NewClient creates a new cluster client.
func NewClient(nodeID string) *Client {
	return &Client{NodeID: nodeID}
}

// client.go

// connectFunc is a package-level variable that can be replaced in tests
// to allow for mocking of the gRPC connection.
var connectFunc = func(c *Client, ctx context.Context, targetAddress string) error {
	// In a real-world scenario, you would use secure credentials.
	opts := []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	}
	conn, err := grpc.DialContext(ctx, targetAddress, opts...)
	if err != nil {
		return err
	}
	c.conn = conn
	c.client = clusterpb.NewClusterServiceClient(conn)
	log.Printf("Successfully connected to peer at %s", targetAddress)
	return nil
}

// Connect establishes a gRPC connection to a peer node.
func (c *Client) Connect(ctx context.Context, targetAddress string) error {
	return connectFunc(c, ctx, targetAddress)
}

// Close disconnects the client.
func (c *Client) Close() {
	if c.conn != nil {
		c.conn.Close()
	}
}

// Join sends a request to join the cluster.
func (c *Client) Join(ctx context.Context, req *clusterpb.JoinRequest) (*clusterpb.JoinResponse, error) {
	log.Printf("Sending Join request to peer")
	return c.client.Join(ctx, req)
}

// BatchUpdateRoutes sends a batch of route updates to a peer.
func (c *Client) BatchUpdateRoutes(ctx context.Context, req *clusterpb.BatchUpdateRoutesRequest) (*clusterpb.BatchUpdateRoutesResponse, error) {
	return c.client.BatchUpdateRoutes(ctx, req)
}

// ForwardPublish sends a publish message to a peer.
func (c *Client) ForwardPublish(ctx context.Context, req *clusterpb.PublishForward) (*clusterpb.ForwardAck, error) {
	return c.client.ForwardPublish(ctx, req)
}