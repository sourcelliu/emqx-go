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

// package transport is responsible for handling the network transport layer of
// the MQTT server. It provides a TCP server that accepts incoming client
// connections and hands them off to the connection handling layer.
package transport

import (
	"log"
	"net"
	"sync"

	"github.com/asynkron/protoactor-go/actor"
	"github.com/turtacn/emqx-go/pkg/connection"
	"github.com/turtacn/emqx-go/pkg/supervisor"
)

// Server manages the accepting and handling of raw TCP connections.
// For each incoming connection, it spawns a new connection actor to handle
// the MQTT protocol handshake and subsequent communication.
type Server struct {
	listener   net.Listener
	wg         sync.WaitGroup
	quit       chan struct{}
	supervisor *supervisor.Supervisor
	brokerPID  *actor.PID
}

// NewServer creates and returns a new transport Server.
//
// sup is the supervisor that will manage the connection actors.
// brokerPID is the process ID of the main broker actor, which will be passed
// to each new connection actor.
func NewServer(sup *supervisor.Supervisor, brokerPID *actor.PID) *Server {
	return &Server{
		quit:       make(chan struct{}),
		supervisor: sup,
		brokerPID:  brokerPID,
	}
}

// Start begins listening for new connections on the specified network address.
// It starts the accept loop in a new goroutine.
func (s *Server) Start(addr string) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	s.listener = ln

	s.wg.Add(1)
	go s.acceptLoop()

	log.Printf("TCP server started, listening on %s", addr)
	return nil
}

// Stop gracefully shuts down the server. It closes the listener, stops the
// accept loop, and waits for all active goroutines to finish.
func (s *Server) Stop() {
	close(s.quit)
	if s.listener != nil {
		s.listener.Close()
	}
	s.wg.Wait()
	log.Println("TCP server stopped")
}

// acceptLoop is the main loop for accepting new client connections.
// It runs in a separate goroutine and continuously accepts new connections
// until the server is stopped. For each new connection, it creates a
// connection actor and spawns it under the server's supervisor.
func (s *Server) acceptLoop() {
	defer s.wg.Done()
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			select {
			case <-s.quit:
				return
			default:
				log.Printf("Error accepting connection: %v", err)
			}
			continue
		}
		// For each connection, create a new connection actor.
		props := connection.New(conn, s.brokerPID)
		s.supervisor.Spawn(props)
	}
}

// Addr returns the network address that the server is listening on.
// It returns nil if the server is not listening.
func (s *Server) Addr() net.Addr {
	if s.listener == nil {
		return nil
	}
	return s.listener.Addr()
}
