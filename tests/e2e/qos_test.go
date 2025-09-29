package e2e

import (
	"sync"
	"testing"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	qosBrokerURL = "tcp://localhost:1883"
	qosTestTopic = "test/emqx-go/qos"
)

func TestMQTTQoS0PublishSubscribe(t *testing.T) {
	t.Log("Starting QoS 0 (At most once) test")

	// Setup subscriber with QoS 0
	subscriber := setupQoSMQTTClient("qos0-subscriber", t)
	defer subscriber.Disconnect(250)

	// Setup publisher
	publisher := setupQoSMQTTClient("qos0-publisher", t)
	defer publisher.Disconnect(250)

	// Channel to receive messages
	messageReceived := make(chan string, 10)
	var receivedMessages []string
	var mutex sync.Mutex

	// Subscribe with QoS 0
	token := subscriber.Subscribe(qosTestTopic+"/qos0", 0, func(client mqtt.Client, msg mqtt.Message) {
		mutex.Lock()
		receivedMsg := string(msg.Payload())
		receivedMessages = append(receivedMessages, receivedMsg)
		mutex.Unlock()

		t.Logf("QoS 0 - Received message: %s", receivedMsg)
		messageReceived <- receivedMsg
	})

	require.True(t, token.Wait(), "Failed to subscribe with QoS 0")
	require.NoError(t, token.Error(), "QoS 0 subscribe error")
	t.Log("Successfully subscribed with QoS 0")

	// Wait for subscription to be established
	time.Sleep(100 * time.Millisecond)

	// Publish with QoS 0
	testMessage := "QoS 0 Test Message"
	t.Logf("Publishing with QoS 0: %s", testMessage)

	pubToken := publisher.Publish(qosTestTopic+"/qos0", 0, false, testMessage)
	require.True(t, pubToken.Wait(), "Failed to publish with QoS 0")
	require.NoError(t, pubToken.Error(), "QoS 0 publish error")

	// Wait for message
	select {
	case receivedMsg := <-messageReceived:
		assert.Equal(t, testMessage, receivedMsg, "QoS 0 message should match")
		t.Log("QoS 0 test completed successfully")
	case <-time.After(3 * time.Second):
		t.Fatal("Timeout waiting for QoS 0 message")
	}
}

func TestMQTTQoS1PublishSubscribe(t *testing.T) {
	t.Log("Starting QoS 1 (At least once) test")

	// Setup subscriber with QoS 1
	subscriber := setupQoSMQTTClient("qos1-subscriber", t)
	defer subscriber.Disconnect(250)

	// Setup publisher
	publisher := setupQoSMQTTClient("qos1-publisher", t)
	defer publisher.Disconnect(250)

	// Channel to receive messages
	messageReceived := make(chan string, 10)
	var receivedMessages []string
	var mutex sync.Mutex

	// Subscribe with QoS 1
	token := subscriber.Subscribe(qosTestTopic+"/qos1", 1, func(client mqtt.Client, msg mqtt.Message) {
		mutex.Lock()
		receivedMsg := string(msg.Payload())
		receivedMessages = append(receivedMessages, receivedMsg)
		mutex.Unlock()

		t.Logf("QoS 1 - Received message: %s, QoS: %d", receivedMsg, msg.Qos())
		messageReceived <- receivedMsg
	})

	require.True(t, token.Wait(), "Failed to subscribe with QoS 1")
	require.NoError(t, token.Error(), "QoS 1 subscribe error")
	t.Log("Successfully subscribed with QoS 1")

	// Wait for subscription to be established
	time.Sleep(100 * time.Millisecond)

	// Publish with QoS 1
	testMessage := "QoS 1 Test Message"
	t.Logf("Publishing with QoS 1: %s", testMessage)

	pubToken := publisher.Publish(qosTestTopic+"/qos1", 1, false, testMessage)
	require.True(t, pubToken.Wait(), "Failed to publish with QoS 1")
	require.NoError(t, pubToken.Error(), "QoS 1 publish error")

	// Wait for message
	select {
	case receivedMsg := <-messageReceived:
		assert.Equal(t, testMessage, receivedMsg, "QoS 1 message should match")
		t.Log("QoS 1 test completed successfully")
	case <-time.After(3 * time.Second):
		t.Fatal("Timeout waiting for QoS 1 message")
	}
}

func TestMQTTQoS2PublishSubscribe(t *testing.T) {
	t.Log("Starting QoS 2 (Exactly once) test")

	// Setup subscriber with QoS 2
	subscriber := setupQoSMQTTClient("qos2-subscriber", t)
	defer subscriber.Disconnect(250)

	// Setup publisher
	publisher := setupQoSMQTTClient("qos2-publisher", t)
	defer publisher.Disconnect(250)

	// Channel to receive messages
	messageReceived := make(chan string, 10)
	var receivedMessages []string
	var mutex sync.Mutex

	// Subscribe with QoS 2
	token := subscriber.Subscribe(qosTestTopic+"/qos2", 2, func(client mqtt.Client, msg mqtt.Message) {
		mutex.Lock()
		receivedMsg := string(msg.Payload())
		receivedMessages = append(receivedMessages, receivedMsg)
		mutex.Unlock()

		t.Logf("QoS 2 - Received message: %s, QoS: %d", receivedMsg, msg.Qos())
		messageReceived <- receivedMsg
	})

	require.True(t, token.Wait(), "Failed to subscribe with QoS 2")
	require.NoError(t, token.Error(), "QoS 2 subscribe error")
	t.Log("Successfully subscribed with QoS 2")

	// Wait for subscription to be established
	time.Sleep(100 * time.Millisecond)

	// Publish with QoS 2
	testMessage := "QoS 2 Test Message"
	t.Logf("Publishing with QoS 2: %s", testMessage)

	pubToken := publisher.Publish(qosTestTopic+"/qos2", 2, false, testMessage)
	require.True(t, pubToken.Wait(), "Failed to publish with QoS 2")
	require.NoError(t, pubToken.Error(), "QoS 2 publish error")

	// Wait for message
	select {
	case receivedMsg := <-messageReceived:
		assert.Equal(t, testMessage, receivedMsg, "QoS 2 message should match")
		t.Log("QoS 2 test completed successfully")
	case <-time.After(5 * time.Second):
		t.Fatal("Timeout waiting for QoS 2 message")
	}
}

func TestMQTTMixedQoSLevels(t *testing.T) {
	t.Log("Starting mixed QoS levels test")

	// Setup multiple clients with different QoS levels
	qos0Sub := setupQoSMQTTClient("mixed-qos0-sub", t)
	defer qos0Sub.Disconnect(250)

	qos1Sub := setupQoSMQTTClient("mixed-qos1-sub", t)
	defer qos1Sub.Disconnect(250)

	qos2Sub := setupQoSMQTTClient("mixed-qos2-sub", t)
	defer qos2Sub.Disconnect(250)

	publisher := setupQoSMQTTClient("mixed-qos-pub", t)
	defer publisher.Disconnect(250)

	// Channels to receive messages
	qos0Received := make(chan string, 5)
	qos1Received := make(chan string, 5)
	qos2Received := make(chan string, 5)

	// Subscribe with different QoS levels to the same topic
	mixedTopic := qosTestTopic + "/mixed"

	// QoS 0 subscriber
	token0 := qos0Sub.Subscribe(mixedTopic, 0, func(client mqtt.Client, msg mqtt.Message) {
		receivedMsg := string(msg.Payload())
		t.Logf("QoS 0 subscriber received: %s, QoS: %d", receivedMsg, msg.Qos())
		qos0Received <- receivedMsg
	})
	require.True(t, token0.Wait(), "Failed to subscribe QoS 0")

	// QoS 1 subscriber
	token1 := qos1Sub.Subscribe(mixedTopic, 1, func(client mqtt.Client, msg mqtt.Message) {
		receivedMsg := string(msg.Payload())
		t.Logf("QoS 1 subscriber received: %s, QoS: %d", receivedMsg, msg.Qos())
		qos1Received <- receivedMsg
	})
	require.True(t, token1.Wait(), "Failed to subscribe QoS 1")

	// QoS 2 subscriber
	token2 := qos2Sub.Subscribe(mixedTopic, 2, func(client mqtt.Client, msg mqtt.Message) {
		receivedMsg := string(msg.Payload())
		t.Logf("QoS 2 subscriber received: %s, QoS: %d", receivedMsg, msg.Qos())
		qos2Received <- receivedMsg
	})
	require.True(t, token2.Wait(), "Failed to subscribe QoS 2")

	// Wait for subscriptions
	time.Sleep(200 * time.Millisecond)

	// Publish with QoS 1 to all subscribers
	testMessage := "Mixed QoS Test Message"
	t.Logf("Publishing with QoS 1 to mixed subscribers: %s", testMessage)

	pubToken := publisher.Publish(mixedTopic, 1, false, testMessage)
	require.True(t, pubToken.Wait(), "Failed to publish to mixed QoS subscribers")
	require.NoError(t, pubToken.Error(), "Mixed QoS publish error")

	// Verify all subscribers receive the message
	timeout := time.After(5 * time.Second)
	receivedCount := 0

	for receivedCount < 3 {
		select {
		case msg := <-qos0Received:
			assert.Equal(t, testMessage, msg, "QoS 0 subscriber should receive message")
			receivedCount++
		case msg := <-qos1Received:
			assert.Equal(t, testMessage, msg, "QoS 1 subscriber should receive message")
			receivedCount++
		case msg := <-qos2Received:
			assert.Equal(t, testMessage, msg, "QoS 2 subscriber should receive message")
			receivedCount++
		case <-timeout:
			t.Fatalf("Timeout waiting for mixed QoS messages. Received %d out of 3", receivedCount)
		}
	}

	t.Log("Mixed QoS levels test completed successfully")
}

func TestMQTTQoSDowngrade(t *testing.T) {
	t.Log("Starting QoS downgrade test")

	// Setup subscriber with QoS 0
	subscriber := setupQoSMQTTClient("qos-downgrade-sub", t)
	defer subscriber.Disconnect(250)

	// Setup publisher
	publisher := setupQoSMQTTClient("qos-downgrade-pub", t)
	defer publisher.Disconnect(250)

	messageReceived := make(chan mqtt.Message, 1)

	// Subscribe with QoS 0
	token := subscriber.Subscribe(qosTestTopic+"/downgrade", 0, func(client mqtt.Client, msg mqtt.Message) {
		t.Logf("QoS downgrade - Received message: %s, QoS: %d", string(msg.Payload()), msg.Qos())
		messageReceived <- msg
	})

	require.True(t, token.Wait(), "Failed to subscribe with QoS 0")
	require.NoError(t, token.Error(), "QoS downgrade subscribe error")

	// Wait for subscription
	time.Sleep(100 * time.Millisecond)

	// Publish with QoS 2, but subscriber has QoS 0 - should be downgraded
	testMessage := "QoS Downgrade Test Message"
	t.Logf("Publishing with QoS 2 to QoS 0 subscriber: %s", testMessage)

	pubToken := publisher.Publish(qosTestTopic+"/downgrade", 2, false, testMessage)
	require.True(t, pubToken.Wait(), "Failed to publish with QoS 2")
	require.NoError(t, pubToken.Error(), "QoS downgrade publish error")

	// Wait for message and verify QoS is downgraded
	select {
	case msg := <-messageReceived:
		assert.Equal(t, testMessage, string(msg.Payload()), "Message content should match")
		// The received QoS should be the minimum of publish QoS and subscribe QoS
		assert.LessOrEqual(t, int(msg.Qos()), 0, "QoS should be downgraded to subscriber's QoS level")
		t.Log("QoS downgrade test completed successfully")
	case <-time.After(3 * time.Second):
		t.Fatal("Timeout waiting for QoS downgrade message")
	}
}

func setupQoSMQTTClient(clientID string, t *testing.T) mqtt.Client {
	opts := mqtt.NewClientOptions()
	opts.AddBroker(qosBrokerURL)
	opts.SetClientID(clientID)
	opts.SetKeepAlive(60 * time.Second)
	opts.SetDefaultPublishHandler(func(client mqtt.Client, msg mqtt.Message) {
		t.Logf("Unexpected message: %s", msg.Payload())
	})
	opts.SetPingTimeout(1 * time.Second)
	opts.SetConnectTimeout(5 * time.Second)

	// Enable store for QoS 1 and 2 message persistence
	opts.SetStore(mqtt.NewMemoryStore())

	client := mqtt.NewClient(opts)
	token := client.Connect()
	require.True(t, token.Wait(), "Failed to connect to MQTT broker")
	require.NoError(t, token.Error(), "Connection error")

	t.Logf("Successfully connected QoS client: %s", clientID)
	return client
}