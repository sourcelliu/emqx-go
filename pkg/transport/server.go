package transport

import (
	"log"
	"net"
	"sync"
	"turtacn/emqx-go/pkg/connection"
	"turtacn/emqx-go/pkg/supervisor"

	"github.com/asynkron/protoactor-go/actor"
)

// Server handles accepting and managing raw TCP connections.
type Server struct {
	listener   net.Listener
	wg         sync.WaitGroup
	quit       chan struct{}
	supervisor *supervisor.Supervisor
	brokerPID  *actor.PID
}

// NewServer creates a new transport server.
func NewServer(sup *supervisor.Supervisor, brokerPID *actor.PID) *Server {
	return &Server{
		quit:       make(chan struct{}),
		supervisor: sup,
		brokerPID:  brokerPID,
	}
}

// Start begins listening for new connections on the specified address.
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

// Stop gracefully shuts down the server.
func (s *Server) Stop() {
	close(s.quit)
	if s.listener != nil {
		s.listener.Close()
	}
	s.wg.Wait()
	log.Println("TCP server stopped")
}

// acceptLoop is the main loop for accepting new connections.
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
		props := connection.New(conn, s.brokerPID)
		s.supervisor.Spawn(props)
	}
}

// Addr returns the network address the server is listening on.
func (s *Server) Addr() net.Addr {
	if s.listener == nil {
		return nil
	}
	return s.listener.Addr()
}
