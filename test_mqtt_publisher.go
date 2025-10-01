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
	opts.SetClientID("test-publisher")
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

	// Publish test message
	topic := "sensor/temperature"
	payload := `{"temperature": 26.5, "timestamp": "2025-10-01T12:51:00Z"}`

	fmt.Printf("Publishing message to topic '%s': %s\n", topic, payload)
	pubToken := client.Publish(topic, 1, false, payload)
	if !pubToken.Wait() || pubToken.Error() != nil {
		log.Fatalf("Failed to publish: %v", pubToken.Error())
	}

	fmt.Println("Message published successfully!")

	// Wait a moment then disconnect
	time.Sleep(100 * time.Millisecond)
	client.Disconnect(250)
	fmt.Println("Disconnected from MQTT broker")
}