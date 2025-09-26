// Copyright 2022 The emqx-go Authors
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

package actor

import "context"

// Actor defines the interface for an actor process.
// An actor is an entity that, in response to a message it receives,
// can concurrently:
//   - send a finite number of messages to other actors;
//   - create a finite number of new actors;
//   - designate the behavior to be used for the next message it receives.
type Actor interface {
	// Start is called when the actor is started.
	// It receives a context and a mailbox to receive messages.
// The context controls the lifecycle of the actor, and the mailbox is used
// to receive incoming messages. The method should block until the actor is
// terminated. It returns an error if the actor fails to start or terminates
// unexpectedly.
	Start(ctx context.Context, mb *Mailbox) error
}

// Mailbox is a channel-based message queue for an actor.
// It uses a buffered channel to store incoming messages, allowing for
// asynchronous message passing between actors.
type Mailbox struct {
	messages chan any
}

// NewMailbox creates a new mailbox with the given buffer size.
// The size parameter determines the capacity of the mailbox's buffer.
// A larger size can help to avoid blocking the sender if the actor is busy,
// but it also increases memory consumption.
func NewMailbox(size int) *Mailbox {
	return &Mailbox{
		messages: make(chan any, size),
	}
}

// Send puts a message into the mailbox.
// This method will block if the mailbox's buffer is full, until there is
// space available. It is the responsibility of the sender to handle this
// case, for example, by using a timeout or a non-blocking send.
func (mb *Mailbox) Send(msg any) {
	mb.messages <- msg
}

// Receive blocks until a message is received from the mailbox or the context
// is canceled.
// It returns the received message and a nil error on success.
// If the context is canceled, it returns nil and the context's error.
// This method is typically called by the actor in a loop to process incoming
// messages.
func (mb *Mailbox) Receive(ctx context.Context) (any, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case msg := <-mb.messages:
		return msg, nil
	}
}

// Chan returns the underlying message channel.
// This allows for more advanced use cases, such as selecting from multiple
// channels at once. The returned channel is read-only to prevent external
// actors from sending messages directly to it, which would bypass the
// mailbox's intended usage.
func (mb *Mailbox) Chan() <-chan any {
	return mb.messages
}
