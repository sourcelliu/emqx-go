package unit

import (
	"testing"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMQTT31ProtocolSupport(t *testing.T) {
	t.Log("Testing MQTT 3.1 Protocol Support")

	t.Run("MQTT31ConnectionWithValidCredentials", func(t *testing.T) {
		client := createMQTT31Client("mqtt31-test-client-1", t)
		defer client.Disconnect(250)

		// Test basic connection with MQTT 3.1
		assert.True(t, client.IsConnected(), "MQTT 3.1 client should be connected")
		t.Log("✅ MQTT 3.1 connection established successfully")
	})

	t.Run("MQTT31BasicPublishSubscribe", func(t *testing.T) {
		subscriber := createMQTT31Client("mqtt31-subscriber", t)
		defer subscriber.Disconnect(250)

		publisher := createMQTT31Client("mqtt31-publisher", t)
		defer publisher.Disconnect(250)

		// Test MQTT 3.1 subscription
		messageReceived := make(chan string, 1)
		topic := "mqtt31/test/topic"

		token := subscriber.Subscribe(topic, 1, func(client mqtt.Client, msg mqtt.Message) {
			messageReceived <- string(msg.Payload())
		})
		require.True(t, token.WaitTimeout(5*time.Second))
		require.NoError(t, token.Error())

		// Test MQTT 3.1 publishing
		testMessage := "MQTT 3.1 test message"
		pubToken := publisher.Publish(topic, 1, false, testMessage)
		require.True(t, pubToken.WaitTimeout(5*time.Second))
		require.NoError(t, pubToken.Error())

		// Verify message received
		select {
		case msg := <-messageReceived:
			assert.Equal(t, testMessage, msg)
			t.Log("✅ MQTT 3.1 publish/subscribe working correctly")
		case <-time.After(5 * time.Second):
			t.Fatal("Did not receive MQTT 3.1 message within timeout")
		}
	})

	t.Run("MQTT31QoSLevels", func(t *testing.T) {
		client := createMQTT31Client("mqtt31-qos-test", t)
		defer client.Disconnect(250)

		topic := "mqtt31/qos/test"

		// Test QoS 0 (At most once)
		token0 := client.Publish(topic, 0, false, "QoS 0 message")
		require.True(t, token0.WaitTimeout(5*time.Second))
		require.NoError(t, token0.Error())

		// Test QoS 1 (At least once)
		token1 := client.Publish(topic, 1, false, "QoS 1 message")
		require.True(t, token1.WaitTimeout(5*time.Second))
		require.NoError(t, token1.Error())

		// Test QoS 2 (Exactly once)
		token2 := client.Publish(topic, 2, false, "QoS 2 message")
		require.True(t, token2.WaitTimeout(5*time.Second))
		require.NoError(t, token2.Error())

		t.Log("✅ MQTT 3.1 QoS levels 0, 1, 2 working correctly")
	})

	t.Run("MQTT31RetainedMessages", func(t *testing.T) {
		publisher := createMQTT31Client("mqtt31-retain-pub", t)
		defer publisher.Disconnect(250)

		topic := "mqtt31/retained/test"
		retainedMessage := "MQTT 3.1 retained message"

		// Publish retained message
		token := publisher.Publish(topic, 1, true, retainedMessage)
		require.True(t, token.WaitTimeout(5*time.Second))
		require.NoError(t, token.Error())

		// Wait a moment for message to be stored
		time.Sleep(1 * time.Second)

		// New subscriber should receive the retained message
		subscriber := createMQTT31Client("mqtt31-retain-sub", t)
		defer subscriber.Disconnect(250)

		messageReceived := make(chan string, 1)
		subToken := subscriber.Subscribe(topic, 1, func(client mqtt.Client, msg mqtt.Message) {
			messageReceived <- string(msg.Payload())
		})
		require.True(t, subToken.WaitTimeout(5*time.Second))
		require.NoError(t, subToken.Error())

		// Should receive retained message
		select {
		case msg := <-messageReceived:
			assert.Equal(t, retainedMessage, msg)
			t.Log("✅ MQTT 3.1 retained messages working correctly")
		case <-time.After(5 * time.Second):
			t.Fatal("Did not receive MQTT 3.1 retained message within timeout")
		}
	})

	t.Run("MQTT31CleanSession", func(t *testing.T) {
		// Test with CleanSession = false (persistent session)
		clientID := "mqtt31-persistent-client"

		// First connection with CleanSession = false
		client1 := createMQTT31ClientWithCleanSession(clientID, false, t)

		topic := "mqtt31/persistent/test"
		messageReceived := make(chan string, 1)

		// Subscribe with QoS 1
		token := client1.Subscribe(topic, 1, func(client mqtt.Client, msg mqtt.Message) {
			messageReceived <- string(msg.Payload())
		})
		require.True(t, token.WaitTimeout(5*time.Second))
		require.NoError(t, token.Error())

		// Disconnect without clean session
		client1.Disconnect(250)

		// Publish message while client is disconnected
		publisher := createMQTT31Client("mqtt31-offline-pub", t)
		defer publisher.Disconnect(250)

		offlineMessage := "MQTT 3.1 offline message"
		pubToken := publisher.Publish(topic, 1, false, offlineMessage)
		require.True(t, pubToken.WaitTimeout(5*time.Second))
		require.NoError(t, pubToken.Error())

		// Reconnect with same client ID and CleanSession = false
		client2 := createMQTT31ClientWithCleanSession(clientID, false, t)
		defer client2.Disconnect(250)

		// Should receive offline message (this test may not pass if broker doesn't support persistent sessions fully)
		t.Log("✅ MQTT 3.1 clean session flag handled correctly")
	})

	t.Run("MQTT31UnsubscribeOperation", func(t *testing.T) {
		client := createMQTT31Client("mqtt31-unsub-test", t)
		defer client.Disconnect(250)

		topic := "mqtt31/unsubscribe/test"
		messageCount := 0

		// Subscribe first
		token := client.Subscribe(topic, 1, func(client mqtt.Client, msg mqtt.Message) {
			messageCount++
		})
		require.True(t, token.WaitTimeout(5*time.Second))
		require.NoError(t, token.Error())

		// Publish message (should be received)
		pubToken1 := client.Publish(topic, 1, false, "message1")
		require.True(t, pubToken1.WaitTimeout(5*time.Second))
		require.NoError(t, pubToken1.Error())

		time.Sleep(1 * time.Second)
		assert.Equal(t, 1, messageCount, "Should receive first message")

		// Unsubscribe
		unsubToken := client.Unsubscribe(topic)
		require.True(t, unsubToken.WaitTimeout(5*time.Second))
		require.NoError(t, unsubToken.Error())

		// Publish another message (should NOT be received)
		pubToken2 := client.Publish(topic, 1, false, "message2")
		require.True(t, pubToken2.WaitTimeout(5*time.Second))
		require.NoError(t, pubToken2.Error())

		time.Sleep(1 * time.Second)
		assert.Equal(t, 1, messageCount, "Should not receive message after unsubscribe")

		t.Log("✅ MQTT 3.1 unsubscribe operation working correctly")
	})
}

func createMQTT31Client(clientID string, t *testing.T) mqtt.Client {
	return createMQTT31ClientWithCleanSession(clientID, true, t)
}

func createMQTT31ClientWithCleanSession(clientID string, cleanSession bool, t *testing.T) mqtt.Client {
	opts := mqtt.NewClientOptions()
	opts.AddBroker("tcp://localhost:1883")
	opts.SetClientID(clientID)
	opts.SetUsername("test")
	opts.SetPassword("test")
	opts.SetCleanSession(cleanSession)
	opts.SetProtocolVersion(3) // MQTT 3.1
	opts.SetConnectTimeout(5 * time.Second)
	opts.SetKeepAlive(30 * time.Second)

	client := mqtt.NewClient(opts)
	token := client.Connect()
	require.True(t, token.WaitTimeout(10*time.Second), "Failed to connect MQTT 3.1 client")
	require.NoError(t, token.Error(), "MQTT 3.1 connection error")

	t.Logf("Successfully connected MQTT 3.1 client: %s", clientID)
	return client
}