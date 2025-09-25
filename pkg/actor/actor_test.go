package actor

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestMailboxSendAndReceive(t *testing.T) {
	mb := NewMailbox(10)
	msg := "hello"

	mb.Send(msg)

	received, err := mb.Receive(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, msg, received)
}

func TestMailboxReceiveWithTimeout(t *testing.T) {
	mb := NewMailbox(10)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	_, err := mb.Receive(ctx)
	assert.Error(t, err)
	assert.Equal(t, context.DeadlineExceeded, err)
}

func TestMailboxChan(t *testing.T) {
	mb := NewMailbox(1)
	msg := "test"

	mb.Send(msg)

	select {
	case received := <-mb.Chan():
		assert.Equal(t, msg, received)
	case <-time.After(1 * time.Second):
		t.Fatal("did not receive message from channel")
	}
}