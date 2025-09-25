package broker

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"sync"

	"github.com/mochi-mqtt/server/v2/packets"
	"github.com/turtacn/emqx-go/pkg/actor"
	"github.com/turtacn/emqx-go/pkg/session"
	"github.com/turtacn/emqx-go/pkg/storage"
	"github.com/turtacn/emqx-go/pkg/supervisor"
	"github.com/turtacn/emqx-go/pkg/topic"
)

// Broker is the main actor responsible for managing client sessions and routing messages.
type Broker struct {
	sup      *supervisor.OneForOneSupervisor
	sessions storage.Store // Store for client sessions, mapping ClientID to mailbox
	topics   *topic.Store
	mu       sync.RWMutex
}

// New creates a new Broker actor.
func New() *Broker {
	return &Broker{
		sup:      supervisor.NewOneForOneSupervisor(),
		sessions: storage.NewMemStore(),
		topics:   topic.NewStore(),
	}
}

// StartServer begins listening for incoming TCP connections.
func (b *Broker) StartServer(ctx context.Context, addr string) error {
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", addr, err)
	}
	defer listener.Close()
	log.Printf("MQTT broker listening on %s", addr)

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				select {
				case <-ctx.Done():
					return
				default:
					log.Printf("Failed to accept connection: %v", err)
				}
				continue
			}
			go b.handleConnection(ctx, conn)
		}
	}()

	<-ctx.Done()
	log.Println("Listener is shutting down.")
	return nil
}

// handleConnection manages a single client connection using the real MQTT protocol.
func (b *Broker) handleConnection(ctx context.Context, conn net.Conn) {
	defer conn.Close()
	log.Printf("Accepted connection from %s", conn.RemoteAddr())

	reader := bufio.NewReader(conn)
	var clientID string
	var sessionMailbox *actor.Mailbox
	connCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	for {
		pk, err := readPacket(reader)
		if err != nil {
			if err != io.EOF {
				log.Printf("Error reading packet from %s: %v", conn.RemoteAddr(), err)
			}
			break
		}

		switch pk.FixedHeader.Type {
		case packets.Connect:
			if pk.Connect.ClientIdentifier == "" {
				log.Printf("CONNECT from %s has empty client ID. Closing.", conn.RemoteAddr())
				return
			}
			clientID = pk.Connect.ClientIdentifier
			sessionMailbox = b.registerSession(connCtx, clientID, conn)

			resp := packets.Packet{
				FixedHeader:    packets.FixedHeader{Type: packets.Connack},
				SessionPresent: false,
				ReasonCode:     packets.CodeSuccess.Code,
			}
			err = writePacket(conn, &resp)

		case packets.Subscribe:
			if sessionMailbox == nil {
				log.Println("SUBSCRIBE received before CONNECT")
				return
			}
			for _, sub := range pk.Filters {
				b.topics.Subscribe(sub.Filter, sessionMailbox)
			}
			resp := packets.Packet{
				FixedHeader: packets.FixedHeader{Type: packets.Suback},
				PacketID:    pk.PacketID,
				ReasonCodes: []byte{packets.CodeGrantedQos0.Code},
			}
			err = writePacket(conn, &resp)

		case packets.Publish:
			b.routePublish(pk.TopicName, pk.Payload)

		case packets.Pingreq:
			resp := packets.Packet{FixedHeader: packets.FixedHeader{Type: packets.Pingresp}}
			err = writePacket(conn, &resp)

		case packets.Disconnect:
			log.Printf("Client %s sent DISCONNECT.", clientID)
			return

		default:
			log.Printf("Received unhandled packet type: %v", pk.FixedHeader.Type)
		}

		if err != nil {
			log.Printf("Error handling packet for %s: %v", clientID, err)
			return
		}
	}

	if clientID != "" {
		b.unregisterSession(clientID)
		log.Printf("Client %s disconnected.", clientID)
	}
}

func (b *Broker) registerSession(ctx context.Context, clientID string, conn net.Conn) *actor.Mailbox {
	if mb, err := b.sessions.Get(clientID); err == nil {
		log.Printf("Client %s is reconnecting, session exists.", clientID)
		return mb.(*actor.Mailbox)
	}

	log.Printf("Registering new session for client %s", clientID)
	sess := session.New(clientID, conn)
	mb := actor.NewMailbox(100)

	spec := supervisor.Spec{
		ID:      fmt.Sprintf("session-%s", clientID),
		Actor:   sess,
		Restart: supervisor.RestartPermanent,
		Mailbox: mb,
	}
	b.sup.StartChild(ctx, spec)

	b.sessions.Set(clientID, mb)
	return mb
}

func (b *Broker) unregisterSession(clientID string) {
	// In a real implementation, we would also need to remove this session from all topic subscriptions.
	b.sessions.Delete(clientID)
}

// routePublish sends a message to all subscribers of a topic.
func (b *Broker) routePublish(topic string, payload []byte) {
	subscribers := b.topics.GetSubscribers(topic)
	msg := session.Publish{
		Topic:   topic,
		Payload: payload,
	}
	for _, mb := range subscribers {
		mb.Send(msg)
	}
}

// readPacket is a helper to read a full MQTT packet from a connection.
func readPacket(r *bufio.Reader) (*packets.Packet, error) {
	fh := new(packets.FixedHeader)
	b, err := r.ReadByte()
	if err != nil {
		return nil, err
	}
	err = fh.Decode(b)
	if err != nil {
		return nil, err
	}
	rem, _, err := packets.DecodeLength(r)
	if err != nil {
		return nil, err
	}
	fh.Remaining = rem

	buf := make([]byte, fh.Remaining)
	if fh.Remaining > 0 {
		_, err = io.ReadFull(r, buf)
		if err != nil {
			return nil, err
		}
	}

	pk := &packets.Packet{FixedHeader: *fh}
	switch pk.FixedHeader.Type {
	case packets.Connect:
		err = pk.ConnectDecode(buf)
	case packets.Publish:
		err = pk.PublishDecode(buf)
	case packets.Subscribe:
		err = pk.SubscribeDecode(buf)
	case packets.Pingreq:
		err = pk.PingreqDecode(buf)
	case packets.Disconnect:
		err = pk.DisconnectDecode(buf)
	}

	return pk, err
}

// writePacket is a helper to encode and write a packet to a connection.
func writePacket(w io.Writer, pk *packets.Packet) error {
	var buf bytes.Buffer
	var err error
	switch pk.FixedHeader.Type {
	case packets.Connack:
		err = pk.ConnackEncode(&buf)
	case packets.Publish:
		err = pk.PublishEncode(&buf)
	case packets.Suback:
		err = pk.SubackEncode(&buf)
	case packets.Pingresp:
		err = pk.PingrespEncode(&buf)
	default:
		return fmt.Errorf("unsupported packet type for writing: %v", pk.FixedHeader.Type)
	}

	if err != nil {
		return err
	}
	_, err = w.Write(buf.Bytes())
	return err
}
