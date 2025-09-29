package e2e

import (
	"testing"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	simpleBrokerURL = "tcp://localhost:1883"
	simpleTestTopic = "test/emqx-go/simple"
)

func TestMQTTSimplePublishSubscribe(t *testing.T) {
	t.Log("Starting simple MQTT publish/subscribe test")

	// Setup subscriber
	subscriber := setupSimpleMQTTClient("test-subscriber", t)
	defer subscriber.Disconnect(250)

	// Setup publisher
	publisher := setupSimpleMQTTClient("test-publisher", t)
	defer publisher.Disconnect(250)

	// Channel to receive one message
	messageReceived := make(chan string, 1)

	// Subscribe to test topic
	token := subscriber.Subscribe(simpleTestTopic, 1, func(client mqtt.Client, msg mqtt.Message) {
		receivedMsg := string(msg.Payload())
		t.Logf("Received message: %s", receivedMsg)
		messageReceived <- receivedMsg
	})

	require.True(t, token.Wait(), "Failed to subscribe to topic")
	require.NoError(t, token.Error(), "Subscribe error")
	t.Logf("Successfully subscribed to topic: %s", simpleTestTopic)

	// Wait for subscription to be established
	time.Sleep(100 * time.Millisecond)

	// Publish one test message
	testMessage := "Hello EMQX-Go Simple Test!"
	t.Logf("Publishing message: %s", testMessage)

	pubToken := publisher.Publish(simpleTestTopic, 1, false, testMessage)
	require.True(t, pubToken.Wait(), "Failed to publish message")
	require.NoError(t, pubToken.Error(), "Publish error")

	// Wait for message to be received
	select {
	case receivedMsg := <-messageReceived:
		assert.Equal(t, testMessage, receivedMsg, "Received message should match published message")
		t.Log("Simple MQTT test completed successfully")
	case <-time.After(3 * time.Second):
		t.Fatal("Timeout waiting for message")
	}
}

func setupSimpleMQTTClient(clientID string, t *testing.T) mqtt.Client {
	opts := mqtt.NewClientOptions()
	opts.AddBroker(simpleBrokerURL)
	opts.SetClientID(clientID)
	opts.SetKeepAlive(60 * time.Second)
	opts.SetDefaultPublishHandler(func(client mqtt.Client, msg mqtt.Message) {
		t.Logf("Unexpected message: %s", msg.Payload())
	})
	opts.SetPingTimeout(1 * time.Second)
	opts.SetConnectTimeout(5 * time.Second)

	client := mqtt.NewClient(opts)
	token := client.Connect()
	require.True(t, token.Wait(), "Failed to connect to MQTT broker")
	require.NoError(t, token.Error(), "Connection error")

	t.Logf("Successfully connected client: %s", clientID)
	return client
}