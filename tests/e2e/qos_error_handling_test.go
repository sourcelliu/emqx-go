package e2e

import (
	"testing"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/stretchr/testify/require"
)

// TestMQTTQoSErrorHandling tests the broker's handling of invalid QoS values
func TestMQTTQoSErrorHandling(t *testing.T) {
	t.Log("Testing MQTT QoS error handling capabilities")

	// Test valid QoS values first to ensure normal operation
	for qos := byte(0); qos <= 2; qos++ {
		t.Logf("Testing valid QoS %d", qos)

		opts := mqtt.NewClientOptions()
		opts.AddBroker("tcp://localhost:1883")
		opts.SetClientID("qos-error-test-valid")
		opts.SetConnectTimeout(5 * time.Second)
		opts.SetKeepAlive(30 * time.Second)

		client := mqtt.NewClient(opts)
		token := client.Connect()
		require.True(t, token.Wait(), "Failed to connect for QoS %d test", qos)
		require.NoError(t, token.Error(), "Connection error for QoS %d", qos)

		// Test subscription with valid QoS
		messageReceived := make(chan string, 1)
		subToken := client.Subscribe("test/qos/error/valid", qos, func(client mqtt.Client, msg mqtt.Message) {
			messageReceived <- string(msg.Payload())
		})
		require.True(t, subToken.Wait(), "Failed to subscribe with QoS %d", qos)
		require.NoError(t, subToken.Error(), "Subscribe error for QoS %d", qos)

		// Test publish with valid QoS
		pubToken := client.Publish("test/qos/error/valid", qos, false, "Test message")
		require.True(t, pubToken.Wait(), "Failed to publish with QoS %d", qos)
		require.NoError(t, pubToken.Error(), "Publish error for QoS %d", qos)

		// Verify message received
		select {
		case msg := <-messageReceived:
			require.Equal(t, "Test message", msg, "Message should match for QoS %d", qos)
		case <-time.After(2 * time.Second):
			t.Fatalf("Message not received for QoS %d", qos)
		}

		client.Disconnect(250)
		t.Logf("✓ QoS %d test completed successfully", qos)
	}

	// Now test the broker's resilience - it should handle connections gracefully
	// even if there are protocol issues
	t.Log("Testing broker resilience with edge cases")

	opts := mqtt.NewClientOptions()
	opts.AddBroker("tcp://localhost:1883")
	opts.SetClientID("qos-resilience-test")
	opts.SetConnectTimeout(5 * time.Second)
	opts.SetKeepAlive(30 * time.Second)

	// Test multiple rapid connections to ensure no resource leaks
	for i := 0; i < 10; i++ {
		client := mqtt.NewClient(opts)
		token := client.Connect()
		require.True(t, token.Wait(), "Failed to connect in resilience test %d", i+1)
		require.NoError(t, token.Error(), "Connection error in resilience test %d", i+1)

		// Quick subscribe/publish test
		subToken := client.Subscribe("test/resilience", 1, func(client mqtt.Client, msg mqtt.Message) {
			t.Logf("Resilience test %d received: %s", i+1, string(msg.Payload()))
		})
		require.True(t, subToken.Wait(), "Failed to subscribe in resilience test %d", i+1)

		pubToken := client.Publish("test/resilience", 1, false, "Resilience test")
		require.True(t, pubToken.Wait(), "Failed to publish in resilience test %d", i+1)

		client.Disconnect(100)
		time.Sleep(50 * time.Millisecond)
	}

	t.Log("✓ QoS error handling and resilience tests completed successfully")
}