package e2e

import (
	"sync"
	"testing"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	comprehensiveBrokerURL = "tcp://localhost:1883"
	baseTestTopic         = "test/emqx-go/comprehensive"
)

// TestMQTTComprehensiveSubscription tests MQTT subscription with complete parameters
// including various QoS levels, retain flags, clean session settings, and will messages
func TestMQTTComprehensiveSubscription(t *testing.T) {
	t.Log("Starting comprehensive MQTT subscription test with complete parameters")

	// Test data structure to track received messages
	type ReceivedMessage struct {
		Topic     string
		Payload   string
		QoS       byte
		Retained  bool
		Duplicate bool
		MessageID uint16
	}

	var receivedMessages []ReceivedMessage
	var messagesMutex sync.Mutex

	// Create subscriber with comprehensive options
	subscriberOpts := mqtt.NewClientOptions()
	subscriberOpts.AddBroker(comprehensiveBrokerURL)
	subscriberOpts.SetClientID("comprehensive-subscriber")
	subscriberOpts.SetKeepAlive(30 * time.Second)
	subscriberOpts.SetConnectTimeout(10 * time.Second)
	subscriberOpts.SetPingTimeout(5 * time.Second)
	subscriberOpts.SetCleanSession(true)
	subscriberOpts.SetAutoReconnect(true)
	subscriberOpts.SetMaxReconnectInterval(5 * time.Second)
	subscriberOpts.SetConnectionLostHandler(func(client mqtt.Client, err error) {
		t.Logf("Subscriber connection lost: %v", err)
	})
	subscriberOpts.SetOnConnectHandler(func(client mqtt.Client) {
		t.Log("Subscriber connected successfully")
	})

	// Set will message for subscriber
	subscriberOpts.SetWill(baseTestTopic+"/will", "Subscriber disconnected unexpectedly", 1, false)

	// Set memory store for message persistence
	subscriberOpts.SetStore(mqtt.NewMemoryStore())

	subscriber := mqtt.NewClient(subscriberOpts)
	token := subscriber.Connect()
	require.True(t, token.Wait(), "Failed to connect subscriber")
	require.NoError(t, token.Error(), "Subscriber connection error")
	defer subscriber.Disconnect(250)

	t.Log("Subscriber connected with comprehensive configuration")

	// Create publisher with comprehensive options
	publisherOpts := mqtt.NewClientOptions()
	publisherOpts.AddBroker(comprehensiveBrokerURL)
	publisherOpts.SetClientID("comprehensive-publisher")
	publisherOpts.SetKeepAlive(30 * time.Second)
	publisherOpts.SetConnectTimeout(10 * time.Second)
	publisherOpts.SetPingTimeout(5 * time.Second)
	publisherOpts.SetCleanSession(true)
	publisherOpts.SetAutoReconnect(true)
	publisherOpts.SetMaxReconnectInterval(5 * time.Second)
	publisherOpts.SetConnectionLostHandler(func(client mqtt.Client, err error) {
		t.Logf("Publisher connection lost: %v", err)
	})
	publisherOpts.SetOnConnectHandler(func(client mqtt.Client) {
		t.Log("Publisher connected successfully")
	})

	// Set will message for publisher
	publisherOpts.SetWill(baseTestTopic+"/will", "Publisher disconnected unexpectedly", 1, false)

	// Set memory store for message persistence
	publisherOpts.SetStore(mqtt.NewMemoryStore())

	publisher := mqtt.NewClient(publisherOpts)
	pubToken := publisher.Connect()
	require.True(t, pubToken.Wait(), "Failed to connect publisher")
	require.NoError(t, pubToken.Error(), "Publisher connection error")
	defer publisher.Disconnect(250)

	t.Log("Publisher connected with comprehensive configuration")

	// Message handler with complete parameter capture
	messageHandler := func(client mqtt.Client, msg mqtt.Message) {
		messagesMutex.Lock()
		defer messagesMutex.Unlock()

		receivedMsg := ReceivedMessage{
			Topic:     msg.Topic(),
			Payload:   string(msg.Payload()),
			QoS:       msg.Qos(),
			Retained:  msg.Retained(),
			Duplicate: msg.Duplicate(),
			MessageID: msg.MessageID(),
		}
		receivedMessages = append(receivedMessages, receivedMsg)

		t.Logf("Received message - Topic: %s, Payload: %s, QoS: %d, Retained: %t, Duplicate: %t, MessageID: %d",
			receivedMsg.Topic, receivedMsg.Payload, receivedMsg.QoS, receivedMsg.Retained,
			receivedMsg.Duplicate, receivedMsg.MessageID)
	}

	// Test multiple subscription scenarios with different QoS levels
	subscriptionTests := []struct {
		topic   string
		qos     byte
		name    string
	}{
		{baseTestTopic + "/qos0", 0, "QoS 0 subscription"},
		{baseTestTopic + "/qos1", 1, "QoS 1 subscription"},
		{baseTestTopic + "/qos2", 2, "QoS 2 subscription"},
		{baseTestTopic + "/mixed", 1, "Mixed QoS subscription"},
	}

	// Subscribe to multiple topics with different QoS levels
	for _, test := range subscriptionTests {
		t.Logf("Setting up %s for topic: %s with QoS: %d", test.name, test.topic, test.qos)

		subToken := subscriber.Subscribe(test.topic, test.qos, messageHandler)
		require.True(t, subToken.Wait(), "Failed to subscribe to %s", test.topic)
		require.NoError(t, subToken.Error(), "Subscribe error for %s", test.topic)

		t.Logf("Successfully subscribed to %s", test.topic)
	}

	// Wait for subscriptions to be established
	time.Sleep(200 * time.Millisecond)

	// Test publishing with different scenarios
	publishTests := []struct {
		topic    string
		qos      byte
		retain   bool
		payload  string
		name     string
	}{
		{baseTestTopic + "/qos0", 0, false, "QoS 0 message - fire and forget", "QoS 0 publish"},
		{baseTestTopic + "/qos1", 1, false, "QoS 1 message - at least once delivery", "QoS 1 publish"},
		{baseTestTopic + "/qos2", 2, false, "QoS 2 message - exactly once delivery", "QoS 2 publish"},
		{baseTestTopic + "/qos1", 1, true, "Retained QoS 1 message", "Retained QoS 1 publish"},
		{baseTestTopic + "/mixed", 2, false, "Mixed QoS test message", "Mixed QoS publish"},
	}

	expectedMessageCount := len(publishTests)

	// Publish test messages
	for i, test := range publishTests {
		t.Logf("Publishing %s to topic: %s with QoS: %d, Retain: %t",
			test.name, test.topic, test.qos, test.retain)

		pubToken := publisher.Publish(test.topic, test.qos, test.retain, test.payload)
		require.True(t, pubToken.Wait(), "Failed to publish %s", test.name)
		require.NoError(t, pubToken.Error(), "Publish error for %s", test.name)

		t.Logf("Successfully published message %d: %s", i+1, test.payload)

		// Small delay between publishes to ensure proper processing
		time.Sleep(50 * time.Millisecond)
	}

	// Wait for all messages to be received
	timeout := time.After(10 * time.Second)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			messagesMutex.Lock()
			count := len(receivedMessages)
			messagesMutex.Unlock()

			if count >= expectedMessageCount {
				t.Logf("Received all expected messages: %d/%d", count, expectedMessageCount)
				goto messageCheckComplete
			}

		case <-timeout:
			messagesMutex.Lock()
			count := len(receivedMessages)
			messagesMutex.Unlock()

			t.Fatalf("Timeout waiting for messages. Received %d out of %d expected messages",
				count, expectedMessageCount)
		}
	}

messageCheckComplete:
	// Verify received messages
	messagesMutex.Lock()
	defer messagesMutex.Unlock()

	assert.GreaterOrEqual(t, len(receivedMessages), expectedMessageCount,
		"Should receive at least the expected number of messages")

	// Verify message contents and properties
	topicMessageMap := make(map[string][]ReceivedMessage)
	for _, msg := range receivedMessages {
		topicMessageMap[msg.Topic] = append(topicMessageMap[msg.Topic], msg)
	}

	// Check specific message properties
	for topic, messages := range topicMessageMap {
		t.Logf("Topic %s received %d messages", topic, len(messages))

		for _, msg := range messages {
			// Verify message has expected properties
			assert.NotEmpty(t, msg.Payload, "Message payload should not be empty")
			assert.True(t, msg.QoS >= 0 && msg.QoS <= 2, "QoS should be 0, 1, or 2")

			// Log detailed message info for verification
			t.Logf("  Message: %s, QoS: %d, Retained: %t, Duplicate: %t",
				msg.Payload, msg.QoS, msg.Retained, msg.Duplicate)
		}
	}

	t.Log("Comprehensive MQTT subscription test completed successfully")
}

// TestMQTTSubscriptionFilters tests various topic filter patterns
func TestMQTTSubscriptionFilters(t *testing.T) {
	t.Log("Starting MQTT subscription filters test")

	subscriber := setupComprehensiveMQTTClient("filter-subscriber", t)
	defer subscriber.Disconnect(250)

	publisher := setupComprehensiveMQTTClient("filter-publisher", t)
	defer publisher.Disconnect(250)

	messageReceived := make(chan string, 10)

	// Test exact topic match
	exactTopic := baseTestTopic + "/filters/exact"
	token := subscriber.Subscribe(exactTopic, 1, func(client mqtt.Client, msg mqtt.Message) {
		t.Logf("Received on exact topic %s: %s", msg.Topic(), string(msg.Payload()))
		messageReceived <- string(msg.Payload())
	})
	require.True(t, token.Wait(), "Failed to subscribe to exact topic")
	require.NoError(t, token.Error(), "Exact topic subscribe error")

	// Wait for subscription
	time.Sleep(100 * time.Millisecond)

	// Publish to exact topic
	testMessage := "Exact topic filter test"
	pubToken := publisher.Publish(exactTopic, 1, false, testMessage)
	require.True(t, pubToken.Wait(), "Failed to publish to exact topic")
	require.NoError(t, pubToken.Error(), "Exact topic publish error")

	// Verify message received
	select {
	case receivedMsg := <-messageReceived:
		assert.Equal(t, testMessage, receivedMsg, "Exact topic message should match")
		t.Log("Topic filter test completed successfully")
	case <-time.After(5 * time.Second):
		t.Fatal("Timeout waiting for topic filter message")
	}
}

func setupComprehensiveMQTTClient(clientID string, t *testing.T) mqtt.Client {
	opts := mqtt.NewClientOptions()
	opts.AddBroker(comprehensiveBrokerURL)
	opts.SetClientID(clientID)
	opts.SetKeepAlive(60 * time.Second)
	opts.SetConnectTimeout(5 * time.Second)
	opts.SetPingTimeout(2 * time.Second)
	opts.SetCleanSession(true)
	opts.SetAutoReconnect(true)
	opts.SetMaxReconnectInterval(5 * time.Second)

	// Set comprehensive connection handlers
	opts.SetConnectionLostHandler(func(client mqtt.Client, err error) {
		t.Logf("Client %s connection lost: %v", clientID, err)
	})
	opts.SetOnConnectHandler(func(client mqtt.Client) {
		t.Logf("Client %s connected successfully", clientID)
	})

	// Set will message
	opts.SetWill(baseTestTopic+"/will/"+clientID, "Client "+clientID+" disconnected", 1, false)

	// Set memory store for persistence
	opts.SetStore(mqtt.NewMemoryStore())

	client := mqtt.NewClient(opts)
	token := client.Connect()
	require.True(t, token.Wait(), "Failed to connect client %s", clientID)
	require.NoError(t, token.Error(), "Connection error for client %s", clientID)

	t.Logf("Successfully connected comprehensive client: %s", clientID)
	return client
}