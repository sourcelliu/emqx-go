package e2e

import (
	"testing"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSubscriptionBugFixes(t *testing.T) {
	t.Log("Testing SUBSCRIBE and UNSUBSCRIBE bug fixes")

	// Test basic subscribe functionality
	t.Run("BasicSubscriptionFunctionality", func(t *testing.T) {
		subscriber := createAuthenticatedMQTTClient("test-subscriber-1", t)
		defer subscriber.Disconnect(250)

		publisher := createAuthenticatedMQTTClient("test-publisher-1", t)
		defer publisher.Disconnect(250)

		// Test subscription to a valid topic
		messageReceived := make(chan string, 1)
		token := subscriber.Subscribe("test/topic", 1, func(client mqtt.Client, msg mqtt.Message) {
			messageReceived <- string(msg.Payload())
		})
		require.True(t, token.WaitTimeout(5*time.Second))
		require.NoError(t, token.Error())

		// Publish a message
		pubToken := publisher.Publish("test/topic", 1, false, "test message")
		require.True(t, pubToken.WaitTimeout(5*time.Second))
		require.NoError(t, pubToken.Error())

		// Wait for message
		select {
		case msg := <-messageReceived:
			assert.Equal(t, "test message", msg)
			t.Log("✅ Basic subscription functionality working")
		case <-time.After(5 * time.Second):
			t.Fatal("Did not receive message within timeout")
		}
	})

	// Test unsubscribe functionality
	t.Run("UnsubscribeFunctionality", func(t *testing.T) {
		client := createAuthenticatedMQTTClient("test-unsubscribe-1", t)
		defer client.Disconnect(250)

		// Subscribe first
		messageCount := 0
		token := client.Subscribe("test/unsubscribe", 1, func(client mqtt.Client, msg mqtt.Message) {
			messageCount++
		})
		require.True(t, token.WaitTimeout(5*time.Second))
		require.NoError(t, token.Error())

		// Publish a message (should be received)
		pubToken := client.Publish("test/unsubscribe", 1, false, "message1")
		require.True(t, pubToken.WaitTimeout(5*time.Second))
		require.NoError(t, pubToken.Error())

		time.Sleep(1 * time.Second)
		assert.Equal(t, 1, messageCount, "Should have received first message")

		// Now unsubscribe
		unsubToken := client.Unsubscribe("test/unsubscribe")
		require.True(t, unsubToken.WaitTimeout(5*time.Second))
		require.NoError(t, unsubToken.Error())

		// Publish another message (should NOT be received)
		pubToken2 := client.Publish("test/unsubscribe", 1, false, "message2")
		require.True(t, pubToken2.WaitTimeout(5*time.Second))
		require.NoError(t, pubToken2.Error())

		time.Sleep(1 * time.Second)
		assert.Equal(t, 1, messageCount, "Should not have received second message after unsubscribe")

		t.Log("✅ Unsubscribe functionality working")
	})

	// Test that server handles UNSUBSCRIBE packets properly (no "unhandled packet type" errors)
	t.Run("NoUnhandledPacketErrors", func(t *testing.T) {
		client := createAuthenticatedMQTTClient("test-packets-1", t)
		defer client.Disconnect(250)

		// Subscribe and immediately unsubscribe multiple times
		for i := 0; i < 3; i++ {
			topic := "test/packet/handling"

			token := client.Subscribe(topic, 1, nil)
			require.True(t, token.WaitTimeout(5*time.Second))
			require.NoError(t, token.Error())

			unsubToken := client.Unsubscribe(topic)
			require.True(t, unsubToken.WaitTimeout(5*time.Second))
			require.NoError(t, unsubToken.Error())
		}

		t.Log("✅ No unhandled packet errors during subscribe/unsubscribe operations")
	})
}

func createAuthenticatedMQTTClient(clientID string, t *testing.T) mqtt.Client {
	opts := mqtt.NewClientOptions()
	opts.AddBroker("tcp://localhost:1883")
	opts.SetClientID(clientID)
	opts.SetUsername("test")
	opts.SetPassword("test")
	opts.SetCleanSession(true)
	opts.SetConnectTimeout(5 * time.Second)

	client := mqtt.NewClient(opts)
	token := client.Connect()
	require.True(t, token.WaitTimeout(10*time.Second), "Failed to connect to MQTT broker")
	require.NoError(t, token.Error(), "Connection error")

	t.Logf("Successfully connected MQTT client: %s", clientID)
	return client
}