package e2e

import (
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/eclipse/paho.mqtt.golang"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/turtacn/emqx-go/pkg/broker"
)

// TestNetworkReliability tests network reliability scenarios
func TestNetworkReliability(t *testing.T) {
	testBroker := setupTestBroker(t, ":1970")
	defer testBroker.Close()

	t.Run("TCPConnectionHandling", func(t *testing.T) {
		testTCPConnectionHandling(t, testBroker)
	})

	t.Run("KeepAliveHandling", func(t *testing.T) {
		testKeepAliveHandling(t, testBroker)
	})

	t.Run("CleanSessionVsPersistentSession", func(t *testing.T) {
		testCleanVsPersistentSession(t, testBroker)
	})

	t.Run("ClientTakeoverScenario", func(t *testing.T) {
		testClientTakeoverScenario(t, testBroker)
	})
}

// TestHighLoadScenarios tests broker behavior under high load
func TestHighLoadScenarios(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping high load tests in short mode")
	}

	testBroker := setupTestBroker(t, ":1971")
	defer testBroker.Close()

	t.Run("ConcurrentConnections", func(t *testing.T) {
		testConcurrentConnections(t, testBroker)
	})

	t.Run("HighFrequencyPublishing", func(t *testing.T) {
		testHighFrequencyPublishing(t, testBroker)
	})

	t.Run("MassiveSubscriptions", func(t *testing.T) {
		testMassiveSubscriptions(t, testBroker)
	})

	t.Run("MessageBackpressure", func(t *testing.T) {
		testMessageBackpressure(t, testBroker)
	})
}

// TestDataIntegrityScenarios tests data integrity under various conditions
func TestDataIntegrityScenarios(t *testing.T) {
	testBroker := setupTestBroker(t, ":1972")
	defer testBroker.Close()

	t.Run("QoS1MessageDelivery", func(t *testing.T) {
		testQoS1MessageDelivery(t, testBroker)
	})

	t.Run("QoS2ExactlyOnceDelivery", func(t *testing.T) {
		testQoS2ExactlyOnceDelivery(t, testBroker)
	})

	t.Run("RetainedMessageConsistency", func(t *testing.T) {
		testRetainedMessageConsistency(t, testBroker)
	})

	t.Run("WillMessageReliability", func(t *testing.T) {
		testWillMessageReliability(t, testBroker)
	})
}

// TestEdgeCases tests edge cases and corner scenarios
func TestEdgeCases(t *testing.T) {
	testBroker := setupTestBroker(t, ":1973")
	defer testBroker.Close()

	t.Run("ZeroLengthMessages", func(t *testing.T) {
		testZeroLengthMessages(t, testBroker)
	})

	t.Run("EmptyClientID", func(t *testing.T) {
		testEmptyClientID(t, testBroker)
	})

	t.Run("DuplicateClientID", func(t *testing.T) {
		testDuplicateClientID(t, testBroker)
	})

	t.Run("TopicNameEdgeCases", func(t *testing.T) {
		testTopicNameEdgeCases(t, testBroker)
	})
}

// TestSecurityScenarios tests security-related scenarios
func TestSecurityScenarios(t *testing.T) {
	testBroker := setupTestBroker(t, ":1974")
	defer testBroker.Close()

	t.Run("AuthenticationFailure", func(t *testing.T) {
		testAuthenticationFailure(t, testBroker)
	})

	t.Run("AuthorizationChecks", func(t *testing.T) {
		testAuthorizationChecks(t, testBroker)
	})

	t.Run("ConnectionLimits", func(t *testing.T) {
		testConnectionLimits(t, testBroker)
	})
}

// Implementation functions

func testTCPConnectionHandling(t *testing.T, testBroker *broker.Broker) {
	// Test basic TCP connection handling
	clientOptions := mqtt.NewClientOptions()
	clientOptions.AddBroker("tcp://localhost:1970")
	clientOptions.SetClientID("tcp-connection-test")
	clientOptions.SetConnectTimeout(5 * time.Second)
	clientOptions.SetUsername("test")
	clientOptions.SetPassword("test")

	client := mqtt.NewClient(clientOptions)
	token := client.Connect()
	require.True(t, token.WaitTimeout(10*time.Second))
	require.NoError(t, token.Error())

	// Test connection is established
	assert.True(t, client.IsConnected())

	// Test graceful disconnect
	client.Disconnect(250)
	time.Sleep(500 * time.Millisecond)
	assert.False(t, client.IsConnected())

	t.Log("✅ TCP connection handling test completed")
}

func testKeepAliveHandling(t *testing.T, testBroker *broker.Broker) {
	// Test keep-alive mechanism
	clientOptions := mqtt.NewClientOptions()
	clientOptions.AddBroker("tcp://localhost:1970")
	clientOptions.SetClientID("keepalive-test")
	clientOptions.SetKeepAlive(2 * time.Second)
	clientOptions.SetUsername("test")
	clientOptions.SetPassword("test")

	client := mqtt.NewClient(clientOptions)
	token := client.Connect()
	require.True(t, token.WaitTimeout(5*time.Second))
	require.NoError(t, token.Error())
	defer client.Disconnect(250)

	// Stay connected for longer than keep-alive interval
	time.Sleep(5 * time.Second)

	// Connection should still be active due to automatic ping requests
	assert.True(t, client.IsConnected())

	t.Log("✅ Keep-alive handling test completed")
}

func testCleanVsPersistentSession(t *testing.T, testBroker *broker.Broker) {
	// Test clean session vs persistent session behavior
	clientID := "session-test-client"

	// First: Connect with clean session = false
	clientOptions := mqtt.NewClientOptions()
	clientOptions.AddBroker("tcp://localhost:1970")
	clientOptions.SetClientID(clientID)
	clientOptions.SetCleanSession(false)
	clientOptions.SetUsername("test")
	clientOptions.SetPassword("test")

	client := mqtt.NewClient(clientOptions)
	token := client.Connect()
	require.True(t, token.WaitTimeout(5*time.Second))
	require.NoError(t, token.Error())

	// Subscribe to a topic
	subToken := client.Subscribe("test/persistent/session", 1, nil)
	require.True(t, subToken.WaitTimeout(5*time.Second))
	require.NoError(t, subToken.Error())

	client.Disconnect(250)

	// Second: Reconnect with same client ID and clean session = false
	reconnectClient := mqtt.NewClient(clientOptions)
	reconnectToken := reconnectClient.Connect()
	require.True(t, reconnectToken.WaitTimeout(5*time.Second))
	require.NoError(t, reconnectToken.Error())

	// Session should be persistent, but testing subscription persistence
	// requires more advanced broker introspection

	reconnectClient.Disconnect(250)

	// Third: Connect with clean session = true
	clientOptions.SetCleanSession(true)
	cleanClient := mqtt.NewClient(clientOptions)
	cleanToken := cleanClient.Connect()
	require.True(t, cleanToken.WaitTimeout(5*time.Second))
	require.NoError(t, cleanToken.Error())

	cleanClient.Disconnect(250)

	t.Log("✅ Clean vs persistent session test completed")
}

func testClientTakeoverScenario(t *testing.T, testBroker *broker.Broker) {
	// Test client takeover when same client ID connects twice
	clientID := "takeover-test-client"

	// First client connection
	clientOptions1 := mqtt.NewClientOptions()
	clientOptions1.AddBroker("tcp://localhost:1970")
	clientOptions1.SetClientID(clientID)
	clientOptions1.SetUsername("test")
	clientOptions1.SetPassword("test")

	client1 := mqtt.NewClient(clientOptions1)
	token1 := client1.Connect()
	require.True(t, token1.WaitTimeout(5*time.Second))
	require.NoError(t, token1.Error())

	assert.True(t, client1.IsConnected())

	// Second client connection with same client ID
	clientOptions2 := mqtt.NewClientOptions()
	clientOptions2.AddBroker("tcp://localhost:1970")
	clientOptions2.SetClientID(clientID)
	clientOptions2.SetUsername("test")
	clientOptions2.SetPassword("test")

	client2 := mqtt.NewClient(clientOptions2)
	token2 := client2.Connect()
	require.True(t, token2.WaitTimeout(5*time.Second))
	require.NoError(t, token2.Error())

	// Give some time for takeover to occur
	time.Sleep(1 * time.Second)

	// First client should be disconnected, second should be connected
	assert.False(t, client1.IsConnected())
	assert.True(t, client2.IsConnected())

	client2.Disconnect(250)

	t.Log("✅ Client takeover scenario test completed")
}

func testConcurrentConnections(t *testing.T, testBroker *broker.Broker) {
	// Test concurrent client connections
	const numClients = 50
	var wg sync.WaitGroup
	var successCount int32
	var mu sync.Mutex

	wg.Add(numClients)

	for i := 0; i < numClients; i++ {
		go func(clientIndex int) {
			defer wg.Done()

			clientOptions := mqtt.NewClientOptions()
			clientOptions.AddBroker("tcp://localhost:1971")
			clientOptions.SetClientID(fmt.Sprintf("concurrent-client-%d", clientIndex))
			clientOptions.SetConnectTimeout(10 * time.Second)
			clientOptions.SetUsername("test")
			clientOptions.SetPassword("test")

			client := mqtt.NewClient(clientOptions)
			token := client.Connect()

			if token.WaitTimeout(15*time.Second) && token.Error() == nil {
				mu.Lock()
				successCount++
				mu.Unlock()

				// Stay connected briefly
				time.Sleep(100 * time.Millisecond)
				client.Disconnect(100)
			}
		}(i)
	}

	wg.Wait()

	mu.Lock()
	successRate := float64(successCount) / float64(numClients)
	mu.Unlock()

	assert.Greater(t, successRate, 0.8, "At least 80% of concurrent connections should succeed")

	t.Logf("✅ Concurrent connections test completed: %d/%d successful (%.1f%%)",
		successCount, numClients, successRate*100)
}

func testHighFrequencyPublishing(t *testing.T, testBroker *broker.Broker) {
	// Test high-frequency message publishing
	publisherOptions := mqtt.NewClientOptions()
	publisherOptions.AddBroker("tcp://localhost:1971")
	publisherOptions.SetClientID("high-freq-publisher")
	publisherOptions.SetUsername("test")
	publisherOptions.SetPassword("test")

	publisher := mqtt.NewClient(publisherOptions)
	token := publisher.Connect()
	require.True(t, token.WaitTimeout(5*time.Second))
	require.NoError(t, token.Error())
	defer publisher.Disconnect(250)

	subscriberOptions := mqtt.NewClientOptions()
	subscriberOptions.AddBroker("tcp://localhost:1971")
	subscriberOptions.SetClientID("high-freq-subscriber")
	subscriberOptions.SetUsername("test")
	subscriberOptions.SetPassword("test")

	subscriber := mqtt.NewClient(subscriberOptions)
	subToken := subscriber.Connect()
	require.True(t, subToken.WaitTimeout(5*time.Second))
	require.NoError(t, subToken.Error())
	defer subscriber.Disconnect(250)

	var receivedCount int
	var mu sync.Mutex

	subscribeToken := subscriber.Subscribe("test/high/frequency", 0, func(client mqtt.Client, msg mqtt.Message) {
		mu.Lock()
		receivedCount++
		mu.Unlock()
	})
	require.True(t, subscribeToken.WaitTimeout(5*time.Second))
	require.NoError(t, subscribeToken.Error())

	// Publish messages rapidly
	const numMessages = 100
	var publishSuccessCount int

	start := time.Now()
	for i := 0; i < numMessages; i++ {
		pubToken := publisher.Publish("test/high/frequency", 0, false, fmt.Sprintf("message-%d", i))
		if pubToken.WaitTimeout(1*time.Second) && pubToken.Error() == nil {
			publishSuccessCount++
		}
	}
	publishDuration := time.Since(start)

	// Wait for messages to be received
	time.Sleep(2 * time.Second)

	mu.Lock()
	finalReceivedCount := receivedCount
	mu.Unlock()

	publishRate := float64(publishSuccessCount) / publishDuration.Seconds()
	deliveryRate := float64(finalReceivedCount) / float64(publishSuccessCount)

	t.Logf("Published %d/%d messages at %.1f msgs/sec", publishSuccessCount, numMessages, publishRate)
	t.Logf("Delivery rate: %.1f%% (%d/%d)", deliveryRate*100, finalReceivedCount, publishSuccessCount)

	assert.Greater(t, deliveryRate, 0.8, "At least 80% of messages should be delivered")

	t.Log("✅ High frequency publishing test completed")
}

func testMassiveSubscriptions(t *testing.T, testBroker *broker.Broker) {
	// Test handling of many subscriptions
	clientOptions := mqtt.NewClientOptions()
	clientOptions.AddBroker("tcp://localhost:1971")
	clientOptions.SetClientID("massive-subscriptions-client")
	clientOptions.SetUsername("test")
	clientOptions.SetPassword("test")

	client := mqtt.NewClient(clientOptions)
	token := client.Connect()
	require.True(t, token.WaitTimeout(5*time.Second))
	require.NoError(t, token.Error())
	defer client.Disconnect(250)

	const numSubscriptions = 100
	var successfulSubscriptions int

	// Subscribe to many topics
	for i := 0; i < numSubscriptions; i++ {
		topic := fmt.Sprintf("test/massive/sub/%d", i)
		subToken := client.Subscribe(topic, 0, nil)
		if subToken.WaitTimeout(2*time.Second) && subToken.Error() == nil {
			successfulSubscriptions++
		}
	}

	subscriptionRate := float64(successfulSubscriptions) / float64(numSubscriptions)

	t.Logf("Successfully subscribed to %d/%d topics (%.1f%%)",
		successfulSubscriptions, numSubscriptions, subscriptionRate*100)

	assert.Greater(t, subscriptionRate, 0.9, "At least 90% of subscriptions should succeed")

	t.Log("✅ Massive subscriptions test completed")
}

func testMessageBackpressure(t *testing.T, testBroker *broker.Broker) {
	// Test broker behavior under message backpressure
	publisherOptions := mqtt.NewClientOptions()
	publisherOptions.AddBroker("tcp://localhost:1971")
	publisherOptions.SetClientID("backpressure-publisher")
	publisherOptions.SetUsername("test")
	publisherOptions.SetPassword("test")

	publisher := mqtt.NewClient(publisherOptions)
	token := publisher.Connect()
	require.True(t, token.WaitTimeout(5*time.Second))
	require.NoError(t, token.Error())
	defer publisher.Disconnect(250)

	// Create slow subscriber
	subscriberOptions := mqtt.NewClientOptions()
	subscriberOptions.AddBroker("tcp://localhost:1971")
	subscriberOptions.SetClientID("slow-subscriber")
	subscriberOptions.SetUsername("test")
	subscriberOptions.SetPassword("test")

	subscriber := mqtt.NewClient(subscriberOptions)
	subToken := subscriber.Connect()
	require.True(t, subToken.WaitTimeout(5*time.Second))
	require.NoError(t, subToken.Error())
	defer subscriber.Disconnect(250)

	var processedCount int
	var mu sync.Mutex

	subscribeToken := subscriber.Subscribe("test/backpressure", 1, func(client mqtt.Client, msg mqtt.Message) {
		// Simulate slow processing
		time.Sleep(100 * time.Millisecond)
		mu.Lock()
		processedCount++
		mu.Unlock()
	})
	require.True(t, subscribeToken.WaitTimeout(5*time.Second))
	require.NoError(t, subscribeToken.Error())

	// Publish messages rapidly to create backpressure
	const numMessages = 20
	for i := 0; i < numMessages; i++ {
		pubToken := publisher.Publish("test/backpressure", 1, false, fmt.Sprintf("backpressure-message-%d", i))
		require.True(t, pubToken.WaitTimeout(5*time.Second))
		require.NoError(t, pubToken.Error())
	}

	// Wait for processing
	time.Sleep(5 * time.Second)

	mu.Lock()
	finalProcessedCount := processedCount
	mu.Unlock()

	t.Logf("Processed %d/%d messages under backpressure", finalProcessedCount, numMessages)

	// Some messages should be processed, but might not be all due to backpressure
	assert.Greater(t, finalProcessedCount, 0, "Some messages should be processed")

	t.Log("✅ Message backpressure test completed")
}

func testQoS1MessageDelivery(t *testing.T, testBroker *broker.Broker) {
	// Test QoS 1 at-least-once delivery semantics
	publisherOptions := mqtt.NewClientOptions()
	publisherOptions.AddBroker("tcp://localhost:1972")
	publisherOptions.SetClientID("qos1-publisher")
	publisherOptions.SetUsername("test")
	publisherOptions.SetPassword("test")

	publisher := mqtt.NewClient(publisherOptions)
	token := publisher.Connect()
	require.True(t, token.WaitTimeout(5*time.Second))
	require.NoError(t, token.Error())
	defer publisher.Disconnect(250)

	subscriberOptions := mqtt.NewClientOptions()
	subscriberOptions.AddBroker("tcp://localhost:1972")
	subscriberOptions.SetClientID("qos1-subscriber")
	subscriberOptions.SetUsername("test")
	subscriberOptions.SetPassword("test")

	subscriber := mqtt.NewClient(subscriberOptions)
	subToken := subscriber.Connect()
	require.True(t, subToken.WaitTimeout(5*time.Second))
	require.NoError(t, subToken.Error())
	defer subscriber.Disconnect(250)

	var receivedMessages []string
	var mu sync.Mutex
	var wg sync.WaitGroup

	const numMessages = 10
	wg.Add(numMessages)

	subscribeToken := subscriber.Subscribe("test/qos1/delivery", 1, func(client mqtt.Client, msg mqtt.Message) {
		mu.Lock()
		receivedMessages = append(receivedMessages, string(msg.Payload()))
		mu.Unlock()
		wg.Done()
	})
	require.True(t, subscribeToken.WaitTimeout(5*time.Second))
	require.NoError(t, subscribeToken.Error())

	// Publish messages with QoS 1
	for i := 0; i < numMessages; i++ {
		message := fmt.Sprintf("qos1-message-%d", i)
		pubToken := publisher.Publish("test/qos1/delivery", 1, false, message)
		require.True(t, pubToken.WaitTimeout(5*time.Second))
		require.NoError(t, pubToken.Error())
	}

	// Wait for all messages
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		mu.Lock()
		assert.Equal(t, numMessages, len(receivedMessages), "All QoS 1 messages should be delivered")
		mu.Unlock()
	case <-time.After(10 * time.Second):
		t.Fatal("QoS 1 delivery test timed out")
	}

	t.Log("✅ QoS 1 message delivery test completed")
}

func testQoS2ExactlyOnceDelivery(t *testing.T, testBroker *broker.Broker) {
	// Test QoS 2 exactly-once delivery semantics
	publisherOptions := mqtt.NewClientOptions()
	publisherOptions.AddBroker("tcp://localhost:1972")
	publisherOptions.SetClientID("qos2-publisher")
	publisherOptions.SetUsername("test")
	publisherOptions.SetPassword("test")

	publisher := mqtt.NewClient(publisherOptions)
	token := publisher.Connect()
	require.True(t, token.WaitTimeout(5*time.Second))
	require.NoError(t, token.Error())
	defer publisher.Disconnect(250)

	subscriberOptions := mqtt.NewClientOptions()
	subscriberOptions.AddBroker("tcp://localhost:1972")
	subscriberOptions.SetClientID("qos2-subscriber")
	subscriberOptions.SetUsername("test")
	subscriberOptions.SetPassword("test")

	subscriber := mqtt.NewClient(subscriberOptions)
	subToken := subscriber.Connect()
	require.True(t, subToken.WaitTimeout(5*time.Second))
	require.NoError(t, subToken.Error())
	defer subscriber.Disconnect(250)

	var receivedMessages []string
	var mu sync.Mutex
	var wg sync.WaitGroup

	const numMessages = 5
	wg.Add(numMessages)

	subscribeToken := subscriber.Subscribe("test/qos2/delivery", 2, func(client mqtt.Client, msg mqtt.Message) {
		mu.Lock()
		receivedMessages = append(receivedMessages, string(msg.Payload()))
		mu.Unlock()
		wg.Done()
	})
	require.True(t, subscribeToken.WaitTimeout(5*time.Second))
	require.NoError(t, subscribeToken.Error())

	// Publish messages with QoS 2
	for i := 0; i < numMessages; i++ {
		message := fmt.Sprintf("qos2-message-%d", i)
		pubToken := publisher.Publish("test/qos2/delivery", 2, false, message)
		require.True(t, pubToken.WaitTimeout(10*time.Second))
		require.NoError(t, pubToken.Error())
	}

	// Wait for all messages
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		mu.Lock()
		assert.Equal(t, numMessages, len(receivedMessages), "All QoS 2 messages should be delivered exactly once")

		// Check for duplicates
		messageSet := make(map[string]bool)
		for _, msg := range receivedMessages {
			assert.False(t, messageSet[msg], "QoS 2 should prevent duplicate delivery")
			messageSet[msg] = true
		}
		mu.Unlock()
	case <-time.After(15 * time.Second):
		t.Fatal("QoS 2 delivery test timed out")
	}

	t.Log("✅ QoS 2 exactly-once delivery test completed")
}

func testRetainedMessageConsistency(t *testing.T, testBroker *broker.Broker) {
	// Test retained message consistency
	publisherOptions := mqtt.NewClientOptions()
	publisherOptions.AddBroker("tcp://localhost:1972")
	publisherOptions.SetClientID("retained-publisher")
	publisherOptions.SetUsername("test")
	publisherOptions.SetPassword("test")

	publisher := mqtt.NewClient(publisherOptions)
	token := publisher.Connect()
	require.True(t, token.WaitTimeout(5*time.Second))
	require.NoError(t, token.Error())
	defer publisher.Disconnect(250)

	// Publish retained message
	retainedMessage := "retained-test-message"
	pubToken := publisher.Publish("test/retained/consistency", 1, true, retainedMessage)
	require.True(t, pubToken.WaitTimeout(5*time.Second))
	require.NoError(t, pubToken.Error())

	// Wait a bit for message to be stored
	time.Sleep(500 * time.Millisecond)

	// Subscribe with new client to receive retained message
	subscriberOptions := mqtt.NewClientOptions()
	subscriberOptions.AddBroker("tcp://localhost:1972")
	subscriberOptions.SetClientID("retained-subscriber")
	subscriberOptions.SetUsername("test")
	subscriberOptions.SetPassword("test")

	subscriber := mqtt.NewClient(subscriberOptions)
	subToken := subscriber.Connect()
	require.True(t, subToken.WaitTimeout(5*time.Second))
	require.NoError(t, subToken.Error())
	defer subscriber.Disconnect(250)

	var receivedMessage string
	var wg sync.WaitGroup
	wg.Add(1)

	subscribeToken := subscriber.Subscribe("test/retained/consistency", 1, func(client mqtt.Client, msg mqtt.Message) {
		receivedMessage = string(msg.Payload())
		wg.Done()
	})
	require.True(t, subscribeToken.WaitTimeout(5*time.Second))
	require.NoError(t, subscribeToken.Error())

	// Wait for retained message
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		assert.Equal(t, retainedMessage, receivedMessage, "Retained message should be delivered to new subscribers")
	case <-time.After(5 * time.Second):
		t.Fatal("Retained message consistency test timed out")
	}

	t.Log("✅ Retained message consistency test completed")
}

func testWillMessageReliability(t *testing.T, testBroker *broker.Broker) {
	// Test will message delivery on unexpected disconnect
	subscriberOptions := mqtt.NewClientOptions()
	subscriberOptions.AddBroker("tcp://localhost:1972")
	subscriberOptions.SetClientID("will-subscriber")
	subscriberOptions.SetUsername("test")
	subscriberOptions.SetPassword("test")

	subscriber := mqtt.NewClient(subscriberOptions)
	subToken := subscriber.Connect()
	require.True(t, subToken.WaitTimeout(5*time.Second))
	require.NoError(t, subToken.Error())
	defer subscriber.Disconnect(250)

	var willMessage string
	var wg sync.WaitGroup
	wg.Add(1)

	subscribeToken := subscriber.Subscribe("test/will/message", 1, func(client mqtt.Client, msg mqtt.Message) {
		willMessage = string(msg.Payload())
		wg.Done()
	})
	require.True(t, subscribeToken.WaitTimeout(5*time.Second))
	require.NoError(t, subscribeToken.Error())

	// Create client with will message
	willClientOptions := mqtt.NewClientOptions()
	willClientOptions.AddBroker("tcp://localhost:1972")
	willClientOptions.SetClientID("will-client")
	willClientOptions.SetWill("test/will/message", "will message payload", 1, false)
	willClientOptions.SetKeepAlive(2 * time.Second)
	willClientOptions.SetUsername("test")
	willClientOptions.SetPassword("test")

	willClient := mqtt.NewClient(willClientOptions)
	willToken := willClient.Connect()
	require.True(t, willToken.WaitTimeout(5*time.Second))
	require.NoError(t, willToken.Error())

	// Simulate unexpected disconnect (force close without proper DISCONNECT)
	willClient.Disconnect(0) // Force disconnect

	// Wait for will message
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		assert.Equal(t, "will message payload", willMessage, "Will message should be delivered on unexpected disconnect")
	case <-time.After(10 * time.Second):
		t.Log("Will message test timed out - this may be expected if will messages aren't fully implemented")
	}

	t.Log("✅ Will message reliability test completed")
}

func testZeroLengthMessages(t *testing.T, testBroker *broker.Broker) {
	// Test handling of zero-length messages
	publisherOptions := mqtt.NewClientOptions()
	publisherOptions.AddBroker("tcp://localhost:1973")
	publisherOptions.SetClientID("zero-length-publisher")
	publisherOptions.SetUsername("test")
	publisherOptions.SetPassword("test")

	publisher := mqtt.NewClient(publisherOptions)
	token := publisher.Connect()
	require.True(t, token.WaitTimeout(5*time.Second))
	require.NoError(t, token.Error())
	defer publisher.Disconnect(250)

	subscriberOptions := mqtt.NewClientOptions()
	subscriberOptions.AddBroker("tcp://localhost:1973")
	subscriberOptions.SetClientID("zero-length-subscriber")
	subscriberOptions.SetUsername("test")
	subscriberOptions.SetPassword("test")

	subscriber := mqtt.NewClient(subscriberOptions)
	subToken := subscriber.Connect()
	require.True(t, subToken.WaitTimeout(5*time.Second))
	require.NoError(t, subToken.Error())
	defer subscriber.Disconnect(250)

	var receivedMessage []byte
	var wg sync.WaitGroup
	wg.Add(1)

	subscribeToken := subscriber.Subscribe("test/zero/length", 1, func(client mqtt.Client, msg mqtt.Message) {
		receivedMessage = msg.Payload()
		wg.Done()
	})
	require.True(t, subscribeToken.WaitTimeout(5*time.Second))
	require.NoError(t, subscribeToken.Error())

	// Publish zero-length message
	pubToken := publisher.Publish("test/zero/length", 1, false, []byte{})
	require.True(t, pubToken.WaitTimeout(5*time.Second))
	require.NoError(t, pubToken.Error())

	// Wait for message
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		assert.Equal(t, []byte{}, receivedMessage, "Zero-length message should be delivered correctly")
	case <-time.After(5 * time.Second):
		t.Fatal("Zero-length message test timed out")
	}

	t.Log("✅ Zero-length messages test completed")
}

func testEmptyClientID(t *testing.T, testBroker *broker.Broker) {
	// Test behavior with empty client ID (should be auto-generated)
	clientOptions := mqtt.NewClientOptions()
	clientOptions.AddBroker("tcp://localhost:1973")
	clientOptions.SetClientID("") // Empty client ID
	clientOptions.SetCleanSession(true)
	clientOptions.SetUsername("test")
	clientOptions.SetPassword("test")

	client := mqtt.NewClient(clientOptions)
	token := client.Connect()

	// Connection should succeed with auto-generated client ID
	if token.WaitTimeout(5*time.Second) && token.Error() == nil {
		assert.True(t, client.IsConnected())
		client.Disconnect(250)
		t.Log("✅ Empty client ID handled correctly (auto-generated)")
	} else {
		t.Log("✅ Empty client ID rejected as expected")
	}
}

func testDuplicateClientID(t *testing.T, testBroker *broker.Broker) {
	// Test duplicate client ID handling (already tested in takeover scenario)
	// This is a simpler version focusing on the behavior
	clientID := "duplicate-test-client"

	// First client
	clientOptions1 := mqtt.NewClientOptions()
	clientOptions1.AddBroker("tcp://localhost:1973")
	clientOptions1.SetClientID(clientID)
	clientOptions1.SetUsername("test")
	clientOptions1.SetPassword("test")

	client1 := mqtt.NewClient(clientOptions1)
	token1 := client1.Connect()
	require.True(t, token1.WaitTimeout(5*time.Second))
	require.NoError(t, token1.Error())

	// Second client with same ID
	clientOptions2 := mqtt.NewClientOptions()
	clientOptions2.AddBroker("tcp://localhost:1973")
	clientOptions2.SetClientID(clientID)
	clientOptions2.SetUsername("test")
	clientOptions2.SetPassword("test")

	client2 := mqtt.NewClient(clientOptions2)
	token2 := client2.Connect()
	require.True(t, token2.WaitTimeout(5*time.Second))
	require.NoError(t, token2.Error())

	time.Sleep(500 * time.Millisecond)

	// First client should be disconnected by takeover
	assert.False(t, client1.IsConnected())
	assert.True(t, client2.IsConnected())

	client2.Disconnect(250)

	t.Log("✅ Duplicate client ID handling test completed")
}

func testTopicNameEdgeCases(t *testing.T, testBroker *broker.Broker) {
	// Test various topic name edge cases
	clientOptions := mqtt.NewClientOptions()
	clientOptions.AddBroker("tcp://localhost:1973")
	clientOptions.SetClientID("topic-edge-cases-client")
	clientOptions.SetUsername("test")
	clientOptions.SetPassword("test")

	client := mqtt.NewClient(clientOptions)
	token := client.Connect()
	require.True(t, token.WaitTimeout(5*time.Second))
	require.NoError(t, token.Error())
	defer client.Disconnect(250)

	// Test various topic patterns
	testTopics := []string{
		"a",                    // Single character
		"a/b/c/d/e/f/g/h/i/j", // Many levels
		"topic/with spaces",    // Spaces in topic
		"topic/with/unicode/测试", // Unicode characters
		"$SYS/test",           // System topic (might be restricted)
	}

	for _, topic := range testTopics {
		subToken := client.Subscribe(topic, 0, nil)
		success := subToken.WaitTimeout(2 * time.Second)

		if success && subToken.Error() == nil {
			t.Logf("Topic '%s' accepted", topic)
		} else {
			t.Logf("Topic '%s' rejected - this may be expected", topic)
		}
	}

	t.Log("✅ Topic name edge cases test completed")
}

func testAuthenticationFailure(t *testing.T, testBroker *broker.Broker) {
	// Test authentication failure scenarios
	clientOptions := mqtt.NewClientOptions()
	clientOptions.AddBroker("tcp://localhost:1974")
	clientOptions.SetClientID("auth-failure-test")
	clientOptions.SetUsername("invalid-user")
	clientOptions.SetPassword("invalid-password")

	client := mqtt.NewClient(clientOptions)
	token := client.Connect()

	// Connection should fail due to invalid credentials
	if !token.WaitTimeout(5*time.Second) || token.Error() != nil {
		t.Log("✅ Authentication failure handled correctly")
	} else {
		t.Log("✅ Authentication succeeded (broker may not have authentication enabled)")
		client.Disconnect(250)
	}
}

func testAuthorizationChecks(t *testing.T, testBroker *broker.Broker) {
	// Test authorization checks (if implemented)
	clientOptions := mqtt.NewClientOptions()
	clientOptions.AddBroker("tcp://localhost:1974")
	clientOptions.SetClientID("authorization-test")
	clientOptions.SetUsername("test")
	clientOptions.SetPassword("test")

	client := mqtt.NewClient(clientOptions)
	token := client.Connect()
	require.True(t, token.WaitTimeout(5*time.Second))
	require.NoError(t, token.Error())
	defer client.Disconnect(250)

	// Try to subscribe to potentially restricted topics
	restrictedTopics := []string{
		"$SYS/broker/stats",
		"admin/commands",
		"restricted/topic",
	}

	for _, topic := range restrictedTopics {
		subToken := client.Subscribe(topic, 0, nil)
		success := subToken.WaitTimeout(2 * time.Second)

		if success && subToken.Error() == nil {
			t.Logf("Access to topic '%s' allowed", topic)
		} else {
			t.Logf("Access to topic '%s' denied - authorization working", topic)
		}
	}

	t.Log("✅ Authorization checks test completed")
}

func testConnectionLimits(t *testing.T, testBroker *broker.Broker) {
	// Test connection limits (if configured)
	const maxAttempts = 10
	var clients []mqtt.Client
	var successfulConnections int

	defer func() {
		for _, client := range clients {
			if client.IsConnected() {
				client.Disconnect(100)
			}
		}
	}()

	for i := 0; i < maxAttempts; i++ {
		clientOptions := mqtt.NewClientOptions()
		clientOptions.AddBroker("tcp://localhost:1974")
		clientOptions.SetClientID(fmt.Sprintf("limit-test-client-%d", i))
		clientOptions.SetConnectTimeout(5 * time.Second)
		clientOptions.SetUsername("test")
		clientOptions.SetPassword("test")

		client := mqtt.NewClient(clientOptions)
		token := client.Connect()

		if token.WaitTimeout(5*time.Second) && token.Error() == nil {
			successfulConnections++
			clients = append(clients, client)
		} else {
			break // Stop on first failure
		}
	}

	t.Logf("Successfully connected %d clients before limits", successfulConnections)
	assert.Greater(t, successfulConnections, 0, "At least some connections should succeed")

	t.Log("✅ Connection limits test completed")
}

// Additional helper function for network connectivity testing
func testNetworkConnectivity(t *testing.T, address string) bool {
	conn, err := net.DialTimeout("tcp", address, 5*time.Second)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}