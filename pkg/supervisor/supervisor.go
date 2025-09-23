package supervisor

import (
	"github.com/asynkron/protoactor-go/actor"
)

// Supervisor manages the actor system and the lifecycle of top-level actors.
type Supervisor struct {
	system *actor.ActorSystem
}

// New creates and initializes a new Supervisor.
func New() *Supervisor {
	system := actor.NewActorSystem()
	return &Supervisor{
		system: system,
	}
}

// Start begins the supervisor's operations.
func (s *Supervisor) Start() {
}

// Stop gracefully shuts down the actor system.
func (s *Supervisor) Stop() {
	s.system.Shutdown()
}

// Spawn starts a new actor under the root supervisor and returns its PID.
func (s *Supervisor) Spawn(props *actor.Props) *actor.PID {
	return s.system.Root.Spawn(props)
}

// Context returns the root context of the actor system.
func (s *Supervisor) Context() *actor.RootContext {
	return s.system.Root
}
