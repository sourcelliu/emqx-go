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
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	clusterpb "github.com/turtacn/emqx-go/pkg/proto/cluster"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

func TestNewManager(t *testing.T) {
	m := NewManager("test-node", "localhost:8081", nil)
	assert.NotNil(t, m)
	assert.Equal(t, "test-node", m.NodeID)
	assert.Equal(t, "localhost:8081", m.NodeAddress)
	assert.NotNil(t, m.peers)
	assert.NotNil(t, m.remoteRoutes)
	assert.Nil(t, m.LocalPublishFunc)
}

// mockClusterServer is a mock implementation of the ClusterServiceServer for testing.
type mockClusterServer struct {
	clusterpb.UnimplementedClusterServiceServer
	JoinFunc           func(ctx context.Context, req *clusterpb.JoinRequest) (*clusterpb.JoinResponse, error)
	BatchUpdateFunc    func(ctx context.Context, req *clusterpb.BatchUpdateRoutesRequest) (*clusterpb.BatchUpdateRoutesResponse, error)
	ForwardPublishFunc func(ctx context.Context, req *clusterpb.PublishForward) (*clusterpb.ForwardAck, error)
}

func (m *mockClusterServer) Join(ctx context.Context, req *clusterpb.JoinRequest) (*clusterpb.JoinResponse, error) {
	if m.JoinFunc != nil {
		return m.JoinFunc(ctx, req)
	}
	return &clusterpb.JoinResponse{Success: true, ClusterId: "test-cluster"}, nil
}

func (m *mockClusterServer) BatchUpdateRoutes(ctx context.Context, req *clusterpb.BatchUpdateRoutesRequest) (*clusterpb.BatchUpdateRoutesResponse, error) {
	if m.BatchUpdateFunc != nil {
		return m.BatchUpdateFunc(ctx, req)
	}
	return &clusterpb.BatchUpdateRoutesResponse{Success: true}, nil
}

func (m *mockClusterServer) ForwardPublish(ctx context.Context, req *clusterpb.PublishForward) (*clusterpb.ForwardAck, error) {
	if m.ForwardPublishFunc != nil {
		return m.ForwardPublishFunc(ctx, req)
	}
	return &clusterpb.ForwardAck{Success: true}, nil
}

// setupTestManager creates a Manager and a mock gRPC server for integration tests.
func setupTestManager(t *testing.T, mockSrv *mockClusterServer) (*Manager, *bufconn.Listener) {
	lis := bufconn.Listen(1024 * 1024)
	s := grpc.NewServer()
	clusterpb.RegisterClusterServiceServer(s, mockSrv)
	go func() {
		if err := s.Serve(lis); err != nil {
			t.Logf("mock server stopped: %v", err)
		}
	}()
	t.Cleanup(func() { s.Stop() })

	manager := NewManager("node-1", "localhost:8081", nil)

	dialer := func(context.Context, string) (net.Conn, error) {
		return lis.Dial()
	}

	originalConnect := connectFunc
	connectFunc = func(c *Client, ctx context.Context, address string) error {
		conn, err := grpc.DialContext(ctx, address,
			grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpc.WithContextDialer(dialer),
		)
		if err != nil {
			return err
		}
		c.conn = conn
		c.client = clusterpb.NewClusterServiceClient(conn)
		return nil
	}
	t.Cleanup(func() { connectFunc = originalConnect })

	return manager, lis
}

func TestManager_AddPeer(t *testing.T) {
	mockSrv := &mockClusterServer{}
	manager, _ := setupTestManager(t, mockSrv)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	manager.AddPeer(ctx, "peer-1", "bufnet")

	manager.mu.RLock()
	defer manager.mu.RUnlock()
	assert.Contains(t, manager.peers, "peer-1")
	require.NotNil(t, manager.peers["peer-1"])
	assert.NotNil(t, manager.peers["peer-1"].conn)
}

func TestManager_BroadcastRouteUpdate(t *testing.T) {
	updateReceived := make(chan *clusterpb.BatchUpdateRoutesRequest, 1)
	mockSrv := &mockClusterServer{
		BatchUpdateFunc: func(ctx context.Context, req *clusterpb.BatchUpdateRoutesRequest) (*clusterpb.BatchUpdateRoutesResponse, error) {
			updateReceived <- req
			return &clusterpb.BatchUpdateRoutesResponse{Success: true}, nil
		},
	}
	manager, _ := setupTestManager(t, mockSrv)
	manager.AddPeer(context.Background(), "peer-1", "bufnet")

	routes := []*clusterpb.Route{{Topic: "test/topic", NodeIds: []string{"node-1"}}}
	manager.BroadcastRouteUpdate(routes)

	select {
	case req := <-updateReceived:
		assert.Equal(t, "node-1", req.FromNode)
		assert.Equal(t, "add", req.OpType)
		assert.Len(t, req.Routes, 1)
		assert.Equal(t, "test/topic", req.Routes[0].Topic)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for route update")
	}
}

func TestManager_ForwardPublish(t *testing.T) {
	publishReceived := make(chan *clusterpb.PublishForward, 1)
	mockSrv := &mockClusterServer{
		ForwardPublishFunc: func(ctx context.Context, req *clusterpb.PublishForward) (*clusterpb.ForwardAck, error) {
			publishReceived <- req
			return &clusterpb.ForwardAck{Success: true}, nil
		},
	}
	manager, _ := setupTestManager(t, mockSrv)
	manager.AddPeer(context.Background(), "peer-1", "bufnet")

	manager.ForwardPublish("test/topic", []byte("hello"), "peer-1")

	select {
	case req := <-publishReceived:
		assert.Equal(t, "node-1", req.FromNode)
		assert.Equal(t, "test/topic", req.Topic)
		assert.Equal(t, []byte("hello"), req.Payload)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for forwarded publish")
	}
}

func TestManager_AddRemoteRoute(t *testing.T) {
	manager := NewManager("node-1", "localhost:8081", nil)
	manager.AddRemoteRoute("test/topic", "node-2")

	routes := manager.GetRemoteSubscribers("test/topic")
	assert.Equal(t, []string{"node-2"}, routes)

	// Test adding the same route again (should not duplicate)
	manager.AddRemoteRoute("test/topic", "node-2")
	routes = manager.GetRemoteSubscribers("test/topic")
	assert.Equal(t, []string{"node-2"}, routes)
}