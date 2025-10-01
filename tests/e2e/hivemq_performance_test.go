package e2e

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestHiveMQStylePerformance tests performance scenarios inspired by HiveMQ Swarm testing
func TestHiveMQStylePerformance(t *testing.T) {
	t.Log("Testing HiveMQ-style Performance and Load Tests")

	t.Run("ConnectionScaling", func(t *testing.T) {
		// Based on HiveMQ's connection handling tests
		// Test concurrent client connections

		const numClients = 50 // Scaled down for e2e testing
		var wg sync.WaitGroup
		var successfulConnections int32
		var connectionErrors int32

		clients := make([]mqtt.Client, numClients)

		// Create and connect clients concurrently
		for i := 0; i < numClients; i++ {
			wg.Add(1)
			go func(clientIndex int) {
				defer wg.Done()

				clientID := fmt.Sprintf("hivemq-scale-client-%d", clientIndex)
				opts := mqtt.NewClientOptions()
				opts.AddBroker("tcp://localhost:1883")
				opts.SetClientID(clientID)
				opts.SetUsername("test")
				opts.SetPassword("test")
				opts.SetProtocolVersion(4)
				opts.SetConnectTimeout(10 * time.Second)
				opts.SetKeepAlive(30 * time.Second)

				client := mqtt.NewClient(opts)
				clients[clientIndex] = client

				token := client.Connect()
				if token.WaitTimeout(15*time.Second) && token.Error() == nil {
					atomic.AddInt32(&successfulConnections, 1)
				} else {
					atomic.AddInt32(&connectionErrors, 1)
					t.Logf("Connection failed for client %d: %v", clientIndex, token.Error())
				}
			}(i)
		}

		wg.Wait()

		successful := atomic.LoadInt32(&successfulConnections)
		errors := atomic.LoadInt32(&connectionErrors)

		t.Logf("Connection scaling results: %d successful, %d errors", successful, errors)
		assert.GreaterOrEqual(t, successful, int32(numClients*0.8), "At least 80% of connections should succeed")

		// Clean up connections
		for _, client := range clients {
			if client != nil && client.IsConnected() {
				client.Disconnect(250)
			}
		}

		t.Log("✅ Connection scaling test completed")
	})

	t.Run("MessageThroughput", func(t *testing.T) {
		// Based on HiveMQ's message throughput testing
		// Test messages per second capacity

		publisher := createMQTTClientWithVersion("throughput-pub", 4, t)
		defer publisher.Disconnect(250)

		subscriber := createMQTTClientWithVersion("throughput-sub", 4, t)
		defer subscriber.Disconnect(250)

		topic := "hivemq/throughput/test"
		const numMessages = 100
		var messagesReceived int32
		var startTime, endTime time.Time

		// Subscribe and measure receive timing
		token := subscriber.Subscribe(topic, 1, func(client mqtt.Client, msg mqtt.Message) {
			received := atomic.AddInt32(&messagesReceived, 1)
			if received == 1 {
				startTime = time.Now()
			}
			if received == numMessages {
				endTime = time.Now()
			}
		})
		require.True(t, token.WaitTimeout(5*time.Second))
		require.NoError(t, token.Error())

		// Publish messages as fast as possible
		publishStart := time.Now()
		for i := 0; i < numMessages; i++ {
			message := fmt.Sprintf("Throughput test message %d", i+1)
			pubToken := publisher.Publish(topic, 1, false, message)
			require.True(t, pubToken.WaitTimeout(5*time.Second))
			require.NoError(t, pubToken.Error())
		}
		publishEnd := time.Now()

		// Wait for all messages to be received
		deadline := time.After(30 * time.Second)
		for atomic.LoadInt32(&messagesReceived) < numMessages {
			select {
			case <-deadline:
				break
			default:
				time.Sleep(10 * time.Millisecond)
			}
		}

		received := atomic.LoadInt32(&messagesReceived)
		publishDuration := publishEnd.Sub(publishStart)
		receiveDuration := endTime.Sub(startTime)

		publishRate := float64(numMessages) / publishDuration.Seconds()
		receiveRate := float64(received) / receiveDuration.Seconds()

		t.Logf("Throughput results: %d/%d messages received", received, numMessages)
		t.Logf("Publish rate: %.2f messages/second", publishRate)
		if !endTime.IsZero() {
			t.Logf("Receive rate: %.2f messages/second", receiveRate)
		}

		assert.Equal(t, int32(numMessages), received, "All messages should be received")
		assert.Greater(t, publishRate, 10.0, "Publish rate should be reasonable")

		t.Log("✅ Message throughput test completed")
	})

	t.Run("TopicSubscriptionScaling", func(t *testing.T) {
		// Based on HiveMQ's topic subscription scaling tests
		// Test increasing number of clients on same topic

		const numSubscribers = 20
		topic := "hivemq/topic/scaling/test"
		var totalReceived int32
		var wg sync.WaitGroup

		subscribers := make([]mqtt.Client, numSubscribers)

		// Create multiple subscribers to same topic
		for i := 0; i < numSubscribers; i++ {
			wg.Add(1)
			go func(subIndex int) {
				defer wg.Done()

				clientID := fmt.Sprintf("hivemq-topic-sub-%d", subIndex)
				subscriber := createMQTTClientWithVersion(clientID, 4, t)
				subscribers[subIndex] = subscriber

				token := subscriber.Subscribe(topic, 1, func(client mqtt.Client, msg mqtt.Message) {
					atomic.AddInt32(&totalReceived, 1)
				})
				require.True(t, token.WaitTimeout(5*time.Second))
				require.NoError(t, token.Error())
			}(i)
		}

		wg.Wait()

		// Single publisher to all subscribers
		publisher := createMQTTClientWithVersion("topic-scale-pub", 4, t)
		defer publisher.Disconnect(250)

		const numMessages = 5
		for i := 0; i < numMessages; i++ {
			message := fmt.Sprintf("Topic scaling message %d", i+1)
			pubToken := publisher.Publish(topic, 1, false, message)
			require.True(t, pubToken.WaitTimeout(5*time.Second))
			require.NoError(t, pubToken.Error())
		}

		// Wait for message distribution
		time.Sleep(3 * time.Second)

		received := atomic.LoadInt32(&totalReceived)
		expected := int32(numSubscribers * numMessages)

		t.Logf("Topic scaling results: %d messages received by %d subscribers", received, numSubscribers)
		assert.Equal(t, expected, received, "All subscribers should receive all messages")

		// Clean up
		for _, subscriber := range subscribers {
			if subscriber != nil {
				subscriber.Disconnect(250)
			}
		}

		t.Log("✅ Topic subscription scaling test completed")
	})

	t.Run("LatencyMeasurement", func(t *testing.T) {
		// Based on HiveMQ's latency measurement testing
		// Test average message delivery time

		publisher := createMQTTClientWithVersion("latency-pub", 4, t)
		defer publisher.Disconnect(250)

		subscriber := createMQTTClientWithVersion("latency-sub", 4, t)
		defer subscriber.Disconnect(250)

		topic := "hivemq/latency/test"
		const numSamples = 10
		latencies := make([]time.Duration, 0, numSamples)
		latencyMutex := sync.Mutex{}

		publishTimes := make(map[string]time.Time)
		timeMutex := sync.Mutex{}

		// Subscribe and measure latency
		token := subscriber.Subscribe(topic, 1, func(client mqtt.Client, msg mqtt.Message) {
			receiveTime := time.Now()
			payload := string(msg.Payload())

			timeMutex.Lock()
			publishTime, exists := publishTimes[payload]
			timeMutex.Unlock()

			if exists {
				latency := receiveTime.Sub(publishTime)
				latencyMutex.Lock()
				latencies = append(latencies, latency)
				latencyMutex.Unlock()
			}
		})
		require.True(t, token.WaitTimeout(5*time.Second))
		require.NoError(t, token.Error())

		// Publish messages with timing
		for i := 0; i < numSamples; i++ {
			message := fmt.Sprintf("Latency test message %d", i+1)
			publishTime := time.Now()

			timeMutex.Lock()
			publishTimes[message] = publishTime
			timeMutex.Unlock()

			pubToken := publisher.Publish(topic, 1, false, message)
			require.True(t, pubToken.WaitTimeout(5*time.Second))
			require.NoError(t, pubToken.Error())

			time.Sleep(100 * time.Millisecond) // Small gap between messages
		}

		// Wait for all messages
		time.Sleep(2 * time.Second)

		latencyMutex.Lock()
		defer latencyMutex.Unlock()

		if len(latencies) > 0 {
			var totalLatency time.Duration
			var maxLatency time.Duration
			minLatency := latencies[0]

			for _, latency := range latencies {
				totalLatency += latency
				if latency > maxLatency {
					maxLatency = latency
				}
				if latency < minLatency {
					minLatency = latency
				}
			}

			avgLatency := totalLatency / time.Duration(len(latencies))

			t.Logf("Latency measurement results (%d samples):", len(latencies))
			t.Logf("  Average: %v", avgLatency)
			t.Logf("  Min: %v", minLatency)
			t.Logf("  Max: %v", maxLatency)

			assert.Less(t, avgLatency, 100*time.Millisecond, "Average latency should be reasonable")
		} else {
			t.Log("⚠️ No latency measurements captured")
		}

		t.Log("✅ Latency measurement test completed")
	})

	t.Run("SpikeLoadTesting", func(t *testing.T) {
		// Based on HiveMQ's spike testing
		// Test sharp load changes and stress recovery

		const normalLoad = 5
		const spikeLoad = 25
		topic := "hivemq/spike/test"

		subscriber := createMQTTClientWithVersion("spike-sub", 4, t)
		defer subscriber.Disconnect(250)

		var messagesReceived int32
		token := subscriber.Subscribe(topic, 1, func(client mqtt.Client, msg mqtt.Message) {
			atomic.AddInt32(&messagesReceived, 1)
		})
		require.True(t, token.WaitTimeout(5*time.Second))
		require.NoError(t, token.Error())

		publisher := createMQTTClientWithVersion("spike-pub", 4, t)
		defer publisher.Disconnect(250)

		// Normal load phase
		t.Log("Starting normal load phase...")
		for i := 0; i < normalLoad; i++ {
			message := fmt.Sprintf("Normal load message %d", i+1)
			pubToken := publisher.Publish(topic, 1, false, message)
			require.True(t, pubToken.WaitTimeout(5*time.Second))
			require.NoError(t, pubToken.Error())
			time.Sleep(100 * time.Millisecond)
		}

		normalPhaseReceived := atomic.LoadInt32(&messagesReceived)

		// Spike load phase
		t.Log("Starting spike load phase...")
		var spikeWg sync.WaitGroup
		for i := 0; i < spikeLoad; i++ {
			spikeWg.Add(1)
			go func(msgIndex int) {
				defer spikeWg.Done()
				message := fmt.Sprintf("Spike load message %d", msgIndex+1)
				pubToken := publisher.Publish(topic, 1, false, message)
				pubToken.WaitTimeout(5 * time.Second)
			}(i)
		}
		spikeWg.Wait()

		// Recovery phase - back to normal load
		time.Sleep(1 * time.Second)
		t.Log("Starting recovery phase...")
		for i := 0; i < normalLoad; i++ {
			message := fmt.Sprintf("Recovery message %d", i+1)
			pubToken := publisher.Publish(topic, 1, false, message)
			require.True(t, pubToken.WaitTimeout(5*time.Second))
			require.NoError(t, pubToken.Error())
			time.Sleep(100 * time.Millisecond)
		}

		// Wait for all messages
		time.Sleep(3 * time.Second)

		totalReceived := atomic.LoadInt32(&messagesReceived)
		totalSent := int32(normalLoad + spikeLoad + normalLoad)

		t.Logf("Spike load results:")
		t.Logf("  Normal phase: %d messages", normalPhaseReceived)
		t.Logf("  Total received: %d/%d messages", totalReceived, totalSent)

		// Allow some tolerance for spike load scenarios
		assert.GreaterOrEqual(t, totalReceived, totalSent*8/10, "Should receive at least 80% of messages during spike test")

		t.Log("✅ Spike load testing completed")
	})
}