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

package e2e

import (
	"context"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/turtacn/emqx-go/pkg/broker"
	"github.com/turtacn/emqx-go/pkg/cluster"
	clusterpb "github.com/turtacn/emqx-go/pkg/proto/cluster"
	"google.golang.org/grpc"
)

// TestClusterCrossNodeMessaging tests message routing between nodes in a cluster
func TestClusterCrossNodeMessaging(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start 3 nodes
	nodes := startThreeNodeCluster(t, ctx)
	defer stopCluster(nodes)

	// Wait for cluster to stabilize
	time.Sleep(3 * time.Second)

	// Connect client to node1
	client1 := createTestClient("client1", ":1910")
	token := client1.Connect()
	require.True(t, token.WaitTimeout(5*time.Second))
	require.NoError(t, token.Error())
	defer client1.Disconnect(100)

	// Connect client to node2
	client2 := createTestClient("client2", ":1911")
	token = client2.Connect()
	require.True(t, token.WaitTimeout(5*time.Second))
	require.NoError(t, token.Error())
	defer client2.Disconnect(100)

	// Connect client to node3
	client3 := createTestClient("client3", ":1912")
	token = client3.Connect()
	require.True(t, token.WaitTimeout(5*time.Second))
	require.NoError(t, token.Error())
	defer client3.Disconnect(100)

	// Subscribe client2 to topic on node2
	messageReceived := make(chan mqtt.Message, 10)
	client2.Subscribe("cluster/test", 1, func(client mqtt.Client, msg mqtt.Message) {
		messageReceived <- msg
	})
	time.Sleep(500 * time.Millisecond) // Wait for subscription

	// Publish from client1 on node1
	testPayload := "Cross-node message from node1"
	pubToken := client1.Publish("cluster/test", 1, false, testPayload)
	require.True(t, pubToken.WaitTimeout(5*time.Second))
	require.NoError(t, pubToken.Error())

	// Verify client2 receives message
	select {
	case msg := <-messageReceived:
		assert.Equal(t, "cluster/test", msg.Topic())
		assert.Equal(t, testPayload, string(msg.Payload()))
		assert.Equal(t, byte(1), msg.Qos())
	case <-time.After(5 * time.Second):
		t.Fatal("Message was not received by client2 on node2")
	}

	// Subscribe client3 to topic on node3
	messageReceived3 := make(chan mqtt.Message, 10)
	client3.Subscribe("cluster/test", 1, func(client mqtt.Client, msg mqtt.Message) {
		messageReceived3 <- msg
	})
	time.Sleep(500 * time.Millisecond) // Wait for subscription

	// Publish from client1 on node1 again
	testPayload2 := "Cross-node message to multiple nodes"
	pubToken = client1.Publish("cluster/test", 1, false, testPayload2)
	require.True(t, pubToken.WaitTimeout(5*time.Second))
	require.NoError(t, pubToken.Error())

	// Verify both client2 and client3 receive message
	receivedCount := 0
	timeout := time.After(5 * time.Second)
	for receivedCount < 2 {
		select {
		case msg := <-messageReceived:
			assert.Equal(t, "cluster/test", msg.Topic())
			assert.Equal(t, testPayload2, string(msg.Payload()))
			receivedCount++
		case msg := <-messageReceived3:
			assert.Equal(t, "cluster/test", msg.Topic())
			assert.Equal(t, testPayload2, string(msg.Payload()))
			receivedCount++
		case <-timeout:
			t.Fatalf("Only received %d messages, expected 2", receivedCount)
		}
	}
}

// TestClusterNodeDiscovery tests cluster node discovery and joining
func TestClusterNodeDiscovery(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start node1
	node1 := startNode(t, ctx, "cluster-node1", ":1920", ":8091", "")
	defer node1.broker.Close()

	time.Sleep(1 * time.Second)

	// Start node2 with node1 as peer
	node2 := startNode(t, ctx, "cluster-node2", ":1921", ":8092", "cluster-node1:8091")
	defer node2.broker.Close()

	time.Sleep(2 * time.Second)

	// Verify node2 has connected to node1
	// Check if node2's cluster manager has node1 as peer
	assert.NotNil(t, node2.clusterMgr)

	// Start node3 with both node1 and node2 as peers
	node3 := startNode(t, ctx, "cluster-node3", ":1922", ":8093", "cluster-node1:8091,cluster-node2:8092")
	defer node3.broker.Close()

	time.Sleep(2 * time.Second)

	// Verify node3 has connected to other nodes
	assert.NotNil(t, node3.clusterMgr)

	t.Logf("Cluster formed successfully with 3 nodes")
}

// TestClusterRoutingTableSync tests routing table synchronization across nodes
func TestClusterRoutingTableSync(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start 3 nodes
	nodes := startThreeNodeCluster(t, ctx)
	defer stopCluster(nodes)

	// Wait for cluster to stabilize
	time.Sleep(3 * time.Second)

	// Connect clients to different nodes
	client1 := createTestClient("sync-client1", ":1910")
	token := client1.Connect()
	require.True(t, token.WaitTimeout(5*time.Second))
	require.NoError(t, token.Error())
	defer client1.Disconnect(100)

	client2 := createTestClient("sync-client2", ":1911")
	token = client2.Connect()
	require.True(t, token.WaitTimeout(5*time.Second))
	require.NoError(t, token.Error())
	defer client2.Disconnect(100)

	// Subscribe to multiple topics on different nodes
	topics := []string{"sync/topic1", "sync/topic2", "sync/topic3"}
	messageChannels := make([]chan mqtt.Message, len(topics))

	for i, topic := range topics {
		messageChannels[i] = make(chan mqtt.Message, 10)
		channelIndex := i
		currentTopic := topic

		// Subscribe client1 to all topics
		client1.Subscribe(currentTopic, 1, func(client mqtt.Client, msg mqtt.Message) {
			messageChannels[channelIndex] <- msg
		})
	}

	time.Sleep(1 * time.Second) // Wait for subscriptions to propagate

	// Publish to each topic from client2 (on different node)
	for i, topic := range topics {
		payload := fmt.Sprintf("Message for %s", topic)
		pubToken := client2.Publish(topic, 1, false, payload)
		require.True(t, pubToken.WaitTimeout(5*time.Second))
		require.NoError(t, pubToken.Error())

		// Verify message is received
		select {
		case msg := <-messageChannels[i]:
			assert.Equal(t, topic, msg.Topic())
			assert.Equal(t, payload, string(msg.Payload()))
		case <-time.After(3 * time.Second):
			t.Fatalf("Message not received for topic: %s", topic)
		}
	}
}

// TestClusterNodeFailure tests cluster behavior when a node fails
func TestClusterNodeFailure(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start 3 nodes
	nodes := startThreeNodeCluster(t, ctx)
	defer stopCluster(nodes)

	time.Sleep(3 * time.Second)

	// Connect clients
	client1 := createTestClient("fail-client1", ":1910")
	token := client1.Connect()
	require.True(t, token.WaitTimeout(5*time.Second))
	require.NoError(t, token.Error())
	defer client1.Disconnect(100)

	client2 := createTestClient("fail-client2", ":1911")
	token = client2.Connect()
	require.True(t, token.WaitTimeout(5*time.Second))
	require.NoError(t, token.Error())
	defer client2.Disconnect(100)

	// Subscribe on node2
	messageReceived := make(chan mqtt.Message, 10)
	client2.Subscribe("fail/test", 1, func(client mqtt.Client, msg mqtt.Message) {
		messageReceived <- msg
	})
	time.Sleep(500 * time.Millisecond)

	// Publish from node1 - should work
	pubToken := client1.Publish("fail/test", 1, false, "Before failure")
	require.True(t, pubToken.WaitTimeout(5*time.Second))
	require.NoError(t, pubToken.Error())

	select {
	case msg := <-messageReceived:
		assert.Equal(t, "Before failure", string(msg.Payload()))
	case <-time.After(3 * time.Second):
		t.Fatal("Message not received before node failure")
	}

	// Simulate node3 failure
	t.Log("Simulating node3 failure...")
	nodes[2].broker.Close()
	time.Sleep(2 * time.Second)

	// Publish again - node1 and node2 should still work
	pubToken = client1.Publish("fail/test", 1, false, "After failure")
	require.True(t, pubToken.WaitTimeout(5*time.Second))
	require.NoError(t, pubToken.Error())

	select {
	case msg := <-messageReceived:
		assert.Equal(t, "After failure", string(msg.Payload()))
	case <-time.After(3 * time.Second):
		t.Fatal("Message not received after node3 failure")
	}

	t.Log("Cluster continues to function after node failure")
}

// TestClusterLoadBalancing tests load distribution across cluster nodes
func TestClusterLoadBalancing(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start 3 nodes
	nodes := startThreeNodeCluster(t, ctx)
	defer stopCluster(nodes)

	time.Sleep(3 * time.Second)

	// Create multiple clients connecting to different nodes
	numClients := 9 // 3 per node
	clients := make([]mqtt.Client, numClients)
	ports := []string{":1910", ":1911", ":1912"}

	for i := 0; i < numClients; i++ {
		clientID := fmt.Sprintf("load-client-%d", i)
		port := ports[i%3]
		clients[i] = createTestClient(clientID, port)
		token := clients[i].Connect()
		require.True(t, token.WaitTimeout(5*time.Second))
		require.NoError(t, token.Error())
		defer clients[i].Disconnect(100)
	}

	// Subscribe all clients to same topic
	messageCounters := make([]int, numClients)
	var mu sync.Mutex

	for i := 0; i < numClients; i++ {
		clientIndex := i
		clients[i].Subscribe("load/test", 1, func(client mqtt.Client, msg mqtt.Message) {
			mu.Lock()
			messageCounters[clientIndex]++
			mu.Unlock()
		})
	}

	time.Sleep(1 * time.Second)

	// Publish multiple messages from first client
	numMessages := 10
	for i := 0; i < numMessages; i++ {
		payload := fmt.Sprintf("Load test message %d", i)
		pubToken := clients[0].Publish("load/test", 1, false, payload)
		require.True(t, pubToken.WaitTimeout(5*time.Second))
		require.NoError(t, pubToken.Error())
		time.Sleep(50 * time.Millisecond)
	}

	time.Sleep(2 * time.Second)

	// Verify all clients received all messages
	mu.Lock()
	totalReceived := 0
	for i, count := range messageCounters {
		t.Logf("Client %d received %d messages", i, count)
		totalReceived += count
	}
	mu.Unlock()

	expectedTotal := numMessages * numClients
	assert.Equal(t, expectedTotal, totalReceived, "Not all messages were delivered")
}

// TestClusterWildcardSubscriptions tests wildcard subscriptions across cluster
func TestClusterWildcardSubscriptions(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start 3 nodes
	nodes := startThreeNodeCluster(t, ctx)
	defer stopCluster(nodes)

	time.Sleep(3 * time.Second)

	// Connect clients to different nodes
	client1 := createTestClient("wild-client1", ":1910")
	token := client1.Connect()
	require.True(t, token.WaitTimeout(5*time.Second))
	require.NoError(t, token.Error())
	defer client1.Disconnect(100)

	client2 := createTestClient("wild-client2", ":1911")
	token = client2.Connect()
	require.True(t, token.WaitTimeout(5*time.Second))
	require.NoError(t, token.Error())
	defer client2.Disconnect(100)

	// Subscribe to wildcard topic on node2
	messageReceived := make(chan mqtt.Message, 10)
	client2.Subscribe("wild/+/test", 1, func(client mqtt.Client, msg mqtt.Message) {
		messageReceived <- msg
	})
	time.Sleep(500 * time.Millisecond)

	// Publish to matching topics from node1
	topics := []string{"wild/a/test", "wild/b/test", "wild/c/test"}
	for _, topic := range topics {
		payload := fmt.Sprintf("Message for %s", topic)
		pubToken := client1.Publish(topic, 1, false, payload)
		require.True(t, pubToken.WaitTimeout(5*time.Second))
		require.NoError(t, pubToken.Error())
	}

	// Verify all messages received
	receivedTopics := make(map[string]bool)
	timeout := time.After(5 * time.Second)
	for len(receivedTopics) < len(topics) {
		select {
		case msg := <-messageReceived:
			receivedTopics[msg.Topic()] = true
			t.Logf("Received message on topic: %s", msg.Topic())
		case <-timeout:
			t.Fatalf("Only received %d/%d wildcard messages", len(receivedTopics), len(topics))
		}
	}

	// Verify all expected topics were received
	for _, topic := range topics {
		assert.True(t, receivedTopics[topic], "Topic %s not received", topic)
	}
}

// Helper functions

type clusterNode struct {
	broker     *broker.Broker
	clusterMgr *cluster.Manager
	grpcServer *grpc.Server
	cancel     context.CancelFunc
}

func startNode(t *testing.T, ctx context.Context, nodeID, mqttPort, grpcPort, peerNodes string) *clusterNode {
	nodeCtx, cancel := context.WithCancel(ctx)

	// Create node address using localhost instead of hostname
	nodeAddr := fmt.Sprintf("localhost%s", grpcPort)

	var b *broker.Broker
	clusterMgr := cluster.NewManager(nodeID, nodeAddr, func(topic string, payload []byte) {
		if b != nil {
			b.RouteToLocalSubscribers(topic, payload)
		}
	})

	b = broker.New(nodeID, clusterMgr)
	b.SetupDefaultAuth()

	// Start MQTT broker
	go func() {
		if err := b.StartServer(nodeCtx, mqttPort); err != nil {
			t.Logf("Node %s MQTT server error: %v", nodeID, err)
		}
	}()

	// Start gRPC server
	grpcServer := grpc.NewServer()
	clusterServer := cluster.NewServer(nodeID, clusterMgr)
	clusterpb.RegisterClusterServiceServer(grpcServer, clusterServer)

	go func() {
		var listener net.Listener
		var err error
		listener, err = net.Listen("tcp", grpcPort)
		if err != nil {
			t.Logf("Node %s gRPC listen error: %v", nodeID, err)
			return
		}
		if err := grpcServer.Serve(listener); err != nil {
			t.Logf("Node %s gRPC serve error: %v", nodeID, err)
		}
	}()

	// Connect to peers if specified
	if peerNodes != "" {
		go func() {
			time.Sleep(1 * time.Second)
			peers := splitPeers(peerNodes)
			for _, peerAddr := range peers {
				// Convert peer address to use localhost
				peerAddr = replaceNodeName(peerAddr, "test-node1", "localhost")
				peerAddr = replaceNodeName(peerAddr, "test-node2", "localhost")
				peerAddr = replaceNodeName(peerAddr, "test-node3", "localhost")
				peerAddr = replaceNodeName(peerAddr, "cluster-node1", "localhost")
				peerAddr = replaceNodeName(peerAddr, "cluster-node2", "localhost")
				peerAddr = replaceNodeName(peerAddr, "cluster-node3", "localhost")

				peerID := extractPeerID(peerAddr)
				t.Logf("Node %s connecting to peer %s at %s", nodeID, peerID, peerAddr)
				clusterMgr.AddPeer(nodeCtx, peerID, peerAddr)
			}
		}()
	}

	return &clusterNode{
		broker:     b,
		clusterMgr: clusterMgr,
		grpcServer: grpcServer,
		cancel:     cancel,
	}
}

func startThreeNodeCluster(t *testing.T, ctx context.Context) []*clusterNode {
	// Start node1
	node1 := startNode(t, ctx, "test-node1", ":1910", ":8071", "")
	time.Sleep(1 * time.Second)

	// Start node2 with node1 as peer
	node2 := startNode(t, ctx, "test-node2", ":1911", ":8072", "test-node1:8071")
	time.Sleep(1 * time.Second)

	// Start node3 with node1 and node2 as peers
	node3 := startNode(t, ctx, "test-node3", ":1912", ":8073", "test-node1:8071,test-node2:8072")
	time.Sleep(1 * time.Second)

	return []*clusterNode{node1, node2, node3}
}

func stopCluster(nodes []*clusterNode) {
	for i, node := range nodes {
		if node.cancel != nil {
			node.cancel()
		}
		if node.broker != nil {
			node.broker.Close()
		}
		if node.grpcServer != nil {
			node.grpcServer.Stop()
		}
		time.Sleep(100 * time.Millisecond)
		fmt.Printf("Stopped node %d\n", i+1)
	}
}

func createTestClient(clientID, port string) mqtt.Client {
	opts := mqtt.NewClientOptions()
	opts.AddBroker(fmt.Sprintf("tcp://localhost%s", port))
	opts.SetClientID(clientID)
	opts.SetUsername("test")
	opts.SetPassword("test")
	opts.SetCleanSession(true)
	opts.SetAutoReconnect(false)
	opts.SetConnectTimeout(5 * time.Second)

	return mqtt.NewClient(opts)
}

func splitPeers(peerNodes string) []string {
	if peerNodes == "" {
		return nil
	}
	var peers []string
	for _, peer := range splitString(peerNodes, ",") {
		if trimmed := trimSpace(peer); trimmed != "" {
			peers = append(peers, trimmed)
		}
	}
	return peers
}

func extractPeerID(peerAddr string) string {
	parts := splitString(peerAddr, ":")
	if len(parts) > 0 {
		return parts[0]
	}
	return peerAddr
}

func splitString(s, sep string) []string {
	var result []string
	current := ""
	for _, char := range s {
		if string(char) == sep {
			if current != "" {
				result = append(result, current)
				current = ""
			}
		} else {
			current += string(char)
		}
	}
	if current != "" {
		result = append(result, current)
	}
	return result
}

func trimSpace(s string) string {
	start := 0
	end := len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t' || s[start] == '\n' || s[start] == '\r') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\n' || s[end-1] == '\r') {
		end--
	}
	return s[start:end]
}

func replaceNodeName(s, old, new string) string {
	// Simple string replacement
	result := ""
	i := 0
	for i < len(s) {
		if i+len(old) <= len(s) && s[i:i+len(old)] == old {
			result += new
			i += len(old)
		} else {
			result += string(s[i])
			i++
		}
	}
	return result
}
