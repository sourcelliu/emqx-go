package actor

import "context"

// Actor defines the interface for an actor process.
// An actor is an entity that, in response to a message it receives,
// can concurrently:
// - send a finite number of messages to other actors;
// - create a finite number of new actors;
// - designate the behavior to be used for the next message it receives.
type Actor interface {
	// Start is called when the actor is started.
	// It receives a context and a mailbox to receive messages.
	Start(ctx context.Context, mb *Mailbox) error
}

// Mailbox is a channel-based message queue for an actor.
// It uses a buffered channel to store incoming messages.
type Mailbox struct {
	messages chan interface{}
}

// NewMailbox creates a new mailbox with the given buffer size.
func NewMailbox(size int) *Mailbox {
	return &Mailbox{
		messages: make(chan interface{}, size),
	}
}

// Send puts a message into the mailbox.
func (mb *Mailbox) Send(msg interface{}) {
	mb.messages <- msg
}

// Receive blocks until a message is received from the mailbox.
func (mb *Mailbox) Receive(ctx context.Context) (interface{}, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case msg := <-mb.messages:
		return msg, nil
	}
}

// Chan returns the underlying message channel.
func (mb *Mailbox) Chan() <-chan interface{} {
	return mb.messages
}
