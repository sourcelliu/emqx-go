package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"turtacn/emqx-go/pkg/broker"
)

func main() {
	log.Println("Starting EMQX-GO Broker PoC...")

	ctx, cancel := context.WithCancel(context.Background())

	// Set up signal handling for graceful shutdown.
	shutdownChan := make(chan os.Signal, 1)
	signal.Notify(shutdownChan, os.Interrupt, syscall.SIGTERM)

	b := broker.New()

	// Start the main broker server.
	go func() {
		if err := b.StartServer(ctx, ":1883"); err != nil {
			log.Fatalf("Broker server failed: %v", err)
		}
	}()

	// Wait for a shutdown signal.
	<-shutdownChan
	log.Println("Shutdown signal received. Shutting down...")
	cancel()
	log.Println("Shutdown complete.")
}