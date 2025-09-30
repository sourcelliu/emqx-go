package e2e

import (
	"fmt"
	"sync"
	"testing"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/stretchr/testify/require"
)

// TestSubscriptionMessageDeliveryDiagnostic performs comprehensive testing
// of message delivery with authentication to identify any issues
func TestSubscriptionMessageDeliveryDiagnostic(t *testing.T) {
	t.Log("=== Starting comprehensive subscription message delivery diagnostic ===")

	// Test scenarios
	testCases := []struct {
		name        string
		publisher   struct{ username, password string }
		subscriber  struct{ username, password string }
		topic       string
		qos         byte
		expectDelivery bool
		description string
	}{
		{
			name:        "same_user_admin_qos0",
			publisher:   struct{ username, password string }{"admin", "admin123"},
			subscriber:  struct{ username, password string }{"admin", "admin123"},
			topic:       "diagnostic/admin/self",
			qos:         0,
			expectDelivery: true,
			description: "Admin user publishing and subscribing to same topic QoS 0",
		},
		{
			name:        "same_user_admin_qos1",
			publisher:   struct{ username, password string }{"admin", "admin123"},
			subscriber:  struct{ username, password string }{"admin", "admin123"},
			topic:       "diagnostic/admin/self/qos1",
			qos:         1,
			expectDelivery: true,
			description: "Admin user publishing and subscribing to same topic QoS 1",
		},
		{
			name:        "cross_user_admin_to_user1",
			publisher:   struct{ username, password string }{"admin", "admin123"},
			subscriber:  struct{ username, password string }{"user1", "password123"},
			topic:       "diagnostic/cross/admin-to-user1",
			qos:         0,
			expectDelivery: true,
			description: "Admin publishing to user1 subscriber",
		},
		{
			name:        "cross_user_user1_to_admin",
			publisher:   struct{ username, password string }{"user1", "password123"},
			subscriber:  struct{ username, password string }{"admin", "admin123"},
			topic:       "diagnostic/cross/user1-to-admin",
			qos:         0,
			expectDelivery: true,
			description: "User1 publishing to admin subscriber",
		},
		{
			name:        "multiple_subscribers_same_topic",
			publisher:   struct{ username, password string }{"admin", "admin123"},
			subscriber:  struct{ username, password string }{"user1", "password123"}, // We'll add more manually
			topic:       "diagnostic/broadcast/all",
			qos:         0,
			expectDelivery: true,
			description: "One publisher, multiple subscribers",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Logf("Testing: %s", tc.description)

			if tc.name == "multiple_subscribers_same_topic" {
				testMultipleSubscribers(t, tc)
				return
			}

			// Create subscriber client
			subOpts := mqtt.NewClientOptions()
			subOpts.AddBroker("tcp://localhost:1883")
			subOpts.SetClientID("diagnostic-sub-" + tc.name)
			subOpts.SetUsername(tc.subscriber.username)
			subOpts.SetPassword(tc.subscriber.password)
			subOpts.SetConnectTimeout(5 * time.Second)
			subOpts.SetAutoReconnect(false)

			subClient := mqtt.NewClient(subOpts)
			subToken := subClient.Connect()
			require.True(t, subToken.Wait(), "Subscriber should connect")
			require.NoError(t, subToken.Error(), "Subscriber connection error")
			defer subClient.Disconnect(100)

			// Set up message channel
			messageReceived := make(chan string, 5)
			subscribeToken := subClient.Subscribe(tc.topic, tc.qos, func(client mqtt.Client, msg mqtt.Message) {
				t.Logf("📨 RECEIVED: topic='%s', payload='%s', qos=%d", msg.Topic(), string(msg.Payload()), msg.Qos())
				messageReceived <- string(msg.Payload())
			})
			require.True(t, subscribeToken.Wait(), "Subscribe should succeed")
			require.NoError(t, subscribeToken.Error(), "Subscribe error")
			t.Logf("✓ Subscriber connected and subscribed to '%s'", tc.topic)

			// Wait a moment for subscription to be fully established
			time.Sleep(100 * time.Millisecond)

			// Create publisher client
			pubOpts := mqtt.NewClientOptions()
			pubOpts.AddBroker("tcp://localhost:1883")
			pubOpts.SetClientID("diagnostic-pub-" + tc.name)
			pubOpts.SetUsername(tc.publisher.username)
			pubOpts.SetPassword(tc.publisher.password)
			pubOpts.SetConnectTimeout(5 * time.Second)
			pubOpts.SetAutoReconnect(false)

			pubClient := mqtt.NewClient(pubOpts)
			pubToken := pubClient.Connect()
			require.True(t, pubToken.Wait(), "Publisher should connect")
			require.NoError(t, pubToken.Error(), "Publisher connection error")
			defer pubClient.Disconnect(100)

			t.Logf("✓ Publisher connected as '%s'", tc.publisher.username)

			// Publish message
			testMessage := "DIAGNOSTIC_MESSAGE_" + tc.name + "_" + time.Now().Format("15:04:05")
			t.Logf("📤 PUBLISHING: topic='%s', payload='%s', qos=%d", tc.topic, testMessage, tc.qos)

			publishToken := pubClient.Publish(tc.topic, tc.qos, false, testMessage)
			require.True(t, publishToken.Wait(), "Publish should succeed")
			require.NoError(t, publishToken.Error(), "Publish error")

			// Wait for message delivery
			if tc.expectDelivery {
				select {
				case receivedMsg := <-messageReceived:
					require.Equal(t, testMessage, receivedMsg, "Message content should match")
					t.Logf("✅ SUCCESS: Message delivered correctly in %s", tc.name)
				case <-time.After(5 * time.Second):
					t.Errorf("❌ FAILURE: Message not received within timeout for %s", tc.name)
					t.Logf("Expected: %s", testMessage)
					t.Logf("Publisher: %s -> Subscriber: %s", tc.publisher.username, tc.subscriber.username)
				}
			} else {
				select {
				case receivedMsg := <-messageReceived:
					t.Errorf("❌ UNEXPECTED: Message should not have been delivered: %s", receivedMsg)
				case <-time.After(2 * time.Second):
					t.Logf("✅ SUCCESS: Message correctly not delivered for %s", tc.name)
				}
			}
		})
	}
}

func testMultipleSubscribers(t *testing.T, tc struct {
	name        string
	publisher   struct{ username, password string }
	subscriber  struct{ username, password string }
	topic       string
	qos         byte
	expectDelivery bool
	description string
}) {
	t.Log("=== Testing multiple subscribers scenario ===")

	// Create multiple subscribers
	subscribers := []struct{ username, password string }{
		{"admin", "admin123"},
		{"user1", "password123"},
		{"test", "test"},
	}

	var clients []mqtt.Client
	var messageChans []chan string
	var wg sync.WaitGroup

	// Connect all subscribers
	for idx, sub := range subscribers {
		opts := mqtt.NewClientOptions()
		opts.AddBroker("tcp://localhost:1883")
		opts.SetClientID("diagnostic-multi-sub-" + sub.username)
		opts.SetUsername(sub.username)
		opts.SetPassword(sub.password)
		opts.SetConnectTimeout(5 * time.Second)

		client := mqtt.NewClient(opts)
		token := client.Connect()
		require.True(t, token.Wait(), "Subscriber %s should connect", sub.username)
		require.NoError(t, token.Error(), "Connection error for %s", sub.username)

		clients = append(clients, client)

		msgChan := make(chan string, 5)
		messageChans = append(messageChans, msgChan)

		subToken := client.Subscribe(tc.topic, tc.qos, func(client mqtt.Client, msg mqtt.Message) {
			t.Logf("📨 %s RECEIVED: '%s'", sub.username, string(msg.Payload()))
			msgChan <- string(msg.Payload())
		})
		require.True(t, subToken.Wait(), "Subscribe should succeed for %s", sub.username)
		require.NoError(t, subToken.Error(), "Subscribe error for %s", sub.username)

		t.Logf("✓ Subscriber %s connected and subscribed", sub.username)
		_ = idx // Use idx to avoid unused variable error
	}

	// Ensure all clients disconnect at the end
	defer func() {
		for _, client := range clients {
			client.Disconnect(100)
		}
	}()

	// Wait for subscriptions to be established
	time.Sleep(200 * time.Millisecond)

	// Create publisher
	pubOpts := mqtt.NewClientOptions()
	pubOpts.AddBroker("tcp://localhost:1883")
	pubOpts.SetClientID("diagnostic-multi-pub")
	pubOpts.SetUsername(tc.publisher.username)
	pubOpts.SetPassword(tc.publisher.password)
	pubOpts.SetConnectTimeout(5 * time.Second)

	pubClient := mqtt.NewClient(pubOpts)
	pubToken := pubClient.Connect()
	require.True(t, pubToken.Wait(), "Publisher should connect")
	require.NoError(t, pubToken.Error(), "Publisher connection error")
	defer pubClient.Disconnect(100)

	// Publish message
	testMessage := "MULTI_SUBSCRIBER_TEST_" + time.Now().Format("15:04:05")
	t.Logf("📤 PUBLISHING to all subscribers: '%s'", testMessage)

	publishToken := pubClient.Publish(tc.topic, tc.qos, false, testMessage)
	require.True(t, publishToken.Wait(), "Publish should succeed")
	require.NoError(t, publishToken.Error(), "Publish error")

	// Check all subscribers receive the message
	receivedCount := 0
	for i, sub := range subscribers {
		wg.Add(1)
		go func(idx int, subscriber struct{ username, password string }) {
			defer wg.Done()
			select {
			case msg := <-messageChans[idx]:
				if msg == testMessage {
					t.Logf("✅ %s received message correctly", subscriber.username)
					receivedCount++
				} else {
					t.Errorf("❌ %s received wrong message: %s", subscriber.username, msg)
				}
			case <-time.After(5 * time.Second):
				t.Errorf("❌ %s did not receive message within timeout", subscriber.username)
			}
		}(i, sub)
	}

	wg.Wait()
	t.Logf("📊 Result: %d out of %d subscribers received the message", receivedCount, len(subscribers))
}

// TestSequentialMessageDelivery tests rapid sequential messages
func TestSequentialMessageDelivery(t *testing.T) {
	t.Log("=== Testing sequential message delivery ===")

	// Setup subscriber
	subOpts := mqtt.NewClientOptions()
	subOpts.AddBroker("tcp://localhost:1883")
	subOpts.SetClientID("sequential-test-sub")
	subOpts.SetUsername("admin")
	subOpts.SetPassword("admin123")

	subClient := mqtt.NewClient(subOpts)
	subToken := subClient.Connect()
	require.True(t, subToken.Wait())
	require.NoError(t, subToken.Error())
	defer subClient.Disconnect(100)

	receivedMessages := make(chan string, 20)
	topic := "diagnostic/sequential/test"

	subscribeToken := subClient.Subscribe(topic, 0, func(client mqtt.Client, msg mqtt.Message) {
		t.Logf("📨 Sequential received: %s", string(msg.Payload()))
		receivedMessages <- string(msg.Payload())
	})
	require.True(t, subscribeToken.Wait())
	require.NoError(t, subscribeToken.Error())

	time.Sleep(100 * time.Millisecond)

	// Setup publisher
	pubOpts := mqtt.NewClientOptions()
	pubOpts.AddBroker("tcp://localhost:1883")
	pubOpts.SetClientID("sequential-test-pub")
	pubOpts.SetUsername("user1")
	pubOpts.SetPassword("password123")

	pubClient := mqtt.NewClient(pubOpts)
	pubToken := pubClient.Connect()
	require.True(t, pubToken.Wait())
	require.NoError(t, pubToken.Error())
	defer pubClient.Disconnect(100)

	// Send multiple messages rapidly
	messageCount := 10
	for i := 0; i < messageCount; i++ {
		msg := fmt.Sprintf("SEQ_MSG_%d", i)
		t.Logf("📤 Publishing sequential message %d: %s", i, msg)

		publishToken := pubClient.Publish(topic, 0, false, msg)
		require.True(t, publishToken.Wait())
		require.NoError(t, publishToken.Error())

		time.Sleep(50 * time.Millisecond) // Small delay between messages
	}

	// Collect received messages
	var received []string
	timeout := time.After(10 * time.Second)

	for len(received) < messageCount {
		select {
		case msg := <-receivedMessages:
			received = append(received, msg)
		case <-timeout:
			break
		}
	}

	t.Logf("📊 Sequential test result: received %d out of %d messages", len(received), messageCount)
	for i, msg := range received {
		t.Logf("  %d: %s", i, msg)
	}

	require.Equal(t, messageCount, len(received), "Should receive all sequential messages")
}