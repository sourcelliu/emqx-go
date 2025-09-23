package broker

import (
	"testing"
	"time"

	"github.com/asynkron/protoactor-go/actor"
	"github.com/stretchr/testify/assert"
)

// --- Test Subscriber Actor ---

// TestSubscriber is an actor that acts as a client in tests.
// It can receive and store forwarded messages.
type TestSubscriber struct {
	ReceivedMessages []*ForwardedPublish
}

func (s *TestSubscriber) Receive(context actor.Context) {
	switch msg := context.Message().(type) {
	case *ForwardedPublish:
		s.ReceivedMessages = append(s.ReceivedMessages, msg)
	}
}

func NewTestSubscriber() actor.Actor {
	return &TestSubscriber{
		ReceivedMessages: make([]*ForwardedPublish, 0),
	}
}

// --- Broker Tests ---

func TestBroker_SubscribeAndPublish(t *testing.T) {
	system := actor.NewActorSystem()

	// Spawn the broker actor
	brokerProps := New()
	brokerPID := system.Root.Spawn(brokerProps)

	// Spawn a subscriber actor
	subscriberActor := &TestSubscriber{ReceivedMessages: make([]*ForwardedPublish, 0)}
	subscriberProps := actor.PropsFromProducer(func() actor.Actor { return subscriberActor })
	subscriberPID := system.Root.Spawn(subscriberProps)

	// --- Subscribe ---
	subscribeMsg := &Subscribe{
		Topic:  "test/topic",
		Sender: subscriberPID,
	}
	system.Root.Send(brokerPID, subscribeMsg)

	// Allow some time for the message to be processed
	time.Sleep(50 * time.Millisecond)

	// --- Publish ---
	publishMsg := &Publish{
		Topic:   "test/topic",
		Payload: []byte("hello world"),
	}
	system.Root.Send(brokerPID, publishMsg)

	// Allow some time for the message to be forwarded
	time.Sleep(50 * time.Millisecond)

	// --- Assert ---
	assert.Len(t, subscriberActor.ReceivedMessages, 1, "Subscriber should have received one message")
	if len(subscriberActor.ReceivedMessages) > 0 {
		received := subscriberActor.ReceivedMessages[0]
		assert.Equal(t, "test/topic", received.Topic)
		assert.Equal(t, []byte("hello world"), received.Payload)
	}
}

func TestBroker_Unsubscribe(t *testing.T) {
	system := actor.NewActorSystem()
	brokerPID := system.Root.Spawn(New())
	subscriberActor := &TestSubscriber{ReceivedMessages: make([]*ForwardedPublish, 0)}
	subscriberPID := system.Root.Spawn(actor.PropsFromProducer(func() actor.Actor { return subscriberActor }))

	// Subscribe
	system.Root.Send(brokerPID, &Subscribe{Topic: "test/topic", Sender: subscriberPID})
	time.Sleep(50 * time.Millisecond)

	// Unsubscribe
	system.Root.Send(brokerPID, &Unsubscribe{Topic: "test/topic", Sender: subscriberPID})
	time.Sleep(50 * time.Millisecond)

	// Publish
	system.Root.Send(brokerPID, &Publish{Topic: "test/topic", Payload: []byte("you should not see this")})
	time.Sleep(50 * time.Millisecond)

	// Assert
	assert.Empty(t, subscriberActor.ReceivedMessages, "Subscriber should not receive messages after unsubscribing")
}
