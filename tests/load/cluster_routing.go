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

// package load contains tests for verifying cluster functionality under load.
package load

import (
	"fmt"
	"testing"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/stretchr/testify/assert"
)

const (
	brokerAddr1 = "tcp://localhost:1883"
	brokerAddr2 = "tcp://localhost:1884" // Mapped in docker-compose.yml
	testTopic   = "cluster/test"
	testPayload = "message across cluster"
)

// TestClusterRouting verifies that a message published to one node is
// correctly routed to a subscriber on another node.
//
// This test assumes a two-node cluster is running, with MQTT ports exposed
// on localhost:1883 and localhost:1884, as defined in docker-compose.yml.
func TestClusterRouting(t *testing.T) {
	// --- Subscriber Client (connects to Node 1) ---
	msgCh := make(chan mqtt.Message)
	subOpts := mqtt.NewClientOptions().AddBroker(brokerAddr1).SetClientID("subscriber-client")
	subOpts.SetDefaultPublishHandler(func(client mqtt.Client, msg mqtt.Message) {
		msgCh <- msg
	})

	subClient := mqtt.NewClient(subOpts)
	if token := subClient.Connect(); token.Wait() && token.Error() != nil {
		t.Fatalf("Subscriber client failed to connect to broker 1: %v", token.Error())
	}
	defer subClient.Disconnect(250)

	if token := subClient.Subscribe(testTopic, 0, nil); token.Wait() && token.Error() != nil {
		t.Fatalf("Subscriber client failed to subscribe: %v", token.Error())
	}
	fmt.Printf("Subscriber connected to %s and subscribed to %s\n", brokerAddr1, testTopic)

	// --- Publisher Client (connects to Node 2) ---
	pubOpts := mqtt.NewClientOptions().AddBroker(brokerAddr2).SetClientID("publisher-client")
	pubClient := mqtt.NewClient(pubOpts)
	if token := pubClient.Connect(); token.Wait() && token.Error() != nil {
		t.Fatalf("Publisher client failed to connect to broker 2: %v", token.Error())
	}
	defer pubClient.Disconnect(250)
	fmt.Printf("Publisher connected to %s\n", brokerAddr2)

	// Give some time for the subscription to propagate through the cluster.
	time.Sleep(2 * time.Second)

	// --- Publish Message ---
	fmt.Printf("Publisher publishing message to %s\n", testTopic)
	if token := pubClient.Publish(testTopic, 0, false, testPayload); token.Wait() && token.Error() != nil {
		t.Fatalf("Publisher client failed to publish: %v", token.Error())
	}

	// --- Verify Message Received by Subscriber ---
	select {
	case receivedMsg := <-msgCh:
		fmt.Println("Subscriber received message!")
		assert.Equal(t, testTopic, receivedMsg.Topic())
		assert.Equal(t, testPayload, string(receivedMsg.Payload()))
	case <-time.After(5 * time.Second):
		t.Fatal("Timed out waiting for message to be routed across the cluster")
	}
}