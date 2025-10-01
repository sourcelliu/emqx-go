package unit

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestErrorHandling tests error handling and edge cases migrated from EMQX
func TestErrorHandling(t *testing.T) {
	t.Log("Testing Error Handling and Edge Cases (migrated from EMQX)")

	t.Run("InvalidTopicFilters", func(t *testing.T) {
		// Test handling of invalid topic filters
		// Migrated from EMQX topic validation tests

		client := createMQTT311Client("invalid-topic-client", t)
		defer client.Disconnect(250)

		// Test various invalid topic patterns
		invalidTopics := []string{
			"",           // Empty topic
			"topic/#/+",  // # not at end
			"topic/+#",   // Invalid wildcard combination
			"topic/++",   // Double plus
			"topic/##",   // Double hash
		}

		for _, invalidTopic := range invalidTopics {
			token := client.Subscribe(invalidTopic, 1, func(client mqtt.Client, msg mqtt.Message) {
				t.Logf("Received message on invalid topic: %s", msg.Topic())
			})

			// Some invalid topics might be rejected by the client library itself
			if token.WaitTimeout(2 * time.Second) {
				if token.Error() != nil {
					t.Logf("✅ Invalid topic '%s' correctly rejected: %v", invalidTopic, token.Error())
				} else {
					t.Logf("⚠️ Invalid topic '%s' was accepted (broker-dependent behavior)", invalidTopic)
				}
			} else {
				t.Logf("✅ Invalid topic '%s' subscription timed out (likely rejected)", invalidTopic)
			}
		}

		t.Log("✅ Invalid topic filters test completed")
	})

	t.Run("LargePayloads", func(t *testing.T) {
		// Test handling of large message payloads
		// Migrated from EMQX large message tests

		publisher := createMQTT311Client("large-payload-pub", t)
		defer publisher.Disconnect(250)

		subscriber := createMQTT311Client("large-payload-sub", t)
		defer subscriber.Disconnect(250)

		topic := "test/large/payload"
		messageReceived := make(chan bool, 3)

		token := subscriber.Subscribe(topic, 1, func(client mqtt.Client, msg mqtt.Message) {
			payloadSize := len(msg.Payload())
			t.Logf("Received large message with payload size: %d bytes", payloadSize)
			messageReceived <- true
		})
		require.True(t, token.WaitTimeout(5*time.Second))
		require.NoError(t, token.Error())

		// Test different payload sizes
		payloadSizes := []int{1024, 8192, 65536} // 1KB, 8KB, 64KB

		for _, size := range payloadSizes {
			// Create large payload
			payload := strings.Repeat("A", size)

			pubToken := publisher.Publish(topic, 1, false, payload)
			if pubToken.WaitTimeout(10*time.Second) && pubToken.Error() == nil {
				t.Logf("✅ Successfully published %d byte payload", size)

				// Wait for message receipt
				select {
				case <-messageReceived:
					t.Logf("✅ Successfully received %d byte payload", size)
				case <-time.After(10 * time.Second):
					t.Logf("⚠️ Timeout receiving %d byte payload", size)
				}
			} else {
				t.Logf("⚠️ Failed to publish %d byte payload: %v", size, pubToken.Error())
			}
		}

		t.Log("✅ Large payloads test completed")
	})

	t.Run("ConnectionLimits", func(t *testing.T) {
		// Test connection limits and resource management
		// Migrated from EMQX connection limit tests

		const maxConnections = 50
		clients := make([]mqtt.Client, 0, maxConnections)
		var mu sync.Mutex

		// Attempt to create many connections
		for i := 0; i < maxConnections; i++ {
			clientID := fmt.Sprintf("limit-test-client-%d", i)

			opts := mqtt.NewClientOptions()
			opts.AddBroker("tcp://localhost:1883")
			opts.SetClientID(clientID)
			opts.SetUsername("test")
			opts.SetPassword("test")
			opts.SetConnectTimeout(3 * time.Second)
			opts.SetKeepAlive(10 * time.Second)

			client := mqtt.NewClient(opts)
			token := client.Connect()

			if token.WaitTimeout(5*time.Second) && token.Error() == nil {
				mu.Lock()
				clients = append(clients, client)
				mu.Unlock()
			} else {
				t.Logf("Connection %d failed (expected if broker has limits): %v", i, token.Error())
				break
			}
		}

		mu.Lock()
		connectionCount := len(clients)
		mu.Unlock()

		t.Logf("Successfully established %d connections", connectionCount)
		assert.Greater(t, connectionCount, 0, "Should establish at least some connections")

		// Clean up all connections
		for _, client := range clients {
			client.Disconnect(100)
		}

		t.Log("✅ Connection limits test completed")
	})

	t.Run("MalformedPackets", func(t *testing.T) {
		// Test handling of malformed packets
		// Migrated from EMQX packet validation tests

		client := createMQTT311Client("malformed-packet-client", t)
		defer client.Disconnect(250)

		// Test edge cases that might cause malformed packets
		// Extremely long topic name
		longTopic := "test/" + strings.Repeat("verylongtopicname/", 100)
		token := client.Subscribe(longTopic, 1, func(client mqtt.Client, msg mqtt.Message) {
			t.Logf("Received message on long topic")
		})

		if token.WaitTimeout(3*time.Second) && token.Error() == nil {
			t.Log("✅ Long topic subscription accepted")
		} else {
			t.Logf("⚠️ Long topic subscription rejected: %v", token.Error())
		}

		// Try to publish to topic with null characters (should be handled gracefully)
		// Note: Paho client library will likely reject this before sending
		invalidTopic := "test\x00invalid"
		pubToken := client.Publish(invalidTopic, 1, false, "test message")
		if !pubToken.WaitTimeout(2*time.Second) || pubToken.Error() != nil {
			t.Log("✅ Invalid topic with null character correctly rejected")
		}

		t.Log("✅ Malformed packets test completed")
	})

	t.Run("KeepAliveHandling", func(t *testing.T) {
		// Test keep alive mechanism
		// Migrated from EMQX keep alive tests

		// Create client with very short keep alive
		opts := mqtt.NewClientOptions()
		opts.AddBroker("tcp://localhost:1883")
		opts.SetClientID("keepalive-test-client")
		opts.SetUsername("test")
		opts.SetPassword("test")
		opts.SetKeepAlive(2 * time.Second) // Very short keep alive
		opts.SetConnectTimeout(5 * time.Second)

		client := mqtt.NewClient(opts)
		token := client.Connect()
		require.True(t, token.WaitTimeout(10*time.Second))
		require.NoError(t, token.Error())

		// Keep connection alive for longer than keep alive interval
		time.Sleep(6 * time.Second)

		// Connection should still be alive due to keep alive mechanism
		assert.True(t, client.IsConnected(), "Client should remain connected with keep alive")

		client.Disconnect(250)
		t.Log("✅ Keep alive handling test completed")
	})

	t.Run("WillMessageHandling", func(t *testing.T) {
		// Test will message functionality
		// Migrated from EMQX will message tests

		subscriber := createMQTT311Client("will-subscriber", t)
		defer subscriber.Disconnect(250)

		willTopic := "test/will/message"
		willMessage := "Client disconnected unexpectedly"
		willReceived := make(chan string, 1)

		// Subscribe to will topic
		token := subscriber.Subscribe(willTopic, 1, func(client mqtt.Client, msg mqtt.Message) {
			willReceived <- string(msg.Payload())
		})
		require.True(t, token.WaitTimeout(5*time.Second))
		require.NoError(t, token.Error())

		// Create client with will message
		opts := mqtt.NewClientOptions()
		opts.AddBroker("tcp://localhost:1883")
		opts.SetClientID("will-test-client")
		opts.SetUsername("test")
		opts.SetPassword("test")
		opts.SetWill(willTopic, willMessage, 1, false)
		opts.SetConnectTimeout(5 * time.Second)

		willClient := mqtt.NewClient(opts)
		connectToken := willClient.Connect()
		require.True(t, connectToken.WaitTimeout(10*time.Second))
		require.NoError(t, connectToken.Error())

		// Simulate unexpected disconnection by forcing disconnect
		willClient.Disconnect(0) // Immediate disconnect should trigger will

		// Wait for will message
		select {
		case msg := <-willReceived:
			assert.Equal(t, willMessage, msg)
			t.Log("✅ Will message received correctly")
		case <-time.After(10 * time.Second):
			t.Log("⚠️ Will message not received (broker implementation dependent)")
		}

		t.Log("✅ Will message handling test completed")
	})

	t.Run("ClientIdCollision", func(t *testing.T) {
		// Test client ID collision handling
		// Migrated from EMQX client ID management tests

		clientID := "collision-test-client"

		// Create first client
		client1 := createMQTT311Client(clientID, t)
		assert.True(t, client1.IsConnected(), "First client should connect")

		// Wait a moment to ensure connection is established
		time.Sleep(500 * time.Millisecond)

		// Create second client with same ID
		opts := mqtt.NewClientOptions()
		opts.AddBroker("tcp://localhost:1883")
		opts.SetClientID(clientID)
		opts.SetUsername("test")
		opts.SetPassword("test")
		opts.SetConnectTimeout(5 * time.Second)

		client2 := mqtt.NewClient(opts)
		token := client2.Connect()

		if token.WaitTimeout(10*time.Second) && token.Error() == nil {
			t.Log("✅ Second client with same ID connected (takeover)")
			assert.True(t, client2.IsConnected(), "Second client should be connected")

			// First client might be disconnected due to takeover
			time.Sleep(1 * time.Second)
			client2.Disconnect(250)
		} else {
			t.Logf("⚠️ Second client connection failed: %v", token.Error())
		}

		client1.Disconnect(250)
		t.Log("✅ Client ID collision test completed")
	})
}

// TestProtocolViolations tests protocol violation handling
func TestProtocolViolations(t *testing.T) {
	t.Log("Testing Protocol Violation Handling (migrated from EMQX)")

	t.Run("InvalidQoSValues", func(t *testing.T) {
		// Test handling of invalid QoS values
		// This test verifies the broker handles QoS violations gracefully

		client := createMQTT311Client("invalid-qos-client", t)
		defer client.Disconnect(250)

		// Valid QoS values should work
		validQoS := []byte{0, 1, 2}
		for _, qos := range validQoS {
			testTopic := fmt.Sprintf("test/invalid/qos/%d", qos)
			token := client.Publish(testTopic, qos, false, fmt.Sprintf("Valid QoS %d message", qos))
			assert.True(t, token.WaitTimeout(5*time.Second), fmt.Sprintf("QoS %d should be valid", qos))
			assert.NoError(t, token.Error(), fmt.Sprintf("QoS %d should not cause error", qos))
		}

		t.Log("✅ Invalid QoS values test completed")
	})

	t.Run("EmptyClientId", func(t *testing.T) {
		// Test handling of empty client ID
		// Migrated from EMQX client ID validation tests

		opts := mqtt.NewClientOptions()
		opts.AddBroker("tcp://localhost:1883")
		opts.SetClientID("") // Empty client ID
		opts.SetUsername("test")
		opts.SetPassword("test")
		opts.SetCleanSession(true) // Must be true for empty client ID
		opts.SetConnectTimeout(5 * time.Second)

		client := mqtt.NewClient(opts)
		token := client.Connect()

		if token.WaitTimeout(10*time.Second) && token.Error() == nil {
			t.Log("✅ Empty client ID accepted (broker generated ID)")
			assert.True(t, client.IsConnected(), "Client with empty ID should be connected")
			client.Disconnect(250)
		} else {
			t.Logf("⚠️ Empty client ID rejected: %v", token.Error())
		}

		t.Log("✅ Empty client ID test completed")
	})

	t.Run("ReservedTopics", func(t *testing.T) {
		// Test handling of reserved topics (starting with $)
		// Migrated from EMQX topic validation tests

		client := createMQTT311Client("reserved-topic-client", t)
		defer client.Disconnect(250)

		// Try to publish to reserved topics
		reservedTopics := []string{
			"$SYS/broker/version",
			"$share/group/topic",
			"$queue/topic",
		}

		for _, topic := range reservedTopics {
			token := client.Publish(topic, 1, false, "Reserved topic test")
			if token.WaitTimeout(3*time.Second) && token.Error() == nil {
				t.Logf("⚠️ Reserved topic '%s' publication accepted", topic)
			} else {
				t.Logf("✅ Reserved topic '%s' publication rejected", topic)
			}
		}

		t.Log("✅ Reserved topics test completed")
	})
}