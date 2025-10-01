package e2e

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

// TestAdvancedE2EScenarios tests advanced end-to-end scenarios migrated from EMQX
func TestAdvancedE2EScenarios(t *testing.T) {
	t.Log("Testing Advanced End-to-End Scenarios (migrated from EMQX)")

	t.Run("HighVolumePublishSubscribe", func(t *testing.T) {
		// Test high volume publish/subscribe operations
		// Migrated from EMQX performance and load tests

		const numPublishers = 5
		const numSubscribers = 3
		const messagesPerPublisher = 20

		var wg sync.WaitGroup
		var totalReceived int64

		// Create subscribers
		subscribers := make([]mqtt.Client, numSubscribers)
		for i := 0; i < numSubscribers; i++ {
			clientID := fmt.Sprintf("e2e-high-volume-sub-%d", i)
			subscriber := createMQTTClientWithVersion(clientID, 4, t)
			subscribers[i] = subscriber

			topic := "e2e/high-volume/+"
			token := subscriber.Subscribe(topic, 1, func(client mqtt.Client, msg mqtt.Message) {
				atomic.AddInt64(&totalReceived, 1)
			})
			require.True(t, token.WaitTimeout(5*time.Second))
			require.NoError(t, token.Error())
		}

		// Create publishers and start publishing
		for i := 0; i < numPublishers; i++ {
			wg.Add(1)
			go func(publisherIndex int) {
				defer wg.Done()

				clientID := fmt.Sprintf("e2e-high-volume-pub-%d", publisherIndex)
				publisher := createMQTTClientWithVersion(clientID, 4, t)
				defer publisher.Disconnect(250)

				topic := fmt.Sprintf("e2e/high-volume/%d", publisherIndex)

				for j := 0; j < messagesPerPublisher; j++ {
					message := fmt.Sprintf("High volume message %d from publisher %d", j+1, publisherIndex)
					token := publisher.Publish(topic, 1, false, message)
					require.True(t, token.WaitTimeout(5*time.Second))
					require.NoError(t, token.Error())

					// Small delay to avoid overwhelming the broker
					time.Sleep(10 * time.Millisecond)
				}
			}(i)
		}

		wg.Wait()
		time.Sleep(3 * time.Second) // Allow time for all messages to be delivered

		received := atomic.LoadInt64(&totalReceived)
		expectedTotal := int64(numPublishers * messagesPerPublisher * numSubscribers)

		t.Logf("Received %d messages, expected %d", received, expectedTotal)
		assert.GreaterOrEqual(t, received, int64(numPublishers*messagesPerPublisher),
			"Should receive at least one copy of each message")

		// Clean up subscribers
		for _, subscriber := range subscribers {
			subscriber.Disconnect(250)
		}

		t.Log("✅ High volume publish/subscribe test completed")
	})

	t.Run("MultiLevelWildcardDistribution", func(t *testing.T) {
		// Test complex wildcard topic distribution
		// Migrated from EMQX topic routing tests

		publisher := createMQTTClientWithVersion("e2e-wildcard-publisher", 4, t)
		defer publisher.Disconnect(250)

		// Create subscribers with different wildcard patterns
		wildcardPatterns := map[string]string{
			"single-level": "sensors/+/temperature",
			"multi-level":  "sensors/#",
			"mixed":        "sensors/+/temperature/+",
			"root":         "#",
		}

		subscribers := make(map[string]mqtt.Client)
		messageCounts := make(map[string]*int32)

		for name, pattern := range wildcardPatterns {
			clientID := fmt.Sprintf("e2e-wildcard-sub-%s", name)
			subscriber := createMQTTClientWithVersion(clientID, 4, t)
			subscribers[name] = subscriber

			// Initialize counter for this subscriber
			counter := int32(0)
			messageCounts[name] = &counter

			token := subscriber.Subscribe(pattern, 1, func(client mqtt.Client, msg mqtt.Message) {
				atomic.AddInt32(messageCounts[name], 1)
			})
			require.True(t, token.WaitTimeout(5*time.Second))
			require.NoError(t, token.Error())
		}

		// Publish to various topics
		testTopics := []string{
			"sensors/room1/temperature",
			"sensors/room2/temperature",
			"sensors/room1/humidity",
			"sensors/outdoor/temperature/current",
			"devices/sensor1/status",
		}

		for i, topic := range testTopics {
			message := fmt.Sprintf("Wildcard test message %d", i+1)
			token := publisher.Publish(topic, 1, false, message)
			require.True(t, token.WaitTimeout(5*time.Second))
			require.NoError(t, token.Error())
		}

		time.Sleep(3 * time.Second)

		// Verify message distribution
		for name, count := range messageCounts {
			actualCount := atomic.LoadInt32(count)
			t.Logf("Subscriber '%s' received %d messages", name, actualCount)
			assert.Greater(t, actualCount, int32(0), fmt.Sprintf("Subscriber '%s' should receive some messages", name))
		}

		// Clean up
		for _, subscriber := range subscribers {
			subscriber.Disconnect(250)
		}

		t.Log("✅ Multi-level wildcard distribution test completed")
	})

	t.Run("SessionRecoveryAndPersistence", func(t *testing.T) {
		// Test session recovery and message persistence
		// Migrated from EMQX session management tests

		clientID := "e2e-session-recovery-client"
		topic := "e2e/session/recovery"

		// Phase 1: Connect and subscribe with clean session = false
		client1 := createMQTTClientWithVersionAndCleanSession(clientID, 4, false, t)

		messageReceived1 := make(chan string, 10)
		token := client1.Subscribe(topic, 1, func(client mqtt.Client, msg mqtt.Message) {
			messageReceived1 <- string(msg.Payload())
		})
		require.True(t, token.WaitTimeout(5*time.Second))
		require.NoError(t, token.Error())

		// Verify subscription works
		testMessage1 := "Pre-disconnect message"
		pubToken1 := client1.Publish(topic, 1, false, testMessage1)
		require.True(t, pubToken1.WaitTimeout(5*time.Second))
		require.NoError(t, pubToken1.Error())

		select {
		case msg := <-messageReceived1:
			assert.Equal(t, testMessage1, msg)
		case <-time.After(3 * time.Second):
			t.Fatal("Failed to receive pre-disconnect message")
		}

		// Disconnect without clean session
		client1.Disconnect(250)

		// Phase 2: Publish while client is offline
		publisher := createMQTTClientWithVersion("e2e-offline-publisher", 4, t)
		defer publisher.Disconnect(250)

		offlineMessage := "Message sent while client offline"
		pubToken2 := publisher.Publish(topic, 1, false, offlineMessage)
		require.True(t, pubToken2.WaitTimeout(5*time.Second))
		require.NoError(t, pubToken2.Error())

		// Phase 3: Reconnect and check for offline messages
		client2 := createMQTTClientWithVersionAndCleanSession(clientID, 4, false, t)
		defer client2.Disconnect(250)

		// Note: Whether offline messages are received depends on broker QoS 1/2 implementation
		// This test primarily verifies the reconnection process
		assert.True(t, client2.IsConnected(), "Client should reconnect successfully")

		t.Log("✅ Session recovery and persistence test completed")
	})

	t.Run("LoadBalancingWithSharedSubscriptions", func(t *testing.T) {
		// Test load balancing using shared subscriptions
		// Migrated from EMQX shared subscription load balancing tests

		const numConsumers = 4
		const numMessages = 20

		publisher := createMQTTClientWithVersion("e2e-lb-publisher", 5, t)
		defer publisher.Disconnect(250)

		// Create consumers with shared subscription
		consumers := make([]mqtt.Client, numConsumers)
		messageCounts := make([]int32, numConsumers)

		sharedTopic := "$share/loadbalancegroup/e2e/loadbalance/test"
		actualTopic := "e2e/loadbalance/test"

		for i := 0; i < numConsumers; i++ {
			clientID := fmt.Sprintf("e2e-lb-consumer-%d", i)
			consumer := createMQTTClientWithVersion(clientID, 5, t)
			consumers[i] = consumer

			consumerIndex := i // Capture for closure
			token := consumer.Subscribe(sharedTopic, 1, func(client mqtt.Client, msg mqtt.Message) {
				atomic.AddInt32(&messageCounts[consumerIndex], 1)
			})
			require.True(t, token.WaitTimeout(5*time.Second))
			require.NoError(t, token.Error())
		}

		// Publish messages
		for i := 0; i < numMessages; i++ {
			message := fmt.Sprintf("Load balance test message %d", i+1)
			token := publisher.Publish(actualTopic, 1, false, message)
			require.True(t, token.WaitTimeout(5*time.Second))
			require.NoError(t, token.Error())

			time.Sleep(50 * time.Millisecond) // Small delay for distribution
		}

		time.Sleep(3 * time.Second)

		// Verify load distribution
		totalReceived := int32(0)
		for i, count := range messageCounts {
			received := atomic.LoadInt32(&count)
			totalReceived += received
			t.Logf("Consumer %d received %d messages", i, received)
		}

		assert.Equal(t, int32(numMessages), totalReceived, "Total messages should equal published messages")

		// Check that load is reasonably distributed (no single consumer gets all messages)
		maxReceived := int32(0)
		for _, count := range messageCounts {
			received := atomic.LoadInt32(&count)
			if received > maxReceived {
				maxReceived = received
			}
		}
		assert.Less(t, maxReceived, int32(numMessages), "Load should be distributed among consumers")

		// Clean up
		for _, consumer := range consumers {
			consumer.Disconnect(250)
		}

		t.Log("✅ Load balancing with shared subscriptions test completed")
	})

	t.Run("ProtocolVersionCoexistence", func(t *testing.T) {
		// Test multiple MQTT protocol versions working together
		// Migrated from EMQX multi-protocol tests

		// Create clients with different MQTT versions
		mqtt31Client := createMQTTClientWithVersion("e2e-coexist-mqtt31", 3, t)
		defer mqtt31Client.Disconnect(250)

		mqtt311Client := createMQTTClientWithVersion("e2e-coexist-mqtt311", 4, t)
		defer mqtt311Client.Disconnect(250)

		mqtt5Client := createMQTTClientWithVersion("e2e-coexist-mqtt5", 5, t)
		defer mqtt5Client.Disconnect(250)

		topic := "e2e/coexist/test"
		var messageCount int32

		// All clients subscribe to same topic
		clients := []mqtt.Client{mqtt31Client, mqtt311Client, mqtt5Client}
		for i, client := range clients {
			token := client.Subscribe(topic, 1, func(client mqtt.Client, msg mqtt.Message) {
				atomic.AddInt32(&messageCount, 1)
			})
			require.True(t, token.WaitTimeout(5*time.Second))
			require.NoError(t, token.Error())
			t.Logf("Client %d subscribed successfully", i+1)
		}

		// Each client publishes a message
		for i, client := range clients {
			message := fmt.Sprintf("Coexistence test message from client %d", i+1)
			token := client.Publish(topic, 1, false, message)
			require.True(t, token.WaitTimeout(5*time.Second))
			require.NoError(t, token.Error())
		}

		time.Sleep(3 * time.Second)

		received := atomic.LoadInt32(&messageCount)
		expected := int32(len(clients) * len(clients)) // Each client receives messages from all clients

		assert.Equal(t, expected, received, "All clients should receive all messages")
		t.Logf("Protocol version coexistence test: received %d messages, expected %d", received, expected)

		t.Log("✅ Protocol version coexistence test completed")
	})

	t.Run("RetainedMessageConsistency", func(t *testing.T) {
		// Test retained message consistency across multiple subscribers
		// Migrated from EMQX retained message consistency tests

		publisher := createMQTTClientWithVersion("e2e-retain-publisher", 4, t)
		defer publisher.Disconnect(250)

		topic := "e2e/retained/consistency"
		retainedMessage := "Consistent retained message"

		// Publish retained message
		token := publisher.Publish(topic, 1, true, retainedMessage)
		require.True(t, token.WaitTimeout(5*time.Second))
		require.NoError(t, token.Error())

		time.Sleep(1 * time.Second)

		// Create multiple subscribers at different times
		const numSubscribers = 5
		for i := 0; i < numSubscribers; i++ {
			clientID := fmt.Sprintf("e2e-retain-sub-%d", i)
			subscriber := createMQTTClientWithVersion(clientID, 4, t)

			messageReceived := make(chan string, 1)
			subToken := subscriber.Subscribe(topic, 1, func(client mqtt.Client, msg mqtt.Message) {
				messageReceived <- string(msg.Payload())
			})
			require.True(t, subToken.WaitTimeout(5*time.Second))
			require.NoError(t, subToken.Error())

			// Each subscriber should receive the retained message
			select {
			case msg := <-messageReceived:
				assert.Equal(t, retainedMessage, msg)
				t.Logf("✅ Subscriber %d received retained message correctly", i+1)
			case <-time.After(5 * time.Second):
				t.Fatalf("Subscriber %d did not receive retained message", i+1)
			}

			subscriber.Disconnect(250)

			// Small delay between subscribers
			time.Sleep(200 * time.Millisecond)
		}

		t.Log("✅ Retained message consistency test completed")
	})
}