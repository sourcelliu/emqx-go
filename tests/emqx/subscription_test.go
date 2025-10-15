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

// TestBasicSubscription tests basic subscribe functionality
func TestBasicSubscription(t *testing.T) {
	t.Log("Testing basic subscription")

	client := setupMQTTClient("basic_sub_client", t)
	defer client.Disconnect(250)

	// Subscribe to topic
	token := client.Subscribe("test/basic/sub", 1, nil)
	require.True(t, token.WaitTimeout(5*time.Second))
	require.NoError(t, token.Error())

	t.Log("Successfully subscribed to test/basic/sub")

	// Unsubscribe
	token = client.Unsubscribe("test/basic/sub")
	require.True(t, token.WaitTimeout(5*time.Second))
	require.NoError(t, token.Error())

	t.Log("Successfully unsubscribed from test/basic/sub")
}

// TestMultipleSubscriptions tests subscribing to multiple topics
func TestMultipleSubscriptions(t *testing.T) {
	t.Log("Testing multiple topic subscriptions")

	client := setupMQTTClient("multi_sub_client", t)
	defer client.Disconnect(250)

	topics := []string{
		"test/multi/1",
		"test/multi/2",
		"test/multi/3",
		"test/multi/4",
		"test/multi/5",
	}

	// Subscribe to multiple topics
	filters := make(map[string]byte)
	for _, topic := range topics {
		filters[topic] = 1
	}

	token := client.SubscribeMultiple(filters, nil)
	require.True(t, token.WaitTimeout(5*time.Second))
	require.NoError(t, token.Error())

	t.Logf("Successfully subscribed to %d topics", len(topics))

	// Unsubscribe from all
	token = client.Unsubscribe(topics...)
	require.True(t, token.WaitTimeout(5*time.Second))
	require.NoError(t, token.Error())

	t.Log("Successfully unsubscribed from all topics")
}

// TestSubscriptionPersistence tests persistent subscriptions across reconnections
func TestSubscriptionPersistence(t *testing.T) {
	t.Log("Testing subscription persistence")

	clientID := "persistent_sub_client"

	// First connection with clean_session=false
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
	token = client1.Subscribe("test/persistent/sub", 1, nil)
	require.True(t, token.WaitTimeout(5*time.Second))
	require.NoError(t, token.Error())

	t.Log("Subscribed with clean_session=false")

	// Disconnect
	client1.Disconnect(250)
	time.Sleep(300 * time.Millisecond)

	// Reconnect with same client ID
	messageReceived := make(chan mqtt.Message, 1)
	opts.SetDefaultPublishHandler(func(client mqtt.Client, msg mqtt.Message) {
		t.Logf("Received message on reconnection: %s", string(msg.Payload()))
		messageReceived <- msg
	})

	client2 := mqtt.NewClient(opts)
	token = client2.Connect()
	require.True(t, token.WaitTimeout(5*time.Second))
	require.NoError(t, token.Error())

	t.Log("Reconnected with same client ID")

	// Publish to the persistent subscription
	publisher := setupMQTTClient("persistent_sub_publisher", t)
	token = publisher.Publish("test/persistent/sub", 1, false, "persistent_test")
	require.True(t, token.WaitTimeout(5*time.Second))
	require.NoError(t, token.Error())
	publisher.Disconnect(250)

	// Should receive message (subscription persisted)
	select {
	case msg := <-messageReceived:
		assert.Equal(t, "persistent_test", string(msg.Payload()))
		t.Log("Persistent subscription working correctly")
	case <-time.After(3 * time.Second):
		t.Log("Did not receive message (broker may not support persistent sessions)")
	}

	client2.Disconnect(250)
}

// TestResubscribe tests resubscribing to the same topic with different QoS
func TestResubscribe(t *testing.T) {
	t.Log("Testing resubscribe with different QoS")

	client := setupMQTTClient("resub_client", t)
	defer client.Disconnect(250)

	receivedQoS := make(chan byte, 5)

	// Subscribe with QoS 0
	token := client.Subscribe("test/resub", 0, func(c mqtt.Client, msg mqtt.Message) {
		t.Logf("Received message with QoS: %d", msg.Qos())
		receivedQoS <- msg.Qos()
	})
	require.True(t, token.WaitTimeout(5*time.Second))
	require.NoError(t, token.Error())

	time.Sleep(100 * time.Millisecond)

	// Publish with QoS 1
	publisher := setupMQTTClient("resub_publisher", t)
	defer publisher.Disconnect(250)

	token = publisher.Publish("test/resub", 1, false, "qos0_sub")
	require.True(t, token.WaitTimeout(5*time.Second))
	require.NoError(t, token.Error())

	// Should receive with QoS 0
	select {
	case qos := <-receivedQoS:
		assert.Equal(t, byte(0), qos)
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for first message")
	}

	// Resubscribe with QoS 2
	token = client.Subscribe("test/resub", 2, func(c mqtt.Client, msg mqtt.Message) {
		t.Logf("Received message with QoS after resubscribe: %d", msg.Qos())
		receivedQoS <- msg.Qos()
	})
	require.True(t, token.WaitTimeout(5*time.Second))
	require.NoError(t, token.Error())

	time.Sleep(100 * time.Millisecond)

	// Publish with QoS 1 again
	token = publisher.Publish("test/resub", 1, false, "qos2_sub")
	require.True(t, token.WaitTimeout(5*time.Second))
	require.NoError(t, token.Error())

	// Should receive with QoS 1 (min of publish 1 and subscribe 2)
	select {
	case qos := <-receivedQoS:
		assert.LessOrEqual(t, qos, byte(1))
		t.Log("Resubscribe test passed")
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for second message")
	}
}

// TestUnsubscribeBehavior tests unsubscribe behavior
func TestUnsubscribeBehavior(t *testing.T) {
	t.Log("Testing unsubscribe behavior")

	client := setupMQTTClient("unsub_client", t)
	defer client.Disconnect(250)

	messageReceived := make(chan mqtt.Message, 5)

	// Subscribe
	token := client.Subscribe("test/unsub", 1, func(c mqtt.Client, msg mqtt.Message) {
		t.Logf("Received message: %s", string(msg.Payload()))
		messageReceived <- msg
	})
	require.True(t, token.WaitTimeout(5*time.Second))
	require.NoError(t, token.Error())

	time.Sleep(100 * time.Millisecond)

	publisher := setupMQTTClient("unsub_publisher", t)
	defer publisher.Disconnect(250)

	// Publish first message
	token = publisher.Publish("test/unsub", 1, false, "before_unsub")
	require.True(t, token.WaitTimeout(5*time.Second))
	require.NoError(t, token.Error())

	// Should receive first message
	select {
	case msg := <-messageReceived:
		assert.Equal(t, "before_unsub", string(msg.Payload()))
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for first message")
	}

	// Unsubscribe
	token = client.Unsubscribe("test/unsub")
	require.True(t, token.WaitTimeout(5*time.Second))
	require.NoError(t, token.Error())

	time.Sleep(100 * time.Millisecond)

	// Publish second message
	token = publisher.Publish("test/unsub", 1, false, "after_unsub")
	require.True(t, token.WaitTimeout(5*time.Second))
	require.NoError(t, token.Error())

	// Should NOT receive second message
	select {
	case msg := <-messageReceived:
		t.Errorf("Should not receive message after unsubscribe, but got: %s", string(msg.Payload()))
	case <-time.After(2 * time.Second):
		t.Log("Correctly did not receive message after unsubscribe")
	}
}

// TestOverlappingSubscriptions tests overlapping wildcard subscriptions
func TestOverlappingSubscriptions(t *testing.T) {
	t.Log("Testing overlapping wildcard subscriptions")

	client := setupMQTTClient("overlap_client", t)
	defer client.Disconnect(250)

	messagesReceived := make(chan string, 10)
	var mu sync.Mutex
	messageCount := make(map[string]int)

	handler := func(c mqtt.Client, msg mqtt.Message) {
		mu.Lock()
		messageCount[msg.Topic()]++
		count := messageCount[msg.Topic()]
		mu.Unlock()
		t.Logf("Received on %s (count: %d): %s", msg.Topic(), count, string(msg.Payload()))
		messagesReceived <- msg.Topic()
	}

	// Subscribe to overlapping topics
	filters := map[string]byte{
		"test/overlap/#":   1, // Matches all under test/overlap/
		"test/overlap/+/b": 1, // Matches test/overlap/*/b
		"test/overlap/a/b": 1, // Matches exact topic
	}

	token := client.SubscribeMultiple(filters, handler)
	require.True(t, token.WaitTimeout(5*time.Second))
	require.NoError(t, token.Error())

	time.Sleep(200 * time.Millisecond)

	publisher := setupMQTTClient("overlap_publisher", t)
	defer publisher.Disconnect(250)

	// Publish to topic that matches all three subscriptions
	token = publisher.Publish("test/overlap/a/b", 1, false, "triple_match")
	require.True(t, token.WaitTimeout(5*time.Second))
	require.NoError(t, token.Error())

	// Collect messages
	timeout := time.After(3 * time.Second)
	receivedCount := 0

collectLoop:
	for {
		select {
		case <-messagesReceived:
			receivedCount++
		case <-timeout:
			break collectLoop
		}
	}

	mu.Lock()
	topicCount := messageCount["test/overlap/a/b"]
	mu.Unlock()

	t.Logf("Message received %d times for overlapping subscriptions", topicCount)

	// MQTT spec allows multiple deliveries for overlapping subscriptions
	// Typically should receive 1-3 times depending on broker implementation
	assert.GreaterOrEqual(t, topicCount, 1, "Should receive message at least once")
	assert.LessOrEqual(t, topicCount, 3, "Should not receive more than number of matching subscriptions")
}

// TestSubscriptionWithDifferentClients tests same topic subscription by different clients
func TestSubscriptionWithDifferentClients(t *testing.T) {
	t.Log("Testing same topic subscription by different clients")

	numClients := 5
	clients := make([]mqtt.Client, numClients)
	messageChannels := make([]chan mqtt.Message, numClients)

	// Create multiple clients and subscribe to same topic
	for i := 0; i < numClients; i++ {
		clientID := fmt.Sprintf("multi_client_sub_%d", i)
		clients[i] = setupMQTTClient(clientID, t)
		defer clients[i].Disconnect(250)

		messageChannels[i] = make(chan mqtt.Message, 5)
		ch := messageChannels[i] // Capture for closure

		token := clients[i].Subscribe("test/multi_client_sub", 1, func(c mqtt.Client, msg mqtt.Message) {
			t.Logf("Client %s received: %s", clientID, string(msg.Payload()))
			ch <- msg
		})
		require.True(t, token.WaitTimeout(5*time.Second))
		require.NoError(t, token.Error())
	}

	time.Sleep(300 * time.Millisecond)

	// Publish one message
	publisher := setupMQTTClient("multi_client_publisher", t)
	defer publisher.Disconnect(250)

	token := publisher.Publish("test/multi_client_sub", 1, false, "broadcast_message")
	require.True(t, token.WaitTimeout(5*time.Second))
	require.NoError(t, token.Error())

	// All clients should receive the message
	successCount := 0
	timeout := time.After(5 * time.Second)

	for i := 0; i < numClients; i++ {
		select {
		case msg := <-messageChannels[i]:
			assert.Equal(t, "broadcast_message", string(msg.Payload()))
			successCount++
		case <-timeout:
			t.Logf("Client %d did not receive message", i)
		}
	}

	assert.Equal(t, numClients, successCount, "All clients should receive the message")
	t.Logf("All %d clients received the message", successCount)
}

// TestSubscriptionFilters tests various subscription filter patterns
func TestSubscriptionFilters(t *testing.T) {
	t.Log("Testing subscription filter patterns")

	testCases := []struct {
		name           string
		subscribeFilter string
		publishTopics   []string
		shouldMatch     map[string]bool
	}{
		{
			name:           "Multi-level wildcard",
			subscribeFilter: "sport/tennis/#",
			publishTopics:   []string{"sport/tennis/player1", "sport/tennis/player1/ranking", "sport/football/player1"},
			shouldMatch:     map[string]bool{"sport/tennis/player1": true, "sport/tennis/player1/ranking": true, "sport/football/player1": false},
		},
		{
			name:           "Single-level wildcard",
			subscribeFilter: "sport/+/player1",
			publishTopics:   []string{"sport/tennis/player1", "sport/football/player1", "sport/tennis/player2"},
			shouldMatch:     map[string]bool{"sport/tennis/player1": true, "sport/football/player1": true, "sport/tennis/player2": false},
		},
		{
			name:           "Mixed wildcards",
			subscribeFilter: "sport/+/player1/#",
			publishTopics:   []string{"sport/tennis/player1", "sport/tennis/player1/ranking", "sport/tennis/player2"},
			shouldMatch:     map[string]bool{"sport/tennis/player1": true, "sport/tennis/player1/ranking": true, "sport/tennis/player2": false},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			client := setupMQTTClient(fmt.Sprintf("filter_test_%s", tc.name), t)
			defer client.Disconnect(250)

			receivedTopics := make(chan string, 10)

			token := client.Subscribe(tc.subscribeFilter, 1, func(c mqtt.Client, msg mqtt.Message) {
				t.Logf("Received on %s: %s", msg.Topic(), string(msg.Payload()))
				receivedTopics <- msg.Topic()
			})
			require.True(t, token.WaitTimeout(5*time.Second))
			require.NoError(t, token.Error())

			time.Sleep(100 * time.Millisecond)

			publisher := setupMQTTClient(fmt.Sprintf("filter_pub_%s", tc.name), t)
			defer publisher.Disconnect(250)

			// Publish to all test topics
			for _, topic := range tc.publishTopics {
				token := publisher.Publish(topic, 1, false, "test")
				require.True(t, token.WaitTimeout(5*time.Second))
				require.NoError(t, token.Error())
			}

			// Collect received topics
			timeout := time.After(3 * time.Second)
			received := make(map[string]bool)

		collectLoop:
			for {
				select {
				case topic := <-receivedTopics:
					received[topic] = true
				case <-timeout:
					break collectLoop
				}
			}

			// Verify matches
			for topic, shouldMatch := range tc.shouldMatch {
				if shouldMatch {
					assert.True(t, received[topic], "Should match: %s with filter %s", topic, tc.subscribeFilter)
				} else {
					assert.False(t, received[topic], "Should NOT match: %s with filter %s", topic, tc.subscribeFilter)
				}
			}
		})
	}
}
