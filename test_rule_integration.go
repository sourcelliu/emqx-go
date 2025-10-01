package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/turtacn/emqx-go/pkg/rules"
)

func main() {
	// Test creating a rule via API
	rule := rules.Rule{
		ID:          "test-console-rule",
		Name:        "Test Console Rule",
		Description: "Test rule for console action",
		SQL:         "topic = 'sensor/temperature'",
		Actions: []rules.Action{
			{
				Type: rules.ActionTypeConsole,
				Parameters: map[string]interface{}{
					"level":    "info",
					"template": "Temperature sensor data: ${payload} from client ${clientid}",
				},
			},
		},
		Status:   rules.RuleStatusEnabled,
		Priority: 1,
	}

	// Create rule via internal API (bypassing HTTP auth for testing)
	fmt.Println("Testing rule engine integration...")

	// Wait for the service to be ready
	fmt.Println("Waiting for service to be ready...")
	time.Sleep(2 * time.Second)

	// Test if the API is accessible
	fmt.Println("Testing API accessibility...")
	resp, err := http.Get("http://localhost:18083/api/rules/action-types")
	if err != nil {
		log.Printf("Error accessing API: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		log.Printf("API returned status: %d", resp.StatusCode)
		// Let's try to create the rule anyway for testing
	} else {
		fmt.Println("API is accessible!")
	}

	// Now test MQTT publishing
	fmt.Println("Testing MQTT message publishing...")

	// MQTT client setup
	opts := mqtt.NewClientOptions()
	opts.AddBroker("tcp://localhost:1883")
	opts.SetClientID("test-rule-publisher")
	opts.SetUsername("admin")
	opts.SetPassword("admin123")
	opts.SetKeepAlive(60 * time.Second)
	opts.SetConnectTimeout(5 * time.Second)

	client := mqtt.NewClient(opts)
	token := client.Connect()
	if !token.Wait() || token.Error() != nil {
		log.Fatalf("Failed to connect: %v", token.Error())
	}

	fmt.Println("Connected to MQTT broker")

	// Publish test message multiple times to see rule processing
	for i := 0; i < 3; i++ {
		topic := "sensor/temperature"
		payload := fmt.Sprintf(`{"temperature": %.1f, "timestamp": "%s", "test_id": %d}`,
			25.5+float64(i), time.Now().Format(time.RFC3339), i)

		fmt.Printf("Publishing message %d to topic '%s': %s\n", i+1, topic, payload)
		pubToken := client.Publish(topic, 1, false, payload)
		if !pubToken.Wait() || pubToken.Error() != nil {
			log.Printf("Failed to publish: %v", pubToken.Error())
		} else {
			fmt.Printf("Message %d published successfully!\n", i+1)
		}

		// Wait between messages
		time.Sleep(500 * time.Millisecond)
	}

	// Wait a moment then disconnect
	time.Sleep(1 * time.Second)
	client.Disconnect(250)
	fmt.Println("Disconnected from MQTT broker")

	fmt.Println("Test completed! Check the server logs for rule processing output.")
}