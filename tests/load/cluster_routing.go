package main

import (
	"fmt"
	"log"
	"os"
	"testing"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// This is not a standard Go test, but is designed to be run with `go run`.
// It requires two broker nodes to be running and accessible.
//
// Usage:
// go run ./tests/load/cluster_routing.go <broker_a_address> <broker_b_address>
// e.g.:
// go run ./tests/load/cluster_routing.go tcp://localhost:1883 tcp://localhost:1884

func main() {
	if len(os.Args) != 3 {
		log.Fatalf("Usage: %s <broker_a_address> <broker_b_address>", os.Args[0])
	}
	brokerA := os.Args[1]
	brokerB := os.Args[2]

	log.Printf("Testing cluster routing between Node A (%s) and Node B (%s)", brokerA, brokerB)

	// Use a dummy testing object to leverage testify assertions
	t := &testing.T{}

	// --- Subscriber on Node A ---
	msgCh := make(chan mqtt.Message)
	subOpts := mqtt.NewClientOptions().AddBroker(brokerA).SetClientID("cluster-subscriber")
	subOpts.SetDefaultPublishHandler(func(client mqtt.Client, msg mqtt.Message) {
		msgCh <- msg
	})
	subClient := mqtt.NewClient(subOpts)
	subToken := subClient.Connect()
	subToken.Wait()
	require.NoError(t, subToken.Error(), "Subscriber should connect to Node A")
	defer subClient.Disconnect(250)

	subSubToken := subClient.Subscribe("cluster/test/topic", 0, nil)
	subSubToken.Wait()
	require.NoError(t, subSubToken.Error(), "Subscriber should subscribe on Node A")
	log.Println("Subscriber connected to Node A and subscribed.")

	// --- Publisher on Node B ---
	pubOpts := mqtt.NewClientOptions().AddBroker(brokerB).SetClientID("cluster-publisher")
	pubClient := mqtt.NewClient(pubOpts)
	pubToken := pubClient.Connect()
	pubToken.Wait()
	require.NoError(t, pubToken.Error(), "Publisher should connect to Node B")
	defer pubClient.Disconnect(250)
	log.Println("Publisher connected to Node B.")

	// --- Publish Message ---
	startTime := time.Now()
	pubMsg := fmt.Sprintf("cluster message at %v", startTime)
	pubPubToken := pubClient.Publish("cluster/test/topic", 0, false, pubMsg)
	pubPubToken.Wait()
	require.NoError(t, pubPubToken.Error(), "Publisher should publish on Node B")
	log.Println("Publisher sent message on Node B.")

	// --- Verification ---
	select {
	case receivedMsg := <-msgCh:
		latency := time.Since(startTime)
		log.Printf("SUCCESS: Subscriber on Node A received message from Node B.")
		log.Printf("Message: %s", string(receivedMsg.Payload()))
		log.Printf("End-to-end Latency: %v", latency)
		assert.Equal(t, "cluster/test/topic", receivedMsg.Topic())
		assert.Equal(t, pubMsg, string(receivedMsg.Payload()))
	case <-time.After(5 * time.Second):
		log.Fatal("FAILURE: Timed out waiting for message to be routed between nodes.")
	}
}