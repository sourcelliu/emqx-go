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

package supervisor

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/turtacn/emqx-go/pkg/actor"
)

// RestartStrategy defines the restart behavior for a supervised child actor.
// This determines whether and when a terminated child process should be restarted.
type RestartStrategy int

const (
	// RestartPermanent indicates that the child actor should always be restarted,
	// regardless of whether it terminated normally or abnormally.
	RestartPermanent RestartStrategy = iota
	// RestartTransient indicates that the child actor should be restarted only if
	// it terminates abnormally (i.e., with an error or a panic). Normal
	// termination will not trigger a restart.
	RestartTransient
	// RestartTemporary indicates that the child actor should never be restarted,
	// regardless of the termination reason.
	RestartTemporary
)

// Spec defines the specification for a child actor process managed by a supervisor.
// It encapsulates all the necessary information for the supervisor to start and
// manage the lifecycle of an actor.
type Spec struct {
	// ID is a unique identifier for the child actor. It is used for logging and
	// management purposes.
	ID string
	// Actor is the actual actor instance to be supervised. It must implement the
	// actor.Actor interface.
	Actor actor.Actor
	// Restart defines the restart strategy for this child.
	Restart RestartStrategy
	// Shutdown is the duration to wait for the actor to gracefully terminate
	// before it is forcefully stopped.
	Shutdown time.Duration
	// Mailbox is the mailbox to be used by the actor for receiving messages.
	Mailbox *actor.Mailbox
	// startFunc is an optional function to start the actor. If nil, the actor's
	// Start method is called directly. This is useful for testing purposes.
	startFunc func(context.Context, *actor.Mailbox) error
}

// Supervisor defines the interface for a supervisor process.
// A supervisor is responsible for starting, stopping, and monitoring its child
// actors, and restarting them when they fail, according to a defined strategy.
type Supervisor interface {
	// Start begins the supervision of a set of child actors defined by the specs.
	// The provided context governs the lifecycle of the supervisor and its
	// children. This method is expected to be non-blocking.
	Start(ctx context.Context, specs []Spec) error
	// StartChild starts and supervises a single child actor. This allows for
	// dynamically adding children to the supervisor.
	StartChild(ctx context.Context, spec Spec)
}

// OneForOneSupervisor implements a one-for-one supervision strategy.
// In this strategy, if a child process terminates, only that specific process is
// restarted. It does not affect any other child processes.
type OneForOneSupervisor struct{}

// NewOneForOneSupervisor creates a new one-for-one supervisor.
func NewOneForOneSupervisor() *OneForOneSupervisor {
	return &OneForOneSupervisor{}
}

// Start launches the initial set of supervised children. This method is non-blocking.
// It iterates through the provided specifications and starts a monitoring goroutine
// for each child using StartChild.
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
// It creates a new context for the child, derived from the supervisor's context,
// and then starts the monitoring loop.
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
