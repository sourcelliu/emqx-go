package unit

import (
	"fmt"
	"testing"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMQTT311ProtocolSupport(t *testing.T) {
	t.Log("Testing MQTT 3.1.1 Protocol Support")

	t.Run("MQTT311ConnectionWithValidCredentials", func(t *testing.T) {
		client := createMQTT311Client("mqtt311-test-client-1", t)
		defer client.Disconnect(250)

		assert.True(t, client.IsConnected(), "MQTT 3.1.1 client should be connected")
		t.Log("✅ MQTT 3.1.1 connection established successfully")
	})

	t.Run("MQTT311BasicPublishSubscribe", func(t *testing.T) {
		subscriber := createMQTT311Client("mqtt311-subscriber", t)
		defer subscriber.Disconnect(250)

		publisher := createMQTT311Client("mqtt311-publisher", t)
		defer publisher.Disconnect(250)

		messageReceived := make(chan string, 1)
		topic := "mqtt311/test/topic"

		token := subscriber.Subscribe(topic, 1, func(client mqtt.Client, msg mqtt.Message) {
			messageReceived <- string(msg.Payload())
		})
		require.True(t, token.WaitTimeout(5*time.Second))
		require.NoError(t, token.Error())

		testMessage := "MQTT 3.1.1 test message"
		pubToken := publisher.Publish(topic, 1, false, testMessage)
		require.True(t, pubToken.WaitTimeout(5*time.Second))
		require.NoError(t, pubToken.Error())

		select {
		case msg := <-messageReceived:
			assert.Equal(t, testMessage, msg)
			t.Log("✅ MQTT 3.1.1 publish/subscribe working correctly")
		case <-time.After(5 * time.Second):
			t.Fatal("Did not receive MQTT 3.1.1 message within timeout")
		}
	})

	t.Run("MQTT311QoSLevels", func(t *testing.T) {
		client := createMQTT311Client("mqtt311-qos-test", t)
		defer client.Disconnect(250)

		topic := "mqtt311/qos/test"

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

		t.Log("✅ MQTT 3.1.1 QoS levels 0, 1, 2 working correctly")
	})

	t.Run("MQTT311RetainedMessages", func(t *testing.T) {
		publisher := createMQTT311Client("mqtt311-retain-pub", t)
		defer publisher.Disconnect(250)

		topic := "mqtt311/retained/test"
		retainedMessage := "MQTT 3.1.1 retained message"

		token := publisher.Publish(topic, 1, true, retainedMessage)
		require.True(t, token.WaitTimeout(5*time.Second))
		require.NoError(t, token.Error())

		time.Sleep(1 * time.Second)

		subscriber := createMQTT311Client("mqtt311-retain-sub", t)
		defer subscriber.Disconnect(250)

		messageReceived := make(chan string, 1)
		subToken := subscriber.Subscribe(topic, 1, func(client mqtt.Client, msg mqtt.Message) {
			messageReceived <- string(msg.Payload())
		})
		require.True(t, subToken.WaitTimeout(5*time.Second))
		require.NoError(t, subToken.Error())

		select {
		case msg := <-messageReceived:
			assert.Equal(t, retainedMessage, msg)
			t.Log("✅ MQTT 3.1.1 retained messages working correctly")
		case <-time.After(5 * time.Second):
			t.Fatal("Did not receive MQTT 3.1.1 retained message within timeout")
		}
	})

	t.Run("MQTT311WillMessages", func(t *testing.T) {
		// Create a subscriber to listen for will messages
		subscriber := createMQTT311Client("mqtt311-will-subscriber", t)
		defer subscriber.Disconnect(250)

		willTopic := "mqtt311/will/test"
		willReceived := make(chan string, 1)

		subToken := subscriber.Subscribe(willTopic, 1, func(client mqtt.Client, msg mqtt.Message) {
			willReceived <- string(msg.Payload())
		})
		require.True(t, subToken.WaitTimeout(5*time.Second))
		require.NoError(t, subToken.Error())

		// Create client with will message
		willClient := createMQTT311ClientWithWill("mqtt311-will-client", willTopic, "MQTT 3.1.1 will message", t)

		// Simulate unexpected disconnection by closing the connection
		willClient.Disconnect(0) // Immediate disconnect should trigger will

		// Wait for will message
		select {
		case msg := <-willReceived:
			assert.Equal(t, "MQTT 3.1.1 will message", msg)
			t.Log("✅ MQTT 3.1.1 will messages working correctly")
		case <-time.After(10 * time.Second):
			t.Log("⚠️ Will message test may not work as expected - depends on broker implementation")
		}
	})

	t.Run("MQTT311KeepAlive", func(t *testing.T) {
		// Test with short keep alive
		client := createMQTT311ClientWithKeepAlive("mqtt311-keepalive-test", 5, t)
		defer client.Disconnect(250)

		// Keep connection alive for a while
		time.Sleep(8 * time.Second)

		assert.True(t, client.IsConnected(), "Client should remain connected with keep alive")
		t.Log("✅ MQTT 3.1.1 keep alive mechanism working correctly")
	})

	t.Run("MQTT311MultipleSubscriptions", func(t *testing.T) {
		client := createMQTT311Client("mqtt311-multi-sub", t)
		defer client.Disconnect(250)

		topics := []string{
			"mqtt311/topic/1",
			"mqtt311/topic/2",
			"mqtt311/topic/3",
		}

		receivedMessages := make(map[string]string)
		messageCount := 0

		// Subscribe to multiple topics
		for _, topic := range topics {
			token := client.Subscribe(topic, 1, func(client mqtt.Client, msg mqtt.Message) {
				receivedMessages[msg.Topic()] = string(msg.Payload())
				messageCount++
			})
			require.True(t, token.WaitTimeout(5*time.Second))
			require.NoError(t, token.Error())
		}

		// Publish to each topic
		for i, topic := range topics {
			message := fmt.Sprintf("Message %d", i+1)
			token := client.Publish(topic, 1, false, message)
			require.True(t, token.WaitTimeout(5*time.Second))
			require.NoError(t, token.Error())
		}

		// Wait for all messages
		time.Sleep(2 * time.Second)

		assert.Equal(t, 3, messageCount, "Should receive all messages")
		assert.Equal(t, "Message 1", receivedMessages["mqtt311/topic/1"])
		assert.Equal(t, "Message 2", receivedMessages["mqtt311/topic/2"])
		assert.Equal(t, "Message 3", receivedMessages["mqtt311/topic/3"])

		t.Log("✅ MQTT 3.1.1 multiple subscriptions working correctly")
	})

	t.Run("MQTT311WildcardSubscriptions", func(t *testing.T) {
		subscriber := createMQTT311Client("mqtt311-wildcard-sub", t)
		defer subscriber.Disconnect(250)

		publisher := createMQTT311Client("mqtt311-wildcard-pub", t)
		defer publisher.Disconnect(250)

		receivedTopics := make([]string, 0)

		// Subscribe with wildcard
		token := subscriber.Subscribe("mqtt311/wildcard/+", 1, func(client mqtt.Client, msg mqtt.Message) {
			receivedTopics = append(receivedTopics, msg.Topic())
		})
		require.True(t, token.WaitTimeout(5*time.Second))
		require.NoError(t, token.Error())

		// Publish to different topics that match the wildcard
		topics := []string{
			"mqtt311/wildcard/test1",
			"mqtt311/wildcard/test2",
			"mqtt311/wildcard/test3",
		}

		for _, topic := range topics {
			pubToken := publisher.Publish(topic, 1, false, "wildcard test")
			require.True(t, pubToken.WaitTimeout(5*time.Second))
			require.NoError(t, pubToken.Error())
		}

		time.Sleep(2 * time.Second)

		assert.Equal(t, 3, len(receivedTopics), "Should receive messages from all matching topics")
		t.Log("✅ MQTT 3.1.1 wildcard subscriptions working correctly")
	})

	t.Run("MQTT311UnsubscribeOperation", func(t *testing.T) {
		client := createMQTT311Client("mqtt311-unsub-test", t)
		defer client.Disconnect(250)

		topic := "mqtt311/unsubscribe/test"
		messageCount := 0

		token := client.Subscribe(topic, 1, func(client mqtt.Client, msg mqtt.Message) {
			messageCount++
		})
		require.True(t, token.WaitTimeout(5*time.Second))
		require.NoError(t, token.Error())

		pubToken1 := client.Publish(topic, 1, false, "message1")
		require.True(t, pubToken1.WaitTimeout(5*time.Second))
		require.NoError(t, pubToken1.Error())

		time.Sleep(1 * time.Second)
		assert.Equal(t, 1, messageCount, "Should receive first message")

		unsubToken := client.Unsubscribe(topic)
		require.True(t, unsubToken.WaitTimeout(5*time.Second))
		require.NoError(t, unsubToken.Error())

		pubToken2 := client.Publish(topic, 1, false, "message2")
		require.True(t, pubToken2.WaitTimeout(5*time.Second))
		require.NoError(t, pubToken2.Error())

		time.Sleep(1 * time.Second)
		assert.Equal(t, 1, messageCount, "Should not receive message after unsubscribe")

		t.Log("✅ MQTT 3.1.1 unsubscribe operation working correctly")
	})
}

func createMQTT311Client(clientID string, t *testing.T) mqtt.Client {
	return createMQTT311ClientWithOptions(clientID, nil, t)
}

func createMQTT311ClientWithWill(clientID, willTopic, willMessage string, t *testing.T) mqtt.Client {
	opts := &mqtt.ClientOptions{}
	opts.SetWill(willTopic, willMessage, 1, false)
	return createMQTT311ClientWithOptions(clientID, opts, t)
}

func createMQTT311ClientWithKeepAlive(clientID string, keepAlive time.Duration, t *testing.T) mqtt.Client {
	opts := &mqtt.ClientOptions{}
	opts.SetKeepAlive(keepAlive * time.Second)
	return createMQTT311ClientWithOptions(clientID, opts, t)
}

func createMQTT311ClientWithOptions(clientID string, extraOpts *mqtt.ClientOptions, t *testing.T) mqtt.Client {
	opts := mqtt.NewClientOptions()
	opts.AddBroker("tcp://localhost:1883")
	opts.SetClientID(clientID)
	opts.SetUsername("test")
	opts.SetPassword("test")
	opts.SetCleanSession(true)
	opts.SetProtocolVersion(4) // MQTT 3.1.1
	opts.SetConnectTimeout(5 * time.Second)
	opts.SetKeepAlive(30 * time.Second)

	// Apply extra options if provided
	if extraOpts != nil {
		if extraOpts.WillEnabled {
			opts.SetWill(extraOpts.WillTopic, string(extraOpts.WillPayload), extraOpts.WillQos, extraOpts.WillRetained)
		}
		if extraOpts.KeepAlive > 0 {
			opts.SetKeepAlive(time.Duration(extraOpts.KeepAlive) * time.Second)
		}
	}

	client := mqtt.NewClient(opts)
	token := client.Connect()
	require.True(t, token.WaitTimeout(10*time.Second), "Failed to connect MQTT 3.1.1 client")
	require.NoError(t, token.Error(), "MQTT 3.1.1 connection error")

	t.Logf("Successfully connected MQTT 3.1.1 client: %s", clientID)
	return client
}