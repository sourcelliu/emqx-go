package e2e

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/eclipse/paho.mqtt.golang"
	"github.com/stretchr/testify/require"
	"github.com/turtacn/emqx-go/pkg/broker"
)

// BenchmarkMQTTPerformance runs performance benchmarks for MQTT operations
func BenchmarkMQTTPerformance(b *testing.B) {
	testBroker := setupTestBroker(b, ":1980")
	defer testBroker.Close()

	b.Run("PublishQoS0", func(b *testing.B) {
		benchmarkPublish(b, testBroker, 0)
	})

	b.Run("PublishQoS1", func(b *testing.B) {
		benchmarkPublish(b, testBroker, 1)
	})

	b.Run("PublishQoS2", func(b *testing.B) {
		benchmarkPublish(b, testBroker, 2)
	})

	b.Run("Subscribe", func(b *testing.B) {
		benchmarkSubscribe(b, testBroker)
	})

	b.Run("ConcurrentPublish", func(b *testing.B) {
		benchmarkConcurrentPublish(b, testBroker)
	})
}

// TestThroughputMeasurement measures message throughput under various conditions
func TestThroughputMeasurement(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping throughput tests in short mode")
	}

	testBroker := setupTestBroker(t, ":1981")
	defer testBroker.Close()

	t.Run("MaxThroughputQoS0", func(t *testing.T) {
		measureThroughput(t, testBroker, 0, 10000, 10*time.Second)
	})

	t.Run("MaxThroughputQoS1", func(t *testing.T) {
		measureThroughput(t, testBroker, 1, 5000, 10*time.Second)
	})

	t.Run("SustainedThroughput", func(t *testing.T) {
		measureSustainedThroughput(t, testBroker, 1000, 30*time.Second)
	})
}

// TestLatencyMeasurement measures message latency
func TestLatencyMeasurement(t *testing.T) {
	testBroker := setupTestBroker(t, ":1982")
	defer testBroker.Close()

	t.Run("PublishLatency", func(t *testing.T) {
		measurePublishLatency(t, testBroker)
	})

	t.Run("EndToEndLatency", func(t *testing.T) {
		measureEndToEndLatency(t, testBroker)
	})
}

// TestResourceUtilization measures resource usage
func TestResourceUtilization(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping resource utilization tests in short mode")
	}

	testBroker := setupTestBroker(t, ":1983")
	defer testBroker.Close()

	t.Run("MemoryUsage", func(t *testing.T) {
		measureMemoryUsage(t, testBroker)
	})

	t.Run("ConnectionScaling", func(t *testing.T) {
		measureConnectionScaling(t, testBroker)
	})
}

// TestStressScenarios runs stress tests
func TestStressScenarios(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress tests in short mode")
	}

	testBroker := setupTestBroker(t, ":1984")
	defer testBroker.Close()

	t.Run("HighConnectionChurn", func(t *testing.T) {
		testHighConnectionChurn(t, testBroker)
	})

	t.Run("MixedWorkload", func(t *testing.T) {
		testMixedWorkload(t, testBroker)
	})

	t.Run("LongRunningStability", func(t *testing.T) {
		testLongRunningStability(t, testBroker)
	})
}

// Implementation functions

func benchmarkPublish(b *testing.B, testBroker *broker.Broker, qos byte) {
	clientOptions := mqtt.NewClientOptions()
	clientOptions.AddBroker("tcp://localhost:1980")
	clientOptions.SetClientID("benchmark-publisher")
	clientOptions.SetUsername("test")
	clientOptions.SetPassword("test")

	client := mqtt.NewClient(clientOptions)
	token := client.Connect()
	require.True(b, token.WaitTimeout(5*time.Second))
	require.NoError(b, token.Error())
	defer client.Disconnect(250)

	message := "benchmark test message"
	topic := fmt.Sprintf("test/benchmark/qos%d", qos)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		pubToken := client.Publish(topic, qos, false, message)
		if !pubToken.WaitTimeout(5*time.Second) || pubToken.Error() != nil {
			b.Fatalf("Publish failed: %v", pubToken.Error())
		}
	}
}

func benchmarkSubscribe(b *testing.B, testBroker *broker.Broker) {
	clientOptions := mqtt.NewClientOptions()
	clientOptions.AddBroker("tcp://localhost:1980")
	clientOptions.SetClientID("benchmark-subscriber")
	clientOptions.SetUsername("test")
	clientOptions.SetPassword("test")

	client := mqtt.NewClient(clientOptions)
	token := client.Connect()
	require.True(b, token.WaitTimeout(5*time.Second))
	require.NoError(b, token.Error())
	defer client.Disconnect(250)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		topic := fmt.Sprintf("test/benchmark/sub/%d", i)
		subToken := client.Subscribe(topic, 1, nil)
		if !subToken.WaitTimeout(5*time.Second) || subToken.Error() != nil {
			b.Fatalf("Subscribe failed: %v", subToken.Error())
		}
	}
}

func benchmarkConcurrentPublish(b *testing.B, testBroker *broker.Broker) {
	const numWorkers = 10
	var wg sync.WaitGroup
	var clients []mqtt.Client

	// Create multiple publishers
	for i := 0; i < numWorkers; i++ {
		clientOptions := mqtt.NewClientOptions()
		clientOptions.AddBroker("tcp://localhost:1980")
		clientOptions.SetClientID(fmt.Sprintf("concurrent-publisher-%d", i))
		clientOptions.SetUsername("test")
		clientOptions.SetPassword("test")

		client := mqtt.NewClient(clientOptions)
		token := client.Connect()
		require.True(b, token.WaitTimeout(5*time.Second))
		require.NoError(b, token.Error())

		clients = append(clients, client)
	}

	defer func() {
		for _, client := range clients {
			client.Disconnect(250)
		}
	}()

	message := "concurrent benchmark message"

	b.ResetTimer()

	wg.Add(numWorkers)
	for i := 0; i < numWorkers; i++ {
		go func(clientIndex int) {
			defer wg.Done()
			client := clients[clientIndex]
			topic := fmt.Sprintf("test/concurrent/benchmark/%d", clientIndex)

			for j := 0; j < b.N/numWorkers; j++ {
				pubToken := client.Publish(topic, 0, false, message)
				if !pubToken.WaitTimeout(5*time.Second) || pubToken.Error() != nil {
					b.Errorf("Concurrent publish failed: %v", pubToken.Error())
					return
				}
			}
		}(i)
	}

	wg.Wait()
}

func measureThroughput(t *testing.T, testBroker *broker.Broker, qos byte, targetMessages int, duration time.Duration) {
	publisherOptions := mqtt.NewClientOptions()
	publisherOptions.AddBroker("tcp://localhost:1981")
	publisherOptions.SetClientID("throughput-publisher")
	publisherOptions.SetUsername("test")
	publisherOptions.SetPassword("test")

	publisher := mqtt.NewClient(publisherOptions)
	token := publisher.Connect()
	require.True(t, token.WaitTimeout(5*time.Second))
	require.NoError(t, token.Error())
	defer publisher.Disconnect(250)

	subscriberOptions := mqtt.NewClientOptions()
	subscriberOptions.AddBroker("tcp://localhost:1981")
	subscriberOptions.SetClientID("throughput-subscriber")
	subscriberOptions.SetUsername("test")
	subscriberOptions.SetPassword("test")

	subscriber := mqtt.NewClient(subscriberOptions)
	subToken := subscriber.Connect()
	require.True(t, subToken.WaitTimeout(5*time.Second))
	require.NoError(t, subToken.Error())
	defer subscriber.Disconnect(250)

	var receivedCount int64
	var publishedCount int64

	topic := fmt.Sprintf("test/throughput/qos%d", qos)

	subscribeToken := subscriber.Subscribe(topic, qos, func(client mqtt.Client, msg mqtt.Message) {
		atomic.AddInt64(&receivedCount, 1)
	})
	require.True(t, subscribeToken.WaitTimeout(5*time.Second))
	require.NoError(t, subscribeToken.Error())

	message := make([]byte, 100) // 100-byte message
	for i := range message {
		message[i] = byte(i % 256)
	}

	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), duration)
	defer cancel()

	// Publishing goroutine
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
				pubToken := publisher.Publish(topic, qos, false, message)
				if pubToken.WaitTimeout(100*time.Millisecond) && pubToken.Error() == nil {
					count := atomic.AddInt64(&publishedCount, 1)
					if int(count) >= targetMessages {
						return
					}
				}
			}
		}
	}()

	// Wait for completion or timeout
	<-ctx.Done()
	elapsed := time.Since(start)

	finalPublished := atomic.LoadInt64(&publishedCount)
	finalReceived := atomic.LoadInt64(&receivedCount)

	publishRate := float64(finalPublished) / elapsed.Seconds()
	receiveRate := float64(finalReceived) / elapsed.Seconds()
	deliveryRatio := float64(finalReceived) / float64(finalPublished)

	t.Logf("QoS %d Throughput Results:", qos)
	t.Logf("  Published: %d messages in %v (%.1f msgs/sec)", finalPublished, elapsed, publishRate)
	t.Logf("  Received: %d messages (%.1f msgs/sec)", finalReceived, receiveRate)
	t.Logf("  Delivery ratio: %.2f%%", deliveryRatio*100)

	if deliveryRatio < 0.95 {
		t.Logf("  Warning: Low delivery ratio %.2f%% for QoS %d", deliveryRatio*100, qos)
	}
}

func measureSustainedThroughput(t *testing.T, testBroker *broker.Broker, targetRate int, duration time.Duration) {
	publisherOptions := mqtt.NewClientOptions()
	publisherOptions.AddBroker("tcp://localhost:1981")
	publisherOptions.SetClientID("sustained-publisher")
	publisherOptions.SetUsername("test")
	publisherOptions.SetPassword("test")

	publisher := mqtt.NewClient(publisherOptions)
	token := publisher.Connect()
	require.True(t, token.WaitTimeout(5*time.Second))
	require.NoError(t, token.Error())
	defer publisher.Disconnect(250)

	subscriberOptions := mqtt.NewClientOptions()
	subscriberOptions.AddBroker("tcp://localhost:1981")
	subscriberOptions.SetClientID("sustained-subscriber")
	subscriberOptions.SetUsername("test")
	subscriberOptions.SetPassword("test")

	subscriber := mqtt.NewClient(subscriberOptions)
	subToken := subscriber.Connect()
	require.True(t, subToken.WaitTimeout(5*time.Second))
	require.NoError(t, subToken.Error())
	defer subscriber.Disconnect(250)

	var receivedCount int64
	var publishedCount int64

	subscribeToken := subscriber.Subscribe("test/sustained/throughput", 1, func(client mqtt.Client, msg mqtt.Message) {
		atomic.AddInt64(&receivedCount, 1)
	})
	require.True(t, subscribeToken.WaitTimeout(5*time.Second))
	require.NoError(t, subscribeToken.Error())

	message := "sustained throughput test message"
	interval := time.Second / time.Duration(targetRate)

	start := time.Now()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	ctx, cancel := context.WithTimeout(context.Background(), duration)
	defer cancel()

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				pubToken := publisher.Publish("test/sustained/throughput", 1, false, message)
				if pubToken.WaitTimeout(100*time.Millisecond) && pubToken.Error() == nil {
					atomic.AddInt64(&publishedCount, 1)
				}
			}
		}
	}()

	<-ctx.Done()
	elapsed := time.Since(start)

	finalPublished := atomic.LoadInt64(&publishedCount)
	finalReceived := atomic.LoadInt64(&receivedCount)

	actualRate := float64(finalPublished) / elapsed.Seconds()
	deliveryRatio := float64(finalReceived) / float64(finalPublished)

	t.Logf("Sustained Throughput Results:")
	t.Logf("  Target rate: %d msgs/sec", targetRate)
	t.Logf("  Actual rate: %.1f msgs/sec", actualRate)
	t.Logf("  Published: %d messages", finalPublished)
	t.Logf("  Received: %d messages", finalReceived)
	t.Logf("  Delivery ratio: %.2f%%", deliveryRatio*100)
}

func measurePublishLatency(t *testing.T, testBroker *broker.Broker) {
	clientOptions := mqtt.NewClientOptions()
	clientOptions.AddBroker("tcp://localhost:1982")
	clientOptions.SetClientID("latency-publisher")
	clientOptions.SetUsername("test")
	clientOptions.SetPassword("test")

	client := mqtt.NewClient(clientOptions)
	token := client.Connect()
	require.True(t, token.WaitTimeout(5*time.Second))
	require.NoError(t, token.Error())
	defer client.Disconnect(250)

	const numSamples = 100
	latencies := make([]time.Duration, numSamples)
	message := "latency test message"

	for i := 0; i < numSamples; i++ {
		start := time.Now()
		pubToken := client.Publish("test/latency/publish", 1, false, message)
		if pubToken.WaitTimeout(5*time.Second) && pubToken.Error() == nil {
			latencies[i] = time.Since(start)
		} else {
			t.Fatalf("Publish failed: %v", pubToken.Error())
		}
		time.Sleep(10 * time.Millisecond) // Small delay between samples
	}

	// Calculate statistics
	var total time.Duration
	var min, max time.Duration = latencies[0], latencies[0]

	for _, latency := range latencies {
		total += latency
		if latency < min {
			min = latency
		}
		if latency > max {
			max = latency
		}
	}

	avg := total / time.Duration(numSamples)

	t.Logf("Publish Latency Results (QoS 1):")
	t.Logf("  Average: %v", avg)
	t.Logf("  Min: %v", min)
	t.Logf("  Max: %v", max)
	t.Logf("  Samples: %d", numSamples)
}

func measureEndToEndLatency(t *testing.T, testBroker *broker.Broker) {
	publisherOptions := mqtt.NewClientOptions()
	publisherOptions.AddBroker("tcp://localhost:1982")
	publisherOptions.SetClientID("e2e-latency-publisher")
	publisherOptions.SetUsername("test")
	publisherOptions.SetPassword("test")

	publisher := mqtt.NewClient(publisherOptions)
	token := publisher.Connect()
	require.True(t, token.WaitTimeout(5*time.Second))
	require.NoError(t, token.Error())
	defer publisher.Disconnect(250)

	subscriberOptions := mqtt.NewClientOptions()
	subscriberOptions.AddBroker("tcp://localhost:1982")
	subscriberOptions.SetClientID("e2e-latency-subscriber")
	subscriberOptions.SetUsername("test")
	subscriberOptions.SetPassword("test")

	subscriber := mqtt.NewClient(subscriberOptions)
	subToken := subscriber.Connect()
	require.True(t, subToken.WaitTimeout(5*time.Second))
	require.NoError(t, subToken.Error())
	defer subscriber.Disconnect(250)

	const numSamples = 50
	latencies := make([]time.Duration, 0, numSamples)
	var mu sync.Mutex
	var wg sync.WaitGroup

	subscribeToken := subscriber.Subscribe("test/latency/e2e", 1, func(client mqtt.Client, msg mqtt.Message) {
		// Extract timestamp from message payload
		sentTime, err := time.Parse(time.RFC3339Nano, string(msg.Payload()))
		if err != nil {
			return
		}
		latency := time.Since(sentTime)

		mu.Lock()
		latencies = append(latencies, latency)
		mu.Unlock()
		wg.Done()
	})
	require.True(t, subscribeToken.WaitTimeout(5*time.Second))
	require.NoError(t, subscribeToken.Error())

	for i := 0; i < numSamples; i++ {
		wg.Add(1)
		timestamp := time.Now().Format(time.RFC3339Nano)
		pubToken := publisher.Publish("test/latency/e2e", 1, false, timestamp)
		require.True(t, pubToken.WaitTimeout(5*time.Second))
		require.NoError(t, pubToken.Error())

		time.Sleep(50 * time.Millisecond) // Small delay between samples
	}

	// Wait for all messages to be received
	wg.Wait()

	mu.Lock()
	receivedLatencies := make([]time.Duration, len(latencies))
	copy(receivedLatencies, latencies)
	mu.Unlock()

	if len(receivedLatencies) == 0 {
		t.Fatal("No latency measurements received")
	}

	// Calculate statistics
	var total time.Duration
	var min, max time.Duration = receivedLatencies[0], receivedLatencies[0]

	for _, latency := range receivedLatencies {
		total += latency
		if latency < min {
			min = latency
		}
		if latency > max {
			max = latency
		}
	}

	avg := total / time.Duration(len(receivedLatencies))

	t.Logf("End-to-End Latency Results:")
	t.Logf("  Average: %v", avg)
	t.Logf("  Min: %v", min)
	t.Logf("  Max: %v", max)
	t.Logf("  Samples: %d", len(receivedLatencies))
}

func measureMemoryUsage(t *testing.T, testBroker *broker.Broker) {
	var m1, m2 runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&m1)

	// Create connections and publish messages
	const numClients = 100
	const messagesPerClient = 100

	var clients []mqtt.Client
	defer func() {
		for _, client := range clients {
			if client.IsConnected() {
				client.Disconnect(100)
			}
		}
	}()

	// Create clients
	for i := 0; i < numClients; i++ {
		clientOptions := mqtt.NewClientOptions()
		clientOptions.AddBroker("tcp://localhost:1983")
		clientOptions.SetClientID(fmt.Sprintf("memory-test-client-%d", i))
		clientOptions.SetUsername("test")
		clientOptions.SetPassword("test")

		client := mqtt.NewClient(clientOptions)
		token := client.Connect()
		require.True(t, token.WaitTimeout(5*time.Second))
		require.NoError(t, token.Error())

		clients = append(clients, client)
	}

	// Subscribe and publish messages
	for i, client := range clients {
		topic := fmt.Sprintf("test/memory/%d", i)
		subToken := client.Subscribe(topic, 0, nil)
		require.True(t, subToken.WaitTimeout(5*time.Second))
		require.NoError(t, subToken.Error())

		for j := 0; j < messagesPerClient; j++ {
			message := fmt.Sprintf("memory test message %d-%d", i, j)
			pubToken := client.Publish(topic, 0, false, message)
			require.True(t, pubToken.WaitTimeout(2*time.Second))
			require.NoError(t, pubToken.Error())
		}
	}

	time.Sleep(2 * time.Second) // Let messages settle

	runtime.GC()
	runtime.ReadMemStats(&m2)

	allocatedMB := float64(m2.Alloc-m1.Alloc) / 1024 / 1024
	totalAllocMB := float64(m2.TotalAlloc-m1.TotalAlloc) / 1024 / 1024

	t.Logf("Memory Usage Results:")
	t.Logf("  Clients: %d", numClients)
	t.Logf("  Messages: %d", numClients*messagesPerClient)
	t.Logf("  Memory allocated: %.2f MB", allocatedMB)
	t.Logf("  Total allocated: %.2f MB", totalAllocMB)
	t.Logf("  Memory per client: %.2f KB", allocatedMB*1024/float64(numClients))
}

func measureConnectionScaling(t *testing.T, testBroker *broker.Broker) {
	connectionCounts := []int{10, 50, 100, 200}

	for _, numConnections := range connectionCounts {
		t.Logf("Testing %d concurrent connections...", numConnections)

		start := time.Now()
		var clients []mqtt.Client
		var successfulConnections int

		for i := 0; i < numConnections; i++ {
			clientOptions := mqtt.NewClientOptions()
			clientOptions.AddBroker("tcp://localhost:1983")
			clientOptions.SetClientID(fmt.Sprintf("scaling-client-%d-%d", numConnections, i))
			clientOptions.SetConnectTimeout(10 * time.Second)
			clientOptions.SetUsername("test")
			clientOptions.SetPassword("test")

			client := mqtt.NewClient(clientOptions)
			token := client.Connect()

			if token.WaitTimeout(10*time.Second) && token.Error() == nil {
				successfulConnections++
				clients = append(clients, client)
			}
		}

		connectionTime := time.Since(start)
		successRate := float64(successfulConnections) / float64(numConnections)

		t.Logf("  Connected %d/%d clients in %v (%.1f%%)",
			successfulConnections, numConnections, connectionTime, successRate*100)
		t.Logf("  Connection rate: %.1f connections/sec",
			float64(successfulConnections)/connectionTime.Seconds())

		// Clean up
		for _, client := range clients {
			client.Disconnect(100)
		}

		time.Sleep(1 * time.Second) // Brief pause between tests
	}
}

func testHighConnectionChurn(t *testing.T, testBroker *broker.Broker) {
	// Test rapid connect/disconnect cycles
	const duration = 30 * time.Second
	const numWorkers = 10

	var connectCount, disconnectCount int64
	var wg sync.WaitGroup

	ctx, cancel := context.WithTimeout(context.Background(), duration)
	defer cancel()

	start := time.Now()

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			clientIndex := 0

			for {
				select {
				case <-ctx.Done():
					return
				default:
					clientOptions := mqtt.NewClientOptions()
					clientOptions.AddBroker("tcp://localhost:1984")
					clientOptions.SetClientID(fmt.Sprintf("churn-client-%d-%d", workerID, clientIndex))
					clientOptions.SetConnectTimeout(5 * time.Second)
					clientOptions.SetUsername("test")
					clientOptions.SetPassword("test")

					client := mqtt.NewClient(clientOptions)
					token := client.Connect()

					if token.WaitTimeout(5*time.Second) && token.Error() == nil {
						atomic.AddInt64(&connectCount, 1)

						// Stay connected briefly
						time.Sleep(100 * time.Millisecond)

						client.Disconnect(100)
						atomic.AddInt64(&disconnectCount, 1)
					}

					clientIndex++
				}
			}
		}(i)
	}

	wg.Wait()
	elapsed := time.Since(start)

	finalConnects := atomic.LoadInt64(&connectCount)
	finalDisconnects := atomic.LoadInt64(&disconnectCount)

	connectRate := float64(finalConnects) / elapsed.Seconds()
	disconnectRate := float64(finalDisconnects) / elapsed.Seconds()

	t.Logf("High Connection Churn Results:")
	t.Logf("  Duration: %v", elapsed)
	t.Logf("  Connections: %d (%.1f/sec)", finalConnects, connectRate)
	t.Logf("  Disconnections: %d (%.1f/sec)", finalDisconnects, disconnectRate)
	t.Logf("  Workers: %d", numWorkers)
}

func testMixedWorkload(t *testing.T, testBroker *broker.Broker) {
	// Test mixed workload with publishers, subscribers, and connection churn
	const duration = 20 * time.Second
	var publishCount, receiveCount int64

	ctx, cancel := context.WithTimeout(context.Background(), duration)
	defer cancel()

	var wg sync.WaitGroup

	// Publishers
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(publisherID int) {
			defer wg.Done()

			clientOptions := mqtt.NewClientOptions()
			clientOptions.AddBroker("tcp://localhost:1984")
			clientOptions.SetClientID(fmt.Sprintf("mixed-publisher-%d", publisherID))
			clientOptions.SetUsername("test")
			clientOptions.SetPassword("test")

			client := mqtt.NewClient(clientOptions)
			token := client.Connect()
			if !token.WaitTimeout(5*time.Second) || token.Error() != nil {
				return
			}
			defer client.Disconnect(250)

			ticker := time.NewTicker(100 * time.Millisecond)
			defer ticker.Stop()

			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					topic := fmt.Sprintf("test/mixed/workload/%d", publisherID%3)
					message := fmt.Sprintf("mixed message from %d", publisherID)
					pubToken := client.Publish(topic, 1, false, message)
					if pubToken.WaitTimeout(1*time.Second) && pubToken.Error() == nil {
						atomic.AddInt64(&publishCount, 1)
					}
				}
			}
		}(i)
	}

	// Subscribers
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func(subscriberID int) {
			defer wg.Done()

			clientOptions := mqtt.NewClientOptions()
			clientOptions.AddBroker("tcp://localhost:1984")
			clientOptions.SetClientID(fmt.Sprintf("mixed-subscriber-%d", subscriberID))
			clientOptions.SetUsername("test")
			clientOptions.SetPassword("test")

			client := mqtt.NewClient(clientOptions)
			token := client.Connect()
			if !token.WaitTimeout(5*time.Second) || token.Error() != nil {
				return
			}
			defer client.Disconnect(250)

			topic := fmt.Sprintf("test/mixed/workload/%d", subscriberID)
			subToken := client.Subscribe(topic, 1, func(client mqtt.Client, msg mqtt.Message) {
				atomic.AddInt64(&receiveCount, 1)
			})
			if !subToken.WaitTimeout(5*time.Second) || subToken.Error() != nil {
				return
			}

			<-ctx.Done()
		}(i)
	}

	// Connection churn
	wg.Add(1)
	go func() {
		defer wg.Done()
		clientIndex := 0

		for {
			select {
			case <-ctx.Done():
				return
			default:
				clientOptions := mqtt.NewClientOptions()
				clientOptions.AddBroker("tcp://localhost:1984")
				clientOptions.SetClientID(fmt.Sprintf("mixed-churn-%d", clientIndex))
				clientOptions.SetUsername("test")
				clientOptions.SetPassword("test")

				client := mqtt.NewClient(clientOptions)
				token := client.Connect()
				if token.WaitTimeout(3*time.Second) && token.Error() == nil {
					time.Sleep(500 * time.Millisecond)
					client.Disconnect(100)
				}
				clientIndex++
				time.Sleep(200 * time.Millisecond)
			}
		}
	}()

	wg.Wait()

	finalPublished := atomic.LoadInt64(&publishCount)
	finalReceived := atomic.LoadInt64(&receiveCount)
	deliveryRatio := float64(finalReceived) / float64(finalPublished)

	t.Logf("Mixed Workload Results:")
	t.Logf("  Published: %d messages", finalPublished)
	t.Logf("  Received: %d messages", finalReceived)
	t.Logf("  Delivery ratio: %.2f%%", deliveryRatio*100)
	t.Logf("  Duration: %v", duration)
}

func testLongRunningStability(t *testing.T, testBroker *broker.Broker) {
	// Test long-running stability (shortened for practical testing)
	const duration = 2 * time.Minute
	const numClients = 20

	var clients []mqtt.Client
	var messageCount int64
	var errorCount int64

	defer func() {
		for _, client := range clients {
			if client.IsConnected() {
				client.Disconnect(250)
			}
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), duration)
	defer cancel()

	// Create persistent clients
	for i := 0; i < numClients; i++ {
		clientOptions := mqtt.NewClientOptions()
		clientOptions.AddBroker("tcp://localhost:1984")
		clientOptions.SetClientID(fmt.Sprintf("stability-client-%d", i))
		clientOptions.SetKeepAlive(30 * time.Second)
		clientOptions.SetUsername("test")
		clientOptions.SetPassword("test")

		client := mqtt.NewClient(clientOptions)
		token := client.Connect()
		require.True(t, token.WaitTimeout(5*time.Second))
		require.NoError(t, token.Error())

		clients = append(clients, client)

		// Subscribe each client
		topic := fmt.Sprintf("test/stability/%d", i%5)
		subToken := client.Subscribe(topic, 1, func(client mqtt.Client, msg mqtt.Message) {
			atomic.AddInt64(&messageCount, 1)
		})
		require.True(t, subToken.WaitTimeout(5*time.Second))
		require.NoError(t, subToken.Error())
	}

	start := time.Now()

	// Publishing loop
	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		messageNum := 0

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				for i := 0; i < 5; i++ {
					topic := fmt.Sprintf("test/stability/%d", i)
					message := fmt.Sprintf("stability message %d", messageNum)

					publisher := clients[messageNum%len(clients)]
					pubToken := publisher.Publish(topic, 1, false, message)
					if !pubToken.WaitTimeout(2*time.Second) || pubToken.Error() != nil {
						atomic.AddInt64(&errorCount, 1)
					}
				}
				messageNum++
			}
		}
	}()

	<-ctx.Done()
	elapsed := time.Since(start)

	finalMessages := atomic.LoadInt64(&messageCount)
	finalErrors := atomic.LoadInt64(&errorCount)

	// Check that all clients are still connected
	connectedCount := 0
	for _, client := range clients {
		if client.IsConnected() {
			connectedCount++
		}
	}

	t.Logf("Long-Running Stability Results:")
	t.Logf("  Duration: %v", elapsed)
	t.Logf("  Clients: %d", numClients)
	t.Logf("  Connected at end: %d/%d", connectedCount, numClients)
	t.Logf("  Messages processed: %d", finalMessages)
	t.Logf("  Errors: %d", finalErrors)
	t.Logf("  Message rate: %.1f msgs/sec", float64(finalMessages)/elapsed.Seconds())

	if connectedCount < numClients {
		t.Logf("  Warning: %d clients disconnected during test", numClients-connectedCount)
	}
}

// Helper function for benchmark setup
func setupTestBroker(tb testing.TB, port string) *broker.Broker {
	testBroker := broker.New("test-broker", nil)
	testBroker.SetupDefaultAuth()

	ctx := context.Background()
	go testBroker.StartServer(ctx, port)

	// Wait for broker to start
	time.Sleep(500 * time.Millisecond)

	return testBroker
}