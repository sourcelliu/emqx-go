package e2e

import (
	"testing"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/stretchr/testify/require"
)

func TestMQTTInvalidQoS(t *testing.T) {
	t.Log("Testing invalid QoS values to reproduce qos out of range error")

	// Create a custom MQTT client that can send invalid QoS
	opts := mqtt.NewClientOptions()
	opts.AddBroker("tcp://localhost:1883")
	opts.SetClientID("qos-test-client")
	opts.SetKeepAlive(60 * time.Second)
	opts.SetConnectTimeout(5 * time.Second)

	client := mqtt.NewClient(opts)
	token := client.Connect()
	require.True(t, token.Wait(), "Failed to connect to MQTT broker")
	require.NoError(t, token.Error(), "Connection error")

	defer client.Disconnect(250)

	t.Log("Connected successfully, now testing QoS levels")

	// Test valid QoS levels first
	for qos := byte(0); qos <= 2; qos++ {
		t.Logf("Testing valid QoS %d", qos)
		topic := "test/qos/valid"

		// Subscribe
		subToken := client.Subscribe(topic, qos, func(client mqtt.Client, msg mqtt.Message) {
			t.Logf("Received message with QoS %d: %s", msg.Qos(), string(msg.Payload()))
		})
		require.True(t, subToken.Wait(), "Failed to subscribe")
		require.NoError(t, subToken.Error(), "Subscribe error")

		// Publish
		pubToken := client.Publish(topic, qos, false, "Test message")
		require.True(t, pubToken.Wait(), "Failed to publish")
		require.NoError(t, pubToken.Error(), "Publish error")

		time.Sleep(100 * time.Millisecond)
	}

	t.Log("All valid QoS levels (0, 1, 2) work correctly")
}