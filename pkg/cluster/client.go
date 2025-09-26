package cluster

import (
	"context"
	"log"

	clusterpb "github.com/turtacn/emqx-go/pkg/proto/cluster"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// Client manages the connection to another node in the cluster.
type Client struct {
	NodeID string
	conn   *grpc.ClientConn
	client clusterpb.ClusterServiceClient
}

// NewClient creates a new cluster client.
func NewClient(nodeID string) *Client {
	return &Client{NodeID: nodeID}
}

// Connect establishes a gRPC connection to a peer node.
func (c *Client) Connect(ctx context.Context, targetAddress string) error {
	conn, err := grpc.DialContext(ctx, targetAddress, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithBlock())
	if err != nil {
		return err
	}
	c.conn = conn
	c.client = clusterpb.NewClusterServiceClient(conn)
	return nil
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