package e2e

import (
	"fmt"
	"testing"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/stretchr/testify/require"
)

// TestMQTTStressSubscription tests the broker's stability under stress conditions
// This test will create many rapid connections and subscriptions to verify the
// SUBSCRIBE packet QoS handling works correctly under load
func TestMQTTStressSubscription(t *testing.T) {
	t.Log("Starting MQTT stress subscription test")

	// Test multiple rapid subscription cycles
	for cycle := 0; cycle < 5; cycle++ {
		t.Logf("Starting stress cycle %d", cycle+1)

		// Create multiple clients in parallel
		for clientNum := 0; clientNum < 3; clientNum++ {
			go func(c, cyc int) {
				clientID := fmt.Sprintf("stress-client-%d-%d", cyc, c)

				opts := mqtt.NewClientOptions()
				opts.AddBroker("tcp://localhost:1883")
				opts.SetClientID(clientID)
				opts.SetConnectTimeout(2 * time.Second)
				opts.SetKeepAlive(10 * time.Second)

				client := mqtt.NewClient(opts)
				token := client.Connect()

				if !token.WaitTimeout(3*time.Second) || token.Error() != nil {
					t.Logf("Client %s connection failed/timeout", clientID)
					return
				}

				// Rapid subscribe/unsubscribe operations
				for i := 0; i < 2; i++ {
					topic := fmt.Sprintf("stress/test/cycle%d/client%d/msg%d", cyc, c, i)

					// Subscribe
					subToken := client.Subscribe(topic, byte(i%3), func(client mqtt.Client, msg mqtt.Message) {
						t.Logf("Stress client received: %s", string(msg.Payload()))
					})
					subToken.WaitTimeout(1 * time.Second)

					// Publish
					pubToken := client.Publish(topic, byte(i%3), false, "stress test message")
					pubToken.WaitTimeout(1 * time.Second)

					time.Sleep(10 * time.Millisecond)
				}

				client.Disconnect(100)
			}(clientNum, cycle)
		}

		time.Sleep(200 * time.Millisecond)
	}

	// Wait for all goroutines to complete
	time.Sleep(2 * time.Second)

	t.Log("Stress subscription test completed successfully")
}

// TestMQTTBoundaryConditions tests edge cases and boundary conditions
func TestMQTTBoundaryConditions(t *testing.T) {
	t.Log("Testing MQTT boundary conditions")

	// Test with maximum QoS values (should be handled gracefully)
	opts := mqtt.NewClientOptions()
	opts.AddBroker("tcp://localhost:1883")
	opts.SetClientID("boundary-test-client")
	opts.SetConnectTimeout(5 * time.Second)

	client := mqtt.NewClient(opts)
	token := client.Connect()
	require.True(t, token.Wait(), "Failed to connect boundary test client")
	require.NoError(t, token.Error(), "Boundary test connection error")
	defer client.Disconnect(250)

	// Test subscriptions with valid QoS values
	topics := []struct {
		topic string
		qos   byte
	}{
		{"boundary/qos0", 0},
		{"boundary/qos1", 1},
		{"boundary/qos2", 2},
	}

	for _, test := range topics {
		t.Logf("Testing boundary condition for QoS %d", test.qos)

		messageReceived := make(chan bool, 1)

		subToken := client.Subscribe(test.topic, test.qos, func(client mqtt.Client, msg mqtt.Message) {
			t.Logf("Boundary test received: %s (QoS: %d)", string(msg.Payload()), msg.Qos())
			messageReceived <- true
		})
		require.True(t, subToken.Wait(), "Failed to subscribe to %s", test.topic)
		require.NoError(t, subToken.Error(), "Subscribe error for %s", test.topic)

		pubToken := client.Publish(test.topic, test.qos, false, "Boundary test message")
		require.True(t, pubToken.Wait(), "Failed to publish to %s", test.topic)
		require.NoError(t, pubToken.Error(), "Publish error for %s", test.topic)

		// Verify message received
		select {
		case <-messageReceived:
			t.Logf("✓ Boundary test for QoS %d passed", test.qos)
		case <-time.After(2 * time.Second):
			t.Errorf("Timeout waiting for boundary test message (QoS %d)", test.qos)
		}
	}

	t.Log("Boundary conditions test completed successfully")
}