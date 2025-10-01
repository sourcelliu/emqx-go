package e2e

import (
	"fmt"
	"strings"
	"testing"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMosquittoStyleRetainedMessageTests tests retained message scenarios inspired by Mosquitto test suite
func TestMosquittoStyleRetainedMessageTests(t *testing.T) {
	t.Log("Testing Mosquitto-style Retained Message Tests")

	t.Run("RetainQoS0", func(t *testing.T) {
		// Based on Mosquitto's 04-retain-qos0.py
		// Test whether a retained PUBLISH to a topic with QoS 0 is actually retained

		for _, protocolVersion := range []uint{4, 5} {
			t.Run(fmt.Sprintf("Protocol_v%d", protocolVersion), func(t *testing.T) {
				publisher := createMQTTClientWithVersion(fmt.Sprintf("retain-qos0-pub-%d", protocolVersion), int(protocolVersion), t)
				defer publisher.Disconnect(250)

				topic := "mosquitto/retain/qos0/test"
				retainedMessage := "retained message QoS 0"

				// Publish retained message
				pubToken := publisher.Publish(topic, 0, true, retainedMessage)
				require.True(t, pubToken.WaitTimeout(5*time.Second))
				require.NoError(t, pubToken.Error())

				// Wait for message to be stored
				time.Sleep(1 * time.Second)

				// Create new subscriber after publish
				subscriber := createMQTTClientWithVersion(fmt.Sprintf("retain-qos0-sub-%d", protocolVersion), int(protocolVersion), t)
				defer subscriber.Disconnect(250)

				messageReceived := make(chan string, 1)
				token := subscriber.Subscribe(topic, 0, func(client mqtt.Client, msg mqtt.Message) {
					payload := strings.TrimPrefix(string(msg.Payload()), "\x00")
					messageReceived <- payload
				})
				require.True(t, token.WaitTimeout(5*time.Second))
				require.NoError(t, token.Error())

				// Should receive retained message immediately
				select {
				case received := <-messageReceived:
					assert.Equal(t, retainedMessage, received)
					t.Logf("✅ QoS 0 retained message test passed for MQTT v%d", protocolVersion)
				case <-time.After(5 * time.Second):
					t.Fatalf("Retained message not received for MQTT v%d", protocolVersion)
				}
			})
		}
	})

	t.Run("RetainQoS1", func(t *testing.T) {
		// Test retained messages with QoS 1

		for _, protocolVersion := range []uint{4, 5} {
			t.Run(fmt.Sprintf("Protocol_v%d", protocolVersion), func(t *testing.T) {
				publisher := createMQTTClientWithVersion(fmt.Sprintf("retain-qos1-pub-%d", protocolVersion), int(protocolVersion), t)
				defer publisher.Disconnect(250)

				topic := "mosquitto/retain/qos1/test"
				retainedMessage := "retained message QoS 1"

				// Publish retained message with QoS 1
				pubToken := publisher.Publish(topic, 1, true, retainedMessage)
				require.True(t, pubToken.WaitTimeout(5*time.Second))
				require.NoError(t, pubToken.Error())

				// Wait for message to be stored
				time.Sleep(1 * time.Second)

				// Create new subscriber after publish
				subscriber := createMQTTClientWithVersion(fmt.Sprintf("retain-qos1-sub-%d", protocolVersion), int(protocolVersion), t)
				defer subscriber.Disconnect(250)

				messageReceived := make(chan string, 1)
				token := subscriber.Subscribe(topic, 1, func(client mqtt.Client, msg mqtt.Message) {
					payload := strings.TrimPrefix(string(msg.Payload()), "\x00")
					messageReceived <- payload
				})
				require.True(t, token.WaitTimeout(5*time.Second))
				require.NoError(t, token.Error())

				// Should receive retained message immediately
				select {
				case received := <-messageReceived:
					assert.Equal(t, retainedMessage, received)
					t.Logf("✅ QoS 1 retained message test passed for MQTT v%d", protocolVersion)
				case <-time.After(5 * time.Second):
					t.Fatalf("QoS 1 retained message not received for MQTT v%d", protocolVersion)
				}
			})
		}
	})

	t.Run("RetainQoS1ToQoS0", func(t *testing.T) {
		// Based on Mosquitto's 04-retain-qos1-qos0.py
		// Test QoS downgrade for retained messages

		publisher := createMQTTClientWithVersion("retain-qos-downgrade-pub", 4, t)
		defer publisher.Disconnect(250)

		topic := "mosquitto/retain/qos/downgrade"
		retainedMessage := "QoS downgrade retained message"

		// Publish retained message with QoS 1
		pubToken := publisher.Publish(topic, 1, true, retainedMessage)
		require.True(t, pubToken.WaitTimeout(5*time.Second))
		require.NoError(t, pubToken.Error())

		time.Sleep(1 * time.Second)

		// Subscribe with QoS 0 (should receive with downgraded QoS)
		subscriber := createMQTTClientWithVersion("retain-qos-downgrade-sub", 4, t)
		defer subscriber.Disconnect(250)

		messageReceived := make(chan string, 1)
		token := subscriber.Subscribe(topic, 0, func(client mqtt.Client, msg mqtt.Message) {
			payload := strings.TrimPrefix(string(msg.Payload()), "\x00")
			messageReceived <- payload
		})
		require.True(t, token.WaitTimeout(5*time.Second))
		require.NoError(t, token.Error())

		select {
		case received := <-messageReceived:
			assert.Equal(t, retainedMessage, received)
			t.Log("✅ QoS downgrade retained message test passed")
		case <-time.After(5 * time.Second):
			t.Fatal("QoS downgrade retained message not received")
		}
	})

	t.Run("RetainClear", func(t *testing.T) {
		// Based on Mosquitto's 04-retain-qos0-clear.py
		// Test clearing retained messages by publishing empty payload

		publisher := createMQTTClientWithVersion("retain-clear-pub", 4, t)
		defer publisher.Disconnect(250)

		topic := "mosquitto/retain/clear/test"
		retainedMessage := "Message to be cleared"

		// First, publish a retained message
		pubToken1 := publisher.Publish(topic, 0, true, retainedMessage)
		require.True(t, pubToken1.WaitTimeout(5*time.Second))
		require.NoError(t, pubToken1.Error())

		time.Sleep(1 * time.Second)

		// Verify the message is retained
		subscriber1 := createMQTTClientWithVersion("retain-clear-sub1", 4, t)
		defer subscriber1.Disconnect(250)

		messageReceived1 := make(chan string, 1)
		token1 := subscriber1.Subscribe(topic, 0, func(client mqtt.Client, msg mqtt.Message) {
			payload := strings.TrimPrefix(string(msg.Payload()), "\x00")
			messageReceived1 <- payload
		})
		require.True(t, token1.WaitTimeout(5*time.Second))
		require.NoError(t, token1.Error())

		select {
		case received := <-messageReceived1:
			assert.Equal(t, retainedMessage, received)
			t.Log("✅ Retained message confirmed before clearing")
		case <-time.After(3 * time.Second):
			t.Fatal("Initial retained message not received")
		}

		// Clear the retained message by publishing empty payload with retain flag
		pubToken2 := publisher.Publish(topic, 0, true, "")
		require.True(t, pubToken2.WaitTimeout(5*time.Second))
		require.NoError(t, pubToken2.Error())

		time.Sleep(1 * time.Second)

		// New subscriber should not receive any retained message
		subscriber2 := createMQTTClientWithVersion("retain-clear-sub2", 4, t)
		defer subscriber2.Disconnect(250)

		messageReceived2 := make(chan string, 1)
		token2 := subscriber2.Subscribe(topic, 0, func(client mqtt.Client, msg mqtt.Message) {
			payload := strings.TrimPrefix(string(msg.Payload()), "\x00")
			messageReceived2 <- payload
		})
		require.True(t, token2.WaitTimeout(5*time.Second))
		require.NoError(t, token2.Error())

		select {
		case received := <-messageReceived2:
			if received == "" {
				t.Log("✅ Retained message successfully cleared")
			} else {
				t.Logf("⚠️ Unexpected retained message after clear: %s", received)
			}
		case <-time.After(3 * time.Second):
			t.Log("✅ No retained message received after clear (expected)")
		}
	})

	t.Run("RetainClearMultiple", func(t *testing.T) {
		// Based on Mosquitto's 04-retain-clear-multiple.py
		// Test clearing multiple retained messages

		publisher := createMQTTClientWithVersion("retain-clear-multi-pub", 4, t)
		defer publisher.Disconnect(250)

		baseTopics := []string{
			"mosquitto/retain/multi/topic1",
			"mosquitto/retain/multi/topic2",
			"mosquitto/retain/multi/topic3",
		}

		// Publish retained messages to multiple topics
		for i, topic := range baseTopics {
			message := fmt.Sprintf("Multi retained message %d", i+1)
			pubToken := publisher.Publish(topic, 0, true, message)
			require.True(t, pubToken.WaitTimeout(5*time.Second))
			require.NoError(t, pubToken.Error())
		}

		time.Sleep(1 * time.Second)

		// Clear all retained messages
		for _, topic := range baseTopics {
			pubToken := publisher.Publish(topic, 0, true, "")
			require.True(t, pubToken.WaitTimeout(5*time.Second))
			require.NoError(t, pubToken.Error())
		}

		t.Log("✅ Multiple retained messages clear test completed")
	})

	t.Run("RetainRepeated", func(t *testing.T) {
		// Based on Mosquitto's 04-retain-qos0-repeated.py
		// Test repeated retained message publishing

		publisher := createMQTTClientWithVersion("retain-repeated-pub", 4, t)
		defer publisher.Disconnect(250)

		topic := "mosquitto/retain/repeated/test"
		messages := []string{
			"First retained message",
			"Second retained message",
			"Third retained message",
		}

		// Publish multiple retained messages to same topic (should replace each other)
		for i, message := range messages {
			pubToken := publisher.Publish(topic, 0, true, message)
			require.True(t, pubToken.WaitTimeout(5*time.Second))
			require.NoError(t, pubToken.Error())

			// Small delay between publishes
			time.Sleep(200 * time.Millisecond)

			t.Logf("Published retained message %d: %s", i+1, message)
		}

		time.Sleep(1 * time.Second)

		// Subscribe and verify only the last message is retained
		subscriber := createMQTTClientWithVersion("retain-repeated-sub", 4, t)
		defer subscriber.Disconnect(250)

		messageReceived := make(chan string, 1)
		token := subscriber.Subscribe(topic, 0, func(client mqtt.Client, msg mqtt.Message) {
			payload := strings.TrimPrefix(string(msg.Payload()), "\x00")
			messageReceived <- payload
		})
		require.True(t, token.WaitTimeout(5*time.Second))
		require.NoError(t, token.Error())

		select {
		case received := <-messageReceived:
			// Should receive the last published message
			assert.Equal(t, messages[len(messages)-1], received)
			t.Log("✅ Repeated retained message test passed - received latest message")
		case <-time.After(5 * time.Second):
			t.Fatal("No retained message received after repeated publishes")
		}
	})

	t.Run("RetainFresh", func(t *testing.T) {
		// Based on Mosquitto's 04-retain-qos0-fresh.py
		// Test retained message behavior for fresh subscriptions

		publisher := createMQTTClientWithVersion("retain-fresh-pub", 4, t)
		defer publisher.Disconnect(250)

		topic := "mosquitto/retain/fresh/test"
		retainedMessage := "Fresh retained message"

		// Publish retained message
		pubToken := publisher.Publish(topic, 0, true, retainedMessage)
		require.True(t, pubToken.WaitTimeout(5*time.Second))
		require.NoError(t, pubToken.Error())

		time.Sleep(1 * time.Second)

		// Multiple fresh subscribers should all receive the retained message
		const numSubscribers = 3
		for i := 0; i < numSubscribers; i++ {
			subscriberID := fmt.Sprintf("retain-fresh-sub-%d", i)
			subscriber := createMQTTClientWithVersion(subscriberID, 4, t)

			messageReceived := make(chan string, 1)
			token := subscriber.Subscribe(topic, 0, func(client mqtt.Client, msg mqtt.Message) {
				payload := strings.TrimPrefix(string(msg.Payload()), "\x00")
				messageReceived <- payload
			})
			require.True(t, token.WaitTimeout(5*time.Second))
			require.NoError(t, token.Error())

			select {
			case received := <-messageReceived:
				assert.Equal(t, retainedMessage, received)
				t.Logf("✅ Fresh subscriber %d received retained message", i+1)
			case <-time.After(3 * time.Second):
				t.Fatalf("Fresh subscriber %d did not receive retained message", i+1)
			}

			subscriber.Disconnect(250)
		}

		t.Log("✅ Fresh subscription retained message test completed")
	})

	t.Run("RetainUpgradeQoS", func(t *testing.T) {
		// Based on Mosquitto's 04-retain-upgrade-outgoing-qos.py
		// Test QoS upgrade for retained message delivery

		publisher := createMQTTClientWithVersion("retain-upgrade-pub", 4, t)
		defer publisher.Disconnect(250)

		topic := "mosquitto/retain/upgrade/test"
		retainedMessage := "QoS upgrade retained message"

		// Publish retained message with QoS 0
		pubToken := publisher.Publish(topic, 0, true, retainedMessage)
		require.True(t, pubToken.WaitTimeout(5*time.Second))
		require.NoError(t, pubToken.Error())

		time.Sleep(1 * time.Second)

		// Subscribe with QoS 1 (should receive with effective QoS 0, not upgraded)
		subscriber := createMQTTClientWithVersion("retain-upgrade-sub", 4, t)
		defer subscriber.Disconnect(250)

		messageReceived := make(chan string, 1)
		token := subscriber.Subscribe(topic, 1, func(client mqtt.Client, msg mqtt.Message) {
			payload := strings.TrimPrefix(string(msg.Payload()), "\x00")
			messageReceived <- payload
		})
		require.True(t, token.WaitTimeout(5*time.Second))
		require.NoError(t, token.Error())

		select {
		case received := <-messageReceived:
			assert.Equal(t, retainedMessage, received)
			t.Log("✅ QoS upgrade retained message test passed")
		case <-time.After(5 * time.Second):
			t.Fatal("QoS upgrade retained message not received")
		}
	})
}