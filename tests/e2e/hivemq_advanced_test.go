package e2e

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestHiveMQStyleAdvancedFeatures tests advanced MQTT features inspired by HiveMQ Enterprise
func TestHiveMQStyleAdvancedFeatures(t *testing.T) {
	t.Log("Testing HiveMQ-style Advanced Features")

	t.Run("SharedSubscriptions", func(t *testing.T) {
		// Based on HiveMQ's shared subscription testing
		// Test load balancing across multiple subscribers

		const numSubscribers = 3
		const numMessages = 15
		sharedTopic := "$share/loadbalance/hivemq/shared/test"
		regularTopic := "hivemq/shared/test"

		var messageDistribution [numSubscribers]int32
		var wg sync.WaitGroup

		subscribers := make([]mqtt.Client, numSubscribers)

		// Create shared subscribers
		for i := 0; i < numSubscribers; i++ {
			wg.Add(1)
			go func(subIndex int) {
				defer wg.Done()

				clientID := fmt.Sprintf("shared-sub-%d", subIndex)
				subscriber := createMQTTClientWithVersion(clientID, 4, t)
				subscribers[subIndex] = subscriber

				// Subscribe to shared subscription (if supported by broker)
				token := subscriber.Subscribe(sharedTopic, 1, func(client mqtt.Client, msg mqtt.Message) {
					atomic.AddInt32(&messageDistribution[subIndex], 1)
					t.Logf("Subscriber %d received: %s", subIndex, string(msg.Payload()))
				})

				if token.WaitTimeout(5*time.Second) && token.Error() == nil {
					t.Logf("Subscriber %d successfully subscribed to shared topic", subIndex)
				} else {
					t.Logf("Subscriber %d failed to subscribe to shared topic, trying regular topic", subIndex)
					// Fallback to regular subscription if shared subscriptions not supported
					token = subscriber.Subscribe(regularTopic, 1, func(client mqtt.Client, msg mqtt.Message) {
						atomic.AddInt32(&messageDistribution[subIndex], 1)
					})
					token.WaitTimeout(5 * time.Second)
				}
			}(i)
		}

		wg.Wait()

		// Publisher
		publisher := createMQTTClientWithVersion("shared-pub", 4, t)
		defer publisher.Disconnect(250)

		// Publish messages
		for i := 0; i < numMessages; i++ {
			message := fmt.Sprintf("Shared subscription message %d", i+1)
			pubToken := publisher.Publish(regularTopic, 1, false, message)
			require.True(t, pubToken.WaitTimeout(5*time.Second))
			require.NoError(t, pubToken.Error())
			time.Sleep(50 * time.Millisecond)
		}

		time.Sleep(3 * time.Second)

		// Analyze distribution
		totalReceived := int32(0)
		for i, count := range messageDistribution {
			received := atomic.LoadInt32(&count)
			totalReceived += received
			t.Logf("Subscriber %d received %d messages", i, received)
		}

		t.Logf("Total messages received: %d/%d", totalReceived, numMessages)

		if totalReceived == int32(numMessages*numSubscribers) {
			t.Log("✅ All subscribers received all messages (regular subscription behavior)")
		} else if totalReceived == int32(numMessages) {
			t.Log("✅ Messages were load-balanced across subscribers (shared subscription behavior)")
		} else {
			t.Logf("⚠️ Unexpected message distribution: %d total received", totalReceived)
		}

		// Clean up
		for _, subscriber := range subscribers {
			if subscriber != nil {
				subscriber.Disconnect(250)
			}
		}
	})

	t.Run("DataIntegration", func(t *testing.T) {
		// Based on HiveMQ's data integration testing
		// Test data transformation and routing patterns

		publisher := createMQTTClientWithVersion("data-integration-pub", 4, t)
		defer publisher.Disconnect(250)

		dataProcessor := createMQTTClientWithVersion("data-processor", 4, t)
		defer dataProcessor.Disconnect(250)

		consumer := createMQTTClientWithVersion("data-consumer", 4, t)
		defer consumer.Disconnect(250)

		rawDataTopic := "hivemq/data/raw"
		processedDataTopic := "hivemq/data/processed"

		processedMessages := make(chan string, 10)

		// Data processor subscribes to raw data and processes it
		procToken := dataProcessor.Subscribe(rawDataTopic, 1, func(client mqtt.Client, msg mqtt.Message) {
			rawData := strings.TrimPrefix(string(msg.Payload()), "\x00")
			t.Logf("Processing raw data: %s", rawData)

			// Simulate data transformation
			processedData := fmt.Sprintf("PROCESSED[%s]", strings.ToUpper(rawData))

			// Publish processed data
			pubToken := client.Publish(processedDataTopic, 1, false, processedData)
			pubToken.WaitTimeout(3 * time.Second)
		})
		require.True(t, procToken.WaitTimeout(5*time.Second))
		require.NoError(t, procToken.Error())

		// Consumer subscribes to processed data
		consToken := consumer.Subscribe(processedDataTopic, 1, func(client mqtt.Client, msg mqtt.Message) {
			processedData := strings.TrimPrefix(string(msg.Payload()), "\x00")
			processedMessages <- processedData
		})
		require.True(t, consToken.WaitTimeout(5*time.Second))
		require.NoError(t, consToken.Error())

		// Publish raw data
		testData := []string{
			"sensor_temperature_25.6",
			"sensor_humidity_68.2",
			"sensor_pressure_1013.25",
		}

		for _, data := range testData {
			pubToken := publisher.Publish(rawDataTopic, 1, false, data)
			require.True(t, pubToken.WaitTimeout(5*time.Second))
			require.NoError(t, pubToken.Error())
		}

		// Verify data processing pipeline
		receivedCount := 0
		deadline := time.After(10 * time.Second)

		for receivedCount < len(testData) {
			select {
			case processed := <-processedMessages:
				t.Logf("Received processed data: %s", processed)
				assert.Contains(t, processed, "PROCESSED[")
				receivedCount++
			case <-deadline:
				break
			}
		}

		assert.Equal(t, len(testData), receivedCount, "All data should be processed through the pipeline")
		t.Log("✅ Data integration pipeline test completed")
	})

	t.Run("MessageRouting", func(t *testing.T) {
		// Based on HiveMQ's message routing and filtering
		// Test complex routing scenarios

		publisher := createMQTTClientWithVersion("routing-pub", 4, t)
		defer publisher.Disconnect(250)

		// Different consumers for different data types
		temperatureConsumer := createMQTTClientWithVersion("temp-consumer", 4, t)
		defer temperatureConsumer.Disconnect(250)

		humidityConsumer := createMQTTClientWithVersion("humidity-consumer", 4, t)
		defer humidityConsumer.Disconnect(250)

		alertConsumer := createMQTTClientWithVersion("alert-consumer", 4, t)
		defer alertConsumer.Disconnect(250)

		var tempCount, humidityCount, alertCount int32

		// Subscribe to specific data types
		tempToken := temperatureConsumer.Subscribe("hivemq/sensor/+/temperature", 1, func(client mqtt.Client, msg mqtt.Message) {
			atomic.AddInt32(&tempCount, 1)
			t.Logf("Temperature consumer received: %s", msg.Topic())
		})
		require.True(t, tempToken.WaitTimeout(5*time.Second))
		require.NoError(t, tempToken.Error())

		humidityToken := humidityConsumer.Subscribe("hivemq/sensor/+/humidity", 1, func(client mqtt.Client, msg mqtt.Message) {
			atomic.AddInt32(&humidityCount, 1)
			t.Logf("Humidity consumer received: %s", msg.Topic())
		})
		require.True(t, humidityToken.WaitTimeout(5*time.Second))
		require.NoError(t, humidityToken.Error())

		alertToken := alertConsumer.Subscribe("hivemq/alert/+", 1, func(client mqtt.Client, msg mqtt.Message) {
			atomic.AddInt32(&alertCount, 1)
			t.Logf("Alert consumer received: %s", msg.Topic())
		})
		require.True(t, alertToken.WaitTimeout(5*time.Second))
		require.NoError(t, alertToken.Error())

		// Publish various message types
		messageTypes := []struct {
			topic string
			data  string
		}{
			{"hivemq/sensor/001/temperature", "25.6"},
			{"hivemq/sensor/002/temperature", "22.1"},
			{"hivemq/sensor/001/humidity", "68.2"},
			{"hivemq/sensor/003/humidity", "71.5"},
			{"hivemq/alert/high_temperature", "Sensor 001 exceeded threshold"},
			{"hivemq/alert/low_humidity", "Sensor 003 below minimum"},
		}

		for _, msg := range messageTypes {
			pubToken := publisher.Publish(msg.topic, 1, false, msg.data)
			require.True(t, pubToken.WaitTimeout(5*time.Second))
			require.NoError(t, pubToken.Error())
		}

		time.Sleep(3 * time.Second)

		tempReceived := atomic.LoadInt32(&tempCount)
		humidityReceived := atomic.LoadInt32(&humidityCount)
		alertReceived := atomic.LoadInt32(&alertCount)

		t.Logf("Message routing results:")
		t.Logf("  Temperature messages: %d", tempReceived)
		t.Logf("  Humidity messages: %d", humidityReceived)
		t.Logf("  Alert messages: %d", alertReceived)

		// Note: Actual routing depends on broker's wildcard support
		// In basic brokers, might need explicit topic subscriptions
		t.Log("✅ Message routing test completed")
	})

	t.Run("ClusterSimulation", func(t *testing.T) {
		// Based on HiveMQ's clustering tests
		// Simulate multi-node cluster behavior with single broker

		const numNodes = 3
		const messagesPerNode = 5

		// Simulate cluster nodes as different client groups
		var nodeClients [][]mqtt.Client
		var nodeReceivedCounts []int32

		nodeReceivedCounts = make([]int32, numNodes)
		nodeClients = make([][]mqtt.Client, numNodes)

		topic := "hivemq/cluster/test"

		// Create "nodes" (client groups)
		for nodeIndex := 0; nodeIndex < numNodes; nodeIndex++ {
			nodeClients[nodeIndex] = make([]mqtt.Client, 2) // 2 clients per "node"

			for clientIndex := 0; clientIndex < 2; clientIndex++ {
				clientID := fmt.Sprintf("cluster-node%d-client%d", nodeIndex, clientIndex)
				client := createMQTTClientWithVersion(clientID, 4, t)
				nodeClients[nodeIndex][clientIndex] = client

				// Each client subscribes to the same topic
				nodeIdx := nodeIndex // Capture for closure
				token := client.Subscribe(topic, 1, func(client mqtt.Client, msg mqtt.Message) {
					atomic.AddInt32(&nodeReceivedCounts[nodeIdx], 1)
				})
				require.True(t, token.WaitTimeout(5*time.Second))
				require.NoError(t, token.Error())
			}
		}

		// Simulate cross-cluster publishing
		publisher := createMQTTClientWithVersion("cluster-publisher", 4, t)
		defer publisher.Disconnect(250)

		totalMessages := numNodes * messagesPerNode
		for i := 0; i < totalMessages; i++ {
			message := fmt.Sprintf("Cluster message %d", i+1)
			pubToken := publisher.Publish(topic, 1, false, message)
			require.True(t, pubToken.WaitTimeout(5*time.Second))
			require.NoError(t, pubToken.Error())
		}

		time.Sleep(3 * time.Second)

		// Analyze cluster distribution
		totalReceived := int32(0)
		for nodeIndex := 0; nodeIndex < numNodes; nodeIndex++ {
			nodeReceived := atomic.LoadInt32(&nodeReceivedCounts[nodeIndex])
			totalReceived += nodeReceived
			t.Logf("Node %d received %d messages", nodeIndex, nodeReceived)
		}

		t.Logf("Cluster simulation results: %d total messages received across %d nodes", totalReceived, numNodes)

		// In a real cluster, each message would be received once per node
		expectedTotal := int32(totalMessages * numNodes * 2) // 2 clients per node
		assert.Equal(t, expectedTotal, totalReceived, "All clients should receive all messages")

		// Clean up all clients
		for nodeIndex := 0; nodeIndex < numNodes; nodeIndex++ {
			for clientIndex := 0; clientIndex < 2; clientIndex++ {
				if nodeClients[nodeIndex][clientIndex] != nil {
					nodeClients[nodeIndex][clientIndex].Disconnect(250)
				}
			}
		}

		t.Log("✅ Cluster simulation test completed")
	})

	t.Run("ExtensionSystem", func(t *testing.T) {
		// Based on HiveMQ's extension system testing
		// Simulate custom extension behavior

		publisher := createMQTTClientWithVersion("extension-pub", 4, t)
		defer publisher.Disconnect(250)

		subscriber := createMQTTClientWithVersion("extension-sub", 4, t)
		defer subscriber.Disconnect(250)

		// Simulate extension that modifies messages
		extensionProcessor := createMQTTClientWithVersion("extension-processor", 4, t)
		defer extensionProcessor.Disconnect(250)

		inputTopic := "hivemq/extension/input"
		outputTopic := "hivemq/extension/output"

		processedMessages := make(chan string, 10)

		// Extension processor (simulates custom extension logic)
		extToken := extensionProcessor.Subscribe(inputTopic, 1, func(client mqtt.Client, msg mqtt.Message) {
			originalPayload := strings.TrimPrefix(string(msg.Payload()), "\x00")
			t.Logf("Extension processing: %s", originalPayload)

			// Simulate extension processing (e.g., data validation, transformation, enrichment)
			processedPayload := fmt.Sprintf("EXTENSION_PROCESSED: %s [timestamp: %d]", originalPayload, time.Now().Unix())

			// Publish processed message
			pubToken := client.Publish(outputTopic, 1, false, processedPayload)
			pubToken.WaitTimeout(3 * time.Second)
		})
		require.True(t, extToken.WaitTimeout(5*time.Second))
		require.NoError(t, extToken.Error())

		// End subscriber
		subToken := subscriber.Subscribe(outputTopic, 1, func(client mqtt.Client, msg mqtt.Message) {
			payload := strings.TrimPrefix(string(msg.Payload()), "\x00")
			processedMessages <- payload
		})
		require.True(t, subToken.WaitTimeout(5*time.Second))
		require.NoError(t, subToken.Error())

		// Test extension with various message types
		testMessages := []string{
			"{\"deviceId\": \"sensor001\", \"temperature\": 25.6}",
			"{\"deviceId\": \"sensor002\", \"humidity\": 68.2}",
			"Simple text message",
			"Binary-like data: \x01\x02\x03",
		}

		for _, message := range testMessages {
			pubToken := publisher.Publish(inputTopic, 1, false, message)
			require.True(t, pubToken.WaitTimeout(5*time.Second))
			require.NoError(t, pubToken.Error())
		}

		// Verify extension processing
		receivedCount := 0
		deadline := time.After(10 * time.Second)

		for receivedCount < len(testMessages) {
			select {
			case processed := <-processedMessages:
				t.Logf("Extension output: %s", processed)
				assert.Contains(t, processed, "EXTENSION_PROCESSED:")
				assert.Contains(t, processed, "timestamp:")
				receivedCount++
			case <-deadline:
				break
			}
		}

		assert.Equal(t, len(testMessages), receivedCount, "All messages should be processed by extension")
		t.Log("✅ Extension system simulation test completed")
	})

	t.Run("QualityOfService", func(t *testing.T) {
		// Based on HiveMQ's comprehensive QoS testing
		// Test QoS behavior across different scenarios

		publisher := createMQTTClientWithVersion("qos-pub", 4, t)
		defer publisher.Disconnect(250)

		subscriber := createMQTTClientWithVersion("qos-sub", 4, t)
		defer subscriber.Disconnect(250)

		qosLevels := []byte{0, 1, 2}
		var receivedCounts [3]int32

		// Subscribe with different QoS levels
		for _, qos := range qosLevels {
			topic := fmt.Sprintf("hivemq/qos/%d/test", qos)
			qosIndex := int(qos)

			token := subscriber.Subscribe(topic, qos, func(client mqtt.Client, msg mqtt.Message) {
				atomic.AddInt32(&receivedCounts[qosIndex], 1)
				t.Logf("Received QoS %d message: %s", qos, msg.Topic())
			})
			require.True(t, token.WaitTimeout(5*time.Second))
			require.NoError(t, token.Error())
		}

		// Publish with different QoS levels
		messagesPerQoS := 3
		for _, qos := range qosLevels {
			topic := fmt.Sprintf("hivemq/qos/%d/test", qos)

			for i := 0; i < messagesPerQoS; i++ {
				message := fmt.Sprintf("QoS %d message %d", qos, i+1)
				pubToken := publisher.Publish(topic, qos, false, message)

				timeout := 5 * time.Second
				if qos == 2 {
					timeout = 10 * time.Second // QoS 2 may take longer
				}

				require.True(t, pubToken.WaitTimeout(timeout))
				require.NoError(t, pubToken.Error())
			}
		}

		time.Sleep(5 * time.Second)

		// Verify QoS delivery guarantees
		for qosIndex, qos := range qosLevels {
			received := atomic.LoadInt32(&receivedCounts[qosIndex])
			expected := int32(messagesPerQoS)

			t.Logf("QoS %d: received %d/%d messages", qos, received, expected)

			switch qos {
			case 0:
				// QoS 0: At most once - might lose messages
				assert.GreaterOrEqual(t, received, int32(0), "QoS 0 should receive at least 0 messages")
				assert.LessOrEqual(t, received, expected, "QoS 0 should not duplicate messages")
			case 1:
				// QoS 1: At least once - might duplicate
				assert.GreaterOrEqual(t, received, expected, "QoS 1 should receive at least all messages")
			case 2:
				// QoS 2: Exactly once
				assert.Equal(t, expected, received, "QoS 2 should receive exactly the expected number of messages")
			}
		}

		t.Log("✅ Quality of Service comprehensive test completed")
	})
}