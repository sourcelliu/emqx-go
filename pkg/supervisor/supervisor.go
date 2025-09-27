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

// Package supervisor provides a fault-tolerance mechanism inspired by Erlang/OTP.
// A supervisor is a specialized actor whose purpose is to monitor and manage the
// lifecycle of other actors, known as its children. This promotes the "let it
// crash" design philosophy, where failures are handled by restarting components
// to a known-good state rather than by defensive programming within the
// components themselves.
package supervisor

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/turtacn/emqx-go/pkg/actor"
	"github.com/turtacn/emqx-go/pkg/metrics"
)

// RestartStrategy defines the supervisor's behavior when a child actor
// terminates.
type RestartStrategy int

const (
	// RestartPermanent means the child actor is always restarted, regardless of
	// how it terminates (normally or with an error).
	RestartPermanent RestartStrategy = iota
	// RestartTransient means the child actor is restarted only if it terminates
	// abnormally (i.e., returns an error or panics). It is not restarted if it
	// terminates normally (returns nil).
	RestartTransient
	// RestartTemporary means the child actor is never restarted, regardless of
	// how it terminates.
	RestartTemporary
)

// Spec defines the configuration for a single child actor that is to be
// managed by a supervisor. It contains the actor instance, its unique ID, and
// its restart strategy.
type Spec struct {
	// ID is a unique identifier for the child actor, used for logging and
	// metrics.
	ID string
	// Actor is the actor instance to be supervised. It must implement the
	// actor.Actor interface.
	Actor actor.Actor
	// Restart defines the condition under which the supervisor should restart
	// this child actor.
	Restart RestartStrategy
	// Mailbox is the mailbox to be used by the actor for receiving messages.
	Mailbox *actor.Mailbox
	// startFunc is an optional function for starting the actor. It is primarily
	// used for testing purposes to inject mock behavior.
	startFunc func(context.Context, *actor.Mailbox) error
}

// Supervisor defines the contract for a supervisor process. It is responsible
// for starting, stopping, and monitoring its child actors according to their
// defined specifications.
type Supervisor interface {
	// Start begins the supervision of a set of child actors defined by their
	// specs. This is typically a non-blocking operation.
	Start(ctx context.Context, specs []Spec) error
	// StartChild allows for a new child actor to be started dynamically under
	// the supervisor's management after the supervisor has already started.
	StartChild(ctx context.Context, spec Spec)
}

// OneForOneSupervisor implements a "one-for-one" supervision strategy. This
// means that if a child actor terminates, the supervisor takes action only on
// that specific child, without affecting its siblings.
type OneForOneSupervisor struct{}

// NewOneForOneSupervisor creates a new supervisor that implements the
// one-for-one restart strategy.
func NewOneForOneSupervisor() *OneForOneSupervisor {
	return &OneForOneSupervisor{}
}

// Start launches the initial set of supervised children. Each child is started
// in its own goroutine. This method is non-blocking and returns immediately.
//
// - ctx: A context that governs the lifecycle of all children started by this
//   supervisor.
// - specs: A slice of specifications, one for each child to be started.
//
// Returns an error if no child specifications are provided.
func (s *OneForOneSupervisor) Start(ctx context.Context, specs []Spec) error {
	if len(specs) == 0 {
		return fmt.Errorf("no child specs provided")
	}
	for _, spec := range specs {
		s.StartChild(ctx, spec)
	}
	return nil
}

// StartChild launches and begins monitoring a single new child actor in its own
// dedicated goroutine.
//
// - ctx: The parent context for the new child.
// - spec: The specification defining the child to be started.
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