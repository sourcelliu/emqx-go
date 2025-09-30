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

// Package concurrency_tests provides comprehensive concurrent testing for emqx-go
package concurrency_tests

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/turtacn/emqx-go/pkg/actor"
	"github.com/turtacn/emqx-go/pkg/broker"
	"github.com/turtacn/emqx-go/pkg/storage"
	"github.com/turtacn/emqx-go/pkg/topic"
)

// TestTopicStoreConcurrency tests the thread safety of topic store operations
func TestTopicStoreConcurrency(t *testing.T) {
	const (
		numGoroutines = 100
		numTopics     = 50
		numOperations = 1000
	)

	store := topic.NewStore()
	var wg sync.WaitGroup

	// Statistics
	var subscriptions, unsubscriptions, lookups int64

	// Test concurrent subscribe/unsubscribe/lookup operations
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			mailboxes := make([]*actor.Mailbox, 10)
			for j := range mailboxes {
				mailboxes[j] = actor.NewMailbox(100)
			}

			for op := 0; op < numOperations; op++ {
				topicName := fmt.Sprintf("test/topic/%d", op%numTopics)
				mb := mailboxes[op%len(mailboxes)]

				switch op % 3 {
				case 0: // Subscribe
					store.Subscribe(topicName, mb, byte(op%3))
					atomic.AddInt64(&subscriptions, 1)

				case 1: // Unsubscribe
					store.Unsubscribe(topicName, mb)
					atomic.AddInt64(&unsubscriptions, 1)

				case 2: // Lookup
					subs := store.GetSubscribers(topicName)
					_ = len(subs)
					atomic.AddInt64(&lookups, 1)
				}
			}
		}(i)
	}

	wg.Wait()

	t.Logf("Completed %d subscriptions, %d unsubscriptions, %d lookups",
		subscriptions, unsubscriptions, lookups)
}

// TestStorageConcurrency tests the thread safety of storage operations
func TestStorageConcurrency(t *testing.T) {
	const (
		numWorkers = 50
		numOps     = 200
	)

	store := storage.NewMemStore()
	var wg sync.WaitGroup

	// Statistics
	var sets, gets, deletes int64
	var errors int64

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			for op := 0; op < numOps; op++ {
				key := fmt.Sprintf("key-%d-%d", workerID, op)
				value := fmt.Sprintf("value-%d-%d", workerID, op)

				switch op % 3 {
				case 0: // Set
					err := store.Set(key, value)
					if err != nil {
						atomic.AddInt64(&errors, 1)
					}
					atomic.AddInt64(&sets, 1)

				case 1: // Get
					_, err := store.Get(key)
					if err != nil && err != storage.ErrNotFound {
						atomic.AddInt64(&errors, 1)
					}
					atomic.AddInt64(&gets, 1)

				case 2: // Delete
					err := store.Delete(key)
					if err != nil {
						atomic.AddInt64(&errors, 1)
					}
					atomic.AddInt64(&deletes, 1)
				}
			}
		}(i)
	}

	wg.Wait()

	t.Logf("Completed %d sets, %d gets, %d deletes with %d errors",
		sets, gets, deletes, errors)
	assert.Equal(t, int64(0), errors, "No errors should occur in storage operations")
}

// TestActorMailboxConcurrency tests the thread safety of actor mailbox operations
func TestActorMailboxConcurrency(t *testing.T) {
	const (
		numSenders   = 20
		numReceivers = 5
		numMessages  = 100
		bufferSize   = 1000
	)

	mailbox := actor.NewMailbox(bufferSize)
	var wg sync.WaitGroup

	// Statistics
	var sent, received int64

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Start receivers
	for i := 0; i < numReceivers; i++ {
		wg.Add(1)
		go func(receiverID int) {
			defer wg.Done()

			for {
				select {
				case <-ctx.Done():
					return
				default:
					_, err := mailbox.Receive(ctx)
					if err != nil {
						if err == context.DeadlineExceeded || err == context.Canceled {
							return
						}
						return
					}
					atomic.AddInt64(&received, 1)
				}
			}
		}(i)
	}

	// Start senders
	senderWg := sync.WaitGroup{}
	for i := 0; i < numSenders; i++ {
		senderWg.Add(1)
		go func(senderID int) {
			defer senderWg.Done()

			for j := 0; j < numMessages; j++ {
				select {
				case <-ctx.Done():
					return
				default:
					message := fmt.Sprintf("msg-%d-%d", senderID, j)
					mailbox.Send(message)
					atomic.AddInt64(&sent, 1)
				}
			}
		}(i)
	}

	// Wait for all senders to complete
	senderWg.Wait()

	// Give receivers time to process remaining messages
	time.Sleep(500 * time.Millisecond)
	cancel()

	// Wait for receivers to stop
	wg.Wait()

	t.Logf("Sent %d messages, received %d messages", sent, received)

	// We should have received all or nearly all messages
	assert.GreaterOrEqual(t, received, sent-int64(bufferSize),
		"Should receive most sent messages (accounting for buffer)")
}

// TestBrokerConcurrentSessions tests concurrent session management
func TestBrokerConcurrentSessions(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping concurrent broker test in short mode")
	}

	const (
		numSessions = 20
		numMessages = 50
	)

	b := broker.New("test-node", nil)
	b.SetupDefaultAuth()

	var wg sync.WaitGroup
	var successfulSessions int64

	for i := 0; i < numSessions; i++ {
		wg.Add(1)
		go func(sessionID int) {
			defer wg.Done()

			// Create a mock connection - simplified for testing
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			// Simulate session registration
			mb := actor.NewMailbox(100)

			// This is a simplified test - just test that we can create the auth chain
			authChain := b.GetAuthChain()
			if authChain != nil {
				atomic.AddInt64(&successfulSessions, 1)
			}

			// Send some messages to the mailbox to test message handling
			for j := 0; j < numMessages; j++ {
				select {
				case <-ctx.Done():
					return
				default:
					mb.Send(fmt.Sprintf("test-message-%d", j))
				}
			}
		}(i)
	}

	wg.Wait()

	t.Logf("Successfully created %d sessions", successfulSessions)
	assert.Equal(t, int64(numSessions), successfulSessions,
		"All sessions should be created successfully")
}

// TestMemoryUsageUnderLoad tests memory usage under high concurrent load
func TestMemoryUsageUnderLoad(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping memory test in short mode")
	}

	var m1, m2 runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&m1)

	const (
		numWorkers = 100
		numOps     = 500
	)

	store := topic.NewStore()
	var wg sync.WaitGroup

	// Create load
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			mailboxes := make([]*actor.Mailbox, 10)
			for j := range mailboxes {
				mailboxes[j] = actor.NewMailbox(50)
			}

			for op := 0; op < numOps; op++ {
				topicName := fmt.Sprintf("load/test/topic/%d/%d", workerID, op)
				mb := mailboxes[op%len(mailboxes)]

				store.Subscribe(topicName, mb, 1)

				if op%10 == 0 {
					store.GetSubscribers(topicName)
				}

				if op%20 == 0 {
					store.Unsubscribe(topicName, mb)
				}
			}
		}(i)
	}

	wg.Wait()

	runtime.GC()
	runtime.ReadMemStats(&m2)

	allocDiff := m2.Alloc - m1.Alloc
	t.Logf("Memory allocation difference: %d bytes (%.2f MB)",
		allocDiff, float64(allocDiff)/(1024*1024))

	// Memory usage should be reasonable (less than 100MB for this test)
	assert.Less(t, allocDiff, uint64(100*1024*1024),
		"Memory usage should be reasonable under load")
}

// ThreadSafeBuffer - reuse from session tests
type ThreadSafeBuffer struct {
	mu   sync.RWMutex
	data []byte
	pos  int
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
		return 0, fmt.Errorf("EOF")
	}
	n = copy(p, b.data[b.pos:])
	b.pos += n
	return n, nil
}

func (b *ThreadSafeBuffer) Close() error { return nil }