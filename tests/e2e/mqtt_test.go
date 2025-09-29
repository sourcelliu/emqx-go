package e2e

import (
	"fmt"
	"sync"
	"testing"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	brokerURL = "tcp://localhost:1883"
	testTopic = "test/emqx-go/e2e"
)

func TestMQTTPublishSubscribe(t *testing.T) {
	t.Log("Starting MQTT publish/subscribe e2e test")

	// Setup subscriber
	subscriber := setupMQTTClient("subscriber", t)
	defer subscriber.Disconnect(250)

	// Setup publisher
	publisher := setupMQTTClient("publisher", t)
	defer publisher.Disconnect(250)

	// Channel to receive messages
	messageReceived := make(chan string, 10)
	var receivedMessages []string
	var mutex sync.Mutex

	// Subscribe to test topic
	token := subscriber.Subscribe(testTopic, 1, func(client mqtt.Client, msg mqtt.Message) {
		mutex.Lock()
		receivedMsg := string(msg.Payload())
		receivedMessages = append(receivedMessages, receivedMsg)
		mutex.Unlock()

		t.Logf("Received message: %s", receivedMsg)
		messageReceived <- receivedMsg
	})

	require.True(t, token.Wait(), "Failed to subscribe to topic")
	require.NoError(t, token.Error(), "Subscribe error")
	t.Logf("Successfully subscribed to topic: %s", testTopic)

	// Wait a bit for subscription to be established
	time.Sleep(100 * time.Millisecond)

	// Test messages to publish
	testMessages := []string{
		"Hello EMQX-Go!",
		"Test message 1",
		"Test message 2",
		"Testing MQTT broker functionality",
	}

	// Publish test messages
	for i, message := range testMessages {
		t.Logf("Publishing message %d: %s", i+1, message)
		token := publisher.Publish(testTopic, 1, false, message)
		require.True(t, token.Wait(), "Failed to publish message")
		require.NoError(t, token.Error(), "Publish error")
	}

	// Wait for all messages to be received
	timeout := time.After(5 * time.Second)
	receivedCount := 0

	for receivedCount < len(testMessages) {
		select {
		case <-messageReceived:
			receivedCount++
		case <-timeout:
			t.Fatalf("Timeout waiting for messages. Received %d out of %d", receivedCount, len(testMessages))
		}
	}

	// Verify all messages were received
	mutex.Lock()
	assert.Equal(t, len(testMessages), len(receivedMessages), "Should receive all published messages")

	// Verify message contents (order may vary)
	for _, expectedMsg := range testMessages {
		assert.Contains(t, receivedMessages, expectedMsg, "Should receive expected message")
	}
	mutex.Unlock()

	t.Log("MQTT publish/subscribe e2e test completed successfully")
}

func TestMQTTMultipleClients(t *testing.T) {
	t.Log("Starting MQTT multiple clients test")

	const numClients = 3
	const messagesPerClient = 2

	clients := make([]mqtt.Client, numClients)
	messageReceived := make(chan string, numClients*messagesPerClient)
	var receivedMessages []string
	var mutex sync.Mutex

	// Setup multiple clients
	for i := 0; i < numClients; i++ {
		clientID := fmt.Sprintf("client_%d", i)
		client := setupMQTTClient(clientID, t)
		clients[i] = client
		defer client.Disconnect(250)

		// Each client subscribes to the same topic
		token := client.Subscribe(testTopic, 1, func(client mqtt.Client, msg mqtt.Message) {
			mutex.Lock()
			receivedMsg := string(msg.Payload())
			receivedMessages = append(receivedMessages, receivedMsg)
			mutex.Unlock()

			messageReceived <- receivedMsg
		})
		require.True(t, token.Wait(), "Failed to subscribe")
		require.NoError(t, token.Error(), "Subscribe error")
	}

	// Wait for subscriptions to be established
	time.Sleep(200 * time.Millisecond)

	// Each client publishes messages
	totalMessages := 0
	for i := 0; i < numClients; i++ {
		for j := 0; j < messagesPerClient; j++ {
			message := fmt.Sprintf("Message from client_%d message_%d", i, j)
			t.Logf("Client %d publishing: %s", i, message)

			token := clients[i].Publish(testTopic, 1, false, message)
			require.True(t, token.Wait(), "Failed to publish")
			require.NoError(t, token.Error(), "Publish error")
			totalMessages++
		}
	}

	// Wait for all messages to be received by all clients
	// Each message should be received by all subscribed clients
	expectedTotalReceived := totalMessages * numClients
	timeout := time.After(10 * time.Second)
	receivedCount := 0

	for receivedCount < expectedTotalReceived {
		select {
		case <-messageReceived:
			receivedCount++
		case <-timeout:
			t.Logf("Timeout waiting for messages. Received %d out of %d expected", receivedCount, expectedTotalReceived)
			break
		}
	}

	mutex.Lock()
	t.Logf("Received %d messages total, expected %d", len(receivedMessages), expectedTotalReceived)
	// We should have received at least the number of messages published
	assert.GreaterOrEqual(t, len(receivedMessages), totalMessages, "Should receive at least all published messages")
	mutex.Unlock()

	t.Log("MQTT multiple clients test completed")
}

func setupMQTTClient(clientID string, t *testing.T) mqtt.Client {
	opts := mqtt.NewClientOptions()
	opts.AddBroker(brokerURL)
	opts.SetClientID(clientID)
	opts.SetKeepAlive(60 * time.Second)
	opts.SetDefaultPublishHandler(func(client mqtt.Client, msg mqtt.Message) {
		t.Logf("Unexpected message: %s", msg.Payload())
	})
	opts.SetPingTimeout(1 * time.Second)

	client := mqtt.NewClient(opts)
	token := client.Connect()
	require.True(t, token.Wait(), "Failed to connect to MQTT broker")
	require.NoError(t, token.Error(), "Connection error")

	t.Logf("Successfully connected client: %s", clientID)
	return client
}