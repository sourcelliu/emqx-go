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

package session

import (
	"context"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/mochi-mqtt/server/v2/packets"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/turtacn/emqx-go/pkg/actor"
	"github.com/turtacn/emqx-go/tests/testutil"
)

// ThreadSafeBuffer is a thread-safe wrapper around a buffer for testing
type ThreadSafeBuffer struct {
	mu   sync.RWMutex
	data []byte
	pos  int
}

func NewThreadSafeBuffer() *ThreadSafeBuffer {
	return &ThreadSafeBuffer{
		data: make([]byte, 0, 1024),
	}
}

func (b *ThreadSafeBuffer) Write(p []byte) (n int, err error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.data = append(b.data, p...)
	return len(p), nil
}

func (b *ThreadSafeBuffer) Read(p []byte) (n int, err error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if b.pos >= len(b.data) {
		return 0, io.EOF
	}

	n = copy(p, b.data[b.pos:])
	b.pos += n
	return n, nil
}

func (b *ThreadSafeBuffer) GetWrittenData() []byte {
	b.mu.RLock()
	defer b.mu.RUnlock()

	result := make([]byte, len(b.data))
	copy(result, b.data)
	return result
}

func (b *ThreadSafeBuffer) Reset() {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.data = b.data[:0]
	b.pos = 0
}

// Implement net.Conn interface for testing
func (b *ThreadSafeBuffer) Close() error                       { return nil }
func (b *ThreadSafeBuffer) LocalAddr() net.Addr                { return nil }
func (b *ThreadSafeBuffer) RemoteAddr() net.Addr               { return nil }
func (b *ThreadSafeBuffer) SetDeadline(t time.Time) error      { return nil }
func (b *ThreadSafeBuffer) SetReadDeadline(t time.Time) error  { return nil }
func (b *ThreadSafeBuffer) SetWriteDeadline(t time.Time) error { return nil }

func TestNew(t *testing.T) {
	conn := NewThreadSafeBuffer()
	s := New("test-client", conn)
	assert.NotNil(t, s)
	assert.Equal(t, "test-client", s.ID)
	assert.Equal(t, conn, s.conn)
}

func TestSession_Start(t *testing.T) {
	conn := NewThreadSafeBuffer()
	s := New("test-client", conn)
	mb := actor.NewMailbox(10)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Channel to coordinate test execution
	sessionStarted := make(chan struct{})
	sessionDone := make(chan error, 1)

	go func() {
		close(sessionStarted) // Signal that session goroutine has started
		err := s.Start(ctx, mb)
		sessionDone <- err
	}()

	// Wait for session to start
	<-sessionStarted

	// Give the session a moment to be ready
	time.Sleep(50 * time.Millisecond)

	// Send a publish message to the session actor
	pubMsg := Publish{
		Topic:   "test/topic",
		Payload: []byte("hello"),
	}
	mb.Send(pubMsg)

	// Give the actor time to process the message and write to buffer
	time.Sleep(200 * time.Millisecond)

	// Cancel context to stop the session
	cancel()

	// Wait for session to complete
	err := <-sessionDone
	assert.ErrorIs(t, err, context.Canceled)

	// Now safely read from the buffer after session has stopped
	data := conn.GetWrittenData()
	require.Greater(t, len(data), 0, "Session should have written data to connection")

	// Create a new buffer with the written data for packet reading
	reader := NewThreadSafeBuffer()
	_, err = reader.Write(data)
	require.NoError(t, err)

	// Verify that the connection received the encoded PUBLISH packet
	pk, err := testutil.ReadPacket(reader)
	require.NoError(t, err)
	require.Equal(t, packets.Publish, pk.FixedHeader.Type)
	assert.Equal(t, "test/topic", pk.TopicName)
	assert.Equal(t, []byte("hello"), pk.Payload)
}

func TestSession_Start_Concurrent(t *testing.T) {
	const numSessions = 10
	const messagesPerSession = 50

	var wg sync.WaitGroup
	errors := make(chan error, numSessions)

	for i := 0; i < numSessions; i++ {
		wg.Add(1)
		go func(sessionID int) {
			defer wg.Done()

			conn := NewThreadSafeBuffer()
			s := New("test-client", conn)
			mb := actor.NewMailbox(100)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			// Start session in separate goroutine
			sessionDone := make(chan error, 1)
			go func() {
				err := s.Start(ctx, mb)
				sessionDone <- err
			}()

			// Send multiple messages concurrently
			for j := 0; j < messagesPerSession; j++ {
				pubMsg := Publish{
					Topic:   "test/topic",
					Payload: []byte("concurrent message"),
				}
				mb.Send(pubMsg)
			}

			// Let messages process
			time.Sleep(100 * time.Millisecond)

			// Stop session
			cancel()
			err := <-sessionDone

			if err != nil && err != context.Canceled {
				errors <- err
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	// Check for any unexpected errors
	for err := range errors {
		t.Errorf("Unexpected error in concurrent session test: %v", err)
	}
}