package main

import (
	"log"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

func main() {
	// First, publish a retained message
	log.Println("Publishing retained message...")
	publishRetainedMessage()

	// Wait a moment
	time.Sleep(1 * time.Second)

	// Then subscribe to see if we get it
	log.Println("Subscribing to topic...")
	subscribeAndWait()
}

func publishRetainedMessage() {
	opts := mqtt.NewClientOptions()
	opts.AddBroker("tcp://localhost:1883")
	opts.SetClientID("publisher")
	opts.SetUsername("test")
	opts.SetPassword("test")
	opts.SetCleanSession(true)

	client := mqtt.NewClient(opts)
	token := client.Connect()
	if !token.WaitTimeout(5*time.Second) {
		log.Fatal("Publisher connection timeout")
	}
	if token.Error() != nil {
		log.Fatalf("Publisher connection error: %v", token.Error())
	}

	topic := "test/retained/simple"
	payload := "Hello retained world!"

	pubToken := client.Publish(topic, 1, true, payload)
	if !pubToken.WaitTimeout(5*time.Second) {
		log.Fatal("Publish timeout")
	}
	if pubToken.Error() != nil {
		log.Fatalf("Publish error: %v", pubToken.Error())
	}

	log.Printf("Published retained message: %s", payload)
	client.Disconnect(250)
}

func subscribeAndWait() {
	opts := mqtt.NewClientOptions()
	opts.AddBroker("tcp://localhost:1883")
	opts.SetClientID("subscriber")
	opts.SetUsername("test")
	opts.SetPassword("test")
	opts.SetCleanSession(true)

	client := mqtt.NewClient(opts)
	token := client.Connect()
	if !token.WaitTimeout(5*time.Second) {
		log.Fatal("Subscriber connection timeout")
	}
	if token.Error() != nil {
		log.Fatalf("Subscriber connection error: %v", token.Error())
	}

	messageReceived := make(chan mqtt.Message, 1)

	client.Subscribe("test/retained/simple", 1, func(client mqtt.Client, msg mqtt.Message) {
		log.Printf("Received message: topic=%s, payload=%s, retained=%v",
			msg.Topic(), string(msg.Payload()), msg.Retained())
		messageReceived <- msg
	})

	select {
	case msg := <-messageReceived:
		if msg.Retained() {
			log.Println("✓ SUCCESS: Received retained message correctly!")
		} else {
			log.Println("✗ FAILURE: Message not marked as retained")
		}
	case <-time.After(5 * time.Second):
		log.Println("✗ FAILURE: Timeout waiting for retained message")
	}

	client.Disconnect(250)
}