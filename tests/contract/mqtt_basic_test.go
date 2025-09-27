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

// package contract contains contract tests for the MQTT broker.
package contract

import (
	"context"
	"log"
	"testing"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/stretchr/testify/assert"
	"github.com/turtacn/emqx-go/pkg/broker"
)

const (
	testBrokerAddr = "tcp://localhost:1883"
	testTopic      = "test/topic"
	testPayload    = "hello world"
)

func TestBrokerE2E(t *testing.T) {
	// Start the broker in a separate goroutine.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// For a single-node test, the cluster manager can be nil.
	b := broker.New("test-node", nil)
	go func() {
		if err := b.StartServer(ctx, ":1883"); err != nil {
			log.Printf("Test broker server failed: %v", err)
		}
	}()
	// Give the broker a moment to start up.
	time.Sleep(100 * time.Millisecond)

	// --- MQTT Client Setup ---
	msgCh := make(chan mqtt.Message)
	opts := mqtt.NewClientOptions().AddBroker(testBrokerAddr).SetClientID("test-client")
	opts.SetDefaultPublishHandler(func(client mqtt.Client, msg mqtt.Message) {
		msgCh <- msg
	})

	client := mqtt.NewClient(opts)
	if token := client.Connect(); token.Wait() && token.Error() != nil {
		t.Fatalf("Failed to connect to broker: %v", token.Error())
	}
	defer client.Disconnect(250)

	// --- Subscribe ---
	if token := client.Subscribe(testTopic, 0, nil); token.Wait() && token.Error() != nil {
		t.Fatalf("Failed to subscribe: %v", token.Error())
	}

	// --- Publish ---
	if token := client.Publish(testTopic, 0, false, testPayload); token.Wait() && token.Error() != nil {
		t.Fatalf("Failed to publish: %v", token.Error())
	}

	// --- Verify Message Received ---
	select {
	case receivedMsg := <-msgCh:
		assert.Equal(t, testTopic, receivedMsg.Topic())
		assert.Equal(t, testPayload, string(receivedMsg.Payload()))
	case <-time.After(2 * time.Second):
		t.Fatal("Timed out waiting for message")
	}
}