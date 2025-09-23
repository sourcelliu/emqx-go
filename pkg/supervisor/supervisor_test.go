package supervisor

import (
	"testing"
	"time"

	"github.com/asynkron/protoactor-go/actor"
	"github.com/stretchr/testify/assert"
)

// --- Test Actor Definition ---

type Ping struct{ Who string }
type Pong struct{ Message string }

type TestActor struct{}

func (a *TestActor) Receive(context actor.Context) {
	switch msg := context.Message().(type) {
	case *Ping:
		context.Respond(&Pong{Message: "Pong to " + msg.Who})
	}
}

func NewTestActor() actor.Actor {
	return &TestActor{}
}

// --- Supervisor Tests ---

func TestSupervisor_New(t *testing.T) {
	sup := New()
	assert.NotNil(t, sup, "Supervisor should not be nil")
	assert.NotNil(t, sup.system, "Actor system should be initialized")
}

func TestSupervisor_LifecycleAndSpawning(t *testing.T) {
	// Create and start the supervisor
	sup := New()
	sup.Start() // For now, this is a no-op, but good to have in the test

	// Define actor properties
	props := actor.PropsFromProducer(NewTestActor)

	// Spawn the actor using the supervisor
	pid := sup.Spawn(props)
	assert.NotNil(t, pid, "Spawning an actor should return a valid PID")

	// Send a message and verify the response
	future := sup.Context().RequestFuture(pid, &Ping{Who: "Test"}, 1*time.Second)
	result, err := future.Result()

	assert.NoError(t, err, "Requesting from actor should not produce an error")
	response, ok := result.(*Pong)
	assert.True(t, ok, "Response should be of type Pong")
	assert.Equal(t, "Pong to Test", response.Message, "Incorrect response message")

	// Stop the supervisor
	sup.Stop()
}

func TestSupervisor_Stop(t *testing.T) {
	sup := New()
	sup.Stop()
	// We just ensure that stopping a new supervisor doesn't panic.
}
