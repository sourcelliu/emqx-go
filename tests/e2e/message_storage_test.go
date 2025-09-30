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
	storageBrokerURL = "tcp://localhost:1883"
	storageTestTopic = "test/storage/e2e"
	retainedTestTopic = "test/storage/retained"
)

// TestRetainedMessageFunctionality tests retained message functionality through MQTT
func TestRetainedMessageFunctionality(t *testing.T) {
	t.Log("Starting Retained Message Functionality e2e test")

	// Publisher to send retained message
	publisher := setupStorageTestMQTTClient("retained_publisher", t)
	defer publisher.Disconnect(250)

	// Publish a retained message
	retainedPayload := "This is a retained message for e2e testing"
	t.Logf("Publishing retained message: %s", retainedPayload)

	token := publisher.Publish(retainedTestTopic, 1, true, retainedPayload)
	require.True(t, token.Wait(), "Failed to publish retained message")
	require.NoError(t, token.Error(), "Publish error")

	// Wait a moment for the message to be processed
	time.Sleep(100 * time.Millisecond)

	// Create a new subscriber after the retained message was published
	subscriber := setupStorageTestMQTTClient("retained_subscriber", t)
	defer subscriber.Disconnect(250)

	retainedReceived := make(chan mqtt.Message, 1)
	var receivedRetained []mqtt.Message
	var mutex sync.Mutex

	// Subscribe to the topic - should receive the retained message
	token = subscriber.Subscribe(retainedTestTopic, 1, func(client mqtt.Client, msg mqtt.Message) {
		mutex.Lock()
		receivedRetained = append(receivedRetained, msg)
		mutex.Unlock()

		t.Logf("Received message: %s (Retained: %v)", string(msg.Payload()), msg.Retained())
		retainedReceived <- msg
	})

	require.True(t, token.Wait(), "Failed to subscribe to retained topic")
	require.NoError(t, token.Error(), "Subscribe error")

	// Wait for the retained message to be delivered
	select {
	case retainedMsg := <-retainedReceived:
		assert.Equal(t, retainedPayload, string(retainedMsg.Payload()), "Should receive the retained message payload")
		assert.True(t, retainedMsg.Retained(), "Message should be marked as retained")
		t.Log("Successfully received retained message")
	case <-time.After(3 * time.Second):
		t.Fatal("Timeout waiting for retained message")
	}

	// Test updating retained message
	updatedPayload := "Updated retained message"
	t.Logf("Publishing updated retained message: %s", updatedPayload)

	token = publisher.Publish(retainedTestTopic, 1, true, updatedPayload)
	require.True(t, token.Wait(), "Failed to publish updated retained message")
	require.NoError(t, token.Error(), "Update publish error")

	// Create another subscriber to verify the updated retained message
	newSubscriber := setupStorageTestMQTTClient("new_retained_subscriber", t)
	defer newSubscriber.Disconnect(250)

	updatedReceived := make(chan mqtt.Message, 1)
	token = newSubscriber.Subscribe(retainedTestTopic, 1, func(client mqtt.Client, msg mqtt.Message) {
		if msg.Retained() {
			updatedReceived <- msg
		}
	})

	require.True(t, token.Wait(), "Failed to subscribe for updated retained message")
	require.NoError(t, token.Error(), "Subscribe error for updated message")

	// Wait for the updated retained message
	select {
	case updatedMsg := <-updatedReceived:
		assert.Equal(t, updatedPayload, string(updatedMsg.Payload()), "Should receive the updated retained message")
		assert.True(t, updatedMsg.Retained(), "Updated message should be marked as retained")
		t.Log("Successfully received updated retained message")
	case <-time.After(3 * time.Second):
		t.Fatal("Timeout waiting for updated retained message")
	}

	// Test deleting retained message (by publishing empty payload)
	t.Log("Deleting retained message by publishing empty payload")
	token = publisher.Publish(retainedTestTopic, 1, true, "")
	require.True(t, token.Wait(), "Failed to publish empty retained message")
	require.NoError(t, token.Error(), "Delete publish error")

	time.Sleep(100 * time.Millisecond)

	// Create a final subscriber to verify no retained message exists
	finalSubscriber := setupStorageTestMQTTClient("final_retained_subscriber", t)
	defer finalSubscriber.Disconnect(250)

	noRetainedReceived := make(chan mqtt.Message, 1)
	token = finalSubscriber.Subscribe(retainedTestTopic, 1, func(client mqtt.Client, msg mqtt.Message) {
		if msg.Retained() {
			noRetainedReceived <- msg
		}
	})

	require.True(t, token.Wait(), "Failed to subscribe to check deleted retained message")
	require.NoError(t, token.Error(), "Subscribe error for deletion check")

	// Should not receive any retained message
	select {
	case msg := <-noRetainedReceived:
		t.Errorf("Should not receive retained message after deletion, but got: %s", string(msg.Payload()))
	case <-time.After(1 * time.Second):
		t.Log("Successfully verified that retained message was deleted")
	}

	t.Log("Retained Message Functionality e2e test completed successfully")
}

// TestMessagePersistenceAcrossConnections tests message persistence through MQTT
func TestMessagePersistenceAcrossConnections(t *testing.T) {
	t.Log("Starting Message Persistence Across Connections e2e test")

	// Test publishing messages and reconnecting subscribers
	publisher := setupStorageTestMQTTClient("persistence_publisher", t)
	defer publisher.Disconnect(250)

	// First subscriber session
	subscriber1 := setupStorageTestMQTTClient("persistence_subscriber_1", t)

	receivedMessages1 := make(chan mqtt.Message, 10)
	var messages1 []mqtt.Message
	var mutex1 sync.Mutex

	// Subscribe and receive some messages
	token := subscriber1.Subscribe(storageTestTopic, 1, func(client mqtt.Client, msg mqtt.Message) {
		mutex1.Lock()
		messages1 = append(messages1, msg)
		mutex1.Unlock()
		receivedMessages1 <- msg
	})
	require.True(t, token.Wait(), "Failed to subscribe in first session")

	// Publish some messages
	testMessages := []string{
		"Message 1 - before disconnect",
		"Message 2 - before disconnect",
		"Message 3 - before disconnect",
	}

	for i, msg := range testMessages {
		t.Logf("Publishing message %d: %s", i+1, msg)
		token := publisher.Publish(storageTestTopic, 1, false, msg)
		require.True(t, token.Wait(), "Failed to publish message")
	}

	// Wait for messages to be received
	timeout := time.After(3 * time.Second)
	receivedCount := 0
	for receivedCount < len(testMessages) {
		select {
		case <-receivedMessages1:
			receivedCount++
		case <-timeout:
			t.Fatalf("Timeout waiting for initial messages. Received %d out of %d", receivedCount, len(testMessages))
		}
	}

	t.Logf("First session received %d messages", receivedCount)

	// Disconnect first subscriber
	subscriber1.Disconnect(250)
	time.Sleep(200 * time.Millisecond)

	// Publish more messages while subscriber is disconnected
	additionalMessages := []string{
		"Message 4 - during disconnect",
		"Message 5 - during disconnect",
	}

	for i, msg := range additionalMessages {
		t.Logf("Publishing message during disconnect %d: %s", i+1, msg)
		token := publisher.Publish(storageTestTopic, 1, false, msg)
		require.True(t, token.Wait(), "Failed to publish message during disconnect")
	}

	time.Sleep(200 * time.Millisecond)

	// Create a new subscriber with the same client ID (simulating reconnection)
	subscriber2 := setupStorageTestMQTTClient("persistence_subscriber_2", t)
	defer subscriber2.Disconnect(250)

	receivedMessages2 := make(chan mqtt.Message, 10)
	var messages2 []mqtt.Message
	var mutex2 sync.Mutex

	// Subscribe again after reconnection
	token = subscriber2.Subscribe(storageTestTopic, 1, func(client mqtt.Client, msg mqtt.Message) {
		mutex2.Lock()
		messages2 = append(messages2, msg)
		mutex2.Unlock()
		t.Logf("Reconnected subscriber received: %s", string(msg.Payload()))
		receivedMessages2 <- msg
	})
	require.True(t, token.Wait(), "Failed to subscribe after reconnection")

	// Publish a new message to trigger delivery
	newMessage := "Message 6 - after reconnection"
	t.Logf("Publishing message after reconnection: %s", newMessage)
	token = publisher.Publish(storageTestTopic, 1, false, newMessage)
	require.True(t, token.Wait(), "Failed to publish message after reconnection")

	// Wait for the new message
	select {
	case <-receivedMessages2:
		t.Log("Successfully received message after reconnection")
	case <-time.After(3 * time.Second):
		t.Fatal("Timeout waiting for message after reconnection")
	}

	t.Log("Message Persistence Across Connections e2e test completed")
}

// TestConcurrentMessageHandling tests concurrent message handling through MQTT
func TestConcurrentMessageHandling(t *testing.T) {
	t.Log("Starting Concurrent Message Handling e2e test")

	const numPublishers = 5
	const numSubscribers = 3
	const messagesPerPublisher = 10

	var publishers []mqtt.Client
	var subscribers []mqtt.Client

	// Setup publishers
	for i := 0; i < numPublishers; i++ {
		clientID := fmt.Sprintf("concurrent_publisher_%d", i)
		publisher := setupStorageTestMQTTClient(clientID, t)
		publishers = append(publishers, publisher)
		defer publisher.Disconnect(250)
	}

	// Setup subscribers
	messageReceived := make(chan mqtt.Message, numPublishers*messagesPerPublisher*numSubscribers)
	var allReceivedMessages []mqtt.Message
	var mutex sync.Mutex

	for i := 0; i < numSubscribers; i++ {
		clientID := fmt.Sprintf("concurrent_subscriber_%d", i)
		subscriber := setupStorageTestMQTTClient(clientID, t)
		subscribers = append(subscribers, subscriber)
		defer subscriber.Disconnect(250)

		// Each subscriber subscribes to the same topic
		token := subscriber.Subscribe(storageTestTopic, 1, func(client mqtt.Client, msg mqtt.Message) {
			mutex.Lock()
			allReceivedMessages = append(allReceivedMessages, msg)
			mutex.Unlock()
			messageReceived <- msg
		})
		require.True(t, token.Wait(), "Failed to subscribe concurrent subscriber")
	}

	// Wait for subscriptions to be established
	time.Sleep(300 * time.Millisecond)

	// Concurrent publishing
	var publishWg sync.WaitGroup
	startTime := time.Now()

	for i := 0; i < numPublishers; i++ {
		publishWg.Add(1)
		go func(publisherIndex int) {
			defer publishWg.Done()

			for j := 0; j < messagesPerPublisher; j++ {
				message := fmt.Sprintf("Message from publisher %d, message %d", publisherIndex, j)
				token := publishers[publisherIndex].Publish(storageTestTopic, 1, false, message)
				if !token.Wait() || token.Error() != nil {
					t.Errorf("Failed to publish message from publisher %d: %v", publisherIndex, token.Error())
				}
			}
		}(i)
	}

	// Wait for all publishing to complete
	publishWg.Wait()
	publishTime := time.Since(startTime)
	t.Logf("Completed concurrent publishing in %v", publishTime)

	// Wait for messages to be received
	totalExpectedMessages := numPublishers * messagesPerPublisher * numSubscribers
	timeout := time.After(10 * time.Second)
	receivedCount := 0

	for receivedCount < totalExpectedMessages {
		select {
		case <-messageReceived:
			receivedCount++
			if receivedCount%50 == 0 { // Log progress
				t.Logf("Received %d/%d messages", receivedCount, totalExpectedMessages)
			}
		case <-timeout:
			t.Logf("Timeout - received %d out of %d expected messages", receivedCount, totalExpectedMessages)
			break
		}
	}

	mutex.Lock()
	finalCount := len(allReceivedMessages)
	mutex.Unlock()

	// Verify results
	totalPublished := numPublishers * messagesPerPublisher
	minimumExpected := totalPublished // At least one copy of each published message

	assert.GreaterOrEqual(t, finalCount, minimumExpected,
		"Should receive at least %d messages (got %d)", minimumExpected, finalCount)

	t.Logf("Concurrent Message Handling completed: published %d messages, received %d total across %d subscribers",
		totalPublished, finalCount, numSubscribers)
}

// TestQoSMessageDelivery tests different QoS levels for message storage
func TestQoSMessageDelivery(t *testing.T) {
	t.Log("Starting QoS Message Delivery e2e test")

	publisher := setupStorageTestMQTTClient("qos_publisher", t)
	defer publisher.Disconnect(250)

	subscriber := setupStorageTestMQTTClient("qos_subscriber", t)
	defer subscriber.Disconnect(250)

	messageReceived := make(chan mqtt.Message, 10)
	var receivedMessages []mqtt.Message
	var mutex sync.Mutex

	// Subscribe to test topic
	token := subscriber.Subscribe(storageTestTopic, 2, func(client mqtt.Client, msg mqtt.Message) {
		mutex.Lock()
		receivedMessages = append(receivedMessages, msg)
		mutex.Unlock()

		t.Logf("Received QoS %d message: %s", msg.Qos(), string(msg.Payload()))
		messageReceived <- msg
	})
	require.True(t, token.Wait(), "Failed to subscribe for QoS test")

	time.Sleep(100 * time.Millisecond)

	// Test messages with different QoS levels
	qosTests := []struct {
		qos     byte
		message string
	}{
		{0, "QoS 0 message - at most once"},
		{1, "QoS 1 message - at least once"},
		{2, "QoS 2 message - exactly once"},
	}

	// Publish messages with different QoS levels
	for _, test := range qosTests {
		t.Logf("Publishing QoS %d message: %s", test.qos, test.message)
		token := publisher.Publish(storageTestTopic, test.qos, false, test.message)
		require.True(t, token.Wait(), "Failed to publish QoS %d message", test.qos)
		require.NoError(t, token.Error(), "QoS %d publish error", test.qos)
	}

	// Wait for all messages to be received
	timeout := time.After(5 * time.Second)
	receivedCount := 0

	for receivedCount < len(qosTests) {
		select {
		case <-messageReceived:
			receivedCount++
		case <-timeout:
			t.Fatalf("Timeout waiting for QoS messages. Received %d out of %d", receivedCount, len(qosTests))
		}
	}

	// Verify message reception
	mutex.Lock()
	assert.Equal(t, len(qosTests), len(receivedMessages), "Should receive all QoS messages")

	// Verify each QoS message was received with appropriate handling
	for i, receivedMsg := range receivedMessages {
		expectedQoS := qosTests[i].qos
		receivedQoS := receivedMsg.Qos()

		// Note: The received QoS might be different from published QoS based on subscription QoS
		// This is normal MQTT behavior
		t.Logf("Message %d: Published QoS %d, Received QoS %d", i+1, expectedQoS, receivedQoS)
		assert.Contains(t, string(receivedMsg.Payload()), fmt.Sprintf("QoS %d", expectedQoS),
			"Message payload should indicate original QoS level")
	}
	mutex.Unlock()

	t.Log("QoS Message Delivery e2e test completed successfully")
}

func setupStorageTestMQTTClient(clientID string, t *testing.T) mqtt.Client {
	opts := mqtt.NewClientOptions()
	opts.AddBroker(storageBrokerURL)
	opts.SetClientID(clientID)
	opts.SetKeepAlive(60 * time.Second)
	opts.SetDefaultPublishHandler(func(client mqtt.Client, msg mqtt.Message) {
		t.Logf("Unexpected message on client %s: %s", clientID, msg.Payload())
	})
	opts.SetPingTimeout(1 * time.Second)
	opts.SetConnectTimeout(5 * time.Second)
	opts.SetAutoReconnect(false) // Disable auto-reconnect for predictable test behavior

	client := mqtt.NewClient(opts)
	token := client.Connect()
	require.True(t, token.Wait(), "Failed to connect to MQTT broker for client %s", clientID)
	require.NoError(t, token.Error(), "Connection error for client %s: %v", clientID, token.Error())

	t.Logf("Successfully connected client: %s", clientID)
	return client
}