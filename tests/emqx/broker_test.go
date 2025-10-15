package emqx

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
	username  = "admin"
	password  = "admin123"
)

// TestBasicPubSub tests basic publish/subscribe functionality
// Equivalent to: t_sub_pub in emqx_broker_SUITE.erl
func TestBasicPubSub(t *testing.T) {
	t.Log("Testing basic MQTT publish/subscribe functionality")

	// Create subscriber
	subscriber := setupMQTTClient("subscriber", t)
	defer subscriber.Disconnect(250)

	// Subscribe to topic
	messageReceived := make(chan mqtt.Message, 1)
	token := subscriber.Subscribe("test/topic", 1, func(client mqtt.Client, msg mqtt.Message) {
		t.Logf("Received message: %s on topic %s", string(msg.Payload()), msg.Topic())
		messageReceived <- msg
	})
	require.True(t, token.WaitTimeout(5*time.Second), "Subscribe timeout")
	require.NoError(t, token.Error(), "Subscribe error")

	time.Sleep(100 * time.Millisecond) // Wait for subscription to be established

	// Create publisher
	publisher := setupMQTTClient("publisher", t)
	defer publisher.Disconnect(250)

	// Publish message
	token = publisher.Publish("test/topic", 1, false, "hello")
	token.Wait()
	require.NoError(t, token.Error(), "Publish error")

	// Verify message received
	select {
	case msg := <-messageReceived:
		assert.Equal(t, "hello", string(msg.Payload()))
		assert.Equal(t, "test/topic", msg.Topic())
		t.Log("Successfully received published message")
	case <-time.After(3 * time.Second):
		t.Fatal("Timeout waiting for message")
	}
}

// TestMultipleSubscribers tests message delivery to multiple subscribers
// Equivalent to: t_fanout in emqx_broker_SUITE.erl
func TestMultipleSubscribers(t *testing.T) {
	t.Log("Testing message fanout to multiple subscribers")

	numSubscribers := 10
	messageReceived := make(chan struct{}, numSubscribers)
	var wg sync.WaitGroup

	// Create multiple subscribers
	subscribers := make([]mqtt.Client, numSubscribers)
	for i := 0; i < numSubscribers; i++ {
		clientID := fmt.Sprintf("subscriber_%d", i)
		client := setupMQTTClient(clientID, t)
		subscribers[i] = client
		defer client.Disconnect(250)

		wg.Add(1)
		token := client.Subscribe("test/fanout", 1, func(c mqtt.Client, msg mqtt.Message) {
			t.Logf("Subscriber %s received: %s", clientID, string(msg.Payload()))
			assert.Equal(t, "fanout_message", string(msg.Payload()))
			messageReceived <- struct{}{}
			wg.Done()
		})
		require.True(t, token.WaitTimeout(5*time.Second), "Subscribe timeout")
		require.NoError(t, token.Error(), "Subscribe error")
	}

	time.Sleep(200 * time.Millisecond) // Wait for all subscriptions

	// Publish message
	publisher := setupMQTTClient("publisher", t)
	defer publisher.Disconnect(250)

	token := publisher.Publish("test/fanout", 1, false, "fanout_message")
	require.True(t, token.WaitTimeout(5*time.Second), "Publish timeout")
	require.NoError(t, token.Error(), "Publish error")

	// Wait for all subscribers to receive
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		t.Logf("All %d subscribers received the message", numSubscribers)
	case <-time.After(10 * time.Second):
		t.Fatalf("Timeout waiting for all subscribers to receive message")
	}
}

// TestSharedSubscription tests shared subscription functionality
// Equivalent to: t_shared_subscribe_3 in emqx_broker_SUITE.erl
func TestSharedSubscription(t *testing.T) {
	t.Log("Testing shared subscription (only one subscriber in group receives message)")

	// Create two subscribers in the same shared subscription group
	subscriber1 := setupMQTTClient("shared_sub_1", t)
	defer subscriber1.Disconnect(250)

	subscriber2 := setupMQTTClient("shared_sub_2", t)
	defer subscriber2.Disconnect(250)

	received1 := make(chan mqtt.Message, 10)
	received2 := make(chan mqtt.Message, 10)

	// Both subscribe to $share/group/topic
	token1 := subscriber1.Subscribe("$share/group/test/shared", 0, func(client mqtt.Client, msg mqtt.Message) {
		t.Log("Subscriber 1 received message")
		received1 <- msg
	})
	require.True(t, token1.WaitTimeout(5*time.Second))
	require.NoError(t, token1.Error())

	token2 := subscriber2.Subscribe("$share/group/test/shared", 0, func(client mqtt.Client, msg mqtt.Message) {
		t.Log("Subscriber 2 received message")
		received2 <- msg
	})
	require.True(t, token2.WaitTimeout(5*time.Second))
	require.NoError(t, token2.Error())

	time.Sleep(100 * time.Millisecond)

	// Publish message
	publisher := setupMQTTClient("shared_publisher", t)
	defer publisher.Disconnect(250)

	token := publisher.Publish("test/shared", 0, false, "shared_message")
	require.True(t, token.WaitTimeout(5*time.Second))
	require.NoError(t, token.Error())

	// Only ONE subscriber should receive the message
	messagesReceived := 0
	timeout := time.After(2 * time.Second)

waitLoop:
	for {
		select {
		case <-received1:
			messagesReceived++
		case <-received2:
			messagesReceived++
		case <-timeout:
			break waitLoop
		}
	}

	assert.Equal(t, 1, messagesReceived, "Only one subscriber in shared group should receive message")
	t.Logf("Shared subscription test passed: exactly 1 message received")
}

// TestNoSubscriberMessageDrop tests that messages are dropped when no subscriber exists
// Equivalent to: t_nosub_pub in emqx_broker_SUITE.erl
func TestNoSubscriberMessageDrop(t *testing.T) {
	t.Log("Testing message drop when no subscriber exists")

	publisher := setupMQTTClient("no_sub_publisher", t)
	defer publisher.Disconnect(250)

	// Publish to topic with no subscribers
	token := publisher.Publish("test/no_subscriber", 0, false, "dropped_message")
	require.True(t, token.WaitTimeout(5*time.Second))
	require.NoError(t, token.Error())

	// Message should be dropped (no error, but no delivery)
	t.Log("Message published to topic with no subscribers (should be dropped)")

	// Verify by subscribing after and not receiving the message
	subscriber := setupMQTTClient("late_subscriber", t)
	defer subscriber.Disconnect(250)

	messageReceived := make(chan mqtt.Message, 1)
	token = subscriber.Subscribe("test/no_subscriber", 0, func(client mqtt.Client, msg mqtt.Message) {
		messageReceived <- msg
	})
	require.True(t, token.WaitTimeout(5*time.Second))
	require.NoError(t, token.Error())

	// Should not receive anything (it's not a retained message)
	select {
	case msg := <-messageReceived:
		t.Errorf("Should not receive message, but got: %s", string(msg.Payload()))
	case <-time.After(1 * time.Second):
		t.Log("Correctly did not receive non-retained message")
	}
}

// TestTopicLevels tests multi-level topic subscription
func TestTopicLevels(t *testing.T) {
	t.Log("Testing multi-level topic subscription")

	subscriber := setupMQTTClient("topic_subscriber", t)
	defer subscriber.Disconnect(250)

	messagesReceived := make(chan string, 10)
	topics := []string{
		"test/level1",
		"test/level1/level2",
		"test/level1/level2/level3",
	}

	// Subscribe to wildcard topic
	token := subscriber.Subscribe("test/level1/#", 1, func(client mqtt.Client, msg mqtt.Message) {
		t.Logf("Received on %s: %s", msg.Topic(), string(msg.Payload()))
		messagesReceived <- msg.Topic()
	})
	require.True(t, token.WaitTimeout(5*time.Second))
	require.NoError(t, token.Error())

	time.Sleep(100 * time.Millisecond)

	// Publish to each topic
	publisher := setupMQTTClient("topic_publisher", t)
	defer publisher.Disconnect(250)

	for _, topic := range topics {
		token := publisher.Publish(topic, 1, false, "test_message")
		require.True(t, token.WaitTimeout(5*time.Second))
		require.NoError(t, token.Error())
	}

	// Verify all messages received
	receivedTopics := make(map[string]bool)
	timeout := time.After(5 * time.Second)

collectLoop:
	for len(receivedTopics) < len(topics) {
		select {
		case topic := <-messagesReceived:
			receivedTopics[topic] = true
		case <-timeout:
			break collectLoop
		}
	}

	assert.Equal(t, len(topics), len(receivedTopics), "Should receive messages from all topic levels")
	for _, topic := range topics {
		assert.True(t, receivedTopics[topic], "Should have received message from topic: %s", topic)
	}
}

// TestSingleLevelWildcard tests single-level wildcard (+) subscription
func TestSingleLevelWildcard(t *testing.T) {
	t.Log("Testing single-level wildcard subscription")

	subscriber := setupMQTTClient("wildcard_subscriber", t)
	defer subscriber.Disconnect(250)

	messagesReceived := make(chan string, 10)

	// Subscribe to test/+/end
	token := subscriber.Subscribe("test/+/end", 1, func(client mqtt.Client, msg mqtt.Message) {
		t.Logf("Received on %s: %s", msg.Topic(), string(msg.Payload()))
		messagesReceived <- msg.Topic()
	})
	require.True(t, token.WaitTimeout(5*time.Second))
	require.NoError(t, token.Error())

	time.Sleep(100 * time.Millisecond)

	publisher := setupMQTTClient("wildcard_publisher", t)
	defer publisher.Disconnect(250)

	// These should match
	matchTopics := []string{
		"test/a/end",
		"test/b/end",
		"test/123/end",
	}

	// These should NOT match
	nonMatchTopics := []string{
		"test/end",           // Missing middle level
		"test/a/b/end",       // Too many levels
		"test/a/notend",      // Wrong end
	}

	// Publish matching messages
	for _, topic := range matchTopics {
		token := publisher.Publish(topic, 1, false, "match")
		require.True(t, token.WaitTimeout(5*time.Second))
		require.NoError(t, token.Error())
	}

	// Publish non-matching messages
	for _, topic := range nonMatchTopics {
		token := publisher.Publish(topic, 1, false, "no_match")
		require.True(t, token.WaitTimeout(5*time.Second))
		require.NoError(t, token.Error())
	}

	// Collect received topics
	receivedTopics := make(map[string]bool)
	timeout := time.After(3 * time.Second)

collectLoop:
	for {
		select {
		case topic := <-messagesReceived:
			receivedTopics[topic] = true
		case <-timeout:
			break collectLoop
		}
	}

	// Verify only matching topics were received
	assert.Equal(t, len(matchTopics), len(receivedTopics), "Should receive only matching topics")
	for _, topic := range matchTopics {
		assert.True(t, receivedTopics[topic], "Should have received: %s", topic)
	}
	for _, topic := range nonMatchTopics {
		assert.False(t, receivedTopics[topic], "Should NOT have received: %s", topic)
	}
}

// setupMQTTClient creates and connects an MQTT client for testing
func setupMQTTClient(clientID string, t *testing.T) mqtt.Client {
	opts := mqtt.NewClientOptions()
	opts.AddBroker(brokerURL)
	opts.SetClientID(clientID)
	opts.SetUsername(username)
	opts.SetPassword(password)
	opts.SetKeepAlive(60 * time.Second)
	opts.SetDefaultPublishHandler(func(client mqtt.Client, msg mqtt.Message) {
		t.Logf("Unexpected message on client %s: topic=%s payload=%s",
			clientID, msg.Topic(), string(msg.Payload()))
	})
	opts.SetPingTimeout(1 * time.Second)
	opts.SetConnectTimeout(5 * time.Second)
	opts.SetAutoReconnect(false)
	// Force clean connection state
	opts.SetCleanSession(true)
	// Set connection timeouts to prevent hanging
	opts.SetWriteTimeout(5 * time.Second)
	opts.SetMaxReconnectInterval(1 * time.Second)
	// Limit ordering and buffering
	opts.SetOrderMatters(false)
	opts.SetMessageChannelDepth(100)

	client := mqtt.NewClient(opts)
	token := client.Connect()
	require.True(t, token.WaitTimeout(5*time.Second), "Failed to connect to MQTT broker for client %s", clientID)
	require.NoError(t, token.Error(), "Connection error for client %s: %v", clientID, token.Error())

	t.Logf("Successfully connected client: %s", clientID)
	return client
}
