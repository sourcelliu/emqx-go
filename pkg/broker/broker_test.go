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

package broker

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/turtacn/emqx-go/pkg/storage"
)

// startTestBroker starts a broker on a random available port and returns the broker instance and its address.
func startTestBroker(ctx context.Context, t *testing.T) (*Broker, string) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := listener.Addr().String()

	b := New("test-node", nil)

	go func() {
		// This loop mimics the core of StartServer but allows us to control the lifecycle.
		for {
			conn, err := listener.Accept()
			if err != nil {
				select {
				case <-ctx.Done(): // If context is canceled, this is an expected exit.
					return
				default: // Otherwise, it's an unexpected error during the test.
					if !t.Failed() { // Avoid logging errors on test shutdown
						t.Logf("failed to accept connection: %v", err)
					}
				}
				return
			}
			go b.handleConnection(ctx, conn)
		}
	}()

	// Cleanup the listener when the test finishes.
	t.Cleanup(func() {
		_ = listener.Close()
	})

	return b, fmt.Sprintf("tcp://%s", addr)
}

func TestBroker_Integration_ConnectDisconnect(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	b, addr := startTestBroker(ctx, t)

	opts := mqtt.NewClientOptions().AddBroker(addr).SetClientID("test-client-connect")
	client := mqtt.NewClient(opts)

	token := client.Connect()
	require.True(t, token.WaitTimeout(2*time.Second), "timed out connecting")
	require.NoError(t, token.Error())
	assert.True(t, client.IsConnected())

	// Check that the session was created in the broker
	_, err := b.sessions.Get("test-client-connect")
	assert.NoError(t, err)

	client.Disconnect(100)
	time.Sleep(200 * time.Millisecond) // Give the broker a moment to process the disconnect

	// Check that the session was removed
	_, err = b.sessions.Get("test-client-connect")
	assert.ErrorIs(t, err, storage.ErrNotFound)
}

func TestBroker_Integration_SubscribePublish(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	_, addr := startTestBroker(ctx, t)

	msgCh := make(chan mqtt.Message, 1)
	opts := mqtt.NewClientOptions().AddBroker(addr).SetClientID("test-client-subpub")
	opts.SetDefaultPublishHandler(func(client mqtt.Client, msg mqtt.Message) {
		msgCh <- msg
	})

	client := mqtt.NewClient(opts)
	token := client.Connect()
	require.True(t, token.WaitTimeout(2*time.Second), "timed out connecting")
	require.NoError(t, token.Error())
	defer client.Disconnect(100)

	// Subscribe
	subToken := client.Subscribe("test/topic", 0, nil)
	require.True(t, subToken.WaitTimeout(1*time.Second), "timed out subscribing")
	require.NoError(t, subToken.Error())

	// Publish
	pubToken := client.Publish("test/topic", 0, false, "hello world")
	require.True(t, pubToken.WaitTimeout(1*time.Second), "timed out publishing")
	require.NoError(t, pubToken.Error())

	// Verify message received
	select {
	case receivedMsg := <-msgCh:
		assert.Equal(t, "test/topic", receivedMsg.Topic())
		assert.Equal(t, "hello world", string(receivedMsg.Payload()))
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for message")
	}
}