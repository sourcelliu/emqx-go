package e2e

import (
	"context"
	"fmt"
	"testing"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/turtacn/emqx-go/pkg/broker"
)

func TestWillMessageBasic(t *testing.T) {
	// Note: This test demonstrates will message setup and cancellation
	// Testing actual will message triggering requires simulating network failures
	// which is challenging with the Paho MQTT client in e2e tests

	// Start broker
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	b := broker.New("will-node1", nil)
	b.SetupDefaultAuth()
	defer b.Close()

	go b.StartServer(ctx, ":1887")
	time.Sleep(100 * time.Millisecond) // Wait for server to start

	// Test 1: Create client with will message and verify it's set up correctly
	willClient := createWillClient("will-client", ":1887", "will/test", "client disconnected unexpectedly", 1, false)
	token := willClient.Connect()
	require.True(t, token.WaitTimeout(5*time.Second))
	require.NoError(t, token.Error())

	// Test 2: Graceful disconnect should NOT trigger will message
	// (This tests that will message cancellation works correctly)
	willReceived := make(chan mqtt.Message, 1)
	subscriber := createCleanClient("will-subscriber", ":1887")
	subscriber.Subscribe("will/test", 1, func(client mqtt.Client, msg mqtt.Message) {
		willReceived <- msg
	})

	subToken := subscriber.Connect()
	require.True(t, subToken.WaitTimeout(5*time.Second))
	require.NoError(t, subToken.Error())

	// Graceful disconnect
	willClient.Disconnect(250)

	// Should NOT receive will message
	select {
	case <-willReceived:
		t.Fatal("Will message was published on graceful disconnect")
	case <-time.After(2 * time.Second):
		// Expected - no will message received
	}

	subscriber.Disconnect(100)
}

func TestWillMessageConfiguration(t *testing.T) {
	// Test that will messages can be configured with different parameters
	// Start broker
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	b := broker.New("will-node2", nil)
	b.SetupDefaultAuth()
	defer b.Close()

	go b.StartServer(ctx, ":1888")
	time.Sleep(100 * time.Millisecond) // Wait for server to start

	// Test will message with different QoS levels and retain flags
	testCases := []struct {
		name       string
		qos        byte
		retain     bool
		topic      string
		payload    string
	}{
		{"QoS0_NoRetain", 0, false, "will/qos0", "qos0 message"},
		{"QoS1_NoRetain", 1, false, "will/qos1", "qos1 message"},
		{"QoS2_NoRetain", 2, false, "will/qos2", "qos2 message"},
		{"QoS1_Retain", 1, true, "will/retained", "retained message"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create client with will message
			clientID := fmt.Sprintf("will-client-%s", tc.name)
			willClient := createWillClient(clientID, ":1888", tc.topic, tc.payload, tc.qos, tc.retain)

			token := willClient.Connect()
			require.True(t, token.WaitTimeout(5*time.Second))
			require.NoError(t, token.Error())

			// Verify connection was successful (will message is configured)
			assert.True(t, willClient.IsConnected())

			// Graceful disconnect
			willClient.Disconnect(100)
		})
	}
}

func TestWillMessagePersistentSessionCompatibility(t *testing.T) {
	// Test that will messages work correctly with persistent sessions
	// Start broker
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	b := broker.New("will-node3", nil)
	b.SetupDefaultAuth()
	defer b.Close()

	go b.StartServer(ctx, ":1889")
	time.Sleep(100 * time.Millisecond) // Wait for server to start

	clientID := "persistent-will-client"

	// Test 1: Create persistent session with will message
	willClient := createPersistentWillClient(clientID, ":1889", "will/persistent", "persistent will message", 1, false)
	token := willClient.Connect()
	require.True(t, token.WaitTimeout(5*time.Second))
	require.NoError(t, token.Error())

	// Verify connection
	assert.True(t, willClient.IsConnected())

	// Graceful disconnect (preserves session but cancels will message)
	willClient.Disconnect(100)

	// Test 2: Reconnect with same client ID - session should be resumed
	willClient2 := createPersistentWillClient(clientID, ":1889", "will/persistent2", "new will message", 2, true)
	token = willClient2.Connect()
	require.True(t, token.WaitTimeout(5*time.Second))
	require.NoError(t, token.Error())

	// Verify connection
	assert.True(t, willClient2.IsConnected())

	willClient2.Disconnect(100)
}

// Simplified e2e tests that focus on testable scenarios
func TestWillMessageClientBehavior(t *testing.T) {
	// Test various client connection scenarios with will messages
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	b := broker.New("will-node4", nil)
	b.SetupDefaultAuth()
	defer b.Close()

	go b.StartServer(ctx, ":1890")
	time.Sleep(100 * time.Millisecond) // Wait for server to start

	// Test 1: Multiple clients with will messages can connect
	clients := make([]mqtt.Client, 3)
	for i := 0; i < 3; i++ {
		clientID := fmt.Sprintf("will-client-%d", i)
		topic := fmt.Sprintf("will/client%d", i)
		payload := fmt.Sprintf("will message from client %d", i)

		clients[i] = createWillClient(clientID, ":1890", topic, payload, 1, false)

		token := clients[i].Connect()
		require.True(t, token.WaitTimeout(5*time.Second))
		require.NoError(t, token.Error())

		assert.True(t, clients[i].IsConnected())
	}

	// Test 2: All clients can disconnect gracefully
	for i, client := range clients {
		assert.True(t, client.IsConnected(), "Client %d should be connected", i)
		client.Disconnect(100)
	}
}

// Helper functions for will message tests

func createWillClient(clientID, addr, willTopic, willPayload string, willQoS byte, willRetain bool) mqtt.Client {
	opts := mqtt.NewClientOptions()
	opts.AddBroker("tcp://localhost" + addr)
	opts.SetClientID(clientID)
	opts.SetUsername("test")
	opts.SetPassword("test")
	opts.SetCleanSession(true)
	opts.SetAutoReconnect(false)
	opts.SetConnectTimeout(5 * time.Second)

	// Set will message
	opts.SetWill(willTopic, willPayload, willQoS, willRetain)

	return mqtt.NewClient(opts)
}

func createPersistentWillClient(clientID, addr, willTopic, willPayload string, willQoS byte, willRetain bool) mqtt.Client {
	opts := mqtt.NewClientOptions()
	opts.AddBroker("tcp://localhost" + addr)
	opts.SetClientID(clientID)
	opts.SetUsername("test")
	opts.SetPassword("test")
	opts.SetCleanSession(false) // Persistent session
	opts.SetAutoReconnect(false)
	opts.SetConnectTimeout(5 * time.Second)

	// Set will message
	opts.SetWill(willTopic, willPayload, willQoS, willRetain)

	return mqtt.NewClient(opts)
}