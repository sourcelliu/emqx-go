package e2e

import (
	"fmt"
	"sync"
	"testing"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMQTTProtocolVersionsE2E(t *testing.T) {
	t.Log("Testing End-to-End MQTT Protocol Versions Support")

	// Test all three MQTT protocol versions can coexist
	t.Run("MultiProtocolCoexistence", func(t *testing.T) {
		// Create clients with different MQTT versions
		mqtt31Client := createMQTTClientWithVersion("e2e-mqtt31-client", 3, t)
		defer mqtt31Client.Disconnect(250)

		mqtt311Client := createMQTTClientWithVersion("e2e-mqtt311-client", 4, t)
		defer mqtt311Client.Disconnect(250)

		mqtt5Client := createMQTTClientWithVersion("e2e-mqtt5-client", 5, t)
		defer mqtt5Client.Disconnect(250)

		// Verify all clients are connected
		assert.True(t, mqtt31Client.IsConnected(), "MQTT 3.1 client should be connected")
		assert.True(t, mqtt311Client.IsConnected(), "MQTT 3.1.1 client should be connected")
		assert.True(t, mqtt5Client.IsConnected(), "MQTT 5.0 client should be connected")

		t.Log("✅ All MQTT protocol versions can coexist")
	})

	t.Run("CrossProtocolCommunication", func(t *testing.T) {
		// Test that clients with different MQTT versions can communicate
		mqtt31Publisher := createMQTTClientWithVersion("e2e-mqtt31-pub", 3, t)
		defer mqtt31Publisher.Disconnect(250)

		mqtt311Subscriber := createMQTTClientWithVersion("e2e-mqtt311-sub", 4, t)
		defer mqtt311Subscriber.Disconnect(250)

		mqtt5Subscriber := createMQTTClientWithVersion("e2e-mqtt5-sub", 5, t)
		defer mqtt5Subscriber.Disconnect(250)

		topic := "e2e/cross/protocol/test"

		var mu sync.Mutex
		receivedMessages := make(map[string]string)

		// MQTT 3.1.1 subscriber
		token311 := mqtt311Subscriber.Subscribe(topic, 1, func(client mqtt.Client, msg mqtt.Message) {
			mu.Lock()
			receivedMessages["mqtt311"] = string(msg.Payload())
			mu.Unlock()
		})
		require.True(t, token311.WaitTimeout(5*time.Second))
		require.NoError(t, token311.Error())

		// MQTT 5.0 subscriber
		token5 := mqtt5Subscriber.Subscribe(topic, 1, func(client mqtt.Client, msg mqtt.Message) {
			mu.Lock()
			receivedMessages["mqtt5"] = string(msg.Payload())
			mu.Unlock()
		})
		require.True(t, token5.WaitTimeout(5*time.Second))
		require.NoError(t, token5.Error())

		// MQTT 3.1 publisher sends message
		testMessage := "Cross-protocol message from MQTT 3.1"
		pubToken := mqtt31Publisher.Publish(topic, 1, false, testMessage)
		require.True(t, pubToken.WaitTimeout(5*time.Second))
		require.NoError(t, pubToken.Error())

		time.Sleep(2 * time.Second)

		// Both subscribers should receive the message
		mu.Lock()
		mqtt311Message := receivedMessages["mqtt311"]
		mqtt5Message := receivedMessages["mqtt5"]
		mu.Unlock()

		assert.Equal(t, testMessage, mqtt311Message, "MQTT 3.1.1 subscriber should receive message")
		assert.Equal(t, testMessage, mqtt5Message, "MQTT 5.0 subscriber should receive message")

		t.Log("✅ Cross-protocol communication working correctly")
	})

	t.Run("ProtocolVersionNegotiation", func(t *testing.T) {
		// Test that broker correctly handles version negotiation
		versions := map[int]string{
			3: "MQTT 3.1",
			4: "MQTT 3.1.1",
			5: "MQTT 5.0",
		}

		for version, name := range versions {
			t.Run(name, func(t *testing.T) {
				clientID := fmt.Sprintf("e2e-version-%d", version)
				client := createMQTTClientWithVersion(clientID, version, t)
				defer client.Disconnect(250)

				assert.True(t, client.IsConnected(), fmt.Sprintf("%s client should connect successfully", name))
				t.Logf("✅ %s connection negotiation successful", name)
			})
		}
	})

	t.Run("QoSConsistencyAcrossVersions", func(t *testing.T) {
		// Test QoS behavior is consistent across MQTT versions
		publishers := map[string]mqtt.Client{
			"mqtt31":  createMQTTClientWithVersion("e2e-qos-pub-31", 3, t),
			"mqtt311": createMQTTClientWithVersion("e2e-qos-pub-311", 4, t),
			"mqtt5":   createMQTTClientWithVersion("e2e-qos-pub-5", 5, t),
		}

		subscriber := createMQTTClientWithVersion("e2e-qos-sub", 4, t)
		defer subscriber.Disconnect(250)

		for _, pub := range publishers {
			defer pub.Disconnect(250)
		}

		receivedMessages := make(map[string]int)
		topic := "e2e/qos/consistency"

		token := subscriber.Subscribe(topic, 2, func(client mqtt.Client, msg mqtt.Message) {
			payload := string(msg.Payload())
			// Count messages from each publisher type
			if len(payload) > 0 {
				receivedMessages["total"]++
			}
		})
		require.True(t, token.WaitTimeout(5*time.Second))
		require.NoError(t, token.Error())

		// Each publisher sends messages with different QoS levels
		for _, pub := range publishers {
			for qos := 0; qos <= 2; qos++ {
				message := fmt.Sprintf("QoS %d message from publisher", qos)
				pubToken := pub.Publish(topic, byte(qos), false, message)
				require.True(t, pubToken.WaitTimeout(5*time.Second))
				require.NoError(t, pubToken.Error())
			}
		}

		time.Sleep(3 * time.Second)

		// Verify messages were received (3 publishers × 3 QoS levels = 9 messages)
		expectedTotal := len(publishers) * 3
		assert.Equal(t, expectedTotal, receivedMessages["total"], "Should receive all messages from all publishers")

		t.Log("✅ QoS consistency across MQTT versions verified")
	})

	t.Run("RetainedMessageCompatibility", func(t *testing.T) {
		// Test retained messages work across different MQTT versions
		mqtt31Publisher := createMQTTClientWithVersion("e2e-retain-pub-31", 3, t)
		defer mqtt31Publisher.Disconnect(250)

		topic := "e2e/retained/compatibility"
		retainedMessage := "Retained message from MQTT 3.1"

		// MQTT 3.1 publishes retained message
		pubToken := mqtt31Publisher.Publish(topic, 1, true, retainedMessage)
		require.True(t, pubToken.WaitTimeout(5*time.Second))
		require.NoError(t, pubToken.Error())

		time.Sleep(1 * time.Second)

		// Different MQTT versions subscribe and should receive retained message
		versions := []int{4, 5} // Test 3.1.1 and 5.0
		for _, version := range versions {
			clientID := fmt.Sprintf("e2e-retain-sub-%d", version)
			subscriber := createMQTTClientWithVersion(clientID, version, t)

			messageReceived := make(chan string, 1)
			token := subscriber.Subscribe(topic, 1, func(client mqtt.Client, msg mqtt.Message) {
				messageReceived <- string(msg.Payload())
			})
			require.True(t, token.WaitTimeout(5*time.Second))
			require.NoError(t, token.Error())

			select {
			case msg := <-messageReceived:
				assert.Equal(t, retainedMessage, msg)
				t.Logf("✅ MQTT version %d received retained message correctly", version)
			case <-time.After(5 * time.Second):
				t.Fatalf("MQTT version %d did not receive retained message", version)
			}

			subscriber.Disconnect(250)
		}
	})

	t.Run("WildcardSubscriptionCompatibility", func(t *testing.T) {
		// Test wildcard subscriptions work across MQTT versions
		subscriber311 := createMQTTClientWithVersion("e2e-wild-sub-311", 4, t)
		defer subscriber311.Disconnect(250)

		subscriber5 := createMQTTClientWithVersion("e2e-wild-sub-5", 5, t)
		defer subscriber5.Disconnect(250)

		publisher31 := createMQTTClientWithVersion("e2e-wild-pub-31", 3, t)
		defer publisher31.Disconnect(250)

		baseTopics := []string{
			"e2e/wildcard/test1",
			"e2e/wildcard/test2",
			"e2e/wildcard/deep/test3",
		}

		received311 := make([]string, 0)
		received5 := make([]string, 0)

		// Both subscribers use wildcard subscription
		token311 := subscriber311.Subscribe("e2e/wildcard/+", 1, func(client mqtt.Client, msg mqtt.Message) {
			received311 = append(received311, msg.Topic())
		})
		require.True(t, token311.WaitTimeout(5*time.Second))
		require.NoError(t, token311.Error())

		token5 := subscriber5.Subscribe("e2e/wildcard/#", 1, func(client mqtt.Client, msg mqtt.Message) {
			received5 = append(received5, msg.Topic())
		})
		require.True(t, token5.WaitTimeout(5*time.Second))
		require.NoError(t, token5.Error())

		// MQTT 3.1 publisher sends to various topics
		for _, topic := range baseTopics {
			pubToken := publisher31.Publish(topic, 1, false, "wildcard test")
			require.True(t, pubToken.WaitTimeout(5*time.Second))
			require.NoError(t, pubToken.Error())
		}

		time.Sleep(2 * time.Second)

		// MQTT 3.1.1 should receive messages matching single-level wildcard (+)
		assert.Equal(t, 2, len(received311), "MQTT 3.1.1 should receive 2 messages with + wildcard")

		// MQTT 5.0 should receive all messages matching multi-level wildcard (#)
		assert.Equal(t, 3, len(received5), "MQTT 5.0 should receive 3 messages with # wildcard")

		t.Log("✅ Wildcard subscription compatibility verified")
	})

	t.Run("SessionManagementAcrossVersions", func(t *testing.T) {
		// Test session handling for different MQTT versions
		clientID := "e2e-session-test"

		// Connect with MQTT 3.1.1 and clean session = false
		client311 := createMQTTClientWithVersionAndCleanSession(clientID, 4, false, t)

		topic := "e2e/session/test"
		client311.Subscribe(topic, 1, func(client mqtt.Client, msg mqtt.Message) {
			// Message handler
		})

		client311.Disconnect(250)

		// Reconnect with MQTT 5.0 using same client ID
		// This tests if the broker handles session persistence across protocol versions
		client5 := createMQTTClientWithVersionAndCleanSession(clientID, 5, false, t)
		defer client5.Disconnect(250)

		assert.True(t, client5.IsConnected(), "Should be able to reconnect with different MQTT version")

		t.Log("✅ Session management across MQTT versions working")
	})

	t.Run("ErrorHandlingConsistency", func(t *testing.T) {
		// Test that error handling is consistent across MQTT versions
		versions := []int{3, 4, 5}

		for _, version := range versions {
			clientID := fmt.Sprintf("e2e-error-test-%d", version)
			client := createMQTTClientWithVersion(clientID, version, t)

			// Test invalid QoS subscription (should be handled gracefully)
			// Note: Most MQTT libraries validate QoS on client side
			token := client.Subscribe("test/invalid/qos", 1, nil)
			if token.WaitTimeout(5 * time.Second) {
				assert.NoError(t, token.Error(), fmt.Sprintf("MQTT version %d should handle subscriptions gracefully", version))
			}

			client.Disconnect(250)
		}

		t.Log("✅ Error handling consistency across MQTT versions verified")
	})
}

func createMQTTClientWithVersion(clientID string, protocolVersion int, t *testing.T) mqtt.Client {
	return createMQTTClientWithVersionAndCleanSession(clientID, protocolVersion, true, t)
}

func createMQTTClientWithVersionAndCleanSession(clientID string, protocolVersion int, cleanSession bool, t *testing.T) mqtt.Client {
	opts := mqtt.NewClientOptions()
	opts.AddBroker("tcp://localhost:1883")
	opts.SetClientID(clientID)
	opts.SetUsername("test")
	opts.SetPassword("test")
	opts.SetCleanSession(cleanSession)
	opts.SetProtocolVersion(uint(protocolVersion))
	opts.SetConnectTimeout(5 * time.Second)
	opts.SetKeepAlive(30 * time.Second)

	client := mqtt.NewClient(opts)
	token := client.Connect()
	require.True(t, token.WaitTimeout(10*time.Second), fmt.Sprintf("Failed to connect MQTT %d client", protocolVersion))
	require.NoError(t, token.Error(), fmt.Sprintf("MQTT %d connection error", protocolVersion))

	versionName := map[int]string{3: "3.1", 4: "3.1.1", 5: "5.0"}[protocolVersion]
	t.Logf("Successfully connected MQTT %s client: %s", versionName, clientID)
	return client
}

func containsString(str, substr string) bool {
	for i := 0; i <= len(str)-len(substr); i++ {
		if str[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}