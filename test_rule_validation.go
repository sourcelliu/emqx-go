package main

import (
	"fmt"
	"log"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

func main() {
	// MQTT client setup
	opts := mqtt.NewClientOptions()
	opts.AddBroker("tcp://localhost:1883")
	opts.SetClientID("test-rule-validation")
	opts.SetUsername("admin")
	opts.SetPassword("admin123")
	opts.SetKeepAlive(60 * time.Second)
	opts.SetConnectTimeout(5 * time.Second)

	client := mqtt.NewClient(opts)
	token := client.Connect()
	if !token.Wait() || token.Error() != nil {
		log.Fatalf("Failed to connect: %v", token.Error())
	}

	fmt.Println("Connected to MQTT broker for rule validation tests")

	// Test cases for rule validation
	testCases := []struct {
		topic   string
		payload string
		desc    string
	}{
		{"sensor/temperature", `{"temperature": 25.0}`, "Exact match for temperature rule"},
		{"sensor/humidity", `{"humidity": 60}`, "Should match general sensor rule only"},
		{"device/status", `{"status": "online"}`, "Should not match any sensor rules"},
		{"sensor/pressure", `{"pressure": 1013.25}`, "Should match general sensor rule only"},
	}

	for i, tc := range testCases {
		fmt.Printf("\nTest %d: %s\n", i+1, tc.desc)
		fmt.Printf("Publishing to topic '%s': %s\n", tc.topic, tc.payload)

		pubToken := client.Publish(tc.topic, 1, false, tc.payload)
		if !pubToken.Wait() || pubToken.Error() != nil {
			log.Printf("Failed to publish: %v", pubToken.Error())
		} else {
			fmt.Println("✓ Published successfully")
		}

		// Wait between messages to see rule processing clearly
		time.Sleep(1 * time.Second)
	}

	// Wait a moment then disconnect
	time.Sleep(1 * time.Second)
	client.Disconnect(250)
	fmt.Println("\nRule validation test completed! Check server logs for rule processing.")
}