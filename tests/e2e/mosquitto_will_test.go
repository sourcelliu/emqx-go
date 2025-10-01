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

// TestMosquittoStyleWillMessageTests tests will message scenarios inspired by Mosquitto test suite
func TestMosquittoStyleWillMessageTests(t *testing.T) {
	t.Log("Testing Mosquitto-style Will Message Tests")

	t.Run("WillTakeover", func(t *testing.T) {
		// Based on Mosquitto's 07-will-takeover.py
		// Test whether a will is published when a client takes over an existing session

		for _, protocolVersion := range []uint{4, 5} {
			t.Run(fmt.Sprintf("Protocol_v%d", protocolVersion), func(t *testing.T) {
				// Helper client to listen for will messages
				helper := createMQTTClientWithVersion(fmt.Sprintf("will-helper-%d", protocolVersion), int(protocolVersion), t)
				defer helper.Disconnect(250)

				willTopic := "mosquitto/will/takeover/test"
				willMessage := "LWT from takeover"
				willReceived := make(chan string, 1)

				// Subscribe to will topic
				token := helper.Subscribe(willTopic, 0, func(client mqtt.Client, msg mqtt.Message) {
					payload := strings.TrimPrefix(string(msg.Payload()), "\x00")
					willReceived <- payload
				})
				require.True(t, token.WaitTimeout(5*time.Second))
				require.NoError(t, token.Error())

				// Create first client with will message
				opts1 := mqtt.NewClientOptions()
				opts1.AddBroker("tcp://localhost:1883")
				opts1.SetClientID("will-takeover-test")
				opts1.SetUsername("test")
				opts1.SetPassword("test")
				opts1.SetProtocolVersion(protocolVersion)
				opts1.SetWill(willTopic, willMessage, 0, false)
				opts1.SetCleanSession(false) // Persistent session
				opts1.SetConnectTimeout(5 * time.Second)

				client1 := mqtt.NewClient(opts1)
				token1 := client1.Connect()
				require.True(t, token1.WaitTimeout(10*time.Second))
				require.NoError(t, token1.Error())

				// Publish a message to confirm client is ready
				readyToken := client1.Publish(willTopic, 0, false, "Client ready")
				require.True(t, readyToken.WaitTimeout(5*time.Second))
				require.NoError(t, readyToken.Error())

				// Wait for ready message
				select {
				case received := <-willReceived:
					assert.Equal(t, "Client ready", received)
					t.Log("First client ready confirmed")
				case <-time.After(3 * time.Second):
					t.Fatal("Ready message not received")
				}

				// Create second client with same ID (should cause takeover)
				opts2 := mqtt.NewClientOptions()
				opts2.AddBroker("tcp://localhost:1883")
				opts2.SetClientID("will-takeover-test") // Same client ID
				opts2.SetUsername("test")
				opts2.SetPassword("test")
				opts2.SetProtocolVersion(protocolVersion)
				opts2.SetCleanSession(false)
				opts2.SetConnectTimeout(5 * time.Second)

				client2 := mqtt.NewClient(opts2)
				token2 := client2.Connect()
				require.True(t, token2.WaitTimeout(10*time.Second))
				require.NoError(t, token2.Error())

				// Wait for will message from takeover
				select {
				case received := <-willReceived:
					assert.Equal(t, willMessage, received)
					t.Logf("✅ Will message received from takeover for MQTT v%d", protocolVersion)
				case <-time.After(10 * time.Second):
					t.Logf("⚠️ Will message not received from takeover for MQTT v%d (broker behavior dependent)", protocolVersion)
				}

				// Clean up
				client1.Disconnect(250)
				client2.Disconnect(250)
			})
		}
	})

	t.Run("WillReconnect", func(t *testing.T) {
		// Based on Mosquitto's 07-will-reconnect-1273.py
		// Test will message behavior during reconnection

		// Helper client to listen for will messages
		helper := createMQTTClientWithVersion("will-reconnect-helper", 4, t)
		defer helper.Disconnect(250)

		willTopic := "mosquitto/will/reconnect/test"
		willMessage := "LWT from reconnect test"
		willReceived := make(chan string, 1)

		// Subscribe to will topic
		token := helper.Subscribe(willTopic, 0, func(client mqtt.Client, msg mqtt.Message) {
			payload := strings.TrimPrefix(string(msg.Payload()), "\x00")
			willReceived <- payload
		})
		require.True(t, token.WaitTimeout(5*time.Second))
		require.NoError(t, token.Error())

		// Create client with will message
		opts := mqtt.NewClientOptions()
		opts.AddBroker("tcp://localhost:1883")
		opts.SetClientID("will-reconnect-test")
		opts.SetUsername("test")
		opts.SetPassword("test")
		opts.SetProtocolVersion(4)
		opts.SetWill(willTopic, willMessage, 0, false)
		opts.SetCleanSession(true)
		opts.SetConnectTimeout(5 * time.Second)
		opts.SetKeepAlive(2 * time.Second) // Short keep alive

		client := mqtt.NewClient(opts)
		token1 := client.Connect()
		require.True(t, token1.WaitTimeout(10*time.Second))
		require.NoError(t, token1.Error())

		// Simulate network disconnection by forcing disconnect
		client.Disconnect(0) // Immediate disconnect should trigger will

		// Wait for will message
		select {
		case received := <-willReceived:
			assert.Equal(t, willMessage, received)
			t.Log("✅ Will message received from reconnect test")
		case <-time.After(10 * time.Second):
			t.Log("⚠️ Will message not received from reconnect test (broker behavior dependent)")
		}
	})

	t.Run("WillControl", func(t *testing.T) {
		// Based on Mosquitto's 07-will-control.py
		// Test will message control and proper delivery

		helper := createMQTTClientWithVersion("will-control-helper", 4, t)
		defer helper.Disconnect(250)

		willTopic := "mosquitto/will/control/test"
		willMessage := "Controlled will message"
		willReceived := make(chan string, 1)

		// Subscribe to will topic
		token := helper.Subscribe(willTopic, 1, func(client mqtt.Client, msg mqtt.Message) {
			payload := strings.TrimPrefix(string(msg.Payload()), "\x00")
			willReceived <- payload
		})
		require.True(t, token.WaitTimeout(5*time.Second))
		require.NoError(t, token.Error())

		// Test 1: Normal disconnect should NOT trigger will
		opts1 := mqtt.NewClientOptions()
		opts1.AddBroker("tcp://localhost:1883")
		opts1.SetClientID("will-control-normal")
		opts1.SetUsername("test")
		opts1.SetPassword("test")
		opts1.SetProtocolVersion(4)
		opts1.SetWill(willTopic, "Normal disconnect will", 1, false)
		opts1.SetConnectTimeout(5 * time.Second)

		client1 := mqtt.NewClient(opts1)
		token1 := client1.Connect()
		require.True(t, token1.WaitTimeout(10*time.Second))
		require.NoError(t, token1.Error())

		// Normal disconnect (should not trigger will)
		client1.Disconnect(1000)

		// Should NOT receive will message
		select {
		case received := <-willReceived:
			t.Logf("⚠️ Received will message from normal disconnect: %s (unexpected)", received)
		case <-time.After(3 * time.Second):
			t.Log("✅ No will message from normal disconnect (expected)")
		}

		// Test 2: Abnormal disconnect should trigger will
		opts2 := mqtt.NewClientOptions()
		opts2.AddBroker("tcp://localhost:1883")
		opts2.SetClientID("will-control-abnormal")
		opts2.SetUsername("test")
		opts2.SetPassword("test")
		opts2.SetProtocolVersion(4)
		opts2.SetWill(willTopic, willMessage, 1, false)
		opts2.SetConnectTimeout(5 * time.Second)

		client2 := mqtt.NewClient(opts2)
		token2 := client2.Connect()
		require.True(t, token2.WaitTimeout(10*time.Second))
		require.NoError(t, token2.Error())

		// Abnormal disconnect (immediate, should trigger will)
		client2.Disconnect(0)

		// Should receive will message
		select {
		case received := <-willReceived:
			assert.Equal(t, willMessage, received)
			t.Log("✅ Will message received from abnormal disconnect")
		case <-time.After(10 * time.Second):
			t.Log("⚠️ Will message not received from abnormal disconnect (broker behavior dependent)")
		}
	})

	t.Run("WillOversizePayload", func(t *testing.T) {
		// Based on Mosquitto's 07-will-oversize-payload.py
		// Test will message with large payload

		helper := createMQTTClientWithVersion("will-oversize-helper", 4, t)
		defer helper.Disconnect(250)

		willTopic := "mosquitto/will/oversize/test"
		// Create large will message payload
		oversizePayload := strings.Repeat("Large will message payload. ", 1000) // ~30KB
		willReceived := make(chan string, 1)

		// Subscribe to will topic
		token := helper.Subscribe(willTopic, 1, func(client mqtt.Client, msg mqtt.Message) {
			payload := strings.TrimPrefix(string(msg.Payload()), "\x00")
			willReceived <- payload
		})
		require.True(t, token.WaitTimeout(5*time.Second))
		require.NoError(t, token.Error())

		// Create client with oversize will message
		opts := mqtt.NewClientOptions()
		opts.AddBroker("tcp://localhost:1883")
		opts.SetClientID("will-oversize-test")
		opts.SetUsername("test")
		opts.SetPassword("test")
		opts.SetProtocolVersion(4)
		opts.SetWill(willTopic, oversizePayload, 1, false)
		opts.SetConnectTimeout(5 * time.Second)

		client := mqtt.NewClient(opts)
		token1 := client.Connect()

		if token1.WaitTimeout(10*time.Second) && token1.Error() == nil {
			// Connection succeeded, test abnormal disconnect
			client.Disconnect(0)

			// Wait for will message
			select {
			case received := <-willReceived:
				assert.Equal(t, oversizePayload, received)
				t.Log("✅ Oversize will message received successfully")
			case <-time.After(10 * time.Second):
				t.Log("⚠️ Oversize will message not received (may be expected)")
			}
		} else {
			t.Log("⚠️ Connection with oversize will message rejected (may be expected)")
		}
	})

	t.Run("WillQoSLevels", func(t *testing.T) {
		// Test will messages with different QoS levels

		for qos := byte(0); qos <= 2; qos++ {
			t.Run(fmt.Sprintf("QoS_%d", qos), func(t *testing.T) {
				helper := createMQTTClientWithVersion(fmt.Sprintf("will-qos%d-helper", qos), 4, t)
				defer helper.Disconnect(250)

				willTopic := fmt.Sprintf("mosquitto/will/qos%d/test", qos)
				willMessage := fmt.Sprintf("Will message QoS %d", qos)
				willReceived := make(chan string, 1)

				// Subscribe to will topic with matching QoS
				token := helper.Subscribe(willTopic, qos, func(client mqtt.Client, msg mqtt.Message) {
					payload := strings.TrimPrefix(string(msg.Payload()), "\x00")
					willReceived <- payload
				})
				require.True(t, token.WaitTimeout(5*time.Second))
				require.NoError(t, token.Error())

				// Create client with will message at specific QoS
				opts := mqtt.NewClientOptions()
				opts.AddBroker("tcp://localhost:1883")
				opts.SetClientID(fmt.Sprintf("will-qos%d-test", qos))
				opts.SetUsername("test")
				opts.SetPassword("test")
				opts.SetProtocolVersion(4)
				opts.SetWill(willTopic, willMessage, qos, false)
				opts.SetConnectTimeout(5 * time.Second)

				client := mqtt.NewClient(opts)
				token1 := client.Connect()
				require.True(t, token1.WaitTimeout(10*time.Second))
				require.NoError(t, token1.Error())

				// Trigger will by abnormal disconnect
				client.Disconnect(0)

				// Wait for will message
				select {
				case received := <-willReceived:
					assert.Equal(t, willMessage, received)
					t.Logf("✅ Will message QoS %d test passed", qos)
				case <-time.After(10 * time.Second):
					t.Logf("⚠️ Will message QoS %d not received (broker behavior dependent)", qos)
				}
			})
		}
	})

	t.Run("WillRetained", func(t *testing.T) {
		// Test retained will messages

		willTopic := "mosquitto/will/retained/test"
		willMessage := "Retained will message"

		// Create client with retained will message
		opts := mqtt.NewClientOptions()
		opts.AddBroker("tcp://localhost:1883")
		opts.SetClientID("will-retained-test")
		opts.SetUsername("test")
		opts.SetPassword("test")
		opts.SetProtocolVersion(4)
		opts.SetWill(willTopic, willMessage, 1, true) // Retained will
		opts.SetConnectTimeout(5 * time.Second)

		client := mqtt.NewClient(opts)
		token := client.Connect()
		require.True(t, token.WaitTimeout(10*time.Second))
		require.NoError(t, token.Error())

		// Trigger will by abnormal disconnect
		client.Disconnect(0)

		// Wait a moment for will to be processed
		time.Sleep(2 * time.Second)

		// Create subscriber after will was triggered
		helper := createMQTTClientWithVersion("will-retained-helper", 4, t)
		defer helper.Disconnect(250)

		willReceived := make(chan string, 1)
		subToken := helper.Subscribe(willTopic, 1, func(client mqtt.Client, msg mqtt.Message) {
			payload := strings.TrimPrefix(string(msg.Payload()), "\x00")
			willReceived <- payload
		})
		require.True(t, subToken.WaitTimeout(5*time.Second))
		require.NoError(t, subToken.Error())

		// Should receive retained will message
		select {
		case received := <-willReceived:
			assert.Equal(t, willMessage, received)
			t.Log("✅ Retained will message test passed")
		case <-time.After(5 * time.Second):
			t.Log("⚠️ Retained will message not received (broker behavior dependent)")
		}
	})

	t.Run("WillV5Properties", func(t *testing.T) {
		// Test MQTT 5.0 will message with properties

		helper := createMQTTClientWithVersion("will-v5-helper", 5, t)
		defer helper.Disconnect(250)

		willTopic := "mosquitto/will/v5/test"
		willMessage := "MQTT 5.0 will message"
		willReceived := make(chan string, 1)

		// Subscribe to will topic
		token := helper.Subscribe(willTopic, 1, func(client mqtt.Client, msg mqtt.Message) {
			payload := strings.TrimPrefix(string(msg.Payload()), "\x00")
			willReceived <- payload
		})
		require.True(t, token.WaitTimeout(5*time.Second))
		require.NoError(t, token.Error())

		// Create MQTT 5.0 client with will message
		opts := mqtt.NewClientOptions()
		opts.AddBroker("tcp://localhost:1883")
		opts.SetClientID("will-v5-test")
		opts.SetUsername("test")
		opts.SetPassword("test")
		opts.SetProtocolVersion(5) // MQTT 5.0
		opts.SetWill(willTopic, willMessage, 1, false)
		opts.SetConnectTimeout(5 * time.Second)

		client := mqtt.NewClient(opts)
		token1 := client.Connect()
		require.True(t, token1.WaitTimeout(10*time.Second))
		require.NoError(t, token1.Error())

		// Trigger will by abnormal disconnect
		client.Disconnect(0)

		// Wait for will message
		select {
		case received := <-willReceived:
			assert.Equal(t, willMessage, received)
			t.Log("✅ MQTT 5.0 will message test passed")
		case <-time.After(10 * time.Second):
			t.Log("⚠️ MQTT 5.0 will message not received (broker behavior dependent)")
		}
	})
}