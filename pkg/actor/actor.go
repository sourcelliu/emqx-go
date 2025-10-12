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

// Package actor provides a lightweight, Erlang/OTP-inspired actor model for
// building concurrent and fault-tolerant systems in Go.
//
// The model is based on two main components:
//   - Actor: An interface representing a concurrent process that communicates
//     exclusively through messages.
//   - Mailbox: A message queue for an actor that enables asynchronous
//     communication.
//
// Actors are designed to be supervised, meaning their lifecycle is managed by a
// supervisor process that can restart them if they fail. This promotes the "let
// it crash" philosophy for building robust applications.
package actor

import "context"

// Actor defines the interface for a process that runs concurrently and
// communicates via messages.
//
// An actor is an entity that, in response to a message it receives,
// can concurrently:
//   - send a finite number of messages to other actors;
//   - create a finite number of new actors;
//   - designate the behavior to be used for the next message it receives.
type Actor interface {
	// Start is called when the actor is started by a supervisor.
	// It receives a context that controls its lifecycle and a mailbox for
	// receiving messages. The method should block until the actor terminates.
	// It returns an error if the actor terminates unexpectedly.
	Start(ctx context.Context, mb *Mailbox) error
}

// Mailbox is a channel-based message queue for an actor.
// It uses a buffered channel to enable asynchronous message passing.
type Mailbox struct {
	messages chan any
}

// NewMailbox creates a new mailbox with the specified buffer size.
// The size determines the capacity of the mailbox's buffer. A larger size can
// help to avoid blocking the sender if the actor is busy, but it also
// increases memory consumption.
func NewMailbox(size int) *Mailbox {
	return &Mailbox{
		messages: make(chan any, size),
	}
}

// Send puts a message into the mailbox.
// This method will block if the mailbox's buffer is full.
func (mb *Mailbox) Send(msg any) {
	mb.messages <- msg
}

// Receive blocks until a message is received from the mailbox or the context
// is canceled.
// It returns the received message or an error if the context is done.
func (mb *Mailbox) Receive(ctx context.Context) (any, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case msg := <-mb.messages:
		return msg, nil
	}
}

// Chan returns the underlying message channel as a read-only channel.
// This allows for more advanced use cases, such as selecting from multiple
// channels at once.
func (mb *Mailbox) Chan() <-chan any {
	return mb.messages
}
