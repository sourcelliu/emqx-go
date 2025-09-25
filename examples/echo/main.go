package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"turtacn/emqx-go/pkg/actor"
	"turtacn/emqx-go/pkg/supervisor"
)

// echoActor is a simple actor that prints messages it receives.
type echoActor struct{}

func (a *echoActor) Start(ctx context.Context, mb *actor.Mailbox) error {
	log.Println("Echo actor started.")
	for {
		msg, err := mb.Receive(ctx)
		if err != nil {
			log.Printf("Echo actor shutting down: %v", err)
			return err
		}
		log.Printf("ECHO: %v", msg)
	}
}

// suicidalActor is an actor that panics after a few seconds.
type suicidalActor struct{}

func (a *suicidalActor) Start(ctx context.Context, mb *actor.Mailbox) error {
	log.Println("Suicidal actor started. I will panic in 3 seconds.")
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(3 * time.Second):
		panic("goodbye, cruel world!")
	}
}

func main() {
	log.Println("Starting mini-OTP PoC...")

	// Create a context that we can cancel to shut down the application.
	ctx, cancel := context.WithCancel(context.Background())

	// Set up a channel to listen for OS signals for graceful shutdown.
	shutdownChan := make(chan os.Signal, 1)
	signal.Notify(shutdownChan, os.Interrupt, syscall.SIGTERM)

	// Create mailboxes for our actors.
	echoMailbox := actor.NewMailbox(100)
	suicidalMailbox := actor.NewMailbox(10)

	// Define the specifications for the child actors.
	specs := []supervisor.Spec{
		{
			ID:      "echo-1",
			Actor:   &echoActor{},
			Restart: supervisor.RestartPermanent,
			Mailbox: echoMailbox,
		},
		{
			ID:      "suicidal-1",
			Actor:   &suicidalActor{},
			Restart: supervisor.RestartPermanent, // It will be restarted after panicking.
			Mailbox: suicidalMailbox,
		},
	}

	// Create and start the supervisor.
	sup := supervisor.NewOneForOneSupervisor()
	go func() {
		if err := sup.Start(ctx, specs); err != nil {
			log.Printf("Supervisor exited with error: %v", err)
		}
	}()

	// Start a goroutine to send messages to the echo actor.
	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		i := 0
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				echoMailbox.Send(i)
				i++
			}
		}
	}()

	// Wait for a shutdown signal.
	<-shutdownChan
	log.Println("Shutdown signal received. Shutting down supervisor and actors...")
	cancel()

	// Give a moment for graceful shutdown to complete.
	time.Sleep(1 * time.Second)
	log.Println("Shutdown complete.")
}