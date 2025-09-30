package e2e

import (
	"context"
	"testing"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/turtacn/emqx-go/pkg/broker"
)

func TestPersistentSessionBasic(t *testing.T) {
	// Start broker
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	b := broker.New("node1", nil)
	b.SetupDefaultAuth()
	defer b.Close()

	go b.StartServer(ctx, ":1884")
	time.Sleep(100 * time.Millisecond) // Wait for server to start

	// Test 1: Create persistent session with subscription
	client1 := createPersistentClient("persistent-client-1", ":1884")
	token := client1.Connect()
	require.True(t, token.WaitTimeout(5*time.Second))
	require.NoError(t, token.Error())

	// Subscribe to a topic
	subToken := client1.Subscribe("test/persistent", 1, nil)
	require.True(t, subToken.WaitTimeout(5*time.Second))
	require.NoError(t, subToken.Error())

	// Disconnect (should preserve session)
	client1.Disconnect(100)

	// Test 2: Publish message while client is offline
	publisher := createCleanClient("publisher", ":1884")
	token = publisher.Connect()
	require.True(t, token.WaitTimeout(5*time.Second))
	require.NoError(t, token.Error())

	pubToken := publisher.Publish("test/persistent", 1, false, "offline message")
	require.True(t, pubToken.WaitTimeout(5*time.Second))
	require.NoError(t, pubToken.Error())

	publisher.Disconnect(100)

	// Test 3: Reconnect and receive offline message
	messageReceived := make(chan mqtt.Message, 1)
	client2 := createPersistentClient("persistent-client-1", ":1884")

	// Set up message handler BEFORE connecting
	client2.Subscribe("test/persistent", 1, func(client mqtt.Client, msg mqtt.Message) {
		messageReceived <- msg
	})

	token = client2.Connect()
	require.True(t, token.WaitTimeout(5*time.Second))
	require.NoError(t, token.Error())

	// Should receive the offline message automatically upon connection
	select {
	case msg := <-messageReceived:
		assert.Equal(t, "test/persistent", msg.Topic())
		assert.Equal(t, "offline message", string(msg.Payload()))
	case <-time.After(3 * time.Second):
		t.Fatal("Did not receive offline message")
	}

	client2.Disconnect(100)
}

func TestCleanSessionOverride(t *testing.T) {
	// Start broker
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	b := broker.New("node2", nil)
	b.SetupDefaultAuth()
	defer b.Close()

	go b.StartServer(ctx, ":1885")
	time.Sleep(100 * time.Millisecond) // Wait for server to start

	clientID := "test-clean-override"

	// Test 1: Create persistent session with subscription
	client1 := createPersistentClient(clientID, ":1885")
	token := client1.Connect()
	require.True(t, token.WaitTimeout(5*time.Second))
	require.NoError(t, token.Error())

	// Subscribe to a topic
	subToken := client1.Subscribe("test/override", 1, nil)
	require.True(t, subToken.WaitTimeout(5*time.Second))
	require.NoError(t, subToken.Error())

	// Disconnect
	client1.Disconnect(100)

	// Test 2: Reconnect with clean session (should clear subscriptions)
	messageReceived := make(chan mqtt.Message, 1)
	client2 := createCleanClient(clientID, ":1885")
	client2.Subscribe("test/different", 1, func(client mqtt.Client, msg mqtt.Message) {
		messageReceived <- msg
	})

	token = client2.Connect()
	require.True(t, token.WaitTimeout(5*time.Second))
	require.NoError(t, token.Error())

	// Test 3: Publish to original topic - should not receive
	publisher := createCleanClient("publisher2", ":1885")
	token = publisher.Connect()
	require.True(t, token.WaitTimeout(5*time.Second))
	require.NoError(t, token.Error())

	pubToken := publisher.Publish("test/override", 1, false, "should not receive")
	require.True(t, pubToken.WaitTimeout(5*time.Second))
	require.NoError(t, pubToken.Error())

	// Should NOT receive message on old subscription
	select {
	case <-messageReceived:
		t.Fatal("Should not have received message on cleared subscription")
	case <-time.After(1 * time.Second):
		// Expected - no message received
	}

	// Test 4: Publish to new topic - should receive
	pubToken = publisher.Publish("test/different", 1, false, "should receive")
	require.True(t, pubToken.WaitTimeout(5*time.Second))
	require.NoError(t, pubToken.Error())

	// Should receive message on new subscription
	select {
	case msg := <-messageReceived:
		assert.Equal(t, "test/different", msg.Topic())
		assert.Equal(t, "should receive", string(msg.Payload()))
	case <-time.After(3 * time.Second):
		t.Fatal("Did not receive message on new subscription")
	}

	client2.Disconnect(100)
	publisher.Disconnect(100)
}

func TestOfflineMessageQueue(t *testing.T) {
	// Start broker
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	b := broker.New("node3", nil)
	b.SetupDefaultAuth()
	defer b.Close()

	go b.StartServer(ctx, ":1886")
	time.Sleep(100 * time.Millisecond) // Wait for server to start

	clientID := "test-offline-queue"

	// Test 1: Create persistent session and disconnect
	client1 := createPersistentClient(clientID, ":1886")
	token := client1.Connect()
	require.True(t, token.WaitTimeout(5*time.Second))
	require.NoError(t, token.Error())

	subToken := client1.Subscribe("test/queue/+", 1, nil)
	require.True(t, subToken.WaitTimeout(5*time.Second))
	require.NoError(t, subToken.Error())

	client1.Disconnect(100)

	// Test 2: Publish multiple messages while offline
	publisher := createCleanClient("queue-publisher", ":1886")
	token = publisher.Connect()
	require.True(t, token.WaitTimeout(5*time.Second))
	require.NoError(t, token.Error())

	messages := []string{"message1", "message2", "message3"}
	for i, msg := range messages {
		topic := "test/queue/" + string(rune('a'+i))
		pubToken := publisher.Publish(topic, 1, false, msg)
		require.True(t, pubToken.WaitTimeout(5*time.Second))
		require.NoError(t, pubToken.Error())
	}

	publisher.Disconnect(100)

	// Test 3: Reconnect and receive all offline messages
	receivedMessages := make(chan mqtt.Message, len(messages))
	client2 := createPersistentClient(clientID, ":1886")
	client2.Subscribe("test/queue/+", 1, func(client mqtt.Client, msg mqtt.Message) {
		receivedMessages <- msg
	})

	token = client2.Connect()
	require.True(t, token.WaitTimeout(5*time.Second))
	require.NoError(t, token.Error())

	// Should receive all offline messages
	receivedCount := 0
	timeout := time.After(5 * time.Second)
	for receivedCount < len(messages) {
		select {
		case msg := <-receivedMessages:
			receivedCount++
			assert.Contains(t, []string{"message1", "message2", "message3"}, string(msg.Payload()))
		case <-timeout:
			t.Fatalf("Only received %d out of %d messages", receivedCount, len(messages))
		}
	}

	assert.Equal(t, len(messages), receivedCount)
	client2.Disconnect(100)
}

// Helper functions
func createPersistentClient(clientID, addr string) mqtt.Client {
	opts := mqtt.NewClientOptions()
	opts.AddBroker("tcp://localhost" + addr)
	opts.SetClientID(clientID)
	opts.SetUsername("test")
	opts.SetPassword("test")
	opts.SetCleanSession(false) // Persistent session
	opts.SetAutoReconnect(false)
	opts.SetConnectTimeout(5 * time.Second)
	return mqtt.NewClient(opts)
}

func createCleanClient(clientID, addr string) mqtt.Client {
	opts := mqtt.NewClientOptions()
	opts.AddBroker("tcp://localhost" + addr)
	opts.SetClientID(clientID)
	opts.SetUsername("test")
	opts.SetPassword("test")
	opts.SetCleanSession(true) // Clean session
	opts.SetAutoReconnect(false)
	opts.SetConnectTimeout(5 * time.Second)
	return mqtt.NewClient(opts)
}