package broker

import (
	"log"

	"github.com/asynkron/protoactor-go/actor"
)

// Subscribe is a message sent to the Broker to subscribe to a topic.
type Subscribe struct {
	Topic  string
	Sender *actor.PID
}

// Unsubscribe is a message sent to the Broker to unsubscribe from a topic.
type Unsubscribe struct {
	Topic  string
	Sender *actor.PID
}

// Publish is a message sent to the Broker to publish to a topic.
type Publish struct {
	Topic   string
	Payload []byte
}

// ForwardedPublish is a message that the Broker sends to a subscriber.
type ForwardedPublish struct {
	Topic   string
	Payload []byte
}

// Broker is the central actor responsible for message routing.
type Broker struct {
	subscriptions map[string][]*actor.PID
}

// New creates the Props for a new Broker actor.
func New() *actor.Props {
	return actor.PropsFromProducer(func() actor.Actor {
		return &Broker{
			subscriptions: make(map[string][]*actor.PID),
		}
	})
}

// Receive is the message handler for the Broker actor.
func (b *Broker) Receive(context actor.Context) {
	switch msg := context.Message().(type) {
	case *actor.Started:
		log.Println("Broker actor started")
	case *Subscribe:
		b.handleSubscribe(context, msg)
	case *Publish:
		b.handlePublish(context, msg)
	case *Unsubscribe:
		b.handleUnsubscribe(context, msg)
	}
}

func (b *Broker) handleSubscribe(context actor.Context, msg *Subscribe) {
	b.subscriptions[msg.Topic] = append(b.subscriptions[msg.Topic], msg.Sender)
}

func (b *Broker) handlePublish(context actor.Context, msg *Publish) {
	if subscribers, found := b.subscriptions[msg.Topic]; found {
		forwardedMsg := &ForwardedPublish{
			Topic:   msg.Topic,
			Payload: msg.Payload,
		}
		for _, subPID := range subscribers {
			context.Send(subPID, forwardedMsg)
		}
	}
}

func (b *Broker) handleUnsubscribe(context actor.Context, msg *Unsubscribe) {
	if subscribers, found := b.subscriptions[msg.Topic]; found {
		for i, pid := range subscribers {
			if pid.Equal(msg.Sender) {
				b.subscriptions[msg.Topic] = append(subscribers[:i], subscribers[i+1:]...)
				break
			}
		}
	}
}
