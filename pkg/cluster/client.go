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
	"time"

	"github.com/turtacn/emqx-go/pkg/chaos"
	clusterpb "github.com/turtacn/emqx-go/pkg/proto/cluster"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// Client provides a gRPC client for communicating with a specific peer node in
// the cluster. It wraps the gRPC connection and the generated gRPC client stub,
// offering methods for cluster-related operations like joining, updating routes,
// and forwarding messages.
type Client struct {
	// NodeID is the unique identifier of the local node.
	NodeID string
	conn   *grpc.ClientConn
	client clusterpb.ClusterServiceClient
}

// NewClient creates a new, un-connected cluster client.
//
// - nodeID: The unique identifier of the local node initiating the connection.
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

// Connect establishes a gRPC connection to a peer node at the given address.
// It uses a configurable connect function to allow for easier testing.
//
// - ctx: A context to control the connection timeout.
// - targetAddress: The network address of the peer's gRPC server.
//
// Returns an error if the connection fails.
func (c *Client) Connect(ctx context.Context, targetAddress string) error {
	return connectFunc(c, ctx, targetAddress)
}

// Close terminates the gRPC connection to the peer node.
func (c *Client) Close() {
	if c.conn != nil {
		c.conn.Close()
	}
}

// Join sends a JoinRequest to the connected peer to become part of the cluster.
//
// - ctx: A context for the gRPC call.
// - req: The join request message containing the local node's information.
//
// Returns the peer's response or an error.
func (c *Client) Join(ctx context.Context, req *clusterpb.JoinRequest) (*clusterpb.JoinResponse, error) {
	log.Printf("Sending Join request to peer")

	// Chaos injection: network delay
	chaos.ApplyNetworkDelay()

	// Chaos injection: packet loss
	if chaos.ShouldDropPacket() {
		log.Printf("[CHAOS] Simulating packet drop for Join request")
		time.Sleep(5 * time.Second) // Simulate timeout
		return nil, context.DeadlineExceeded
	}

	return c.client.Join(ctx, req)
}

// BatchUpdateRoutes sends a set of new routing entries to the peer.
//
// - ctx: A context for the gRPC call.
// - req: The request containing the routes to be added.
//
// Returns a response from the peer or an error.
func (c *Client) BatchUpdateRoutes(ctx context.Context, req *clusterpb.BatchUpdateRoutesRequest) (*clusterpb.BatchUpdateRoutesResponse, error) {
	// Chaos injection: network delay
	chaos.ApplyNetworkDelay()

	// Chaos injection: packet loss
	if chaos.ShouldDropPacket() {
		log.Printf("[CHAOS] Simulating packet drop for BatchUpdateRoutes")
		time.Sleep(2 * time.Second)
		return nil, context.DeadlineExceeded
	}

	return c.client.BatchUpdateRoutes(ctx, req)
}

// ForwardPublish sends a message to the peer to be published to its local
// subscribers.
//
// - ctx: A context for the gRPC call.
// - req: The message to be forwarded.
//
// Returns an acknowledgment from the peer or an error.
func (c *Client) ForwardPublish(ctx context.Context, req *clusterpb.PublishForward) (*clusterpb.ForwardAck, error) {
	// Chaos injection: network delay
	chaos.ApplyNetworkDelay()

	// Chaos injection: packet loss
	if chaos.ShouldDropPacket() {
		log.Printf("[CHAOS] Simulating packet drop for ForwardPublish")
		time.Sleep(1 * time.Second)
		return nil, context.DeadlineExceeded
	}

	return c.client.ForwardPublish(ctx, req)
}