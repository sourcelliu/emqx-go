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

// package supervisor provides an OTP-style supervisor for managing the
// lifecycle of concurrent actors.
package supervisor

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/turtacn/emqx-go/pkg/actor"
	"github.com/turtacn/emqx-go/pkg/metrics"
)

// RestartStrategy defines the restart behavior for a supervised child actor.
type RestartStrategy int

const (
	// RestartPermanent indicates that the child actor should always be restarted.
	RestartPermanent RestartStrategy = iota
	// RestartTransient indicates that the child actor should be restarted only if
	// it terminates abnormally (i.e., with an error or a panic).
	RestartTransient
	// RestartTemporary indicates that the child actor should never be restarted.
	RestartTemporary
)

// Spec defines the specification for a child actor process managed by a supervisor.
type Spec struct {
	// ID is a unique identifier for the child actor, used for logging.
	ID string
	// Actor is the actor instance to be supervised.
	Actor actor.Actor
	// Restart defines the restart strategy for this child.
	Restart RestartStrategy
	// Mailbox is the mailbox to be used by the actor.
	Mailbox *actor.Mailbox
	// startFunc is an optional function for starting the actor, useful for testing.
	startFunc func(context.Context, *actor.Mailbox) error
}

// Supervisor defines the interface for a supervisor process.
type Supervisor interface {
	// Start begins the supervision of a set of child actors.
	Start(ctx context.Context, specs []Spec) error
	// StartChild starts and supervises a single child actor dynamically.
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
	childCtx, cancel := context.WithCancel(ctx)
	go s.monitorChild(childCtx, cancel, spec)
}

// monitorChild is the internal loop that monitors a single child actor.
// It handles actor termination, panics, and restart logic.
func (s *OneForOneSupervisor) monitorChild(ctx context.Context, cancel context.CancelFunc, spec Spec) {
	defer cancel()

	for {
		var err error
		func() {
			// Recover from panics within the child actor.
			defer func() {
				if r := recover(); r != nil {
					err = fmt.Errorf("actor %s panicked: %v", spec.ID, r)
				}
			}()
			err = s.startActor(ctx, spec)
		}()

		log.Printf("Actor %s terminated. Reason: %v", spec.ID, err)

		// If the supervisor's context is done, do not restart.
		select {
		case <-ctx.Done():
			log.Printf("Supervisor context is done, not restarting actor %s.", spec.ID)
			return
		default:
			// Proceed to restart logic.
		}

		shouldRestart := false
		switch spec.Restart {
		case RestartPermanent:
			shouldRestart = true
		case RestartTransient:
			if err != nil {
				shouldRestart = true
			}
		case RestartTemporary:
			shouldRestart = false
		}

		if !shouldRestart {
			log.Printf("Actor %s will not be restarted based on strategy.", spec.ID)
			return
		}

		// Increment the restart metric for this specific actor.
		metrics.SupervisorRestartsTotal.WithLabelValues(spec.ID).Inc()
		log.Printf("Restarting actor %s...", spec.ID)
		// A small delay to prevent rapid-fire restarts in case of persistent issues.
		time.Sleep(1 * time.Second)
	}
}

// startActor launches the actor's Start method.
func (s *OneForOneSupervisor) startActor(ctx context.Context, spec Spec) error {
	log.Printf("Starting actor %s...", spec.ID)
	if spec.startFunc != nil {
		return spec.startFunc(ctx, spec.Mailbox)
	}
	return spec.Actor.Start(ctx, spec.Mailbox)
}