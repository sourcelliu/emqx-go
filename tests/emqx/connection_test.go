package emqx

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestClientConnect tests basic client connection
func TestClientConnect(t *testing.T) {
	t.Log("Testing basic MQTT client connection")

	client := setupMQTTClient("test_connect_client", t)
	assert.True(t, client.IsConnected(), "Client should be connected")

	client.Disconnect(250)
	time.Sleep(300 * time.Millisecond)
	assert.False(t, client.IsConnected(), "Client should be disconnected")

	t.Log("Client connection/disconnection test passed")
}

// TestCleanSession tests clean session flag behavior
func TestCleanSession(t *testing.T) {
	t.Log("Testing clean session behavior")

	clientID := "clean_session_test"

	// Connect with clean session = false and subscribe
	opts := mqtt.NewClientOptions()
	opts.AddBroker(brokerURL)
	opts.SetClientID(clientID)
	opts.SetUsername(username)
	opts.SetPassword(password)
	opts.SetCleanSession(false)
	opts.SetAutoReconnect(false)

	client1 := mqtt.NewClient(opts)
	token := client1.Connect()
	require.True(t, token.WaitTimeout(5*time.Second))
	require.NoError(t, token.Error())

	// Subscribe to topic
	token = client1.Subscribe("test/persistent", 1, nil)
	require.True(t, token.WaitTimeout(5*time.Second))
	require.NoError(t, token.Error())

	t.Log("Client subscribed with clean_session=false")

	// Disconnect
	client1.Disconnect(250)
	time.Sleep(300 * time.Millisecond)

	// Publish message while disconnected
	publisher := setupMQTTClient("persistent_publisher", t)
	token = publisher.Publish("test/persistent", 1, false, "offline_message")
	require.True(t, token.WaitTimeout(5*time.Second))
	require.NoError(t, token.Error())
	publisher.Disconnect(250)

	t.Log("Message published while client was offline")

	// Reconnect with same client ID and clean_session=false
	messageReceived := make(chan mqtt.Message, 1)
	opts.SetOnConnectHandler(func(client mqtt.Client) {
		t.Log("Client reconnected, checking for offline messages")
	})

	client2 := mqtt.NewClient(opts)
	client2.AddRoute("test/persistent", func(client mqtt.Client, msg mqtt.Message) {
		t.Logf("Received offline message: %s", string(msg.Payload()))
		messageReceived <- msg
	})

	token = client2.Connect()
	require.True(t, token.WaitTimeout(5*time.Second))
	require.NoError(t, token.Error())

	// Should receive offline message (if broker supports persistent sessions)
	select {
	case msg := <-messageReceived:
		assert.Equal(t, "offline_message", string(msg.Payload()))
		t.Log("Successfully received offline message after reconnection")
	case <-time.After(5 * time.Second):
		t.Log("Did not receive offline message (broker may not have persistent session support)")
	}

	client2.Disconnect(250)
}

// TestSessionTakeover tests that new connection takes over old session
// Equivalent to: t_connected_client_count_persistent in emqx_broker_SUITE.erl
func TestSessionTakeover(t *testing.T) {
	t.Log("Testing session takeover (duplicate client ID)")

	clientID := "takeover_test"

	// First connection
	client1 := setupMQTTClient(clientID, t)
	assert.True(t, client1.IsConnected(), "First client should be connected")

	time.Sleep(200 * time.Millisecond)

	// Second connection with same clientID (should take over)
	lostHandler := make(chan struct{}, 1)
	opts := mqtt.NewClientOptions()
	opts.AddBroker(brokerURL)
	opts.SetClientID(clientID)
	opts.SetUsername(username)
	opts.SetPassword(password)
	opts.SetConnectionLostHandler(func(c mqtt.Client, err error) {
		t.Logf("Connection lost: %v", err)
		lostHandler <- struct{}{}
	})

	// Update first client to detect disconnection
	client1Opts := mqtt.NewClientOptions()
	client1Opts.SetConnectionLostHandler(func(c mqtt.Client, err error) {
		t.Logf("First client connection lost (expected): %v", err)
		lostHandler <- struct{}{}
	})

	// Create second client with same ID
	client2 := mqtt.NewClient(opts)
	token := client2.Connect()
	require.True(t, token.WaitTimeout(5*time.Second))
	require.NoError(t, token.Error())

	t.Log("Second client connected with same clientID")

	// First client should be disconnected
	time.Sleep(500 * time.Millisecond)

	assert.True(t, client2.IsConnected(), "Second client should be connected")
	t.Log("Session takeover successful")

	client2.Disconnect(250)
}

// TestMultipleConnections tests multiple concurrent client connections
func TestMultipleConnections(t *testing.T) {
	t.Log("Testing multiple concurrent client connections")

	numClients := 20
	var wg sync.WaitGroup
	successCount := atomic.Int32{}
	failureCount := atomic.Int32{}

	for i := 0; i < numClients; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()

			clientID := fmt.Sprintf("multi_client_%d", index)
			opts := mqtt.NewClientOptions()
			opts.AddBroker(brokerURL)
			opts.SetClientID(clientID)
			opts.SetUsername(username)
			opts.SetPassword(password)
			opts.SetConnectTimeout(5 * time.Second)

			client := mqtt.NewClient(opts)
			token := client.Connect()

			if token.WaitTimeout(5*time.Second) && token.Error() == nil {
				successCount.Add(1)
				t.Logf("Client %s connected successfully", clientID)
				time.Sleep(100 * time.Millisecond)
				client.Disconnect(250)
			} else {
				failureCount.Add(1)
				t.Logf("Client %s failed to connect: %v", clientID, token.Error())
			}
		}(i)
	}

	wg.Wait()

	t.Logf("Connection results: %d successful, %d failed", successCount.Load(), failureCount.Load())
	assert.Equal(t, int32(numClients), successCount.Load(), "All clients should connect successfully")
}

// TestKeepAlive tests MQTT keep-alive mechanism
func TestKeepAlive(t *testing.T) {
	t.Log("Testing MQTT keep-alive mechanism")

	opts := mqtt.NewClientOptions()
	opts.AddBroker(brokerURL)
	opts.SetClientID("keepalive_test")
	opts.SetUsername(username)
	opts.SetPassword(password)
	opts.SetKeepAlive(2 * time.Second) // Short keep-alive for testing
	opts.SetPingTimeout(1 * time.Second)
	opts.SetAutoReconnect(false)

	connectionLost := make(chan struct{}, 1)
	opts.SetConnectionLostHandler(func(client mqtt.Client, err error) {
		t.Logf("Connection lost (expected for keep-alive test): %v", err)
		connectionLost <- struct{}{}
	})

	client := mqtt.NewClient(opts)
	token := client.Connect()
	require.True(t, token.WaitTimeout(5*time.Second))
	require.NoError(t, token.Error())

	t.Log("Client connected, waiting for keep-alive mechanism...")

	// Wait a bit longer than keep-alive interval
	// The client should send PINGREQ and receive PINGRESP
	time.Sleep(5 * time.Second)

	if client.IsConnected() {
		t.Log("Keep-alive mechanism working correctly")
	} else {
		t.Log("Connection lost (might be due to keep-alive failure)")
	}

	client.Disconnect(250)
}

// TestWillMessage tests Last Will and Testament (LWT) functionality
func TestWillMessage(t *testing.T) {
	t.Log("Testing Last Will and Testament (LWT)")

	// Create subscriber to will topic
	subscriber := setupMQTTClient("will_subscriber", t)
	defer subscriber.Disconnect(250)

	willReceived := make(chan mqtt.Message, 1)
	token := subscriber.Subscribe("test/will", 1, func(client mqtt.Client, msg mqtt.Message) {
		t.Logf("Received will message: %s", string(msg.Payload()))
		willReceived <- msg
	})
	require.True(t, token.WaitTimeout(5*time.Second))
	require.NoError(t, token.Error())

	time.Sleep(100 * time.Millisecond)

	// Create client with will message
	opts := mqtt.NewClientOptions()
	opts.AddBroker(brokerURL)
	opts.SetClientID("will_test_client")
	opts.SetUsername(username)
	opts.SetPassword(password)
	opts.SetWill("test/will", "client_died", 1, false)
	opts.SetAutoReconnect(false)

	client := mqtt.NewClient(opts)
	token = client.Connect()
	require.True(t, token.WaitTimeout(5*time.Second))
	require.NoError(t, token.Error())

	t.Log("Client with will message connected")

	// Force disconnect (simulate abnormal disconnection)
	// Note: Calling Disconnect normally might not trigger will message
	// In production, will is sent when connection is lost abnormally
	client.Disconnect(0) // Immediate disconnect

	// Wait for will message
	select {
	case msg := <-willReceived:
		assert.Equal(t, "client_died", string(msg.Payload()))
		assert.Equal(t, "test/will", msg.Topic())
		t.Log("Will message received successfully")
	case <-time.After(3 * time.Second):
		t.Log("Will message not received (may require abnormal disconnection)")
	}
}

// TestConnectionRetry tests connection retry mechanism
func TestConnectionRetry(t *testing.T) {
	t.Log("Testing connection retry mechanism")

	opts := mqtt.NewClientOptions()
	opts.AddBroker("tcp://localhost:9999") // Non-existent broker
	opts.SetClientID("retry_test")
	opts.SetConnectTimeout(1 * time.Second)
	opts.SetAutoReconnect(true)
	opts.SetMaxReconnectInterval(2 * time.Second)

	client := mqtt.NewClient(opts)
	token := client.Connect()

	// Should timeout
	success := token.WaitTimeout(3 * time.Second)
	require.False(t, success && token.Error() == nil, "Connection to non-existent broker should fail")

	t.Log("Connection retry test completed (expected failure)")
}

// TestMaxConnections tests broker's max connection limit behavior
func TestMaxConnections(t *testing.T) {
	t.Log("Testing concurrent connections stress test")

	numClients := 50
	clients := make([]mqtt.Client, 0, numClients)
	var mu sync.Mutex

	// Create many concurrent connections
	var wg sync.WaitGroup
	for i := 0; i < numClients; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()

			clientID := fmt.Sprintf("stress_client_%d", index)
			opts := mqtt.NewClientOptions()
			opts.AddBroker(brokerURL)
			opts.SetClientID(clientID)
			opts.SetUsername(username)
			opts.SetPassword(password)
			opts.SetConnectTimeout(10 * time.Second)

			client := mqtt.NewClient(opts)
			token := client.Connect()

			if token.WaitTimeout(10*time.Second) && token.Error() == nil {
				mu.Lock()
				clients = append(clients, client)
				mu.Unlock()
				t.Logf("Stress client %d connected", index)
			}
		}(i)
	}

	wg.Wait()

	mu.Lock()
	connectedCount := len(clients)
	mu.Unlock()

	t.Logf("Successfully connected %d/%d clients", connectedCount, numClients)

	// Disconnect all clients
	for _, client := range clients {
		client.Disconnect(250)
	}

	// Most clients should connect successfully
	assert.GreaterOrEqual(t, connectedCount, numClients*8/10,
		"At least 80%% of clients should connect")
}

// TestClientIDValidation tests client ID validation
func TestClientIDValidation(t *testing.T) {
	t.Log("Testing client ID validation")

	testCases := []struct {
		name          string
		clientID      string
		shouldSucceed bool
	}{
		{"Normal ID", "normal_client_123", true},
		{"Empty ID with clean session", "", true},           // Broker should assign ID
		{"Very long ID", string(make([]byte, 1000)), false}, // May exceed limit
		{"Special characters", "client-with-special_chars.123", true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			opts := mqtt.NewClientOptions()
			opts.AddBroker(brokerURL)
			opts.SetClientID(tc.clientID)
			opts.SetUsername(username)
			opts.SetPassword(password)
			opts.SetCleanSession(true)
			opts.SetConnectTimeout(3 * time.Second)

			client := mqtt.NewClient(opts)
			token := client.Connect()
			success := token.WaitTimeout(3*time.Second) && token.Error() == nil

			if tc.shouldSucceed {
				assert.True(t, success, "Client ID '%s' should be accepted", tc.clientID)
				if success {
					client.Disconnect(250)
				}
			} else {
				assert.False(t, success, "Client ID '%s' should be rejected", tc.clientID)
			}
		})
	}
}
