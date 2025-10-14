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

package actor

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNewMailbox(t *testing.T) {
	mb := NewMailbox(10)
	assert.NotNil(t, mb)
	assert.Equal(t, 10, cap(mb.messages))
}

func TestMailboxSendAndReceive(t *testing.T) {
	mb := NewMailbox(1)
	msg := "hello"

	// Test non-blocking send
	mb.Send(msg)

	// Test receive
	receivedMsg, err := mb.Receive(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, msg, receivedMsg)
}

func TestMailboxReceiveWithContextCancellation(t *testing.T) {
	mb := NewMailbox(1)
	ctx, cancel := context.WithCancel(context.Background())

	// Cancel the context immediately
	cancel()

	_, err := mb.Receive(ctx)
	assert.Error(t, err)
	assert.Equal(t, context.Canceled, err)
}

func TestMailboxChan(t *testing.T) {
	mb := NewMailbox(1)
	msg := "test"
	mb.Send(msg)

	ch := mb.Chan()
	receivedMsg := <-ch
	assert.Equal(t, msg, receivedMsg)
}

func TestMailboxBlockingSend(t *testing.T) {
	// Mailbox with buffer size 1
	mb := NewMailbox(1)
	mb.Send("first")

	sendComplete := make(chan bool)
	go func() {
		// This send should block until a receive happens
		mb.Send("second")
		sendComplete <- true
	}()

	// Give the goroutine a moment to block
	time.Sleep(10 * time.Millisecond)

	// First receive should unblock the send
	received, err := mb.Receive(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, "first", received)

	// The second send should now be able to complete
	select {
	case <-sendComplete:
		// Success
	case <-time.After(50 * time.Millisecond):
		t.Fatal("Send did not complete after receive")
	}

	// Receive the second message
	received, err = mb.Receive(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, "second", received)
}
