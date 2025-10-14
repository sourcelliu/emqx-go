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

// package main provides a simple example of the actor and supervisor packages.
package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/turtacn/emqx-go/pkg/actor"
	"github.com/turtacn/emqx-go/pkg/supervisor"
)

// echoActor is a simple actor that prints any message it receives.
type echoActor struct{}

// Start is the main loop for the echoActor. It waits for messages and prints them.
func (a *echoActor) Start(ctx context.Context, mb *actor.Mailbox) error {
	log.Println("Echo actor started.")
	for {
		msg, err := mb.Receive(ctx)
		if err != nil {
			// Context canceled, the actor should shut down.
			log.Printf("Echo actor shutting down: %v", err)
			return err
		}
		fmt.Printf("Received message: %v\n", msg)
	}
}

// panicActor is an actor that panics to demonstrate supervisor restarts.
type panicActor struct {
	mb *actor.Mailbox
}

func (p *panicActor) Start(ctx context.Context, mb *actor.Mailbox) error {
	log.Println("Panic actor started. It will panic in 3 seconds.")
	p.mb = mb
	time.Sleep(3 * time.Second)
	panic("Simulating an actor panic")
}

func main() {
	log.Println("Starting supervisor example...")

	// Create a context that we can cancel to shut down the supervisor and its actors.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create a new one-for-one supervisor.
	sup := supervisor.NewOneForOneSupervisor()

	// Create a mailbox for the echo actor.
	echoMailbox := actor.NewMailbox(10)

	// Define the specifications for the actors we want to supervise.
	specs := []supervisor.Spec{
		{
			ID:      "echo-1",
			Actor:   &echoActor{},
			Restart: supervisor.RestartPermanent,
			Mailbox: echoMailbox,
		},
		{
			ID:      "panic-actor-1",
			Actor:   &panicActor{},
			Restart: supervisor.RestartPermanent, // It will be restarted after it panics.
			Mailbox: actor.NewMailbox(1),
		},
	}

	// Start the supervisor with the defined actor specs.
	if err := sup.Start(ctx, specs); err != nil {
		log.Fatalf("Failed to start supervisor: %v", err)
	}

	// Send a message to the echo actor.
	log.Println("Sending a message to the echo actor...")
	echoMailbox.Send("Hello, Actor!")

	// Keep the main goroutine alive to observe the output.
	// The panic actor will panic after 3 seconds and be restarted by the supervisor.
	// After 10 seconds, we'll cancel the context to gracefully shut down.
	log.Println("Example running. The panic actor will crash and be restarted. Waiting for 10 seconds before shutdown...")
	time.Sleep(10 * time.Second)

	log.Println("Shutting down supervisor...")
}
