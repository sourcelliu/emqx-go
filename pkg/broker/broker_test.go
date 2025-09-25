package broker

import (
	"context"
	"fmt"
	"log"
	"net"
	"strings"
	"testing"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Helper to get a free port for the listener
func getFreePort() (int, error) {
	addr, err := net.ResolveTCPAddr("tcp", "localhost:0")
	if err != nil {
		return 0, err
	}
	l, err := net.ListenTCP("tcp", addr)
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

func TestBroker_MqttContract_PubSub(t *testing.T) {
	// --- Setup ---
	port, err := getFreePort()
	require.NoError(t, err)
	addr := fmt.Sprintf("tcp://localhost:%d", port)

	broker := New()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		err := broker.StartServer(ctx, fmt.Sprintf(":%d", port))
		if err != nil && !strings.Contains(err.Error(), "use of closed network connection") {
			log.Printf("Broker server exited with error: %v", err)
		}
	}()
	time.Sleep(100 * time.Millisecond)

	// --- Subscriber Client ---
	msgCh := make(chan mqtt.Message)
	subOpts := mqtt.NewClientOptions().AddBroker(addr).SetClientID("subscriber")
	subOpts.SetDefaultPublishHandler(func(client mqtt.Client, msg mqtt.Message) {
		msgCh <- msg
	})
	subClient := mqtt.NewClient(subOpts)
	subToken := subClient.Connect()
	subToken.Wait()
	require.NoError(t, subToken.Error(), "Subscriber should connect")

	subSubToken := subClient.Subscribe("test/topic", 0, nil)
	subSubToken.Wait()
	require.NoError(t, subSubToken.Error(), "Subscriber should subscribe")
	log.Println("Subscriber connected and subscribed.")

	// --- Publisher Client ---
	pubOpts := mqtt.NewClientOptions().AddBroker(addr).SetClientID("publisher")
	pubClient := mqtt.NewClient(pubOpts)
	pubToken := pubClient.Connect()
	pubToken.Wait()
	require.NoError(t, pubToken.Error(), "Publisher should connect")
	log.Println("Publisher connected.")

	// --- Publish Message ---
	pubMsg := "hello from publisher"
	pubPubToken := pubClient.Publish("test/topic", 0, false, pubMsg)
	pubPubToken.Wait()
	require.NoError(t, pubPubToken.Error(), "Publisher should publish")
	log.Println("Publisher sent message.")

	// --- Verification ---
	select {
	case receivedMsg := <-msgCh:
		assert.Equal(t, "test/topic", receivedMsg.Topic())
		assert.Equal(t, pubMsg, string(receivedMsg.Payload()))
		log.Println("Subscriber received message successfully.")
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for message from publisher")
	}

	// --- Cleanup ---
	subClient.Disconnect(250)
	pubClient.Disconnect(250)
}
