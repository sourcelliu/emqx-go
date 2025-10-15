package emqx

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestQoS0AtMostOnce tests QoS 0 (At most once) message delivery
func TestQoS0AtMostOnce(t *testing.T) {
	t.Log("Testing QoS 0 (At most once) message delivery")

	subscriber := setupMQTTClient("qos0_subscriber", t)
	defer subscriber.Disconnect(250)

	messagesReceived := atomic.Int32{}
	token := subscriber.Subscribe("test/qos0", 0, func(client mqtt.Client, msg mqtt.Message) {
		t.Logf("QoS 0 message received: %s (QoS: %d)", string(msg.Payload()), msg.Qos())
		assert.Equal(t, byte(0), msg.Qos(), "Message QoS should be 0")
		messagesReceived.Add(1)
	})
	require.True(t, token.WaitTimeout(5*time.Second))
	require.NoError(t, token.Error())

	time.Sleep(100 * time.Millisecond)

	publisher := setupMQTTClient("qos0_publisher", t)
	defer publisher.Disconnect(250)

	// Publish QoS 0 messages
	numMessages := 10
	for i := 0; i < numMessages; i++ {
		payload := fmt.Sprintf("qos0_message_%d", i)
		token := publisher.Publish("test/qos0", 0, false, payload)
		// QoS 0 returns immediately, no ACK
		require.True(t, token.WaitTimeout(1*time.Second))
	}

	// Wait for messages to be delivered
	time.Sleep(2 * time.Second)

	received := messagesReceived.Load()
	t.Logf("Received %d messages with QoS 0", received)

	// QoS 0 may lose messages, but typically should receive most
	assert.GreaterOrEqual(t, received, int32(1), "Should receive at least some QoS 0 messages")
	assert.LessOrEqual(t, received, int32(numMessages), "Should not receive more than sent")
}

// TestQoS1AtLeastOnce tests QoS 1 (At least once) message delivery
func TestQoS1AtLeastOnce(t *testing.T) {
	t.Log("Testing QoS 1 (At least once) message delivery")

	subscriber := setupMQTTClient("qos1_subscriber", t)
	defer subscriber.Disconnect(250)

	receivedMessages := make(map[string]int)
	var mu sync.Mutex

	token := subscriber.Subscribe("test/qos1", 1, func(client mqtt.Client, msg mqtt.Message) {
		mu.Lock()
		payload := string(msg.Payload())
		receivedMessages[payload]++
		t.Logf("QoS 1 message received: %s (count: %d, QoS: %d)",
			payload, receivedMessages[payload], msg.Qos())
		mu.Unlock()
	})
	require.True(t, token.WaitTimeout(5*time.Second))
	require.NoError(t, token.Error())

	time.Sleep(100 * time.Millisecond)

	publisher := setupMQTTClient("qos1_publisher", t)
	defer publisher.Disconnect(250)

	// Publish QoS 1 messages
	numMessages := 5
	for i := 0; i < numMessages; i++ {
		payload := fmt.Sprintf("qos1_message_%d", i)
		token := publisher.Publish("test/qos1", 1, false, payload)
		require.True(t, token.WaitTimeout(5*time.Second), "QoS 1 should wait for PUBACK")
		require.NoError(t, token.Error())
		t.Logf("Published QoS 1 message %d", i)
	}

	// Wait for all messages
	time.Sleep(2 * time.Second)

	mu.Lock()
	defer mu.Unlock()

	t.Logf("Received messages: %+v", receivedMessages)

	// QoS 1 guarantees at least once delivery
	assert.GreaterOrEqual(t, len(receivedMessages), numMessages,
		"Should receive all QoS 1 messages at least once")

	// Each message should be received at least once
	for i := 0; i < numMessages; i++ {
		payload := fmt.Sprintf("qos1_message_%d", i)
		count, exists := receivedMessages[payload]
		assert.True(t, exists, "Message %s should be received", payload)
		assert.GreaterOrEqual(t, count, 1, "QoS 1 message should be delivered at least once")
	}
}

// TestQoS2ExactlyOnce tests QoS 2 (Exactly once) message delivery
func TestQoS2ExactlyOnce(t *testing.T) {
	t.Log("Testing QoS 2 (Exactly once) message delivery")

	subscriber := setupMQTTClient("qos2_subscriber", t)
	defer subscriber.Disconnect(250)

	receivedMessages := make(map[string]int)
	var mu sync.Mutex

	token := subscriber.Subscribe("test/qos2", 2, func(client mqtt.Client, msg mqtt.Message) {
		mu.Lock()
		payload := string(msg.Payload())
		receivedMessages[payload]++
		t.Logf("QoS 2 message received: %s (count: %d, QoS: %d)",
			payload, receivedMessages[payload], msg.Qos())
		mu.Unlock()
	})
	require.True(t, token.WaitTimeout(5*time.Second))
	require.NoError(t, token.Error())

	time.Sleep(100 * time.Millisecond)

	publisher := setupMQTTClient("qos2_publisher", t)
	defer publisher.Disconnect(250)

	// Publish QoS 2 messages
	numMessages := 5
	for i := 0; i < numMessages; i++ {
		payload := fmt.Sprintf("qos2_message_%d", i)
		token := publisher.Publish("test/qos2", 2, false, payload)
		require.True(t, token.WaitTimeout(10*time.Second), "QoS 2 should complete handshake")
		require.NoError(t, token.Error())
		t.Logf("Published QoS 2 message %d", i)
	}

	// Wait for all messages
	time.Sleep(3 * time.Second)

	mu.Lock()
	defer mu.Unlock()

	t.Logf("Received messages: %+v", receivedMessages)

	// QoS 2 guarantees exactly once delivery
	assert.Equal(t, numMessages, len(receivedMessages),
		"Should receive all QoS 2 messages")

	// Each message should be received exactly once
	for i := 0; i < numMessages; i++ {
		payload := fmt.Sprintf("qos2_message_%d", i)
		count, exists := receivedMessages[payload]
		assert.True(t, exists, "Message %s should be received", payload)
		assert.Equal(t, 1, count, "QoS 2 message should be delivered exactly once")
	}
}

// TestQoSDowngrade tests QoS downgrade when subscriber QoS < publisher QoS
func TestQoSDowngrade(t *testing.T) {
	t.Log("Testing QoS downgrade mechanism")

	// Subscribe with QoS 0
	subscriber := setupMQTTClient("qos_downgrade_subscriber", t)
	defer subscriber.Disconnect(250)

	receivedQoS := make(chan byte, 10)
	token := subscriber.Subscribe("test/qos_downgrade", 0, func(client mqtt.Client, msg mqtt.Message) {
		t.Logf("Received message with QoS: %d", msg.Qos())
		receivedQoS <- msg.Qos()
	})
	require.True(t, token.WaitTimeout(5*time.Second))
	require.NoError(t, token.Error())

	time.Sleep(100 * time.Millisecond)

	publisher := setupMQTTClient("qos_downgrade_publisher", t)
	defer publisher.Disconnect(250)

	// Publish with QoS 2, but subscriber has QoS 0
	token = publisher.Publish("test/qos_downgrade", 2, false, "downgrade_test")
	require.True(t, token.WaitTimeout(5*time.Second))
	require.NoError(t, token.Error())

	// Received QoS should be min(publish QoS, subscribe QoS) = min(2, 0) = 0
	select {
	case qos := <-receivedQoS:
		assert.Equal(t, byte(0), qos, "QoS should be downgraded to subscriber's QoS")
		t.Log("QoS downgrade test passed")
	case <-time.After(3 * time.Second):
		t.Fatal("Timeout waiting for message")
	}
}

// TestMixedQoSSubscriptions tests different QoS subscriptions on same topic
func TestMixedQoSSubscriptions(t *testing.T) {
	t.Log("Testing mixed QoS subscriptions on same topic")

	topic := "test/mixed_qos"

	// Create subscribers with different QoS levels
	sub0 := setupMQTTClient("mixed_qos_sub0", t)
	defer sub0.Disconnect(250)

	sub1 := setupMQTTClient("mixed_qos_sub1", t)
	defer sub1.Disconnect(250)

	sub2 := setupMQTTClient("mixed_qos_sub2", t)
	defer sub2.Disconnect(250)

	received0 := make(chan mqtt.Message, 5)
	received1 := make(chan mqtt.Message, 5)
	received2 := make(chan mqtt.Message, 5)

	// Subscribe with QoS 0
	token := sub0.Subscribe(topic, 0, func(client mqtt.Client, msg mqtt.Message) {
		t.Logf("Sub0 (QoS 0) received: %s (QoS: %d)", string(msg.Payload()), msg.Qos())
		received0 <- msg
	})
	require.True(t, token.WaitTimeout(5*time.Second))

	// Subscribe with QoS 1
	token = sub1.Subscribe(topic, 1, func(client mqtt.Client, msg mqtt.Message) {
		t.Logf("Sub1 (QoS 1) received: %s (QoS: %d)", string(msg.Payload()), msg.Qos())
		received1 <- msg
	})
	require.True(t, token.WaitTimeout(5*time.Second))

	// Subscribe with QoS 2
	token = sub2.Subscribe(topic, 2, func(client mqtt.Client, msg mqtt.Message) {
		t.Logf("Sub2 (QoS 2) received: %s (QoS: %d)", string(msg.Payload()), msg.Qos())
		received2 <- msg
	})
	require.True(t, token.WaitTimeout(5*time.Second))

	time.Sleep(200 * time.Millisecond)

	// Publish with QoS 2
	publisher := setupMQTTClient("mixed_qos_publisher", t)
	defer publisher.Disconnect(250)

	token = publisher.Publish(topic, 2, false, "mixed_qos_message")
	require.True(t, token.WaitTimeout(5*time.Second))
	require.NoError(t, token.Error())

	// All subscribers should receive the message
	timeout := time.After(5 * time.Second)

	// Check subscriber with QoS 0
	select {
	case msg := <-received0:
		assert.Equal(t, "mixed_qos_message", string(msg.Payload()))
		// QoS should be downgraded to 0
		assert.Equal(t, byte(0), msg.Qos())
	case <-timeout:
		t.Error("Subscriber 0 did not receive message")
	}

	// Check subscriber with QoS 1
	select {
	case msg := <-received1:
		assert.Equal(t, "mixed_qos_message", string(msg.Payload()))
		// QoS should be downgraded to 1
		assert.LessOrEqual(t, msg.Qos(), byte(1))
	case <-timeout:
		t.Error("Subscriber 1 did not receive message")
	}

	// Check subscriber with QoS 2
	select {
	case msg := <-received2:
		assert.Equal(t, "mixed_qos_message", string(msg.Payload()))
		// QoS can be up to 2
		assert.LessOrEqual(t, msg.Qos(), byte(2))
	case <-timeout:
		t.Error("Subscriber 2 did not receive message")
	}

	t.Log("All subscribers with different QoS levels received the message")
}

// TestQoSWithRetainedMessage tests QoS behavior with retained messages
func TestQoSWithRetainedMessage(t *testing.T) {
	t.Log("Testing QoS with retained messages")

	topic := "test/qos_retained"

	// Publish retained message with QoS 1
	publisher := setupMQTTClient("qos_retained_publisher", t)
	defer publisher.Disconnect(250)

	token := publisher.Publish(topic, 1, true, "retained_qos1_message")
	require.True(t, token.WaitTimeout(5*time.Second))
	require.NoError(t, token.Error())

	time.Sleep(200 * time.Millisecond)

	// Subscribe with QoS 2 (should receive retained message)
	subscriber := setupMQTTClient("qos_retained_subscriber", t)
	defer subscriber.Disconnect(250)

	messageReceived := make(chan mqtt.Message, 1)
	token = subscriber.Subscribe(topic, 2, func(client mqtt.Client, msg mqtt.Message) {
		t.Logf("Received retained message: %s (QoS: %d, Retained: %v)",
			string(msg.Payload()), msg.Qos(), msg.Retained())
		messageReceived <- msg
	})
	require.True(t, token.WaitTimeout(5*time.Second))
	require.NoError(t, token.Error())

	// Should receive retained message
	select {
	case msg := <-messageReceived:
		assert.Equal(t, "retained_qos1_message", string(msg.Payload()))
		assert.True(t, msg.Retained(), "Message should be marked as retained")
		// QoS should be min(publish QoS, subscribe QoS) = min(1, 2) = 1
		assert.LessOrEqual(t, msg.Qos(), byte(1))
		t.Log("Retained message with QoS received correctly")
	case <-time.After(3 * time.Second):
		t.Fatal("Timeout waiting for retained message")
	}

	// Clean up: delete retained message
	token = publisher.Publish(topic, 0, true, "")
	require.True(t, token.WaitTimeout(5*time.Second))
}

// TestQoSOrderPreservation tests message order preservation for QoS > 0
func TestQoSOrderPreservation(t *testing.T) {
	t.Log("Testing message order preservation with QoS 1")

	subscriber := setupMQTTClient("order_subscriber", t)
	defer subscriber.Disconnect(250)

	receivedOrder := make([]string, 0, 10)
	var mu sync.Mutex

	token := subscriber.Subscribe("test/order", 1, func(client mqtt.Client, msg mqtt.Message) {
		mu.Lock()
		receivedOrder = append(receivedOrder, string(msg.Payload()))
		mu.Unlock()
		t.Logf("Received in order: %s", string(msg.Payload()))
	})
	require.True(t, token.WaitTimeout(5*time.Second))
	require.NoError(t, token.Error())

	time.Sleep(100 * time.Millisecond)

	publisher := setupMQTTClient("order_publisher", t)
	defer publisher.Disconnect(250)

	// Publish messages in order
	expectedOrder := make([]string, 10)
	for i := 0; i < 10; i++ {
		payload := fmt.Sprintf("message_%02d", i)
		expectedOrder[i] = payload

		token := publisher.Publish("test/order", 1, false, payload)
		require.True(t, token.WaitTimeout(5*time.Second))
		require.NoError(t, token.Error())
	}

	// Wait for all messages
	time.Sleep(3 * time.Second)

	mu.Lock()
	defer mu.Unlock()

	t.Logf("Expected order: %v", expectedOrder)
	t.Logf("Received order: %v", receivedOrder)

	// MQTT should preserve message order for the same client
	assert.Equal(t, expectedOrder, receivedOrder,
		"Messages should be received in the same order they were published")
}
