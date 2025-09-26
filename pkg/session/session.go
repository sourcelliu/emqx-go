package session

import (
	"bytes"
	"context"
	"io"
	"log"

	"github.com/mochi-mqtt/server/v2/packets"
	"github.com/turtacn/emqx-go/pkg/actor"
)

// Publish is the message type for publishing a message to a client.
type Publish struct {
	Topic   string
	Payload []byte
}

// Session is an actor that manages writing messages to a single client's connection.
type Session struct {
	ID   string
	conn io.Writer
}

// New creates a new Session actor.
func New(id string, conn io.Writer) *Session {
	return &Session{
		ID:   id,
		conn: conn,
	}
}

// Start is the main loop for the Session actor.
// It listens for Publish messages and writes them to the client's connection.
func (s *Session) Start(ctx context.Context, mb *actor.Mailbox) error {
	log.Printf("Session actor started for client %s", s.ID)
	for {
		msg, err := mb.Receive(ctx)
		if err != nil {
			log.Printf("Session actor for client %s shutting down: %v", s.ID, err)
			return err
		}

		switch m := msg.(type) {
		case Publish:
			pk := &packets.Packet{
				FixedHeader: packets.FixedHeader{Type: packets.Publish, Qos: 0},
				TopicName:   m.Topic,
				Payload:     m.Payload,
			}
			var buf bytes.Buffer
			if err := pk.PublishEncode(&buf); err != nil {
				log.Printf("Error encoding publish packet for %s: %v", s.ID, err)
				continue
			}
			if _, err := s.conn.Write(buf.Bytes()); err != nil {
				log.Printf("Error writing to client %s: %v", s.ID, err)
				// Returning an error will cause the supervisor to restart this actor,
				// but the connection will likely be dead. A real implementation
				// would need more sophisticated error handling here.
				return err
			}
		default:
			log.Printf("Session actor for %s received unknown message type: %T", s.ID, m)
		}
	}
}