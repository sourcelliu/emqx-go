package main

import (
	"log"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

func init() {
	// Disable MQTT client debug logging to focus on broker logs
	// mqtt.DEBUG = log.New(log.Writer(), "[MQTT-DEBUG] ", log.LstdFlags)
	// mqtt.WARN = log.New(log.Writer(), "[MQTT-WARN] ", log.LstdFlags)
	// mqtt.CRITICAL = log.New(log.Writer(), "[MQTT-CRITICAL] ", log.LstdFlags)
	// mqtt.ERROR = log.New(log.Writer(), "[MQTT-ERROR] ", log.LstdFlags)
}

func main() {
	opts := mqtt.NewClientOptions()
	opts.AddBroker("tcp://localhost:1883")
	opts.SetClientID("test-retained-publisher")
	opts.SetUsername("test")
	opts.SetPassword("test")
	opts.SetCleanSession(true)

	client := mqtt.NewClient(opts)
	token := client.Connect()
	if !token.WaitTimeout(5*time.Second) {
		log.Fatal("Connection timeout")
	}
	if token.Error() != nil {
		log.Fatalf("Connection error: %v", token.Error())
	}

	log.Println("Connected successfully")

	// Publish a retained message
	topic := "test/simple/retained"
	payload := "Hello retained world"

	log.Printf("Publishing retained message to topic: %s", topic)
	pubToken := client.Publish(topic, 1, true, payload)
	if !pubToken.WaitTimeout(5*time.Second) {
		log.Fatal("Publish timeout")
	}
	if pubToken.Error() != nil {
		log.Fatalf("Publish error: %v", pubToken.Error())
	}

	log.Println("Published successfully")

	// Wait a moment to ensure the packet is processed by broker
	time.Sleep(2 * time.Second)

	client.Disconnect(250)
}