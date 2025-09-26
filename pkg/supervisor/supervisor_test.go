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

package supervisor

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/turtacn/emqx-go/pkg/actor"
)

// MockActor is a mock implementation of the actor.Actor interface for testing.
type MockActor struct {
	startFunc func(ctx context.Context, mb *actor.Mailbox) error
}

func (m *MockActor) Start(ctx context.Context, mb *actor.Mailbox) error {
	if m.startFunc != nil {
		return m.startFunc(ctx, mb)
	}
	<-ctx.Done()
	return nil
}

func TestOneForOneSupervisor_Start(t *testing.T) {
	sup := NewOneForOneSupervisor()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var started sync.WaitGroup
	started.Add(2)

	mockActor1 := &MockActor{
		startFunc: func(ctx context.Context, mb *actor.Mailbox) error {
			started.Done()
			<-ctx.Done()
			return nil
		},
	}
	mockActor2 := &MockActor{
		startFunc: func(ctx context.Context, mb *actor.Mailbox) error {
			started.Done()
			<-ctx.Done()
			return nil
		},
	}

	specs := []Spec{
		{ID: "actor1", Actor: mockActor1, Restart: RestartPermanent, Mailbox: actor.NewMailbox(1)},
		{ID: "actor2", Actor: mockActor2, Restart: RestartPermanent, Mailbox: actor.NewMailbox(1)},
	}

	err := sup.Start(ctx, specs)
	assert.NoError(t, err)

	// Wait for both actors to start
	waitChan := make(chan struct{})
	go func() {
		started.Wait()
		close(waitChan)
	}()

	select {
	case <-waitChan:
		// Success
	case <-time.After(1 * time.Second):
		t.Fatal("Actors did not start in time")
	}
}

func TestOneForOneSupervisor_RestartPermanent_Panic(t *testing.T) {
	sup := NewOneForOneSupervisor()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	restartCount := 0
	var mu sync.Mutex

	panicActor := &MockActor{
		startFunc: func(ctx context.Context, mb *actor.Mailbox) error {
			mu.Lock()
			count := restartCount
			restartCount++
			mu.Unlock()

			if count == 0 {
				panic("I am a panicked actor!")
			}
			// After the first panic, it should run until the context is canceled.
			<-ctx.Done()
			return nil
		},
	}

	spec := Spec{
		ID:      "panicActor",
		Actor:   panicActor,
		Restart: RestartPermanent,
		Mailbox: actor.NewMailbox(1),
	}

	sup.StartChild(ctx, spec)

	// Give the supervisor time to detect the panic and restart the actor.
	time.Sleep(1500 * time.Millisecond)

	mu.Lock()
	finalRestartCount := restartCount
	mu.Unlock()

	// The actor should have been started once, panicked, and then restarted once.
	assert.Equal(t, 2, finalRestartCount, "Actor should have been started twice (initial + 1 restart)")
}

func TestOneForOneSupervisor_RestartTransient(t *testing.T) {
	sup := NewOneForOneSupervisor()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	restartCount := 0
	var mu sync.Mutex

	mockActor := &MockActor{
		startFunc: func(ctx context.Context, mb *actor.Mailbox) error {
			mu.Lock()
			restartCount++
			mu.Unlock()
			// Terminate with an error on the first run
			if restartCount == 1 {
				return errors.New("transient error")
			}
			<-ctx.Done()
			return nil
		},
	}

	spec := Spec{
		ID:      "transientActor",
		Actor:   mockActor,
		Restart: RestartTransient,
		Mailbox: actor.NewMailbox(1),
	}

	sup.StartChild(ctx, spec)
	time.Sleep(1500 * time.Millisecond)

	mu.Lock()
	assert.Equal(t, 2, restartCount, "Actor should restart on transient error")
	mu.Unlock()
}

func TestOneForOneSupervisor_RestartTemporary(t *testing.T) {
	sup := NewOneForOneSupervisor()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	startCount := 0
	var mu sync.Mutex

	mockActor := &MockActor{
		startFunc: func(ctx context.Context, mb *actor.Mailbox) error {
			mu.Lock()
			startCount++
			mu.Unlock()
			return errors.New("some error") // Terminate immediately
		},
	}

	spec := Spec{
		ID:      "temporaryActor",
		Actor:   mockActor,
		Restart: RestartTemporary,
		Mailbox: actor.NewMailbox(1),
	}

	sup.StartChild(ctx, spec)
	time.Sleep(500 * time.Millisecond)

	mu.Lock()
	assert.Equal(t, 1, startCount, "Actor should not be restarted")
	mu.Unlock()
}