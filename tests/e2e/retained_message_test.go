// Copyright 2023 The emqx-go Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package e2e

import (
	"context"
	"fmt"
	"log"
	"testing"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/turtacn/emqx-go/pkg/broker"
)

// TestRetainedMessageE2E tests retained message functionality end-to-end
func TestRetainedMessageE2E(t *testing.T) {
	t.Log("Starting Retained Message E2E test")

	// Start the broker
	b := broker.New("test-node", nil)
	b.SetupDefaultAuth()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start broker server
	go func() {
		if err := b.StartServer(ctx, ":1883"); err != nil {
			log.Printf("Test broker server failed: %v", err)
		}
	}()

	// Give broker time to start
	time.Sleep(100 * time.Millisecond)

	// Test 1: Publish a retained message
	t.Log("Test 1: Publishing retained message")

	publisher := createMQTTClient("retained-publisher")
	defer publisher.Disconnect(250)

	token := publisher.Connect()
	require.True(t, token.WaitTimeout(5*time.Second), "Publisher failed to connect")
	require.NoError(t, token.Error(), "Publisher connection error")

	// Publish a retained message
	retainedTopic := "test/retained/e2e"
	retainedPayload := "This is a retained message for E2E testing"

	pubToken := publisher.Publish(retainedTopic, 1, true, retainedPayload)
	require.True(t, pubToken.WaitTimeout(5*time.Second), "Failed to publish retained message")
	require.NoError(t, pubToken.Error(), "Publish error")

	t.Logf("Published retained message to topic: %s", retainedTopic)

	// Test 2: Subscribe and receive the retained message
	t.Log("Test 2: Subscribing to receive retained message")

	subscriber := createMQTTClient("retained-subscriber")
	defer subscriber.Disconnect(250)

	token = subscriber.Connect()
	require.True(t, token.WaitTimeout(5*time.Second), "Subscriber failed to connect")
	require.NoError(t, token.Error(), "Subscriber connection error")

	// Channel to receive messages
	messageReceived := make(chan mqtt.Message, 1)

	// Set up message handler
	subscriber.Subscribe(retainedTopic, 1, func(client mqtt.Client, msg mqtt.Message) {
		t.Logf("Received message: topic=%s, payload=%s, retained=%v",
			msg.Topic(), string(msg.Payload()), msg.Retained())
		messageReceived <- msg
	})

	// Wait for retained message
	select {
	case msg := <-messageReceived:
		assert.Equal(t, retainedTopic, msg.Topic())
		assert.Equal(t, retainedPayload, string(msg.Payload()))
		assert.True(t, msg.Retained(), "Message should be marked as retained")
		assert.Equal(t, byte(1), msg.Qos())
		t.Log("✓ Retained message received correctly")
	case <-time.After(5 * time.Second):
		t.Fatal("Timeout waiting for retained message")
	}

	// Test 3: Update the retained message
	t.Log("Test 3: Updating retained message")

	updatedPayload := "Updated retained message content"
	pubToken = publisher.Publish(retainedTopic, 1, true, updatedPayload)
	require.True(t, pubToken.WaitTimeout(5*time.Second), "Failed to publish updated retained message")
	require.NoError(t, pubToken.Error(), "Updated publish error")

	// Subscribe with a new client to get the updated message
	subscriber2 := createMQTTClient("retained-subscriber-2")
	defer subscriber2.Disconnect(250)

	token = subscriber2.Connect()
	require.True(t, token.WaitTimeout(5*time.Second), "Subscriber2 failed to connect")
	require.NoError(t, token.Error(), "Subscriber2 connection error")

	messageReceived2 := make(chan mqtt.Message, 1)
	subscriber2.Subscribe(retainedTopic, 1, func(client mqtt.Client, msg mqtt.Message) {
		messageReceived2 <- msg
	})

	// Wait for updated retained message
	select {
	case msg := <-messageReceived2:
		assert.Equal(t, retainedTopic, msg.Topic())
		assert.Equal(t, updatedPayload, string(msg.Payload()))
		assert.True(t, msg.Retained(), "Updated message should be marked as retained")
		t.Log("✓ Updated retained message received correctly")
	case <-time.After(5 * time.Second):
		t.Fatal("Timeout waiting for updated retained message")
	}

	// Test 4: Delete retained message (empty payload)
	t.Log("Test 4: Deleting retained message with empty payload")

	pubToken = publisher.Publish(retainedTopic, 1, true, "")
	require.True(t, pubToken.WaitTimeout(5*time.Second), "Failed to publish delete message")
	require.NoError(t, pubToken.Error(), "Delete publish error")

	// Subscribe with a new client - should not receive any retained message
	subscriber3 := createMQTTClient("retained-subscriber-3")
	defer subscriber3.Disconnect(250)

	token = subscriber3.Connect()
	require.True(t, token.WaitTimeout(5*time.Second), "Subscriber3 failed to connect")
	require.NoError(t, token.Error(), "Subscriber3 connection error")

	messageReceived3 := make(chan mqtt.Message, 1)
	subscriber3.Subscribe(retainedTopic, 1, func(client mqtt.Client, msg mqtt.Message) {
		if msg.Retained() {
			messageReceived3 <- msg
		}
	})

	// Should NOT receive any retained message
	select {
	case msg := <-messageReceived3:
		t.Fatalf("Should not have received retained message after deletion, got: %s", string(msg.Payload()))
	case <-time.After(2 * time.Second):
		t.Log("✓ No retained message received after deletion (correct behavior)")
	}

	// Test 5: Multiple topics with retained messages
	t.Log("Test 5: Testing multiple retained messages on different topics")

	topics := []string{
		"test/retained/topic1",
		"test/retained/topic2",
		"test/retained/topic3",
	}

	// Publish to multiple topics
	for i, topic := range topics {
		payload := fmt.Sprintf("Retained message %d", i+1)
		pubToken = publisher.Publish(topic, 1, true, payload)
		require.True(t, pubToken.WaitTimeout(5*time.Second), "Failed to publish to %s", topic)
		require.NoError(t, pubToken.Error(), "Publish error for %s", topic)
	}

	// Subscribe to all topics and verify retained messages
	subscriber4 := createMQTTClient("retained-subscriber-4")
	defer subscriber4.Disconnect(250)

	token = subscriber4.Connect()
	require.True(t, token.WaitTimeout(5*time.Second), "Subscriber4 failed to connect")
	require.NoError(t, token.Error(), "Subscriber4 connection error")

	receivedTopics := make(map[string]string)
	allReceived := make(chan struct{})

	subscriber4.Subscribe("test/retained/+", 1, func(client mqtt.Client, msg mqtt.Message) {
		if msg.Retained() {
			receivedTopics[msg.Topic()] = string(msg.Payload())
			if len(receivedTopics) == len(topics) {
				close(allReceived)
			}
		}
	})

	// Wait for all retained messages
	select {
	case <-allReceived:
		for i, topic := range topics {
			expectedPayload := fmt.Sprintf("Retained message %d", i+1)
			actualPayload, exists := receivedTopics[topic]
			assert.True(t, exists, "Should have received retained message for %s", topic)
			assert.Equal(t, expectedPayload, actualPayload, "Payload mismatch for %s", topic)
		}
		t.Log("✓ All retained messages received correctly for multiple topics")
	case <-time.After(5 * time.Second):
		t.Fatalf("Timeout waiting for retained messages. Received: %v", receivedTopics)
	}

	t.Log("Retained Message E2E test completed successfully")
}

// TestRetainedMessageQoSDowngrade tests QoS downgrade behavior with retained messages
func TestRetainedMessageQoSDowngrade(t *testing.T) {
	t.Log("Starting Retained Message QoS Downgrade test")

	// Start the broker (reuse existing broker)
	time.Sleep(100 * time.Millisecond) // Ensure broker is ready

	// Test QoS downgrade: Publish QoS 2, Subscribe QoS 1
	publisher := createMQTTClient("qos-publisher")
	defer publisher.Disconnect(250)

	token := publisher.Connect()
	require.True(t, token.WaitTimeout(5*time.Second))
	require.NoError(t, token.Error())

	// Publish retained message with QoS 2
	topic := "test/retained/qos/downgrade"
	payload := "QoS downgrade test message"

	pubToken := publisher.Publish(topic, 2, true, payload) // QoS 2
	require.True(t, pubToken.WaitTimeout(5*time.Second))
	require.NoError(t, pubToken.Error())

	// Subscribe with QoS 1
	subscriber := createMQTTClient("qos-subscriber")
	defer subscriber.Disconnect(250)

	token = subscriber.Connect()
	require.True(t, token.WaitTimeout(5*time.Second))
	require.NoError(t, token.Error())

	messageReceived := make(chan mqtt.Message, 1)
	subscriber.Subscribe(topic, 1, func(client mqtt.Client, msg mqtt.Message) { // QoS 1
		messageReceived <- msg
	})

	// Verify QoS downgrade
	select {
	case msg := <-messageReceived:
		assert.Equal(t, topic, msg.Topic())
		assert.Equal(t, payload, string(msg.Payload()))
		assert.True(t, msg.Retained())
		// The effective QoS should be min(publish QoS, subscription QoS) = min(2, 1) = 1
		assert.Equal(t, byte(1), msg.Qos(), "QoS should be downgraded to subscription QoS")
		t.Log("✓ QoS downgrade working correctly")
	case <-time.After(5 * time.Second):
		t.Fatal("Timeout waiting for QoS downgrade test message")
	}

	t.Log("Retained Message QoS Downgrade test completed successfully")
}

func createMQTTClient(clientID string) mqtt.Client {
	opts := mqtt.NewClientOptions()
	opts.AddBroker("tcp://localhost:1883")
	opts.SetClientID(clientID)
	opts.SetUsername("test")
	opts.SetPassword("test")
	opts.SetCleanSession(true)

	return mqtt.NewClient(opts)
}