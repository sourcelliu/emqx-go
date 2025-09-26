package supervisor

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/turtacn/emqx-go/pkg/actor"
)

// RestartStrategy defines when a supervisor should restart a child actor.
type RestartStrategy int

const (
	// RestartPermanent means the child is always restarted.
	RestartPermanent RestartStrategy = iota
	// RestartTransient means the child is restarted only on abnormal termination.
	RestartTransient
	// RestartTemporary means the child is never restarted.
	RestartTemporary
)

// Spec defines the specification for a child actor process.
type Spec struct {
	ID        string
	Actor     actor.Actor
	Restart   RestartStrategy
	Shutdown  time.Duration
	Mailbox   *actor.Mailbox
	startFunc func(context.Context, *actor.Mailbox) error
}

// Supervisor defines the interface for a supervisor process.
type Supervisor interface {
	Start(ctx context.Context, specs []Spec) error
	StartChild(ctx context.Context, spec Spec)
}

// OneForOneSupervisor implements a one-for-one supervision strategy.
// If a child process terminates, only that process is restarted.
type OneForOneSupervisor struct{}

// NewOneForOneSupervisor creates a new one-for-one supervisor.
func NewOneForOneSupervisor() *OneForOneSupervisor {
	return &OneForOneSupervisor{}
}

// Start launches the initial set of supervised children. This method is non-blocking.
func (s *OneForOneSupervisor) Start(ctx context.Context, specs []Spec) error {
	if len(specs) == 0 {
		return fmt.Errorf("no child specs provided")
	}
	for _, spec := range specs {
		s.StartChild(ctx, spec)
	}
	return nil
}

// StartChild launches and monitors a single new child actor in its own goroutine.
func (s *OneForOneSupervisor) StartChild(ctx context.Context, spec Spec) {
	// Each child gets its own cancellable context, derived from the parent.
	childCtx, cancel := context.WithCancel(ctx)
	go s.monitorChild(childCtx, cancel, spec)
}

// monitorChild is the internal loop that monitors a single child actor.
func (s *OneForOneSupervisor) monitorChild(ctx context.Context, cancel context.CancelFunc, spec Spec) {
	defer cancel() // Ensure the context for this child is eventually cancelled.

	for {
		var err error
		func() {
			// This deferred recover is the key to supervisor resilience.
			// It catches panics from the child actor.
			defer func() {
				if r := recover(); r != nil {
					err = fmt.Errorf("actor %s panicked: %v", spec.ID, r)
				}
			}()
			// The actor's start function is called here.
			// If it panics, the recover block above will catch it.
			err = s.startChild(ctx, spec)
		}()

		if err != nil {
			log.Printf("Actor %s terminated with error: %v", spec.ID, err)
		} else {
			log.Printf("Actor %s terminated normally.", spec.ID)
		}

		// Check if the supervisor's context is done. If so, don't restart.
		select {
		case <-ctx.Done():
			log.Printf("Supervisor context is done, not restarting actor %s.", spec.ID)
			return
		default:
			// Continue to restart logic.
		}

		// Implement restart logic based on the outcome.
		shouldRestart := false
		switch spec.Restart {
		case RestartPermanent:
			shouldRestart = true
		case RestartTransient:
			// Restart only if there was an error (abnormal termination).
			if err != nil {
				shouldRestart = true
			}
		case RestartTemporary:
			// Never restart.
			shouldRestart = false
		}

		if !shouldRestart {
			log.Printf("Actor %s will not be restarted based on strategy %v.", spec.ID, spec.Restart)
			return
		}

		log.Printf("Restarting actor %s...", spec.ID)
		// Small delay before restarting to prevent rapid-fire restarts.
		time.Sleep(1 * time.Second)
	}
}

// startChild launches the actor's Start method.
func (s *OneForOneSupervisor) startChild(ctx context.Context, spec Spec) error {
	log.Printf("Starting actor %s...", spec.ID)
	// If a custom start function is provided, use it.
	// This is useful for testing or wrapping the actor's start logic.
	if spec.startFunc != nil {
		return spec.startFunc(ctx, spec.Mailbox)
	}
	return spec.Actor.Start(ctx, spec.Mailbox)
}