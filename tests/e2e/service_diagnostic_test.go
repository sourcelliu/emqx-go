package e2e

import (
	"net"
	"testing"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

// TestServiceDiagnostic performs a comprehensive diagnostic of the emqx-go service
func TestServiceDiagnostic(t *testing.T) {
	t.Log("Starting emqx-go service diagnostic test")

	// Test 1: Check if MQTT port 1883 is accessible
	t.Log("Checking MQTT port 1883 connectivity...")
	conn, err := net.DialTimeout("tcp", "localhost:1883", 5*time.Second)
	if err != nil {
		t.Errorf("Failed to connect to MQTT port 1883: %v", err)
		return
	}
	conn.Close()
	t.Log("✓ MQTT port 1883 is accessible")

	// Test 2: Check if gRPC port 8081 is accessible
	t.Log("Checking gRPC port 8081 connectivity...")
	conn, err = net.DialTimeout("tcp", "localhost:8081", 5*time.Second)
	if err != nil {
		t.Errorf("Failed to connect to gRPC port 8081: %v", err)
		return
	}
	conn.Close()
	t.Log("✓ gRPC port 8081 is accessible")

	// Test 3: Check if metrics port 8082 is accessible
	t.Log("Checking metrics port 8082 connectivity...")
	conn, err = net.DialTimeout("tcp", "localhost:8082", 5*time.Second)
	if err != nil {
		t.Errorf("Failed to connect to metrics port 8082: %v", err)
		return
	}
	conn.Close()
	t.Log("✓ Metrics port 8082 is accessible")

	// Test 4: Test MQTT protocol functionality
	t.Log("Testing MQTT protocol functionality...")

	opts := mqtt.NewClientOptions()
	opts.AddBroker("tcp://localhost:1883")
	opts.SetClientID("diagnostic-client")
	opts.SetConnectTimeout(10 * time.Second)
	opts.SetKeepAlive(30 * time.Second)

	// Add connection lost handler to detect any connection issues
	connectionLostChan := make(chan error, 1)
	opts.SetConnectionLostHandler(func(client mqtt.Client, err error) {
		t.Logf("⚠️ Connection lost during diagnostic: %v", err)
		connectionLostChan <- err
	})

	client := mqtt.NewClient(opts)
	token := client.Connect()

	// Wait for connection with timeout
	if !token.WaitTimeout(10 * time.Second) {
		t.Errorf("Connection timeout after 10 seconds")
		return
	}

	if token.Error() != nil {
		t.Errorf("MQTT connection failed: %v", token.Error())
		return
	}

	t.Log("✓ MQTT connection established successfully")

	// Test 5: Test subscribe functionality
	subscribed := make(chan bool, 1)
	messageReceived := make(chan string, 1)

	subToken := client.Subscribe("diagnostic/test", 1, func(client mqtt.Client, msg mqtt.Message) {
		t.Logf("✓ Received diagnostic message: %s", string(msg.Payload()))
		messageReceived <- string(msg.Payload())
	})

	if !subToken.WaitTimeout(5 * time.Second) {
		t.Errorf("Subscribe timeout after 5 seconds")
		client.Disconnect(250)
		return
	}

	if subToken.Error() != nil {
		t.Errorf("Subscribe failed: %v", subToken.Error())
		client.Disconnect(250)
		return
	}

	t.Log("✓ MQTT subscription successful")
	subscribed <- true

	// Test 6: Test publish functionality
	testMessage := "Diagnostic test message"
	pubToken := client.Publish("diagnostic/test", 1, false, testMessage)

	if !pubToken.WaitTimeout(5 * time.Second) {
		t.Errorf("Publish timeout after 5 seconds")
		client.Disconnect(250)
		return
	}

	if pubToken.Error() != nil {
		t.Errorf("Publish failed: %v", pubToken.Error())
		client.Disconnect(250)
		return
	}

	t.Log("✓ MQTT publish successful")

	// Test 7: Verify message delivery
	select {
	case msg := <-messageReceived:
		if msg == testMessage {
			t.Log("✓ Message delivery successful")
		} else {
			t.Errorf("Message content mismatch. Expected: %s, Got: %s", testMessage, msg)
		}
	case <-time.After(5 * time.Second):
		t.Errorf("Message delivery timeout - message not received within 5 seconds")
	case err := <-connectionLostChan:
		t.Errorf("Connection lost during message delivery test: %v", err)
	}

	// Test 8: Clean disconnect
	client.Disconnect(250)
	t.Log("✓ MQTT disconnect successful")

	// Test 9: Test rapid connection cycling to detect memory leaks or resource issues
	t.Log("Testing connection stability with rapid cycling...")
	for i := 0; i < 5; i++ {
		opts := mqtt.NewClientOptions()
		opts.AddBroker("tcp://localhost:1883")
		opts.SetClientID("diagnostic-cycle-" + string(rune('0'+i)))
		opts.SetConnectTimeout(5 * time.Second)

		cycleClient := mqtt.NewClient(opts)
		token := cycleClient.Connect()

		if !token.WaitTimeout(5 * time.Second) {
			t.Errorf("Cycle %d: Connection timeout", i+1)
			continue
		}

		if token.Error() != nil {
			t.Errorf("Cycle %d: Connection failed: %v", i+1, token.Error())
			continue
		}

		cycleClient.Disconnect(100)
		time.Sleep(100 * time.Millisecond)
	}
	t.Log("✓ Connection cycling test completed")

	t.Log("Service diagnostic test completed successfully - no errors detected")
}