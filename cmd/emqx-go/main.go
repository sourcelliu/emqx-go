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

// package main is the entrypoint for the EMQX-Go application.
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/turtacn/emqx-go/pkg/broker"
)

func main() {
	log.Println("Starting EMQX-GO Broker PoC (Phase 2)...")

	// Create a context that can be canceled to shut down the server.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create and start the MQTT broker.
	b := broker.New()
	go func() {
		if err := b.StartServer(ctx, ":1883"); err != nil {
			log.Fatalf("Broker server failed: %v", err)
		}
	}()

	// Wait for a shutdown signal.
	shutdownChan := make(chan os.Signal, 1)
	signal.Notify(shutdownChan, os.Interrupt, syscall.SIGTERM)
	<-shutdownChan

	log.Println("Shutdown signal received. Shutting down...")
	// The deferred cancel() will handle the shutdown of the server.
}