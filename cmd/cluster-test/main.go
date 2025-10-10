package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

// TestConfig holds test configuration
type TestConfig struct {
	Node1Host         string
	Node1Port         int
	Node2Host         string
	Node2Port         int
	Username          string
	Password          string
	Topic             string
	TestMessage       string
	RouteWaitTime     time.Duration
	DeliveryWaitTime  time.Duration
	Verbose           bool
}

// DefaultTestConfig returns default test configuration
func DefaultTestConfig() *TestConfig {
	return &TestConfig{
		Node1Host:        "localhost",
		Node1Port:        1883,
		Node2Host:        "localhost",
		Node2Port:        1884,
		Username:         "test",
		Password:         "test",
		Topic:            "cluster/test",
		TestMessage:      "Hello from Node1 to Node2!",
		RouteWaitTime:    2 * time.Second,
		DeliveryWaitTime: 3 * time.Second,
		Verbose:          false,
	}
}

func main() {
	config := DefaultTestConfig()

	// Parse command line flags
	flag.StringVar(&config.Node1Host, "node1-host", config.Node1Host, "Node1 hostname")
	flag.IntVar(&config.Node1Port, "node1-port", config.Node1Port, "Node1 MQTT port")
	flag.StringVar(&config.Node2Host, "node2-host", config.Node2Host, "Node2 hostname")
	flag.IntVar(&config.Node2Port, "node2-port", config.Node2Port, "Node2 MQTT port")
	flag.StringVar(&config.Username, "username", config.Username, "MQTT username")
	flag.StringVar(&config.Password, "password", config.Password, "MQTT password")
	flag.StringVar(&config.Topic, "topic", config.Topic, "Test topic")
	flag.StringVar(&config.TestMessage, "message", config.TestMessage, "Test message")
	flag.DurationVar(&config.RouteWaitTime, "route-wait", config.RouteWaitTime, "Time to wait for route propagation")
	flag.DurationVar(&config.DeliveryWaitTime, "delivery-wait", config.DeliveryWaitTime, "Time to wait for message delivery")
	flag.BoolVar(&config.Verbose, "verbose", config.Verbose, "Enable verbose logging")
	flag.Parse()

	// Run test
	exitCode := runTest(config)
	os.Exit(exitCode)
}

func runTest(config *TestConfig) int {
	printHeader()

	var receivedMessages []string
	var mutex sync.Mutex
	messageReceived := make(chan string, 10)
	errorOccurred := false

	// Setup connection error logging
	if config.Verbose {
		mqtt.ERROR = &verboseLogger{prefix: "[ERROR] "}
		mqtt.CRITICAL = &verboseLogger{prefix: "[CRITICAL] "}
		mqtt.WARN = &verboseLogger{prefix: "[WARN] "}
		mqtt.DEBUG = &verboseLogger{prefix: "[DEBUG] "}
	}

	// Step 1: Create subscriber connecting to node2
	fmt.Printf("\n1. Creating subscriber connecting to %s:%d...\n", config.Node2Host, config.Node2Port)

	subOpts := mqtt.NewClientOptions()
	subOpts.AddBroker(fmt.Sprintf("tcp://%s:%d", config.Node2Host, config.Node2Port))
	subOpts.SetClientID("test-subscriber")
	subOpts.SetUsername(config.Username)
	subOpts.SetPassword(config.Password)
	subOpts.SetKeepAlive(60 * time.Second)
	subOpts.SetAutoReconnect(false)
	subOpts.SetConnectTimeout(5 * time.Second)
	subOpts.SetConnectionLostHandler(func(client mqtt.Client, err error) {
		fmt.Printf("✗ Subscriber connection lost: %v\n", err)
		errorOccurred = true
	})

	subscriber := mqtt.NewClient(subOpts)
	token := subscriber.Connect()

	if !token.WaitTimeout(5 * time.Second) {
		fmt.Println("✗ Subscriber connection timeout")
		return 1
	}

	if token.Error() != nil {
		fmt.Printf("✗ Failed to connect subscriber: %v\n", token.Error())
		fmt.Printf("  → Make sure node2 is running on %s:%d\n", config.Node2Host, config.Node2Port)
		fmt.Println("  → Check if the cluster is started (run: ./scripts/start-cluster.sh)")
		return 1
	}

	fmt.Printf("✓ Subscriber connected to node2 (%s:%d)\n", config.Node2Host, config.Node2Port)
	defer subscriber.Disconnect(250)

	// Step 2: Subscribe to topic
	fmt.Printf("\n2. Subscribing to topic '%s'...\n", config.Topic)

	token = subscriber.Subscribe(config.Topic, 1, func(client mqtt.Client, msg mqtt.Message) {
		mutex.Lock()
		receivedMsg := string(msg.Payload())
		receivedMessages = append(receivedMessages, receivedMsg)
		mutex.Unlock()

		timestamp := time.Now().Format("15:04:05.000")
		fmt.Printf("✓ [%s] RECEIVED: '%s' on topic '%s' (QoS: %d, Retained: %t)\n",
			timestamp, receivedMsg, msg.Topic(), msg.Qos(), msg.Retained())
		messageReceived <- receivedMsg
	})

	if !token.WaitTimeout(5 * time.Second) {
		fmt.Println("✗ Subscribe timeout")
		return 1
	}

	if token.Error() != nil {
		fmt.Printf("✗ Failed to subscribe: %v\n", token.Error())
		return 1
	}

	fmt.Println("✓ Subscribed successfully")

	// Wait for route propagation
	fmt.Printf("\n3. Waiting for route propagation (%v)...\n", config.RouteWaitTime)
	time.Sleep(config.RouteWaitTime)

	// Step 3: Create publisher connecting to node1
	fmt.Printf("\n4. Creating publisher connecting to %s:%d...\n", config.Node1Host, config.Node1Port)

	pubOpts := mqtt.NewClientOptions()
	pubOpts.AddBroker(fmt.Sprintf("tcp://%s:%d", config.Node1Host, config.Node1Port))
	pubOpts.SetClientID("test-publisher")
	pubOpts.SetUsername(config.Username)
	pubOpts.SetPassword(config.Password)
	pubOpts.SetKeepAlive(60 * time.Second)
	pubOpts.SetAutoReconnect(false)
	pubOpts.SetConnectTimeout(5 * time.Second)
	pubOpts.SetConnectionLostHandler(func(client mqtt.Client, err error) {
		fmt.Printf("✗ Publisher connection lost: %v\n", err)
		errorOccurred = true
	})

	publisher := mqtt.NewClient(pubOpts)
	token = publisher.Connect()

	if !token.WaitTimeout(5 * time.Second) {
		fmt.Println("✗ Publisher connection timeout")
		return 1
	}

	if token.Error() != nil {
		fmt.Printf("✗ Failed to connect publisher: %v\n", token.Error())
		fmt.Printf("  → Make sure node1 is running on %s:%d\n", config.Node1Host, config.Node1Port)
		fmt.Println("  → Check if the cluster is started (run: ./scripts/start-cluster.sh)")
		return 1
	}

	fmt.Printf("✓ Publisher connected to node1 (%s:%d)\n", config.Node1Host, config.Node1Port)
	defer publisher.Disconnect(250)

	// Step 4: Publish message
	fmt.Printf("\n5. Publishing message from node1 to topic '%s'...\n", config.Topic)
	fmt.Printf("   Message: \"%s\"\n", config.TestMessage)

	publishTime := time.Now()
	token = publisher.Publish(config.Topic, 1, false, config.TestMessage)

	if !token.WaitTimeout(5 * time.Second) {
		fmt.Println("✗ Publish timeout")
		return 1
	}

	if token.Error() != nil {
		fmt.Printf("✗ Failed to publish: %v\n", token.Error())
		return 1
	}

	fmt.Println("✓ Message published successfully")

	// Step 5: Wait for message delivery
	fmt.Printf("\n6. Waiting for cross-node message delivery (%v)...\n", config.DeliveryWaitTime)
	timeout := time.After(config.DeliveryWaitTime)
	success := false

	select {
	case msg := <-messageReceived:
		deliveryTime := time.Since(publishTime)
		if msg == config.TestMessage {
			success = true
			fmt.Printf("✓ Message received in %v\n", deliveryTime.Round(time.Millisecond))
		} else {
			fmt.Printf("✗ Received unexpected message: '%s'\n", msg)
		}
	case <-timeout:
		fmt.Printf("✗ Timeout: No message received within %v\n", config.DeliveryWaitTime)
		if errorOccurred {
			fmt.Println("  → Connection error occurred during test")
		}
	}

	// Print test results
	printResults(success, config, receivedMessages, &mutex)

	if success {
		return 0
	}
	return 1
}

func printHeader() {
	fmt.Println(strings.Repeat("=", 70))
	fmt.Println("EMQX-Go Cluster Cross-Node Messaging Test")
	fmt.Println(strings.Repeat("=", 70))
	fmt.Printf("Test started at: %s\n", time.Now().Format("2006-01-02 15:04:05"))
}

func printResults(success bool, config *TestConfig, receivedMessages []string, mutex *sync.Mutex) {
	fmt.Println("\n" + strings.Repeat("=", 70))
	fmt.Println("Test Results:")
	fmt.Println(strings.Repeat("=", 70))

	if success {
		fmt.Println("✓✓✓ SUCCESS! Cross-node messaging works!")
		fmt.Printf("✓ Message '%s' successfully routed from node1 to node2\n", config.TestMessage)
		fmt.Printf("  → Publisher: %s:%d\n", config.Node1Host, config.Node1Port)
		fmt.Printf("  → Subscriber: %s:%d\n", config.Node2Host, config.Node2Port)
		fmt.Printf("  → Topic: %s\n", config.Topic)
	} else {
		fmt.Println("✗✗✗ FAILED! Cross-node messaging did not work!")
		fmt.Printf("✗ Expected to receive: '%s'\n", config.TestMessage)

		mutex.Lock()
		if len(receivedMessages) > 0 {
			fmt.Printf("✗ Actually received %d message(s):\n", len(receivedMessages))
			for i, msg := range receivedMessages {
				fmt.Printf("    %d. '%s'\n", i+1, msg)
			}
		} else {
			fmt.Println("✗ No messages received")
		}
		mutex.Unlock()

		fmt.Println("\nTroubleshooting:")
		fmt.Println("  1. Check if both nodes are running:")
		fmt.Println("     ps aux | grep emqx-go")
		fmt.Println("  2. Check cluster logs:")
		fmt.Println("     tail -f ./logs/node1.log")
		fmt.Println("     tail -f ./logs/node2.log")
		fmt.Println("  3. Verify nodes joined the cluster (look for 'Successfully joined cluster')")
		fmt.Println("  4. Verify route propagation (look for 'BatchUpdateRoutes')")
		fmt.Println("  5. Check for forwarding errors (look for 'Cannot forward publish')")
	}

	fmt.Println(strings.Repeat("=", 70))
	fmt.Printf("Test completed at: %s\n", time.Now().Format("2006-01-02 15:04:05"))
}

// verboseLogger implements mqtt.Logger for verbose output
type verboseLogger struct {
	prefix string
}

func (l *verboseLogger) Println(v ...interface{}) {
	fmt.Print(l.prefix)
	fmt.Println(v...)
}

func (l *verboseLogger) Printf(format string, v ...interface{}) {
	fmt.Print(l.prefix)
	fmt.Printf(format, v...)
}
