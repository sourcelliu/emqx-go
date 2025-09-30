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
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	clusterpb "github.com/turtacn/emqx-go/pkg/proto/cluster"
)

func TestDefaultMriaConfig(t *testing.T) {
	config := DefaultMriaConfig("test-node", "localhost:8080")

	assert.Equal(t, "test-node", config.NodeID)
	assert.Equal(t, "localhost:8080", config.ListenAddress)
	assert.Equal(t, CoreNode, config.NodeRole)
	assert.Equal(t, 5*time.Second, config.HeartbeatInterval)
	assert.Equal(t, 10*time.Second, config.ElectionTimeout)
	assert.Equal(t, 100, config.MaxReplicantNodes)
	assert.True(t, config.AutoDiscoveryEnabled)
}

func TestNewMriaManager(t *testing.T) {
	config := DefaultMriaConfig("test-node", "localhost:8080")
	manager := NewMriaManager(config, nil)

	assert.NotNil(t, manager)
	assert.Equal(t, "test-node", manager.config.NodeID)
	assert.Equal(t, CoreNode, manager.config.NodeRole)
	assert.NotNil(t, manager.nodeInfo)
	assert.NotNil(t, manager.clusterState)
	assert.Equal(t, "test-node", manager.nodeInfo.NodeId)
	assert.Equal(t, "localhost:8080", manager.nodeInfo.Address)
}

func TestMriaManagerGetNodeRole(t *testing.T) {
	config := DefaultMriaConfig("test-node", "localhost:8080")
	config.NodeRole = ReplicantNode
	manager := NewMriaManager(config, nil)

	assert.Equal(t, ReplicantNode, manager.GetNodeRole())
}

func TestMriaManagerGetClusterState(t *testing.T) {
	config := DefaultMriaConfig("test-node", "localhost:8080")
	manager := NewMriaManager(config, nil)

	state := manager.GetClusterState()
	assert.NotNil(t, state)
	assert.NotEmpty(t, state.ClusterID)
	assert.Equal(t, "", state.Leader)
	assert.NotNil(t, state.CoreNodes)
	assert.NotNil(t, state.ReplicantNodes)
}

func TestMriaManagerStartStop(t *testing.T) {
	config := DefaultMriaConfig("test-node", "localhost:0")
	manager := NewMriaManager(config, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Test start
	err := manager.Start(ctx)
	assert.NoError(t, err)

	// Check that node is added to cluster state
	state := manager.GetClusterState()
	assert.Contains(t, state.CoreNodes, "test-node")

	// Test stop
	err = manager.Stop()
	assert.NoError(t, err)
}

func TestMriaManagerCoreNodes(t *testing.T) {
	config := DefaultMriaConfig("core-1", "localhost:8081")
	manager := NewMriaManager(config, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := manager.Start(ctx)
	require.NoError(t, err)
	defer manager.Stop()

	// Test GetCoreNodes
	coreNodes := manager.GetCoreNodes()
	assert.Len(t, coreNodes, 1)
	assert.Equal(t, "core-1", coreNodes[0].NodeId)

	// Test GetReplicantNodes (should be empty)
	replicantNodes := manager.GetReplicantNodes()
	assert.Len(t, replicantNodes, 0)
}

func TestMriaManagerReplicantNodes(t *testing.T) {
	config := DefaultMriaConfig("replicant-1", "localhost:8082")
	config.NodeRole = ReplicantNode
	manager := NewMriaManager(config, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := manager.Start(ctx)
	require.NoError(t, err)
	defer manager.Stop()

	// Test GetReplicantNodes
	replicantNodes := manager.GetReplicantNodes()
	assert.Len(t, replicantNodes, 1)
	assert.Equal(t, "replicant-1", replicantNodes[0].NodeId)

	// Test GetCoreNodes (should be empty)
	coreNodes := manager.GetCoreNodes()
	assert.Len(t, coreNodes, 0)
}

func TestMriaManagerLeaderElection(t *testing.T) {
	// Create two core nodes
	config1 := DefaultMriaConfig("core-1", "localhost:8083")
	manager1 := NewMriaManager(config1, nil)

	config2 := DefaultMriaConfig("core-2", "localhost:8084")
	manager2 := NewMriaManager(config2, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// Start both managers
	err := manager1.Start(ctx)
	require.NoError(t, err)
	defer manager1.Stop()

	err = manager2.Start(ctx)
	require.NoError(t, err)
	defer manager2.Stop()

	// Manually add nodes to each other's cluster state for testing
	manager1.clusterState.CoreNodes["core-2"] = manager2.nodeInfo
	manager2.clusterState.CoreNodes["core-1"] = manager1.nodeInfo

	// Trigger leader election
	manager1.performLeaderElection()
	manager2.performLeaderElection()

	// core-1 should be leader (lexicographically smaller)
	assert.True(t, manager1.IsLeader())
	assert.False(t, manager2.IsLeader())

	state1 := manager1.GetClusterState()
	state2 := manager2.GetClusterState()
	assert.Equal(t, "core-1", state1.Leader)
	assert.Equal(t, "core-1", state2.Leader)
}

func TestMriaManagerForwardMessageToCore(t *testing.T) {
	// Test replicant node forwarding to core
	config := DefaultMriaConfig("replicant-1", "localhost:8085")
	config.NodeRole = ReplicantNode
	manager := NewMriaManager(config, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := manager.Start(ctx)
	require.NoError(t, err)
	defer manager.Stop()

	// No core nodes - should return error
	err = manager.ForwardMessageToCore("test/topic", []byte("test message"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no core nodes available")

	// Test core node - should not forward
	coreConfig := DefaultMriaConfig("core-1", "localhost:8086")
	coreManager := NewMriaManager(coreConfig, nil)

	err = coreManager.Start(ctx)
	require.NoError(t, err)
	defer coreManager.Stop()

	err = coreManager.ForwardMessageToCore("test/topic", []byte("test message"))
	assert.NoError(t, err) // Core nodes don't forward to themselves
}

func TestMriaManagerReplicateToReplicants(t *testing.T) {
	// Test core node replication
	config := DefaultMriaConfig("core-1", "localhost:8087")
	manager := NewMriaManager(config, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := manager.Start(ctx)
	require.NoError(t, err)
	defer manager.Stop()

	// Test replication with no replicants
	err = manager.ReplicateToReplicants("test data")
	assert.NoError(t, err) // Should succeed but do nothing

	// Test replicant node - should return error
	replicantConfig := DefaultMriaConfig("replicant-1", "localhost:8088")
	replicantConfig.NodeRole = ReplicantNode
	replicantManager := NewMriaManager(replicantConfig, nil)

	err = replicantManager.ReplicateToReplicants("test data")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "only core nodes can replicate data")
}

func TestMriaManagerCallbacks(t *testing.T) {
	config := DefaultMriaConfig("test-node", "localhost:8089")
	manager := NewMriaManager(config, nil)

	// Test callback setup
	var nodeJoinCalled bool
	var nodeLeaveCalled bool
	var becomeLeaderCalled bool
	var loseLeadershipCalled bool

	manager.SetCallbacks(
		func(nodeInfo *clusterpb.NodeInfo) { nodeJoinCalled = true },
		func(nodeID string) { nodeLeaveCalled = true },
		func() { becomeLeaderCalled = true },
		func() { loseLeadershipCalled = true },
	)

	assert.NotNil(t, manager.onNodeJoin)
	assert.NotNil(t, manager.onNodeLeave)
	assert.NotNil(t, manager.onBecomeLeader)
	assert.NotNil(t, manager.onLoseLeadership)

	// Test callback invocation
	if manager.onNodeJoin != nil {
		manager.onNodeJoin(&clusterpb.NodeInfo{NodeId: "test"})
	}
	if manager.onNodeLeave != nil {
		manager.onNodeLeave("test")
	}
	if manager.onBecomeLeader != nil {
		manager.onBecomeLeader()
	}
	if manager.onLoseLeadership != nil {
		manager.onLoseLeadership()
	}

	assert.True(t, nodeJoinCalled)
	assert.True(t, nodeLeaveCalled)
	assert.True(t, becomeLeaderCalled)
	assert.True(t, loseLeadershipCalled)
}

func TestClusterStateCreation(t *testing.T) {
	config := DefaultMriaConfig("test-node", "localhost:8090")
	manager := NewMriaManager(config, nil)

	state := manager.clusterState
	assert.NotNil(t, state)
	assert.NotEmpty(t, state.ClusterID)
	assert.Equal(t, "", state.Leader)
	assert.NotNil(t, state.CoreNodes)
	assert.NotNil(t, state.ReplicantNodes)
	assert.Equal(t, 0, len(state.CoreNodes))
	assert.Equal(t, 0, len(state.ReplicantNodes))
}

func TestNodeInfoCreation(t *testing.T) {
	config := DefaultMriaConfig("test-node", "localhost:8091")
	manager := NewMriaManager(config, nil)

	nodeInfo := manager.nodeInfo
	assert.NotNil(t, nodeInfo)
	assert.Equal(t, "test-node", nodeInfo.NodeId)
	assert.Equal(t, "localhost:8091", nodeInfo.Address)
	assert.Equal(t, "1.0.0-mria", nodeInfo.Version)
	assert.Equal(t, int32(NodeStatusStarting), nodeInfo.Status)
	assert.Greater(t, nodeInfo.LastSeen, int64(0))
}

func TestMriaManagerConcurrency(t *testing.T) {
	config := DefaultMriaConfig("test-node", "localhost:8092")
	manager := NewMriaManager(config, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	err := manager.Start(ctx)
	require.NoError(t, err)
	defer manager.Stop()

	// Test concurrent access to cluster state
	done := make(chan bool, 2)

	go func() {
		for i := 0; i < 100; i++ {
			state := manager.GetClusterState()
			assert.NotNil(t, state)
			time.Sleep(time.Millisecond)
		}
		done <- true
	}()

	go func() {
		for i := 0; i < 100; i++ {
			nodes := manager.GetCoreNodes()
			assert.NotNil(t, nodes)
			time.Sleep(time.Millisecond)
		}
		done <- true
	}()

	// Wait for both goroutines to complete
	<-done
	<-done
}

func TestNodeRoleValidation(t *testing.T) {
	tests := []struct {
		name string
		role NodeRole
		valid bool
	}{
		{"Core node", CoreNode, true},
		{"Replicant node", ReplicantNode, true},
		{"Invalid role", NodeRole("invalid"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := DefaultMriaConfig("test-node", "localhost:8093")
			config.NodeRole = tt.role
			manager := NewMriaManager(config, nil)

			assert.Equal(t, tt.role, manager.GetNodeRole())

			if tt.valid {
				assert.Contains(t, []NodeRole{CoreNode, ReplicantNode}, manager.GetNodeRole())
			}
		})
	}
}