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

// mockActor is a controllable actor for testing purposes.
type mockActor struct {
	startFunc func(ctx context.Context, mb *actor.Mailbox) error
}

func (m *mockActor) Start(ctx context.Context, mb *actor.Mailbox) error {
	if m.startFunc != nil {
		return m.startFunc(ctx, mb)
	}
	// Block until context is cancelled by default
	<-ctx.Done()
	return nil
}

func TestSupervisor_StartAndShutdown(t *testing.T) {
	sup := NewOneForOneSupervisor()
	ctx, cancel := context.WithCancel(context.Background())

	var wg sync.WaitGroup
	wg.Add(1)

	spec := Spec{
		ID:      "test-actor",
		Actor: &mockActor{startFunc: func(ctx context.Context, mb *actor.Mailbox) error {
			defer wg.Done()
			<-ctx.Done()
			return nil
		}},
		Restart: RestartPermanent,
		Mailbox: actor.NewMailbox(1),
	}

	err := sup.Start(ctx, []Spec{spec})
	assert.NoError(t, err)

	// Wait a bit for the actor to start
	time.Sleep(100 * time.Millisecond)

	// In this test, we now need a way to wait for the supervisor to finish.
	// We'll wait for the child actor's goroutine to complete.
	// Since the supervisor is no longer blocking, we just cancel and wait.

	// Cancel the supervisor's context
	cancel()

	// Wait for the actor to finish
	wg.Wait()
}

func TestSupervisor_OneForOne_PermanentRestart(t *testing.T) {
	sup := NewOneForOneSupervisor()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	restartCount := 0
	var mu sync.Mutex

	spec := Spec{
		ID: "actor-to-restart",
		Actor: &mockActor{startFunc: func(ctx context.Context, mb *actor.Mailbox) error {
			mu.Lock()
			restartCount++
			mu.Unlock()
			// Simulate an immediate crash
			return errors.New("i have failed")
		}},
		Restart: RestartPermanent,
		Mailbox: actor.NewMailbox(1),
	}

	err := sup.Start(ctx, []Spec{spec})
	assert.NoError(t, err)

	// Wait for the supervisor to do its work
	<-ctx.Done()

	mu.Lock()
	defer mu.Unlock()
	// The actor should start once, fail, and be restarted at least once.
	// Depending on timing, it might restart multiple times.
	assert.Greater(t, restartCount, 1, "Actor should have been restarted")
}

func TestSupervisor_OneForOne_PanicRestart(t *testing.T) {
	sup := NewOneForOneSupervisor()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	startCount := 0
	var mu sync.Mutex

	// This actor will genuinely panic, and the test relies on the supervisor
	// to recover from it and restart the actor.
	panickingActor := &mockActor{
		startFunc: func(ctx context.Context, mb *actor.Mailbox) error {
			mu.Lock()
			startCount++
			mu.Unlock()
			panic("something went horribly wrong")
		},
	}

	spec := Spec{
		ID:      "panicking-actor",
		Actor:   panickingActor,
		Restart: RestartPermanent,
		Mailbox: actor.NewMailbox(1),
	}

	err := sup.Start(ctx, []Spec{spec})
	assert.NoError(t, err)

	// Wait for the supervisor to restart the actor multiple times.
	<-ctx.Done()

	mu.Lock()
	defer mu.Unlock()
	// The actor should have been started more than once, proving the supervisor
	// caught the panic and correctly applied the restart strategy.
	assert.Greater(t, startCount, 1, "Actor should have panicked and been restarted by the supervisor")
}

func TestSupervisor_OneForOne_NoRestart(t *testing.T) {
	sup := NewOneForOneSupervisor()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	startCount := 0
	var mu sync.Mutex

	spec := Spec{
		ID: "temp-actor",
		Actor: &mockActor{startFunc: func(ctx context.Context, mb *actor.Mailbox) error {
			mu.Lock()
			startCount++
			mu.Unlock()
			// Terminate normally after a short time
			return nil
		}},
		Restart: RestartTemporary, // Should not restart
		Mailbox: actor.NewMailbox(1),
	}

	err := sup.Start(ctx, []Spec{spec})
	assert.NoError(t, err)

	<-ctx.Done()

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, 1, startCount, "Temporary actor should only start once")
}

func TestSupervisor_Strategies(t *testing.T) {
	t.Run("start with no specs", func(t *testing.T) {
		sup := NewOneForOneSupervisor()
		err := sup.Start(context.Background(), []Spec{})
		assert.Error(t, err)
		assert.Equal(t, "no child specs provided", err.Error())
	})

	t.Run("transient restart on error", func(t *testing.T) {
		sup := NewOneForOneSupervisor()
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		startCount := 0
		var mu sync.Mutex

		spec := Spec{
			ID: "transient-actor-fail",
			Actor: &mockActor{startFunc: func(ctx context.Context, mb *actor.Mailbox) error {
				mu.Lock()
				startCount++
				mu.Unlock()
				return errors.New("i failed")
			}},
			Restart: RestartTransient,
			Mailbox: actor.NewMailbox(1),
		}
		err := sup.Start(ctx, []Spec{spec})
		assert.NoError(t, err)
		<-ctx.Done()

		mu.Lock()
		defer mu.Unlock()
		assert.Greater(t, startCount, 1, "Transient actor should restart after failure")
	})

	t.Run("transient no restart on success", func(t *testing.T) {
		sup := NewOneForOneSupervisor()
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		startCount := 0
		var mu sync.Mutex

		spec := Spec{
			ID: "transient-actor-success",
			Actor: &mockActor{startFunc: func(ctx context.Context, mb *actor.Mailbox) error {
				mu.Lock()
				startCount++
				mu.Unlock()
				return nil // Normal termination
			}},
			Restart: RestartTransient,
			Mailbox: actor.NewMailbox(1),
		}
		err := sup.Start(ctx, []Spec{spec})
		assert.NoError(t, err)
		<-ctx.Done()

		mu.Lock()
		defer mu.Unlock()
		assert.Equal(t, 1, startCount, "Transient actor should not restart after normal termination")
	})
}
