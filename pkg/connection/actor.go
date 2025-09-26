package connection

import (
	"net"
	"github.com/turtacn/emqx-go/pkg/broker"
	"github.com/turtacn/emqx-go/pkg/protocol/mqtt"

	"github.com/asynkron/protoactor-go/actor"
)

// Connection actor manages a single client connection.
type Connection struct {
	conn      net.Conn
	brokerPID *actor.PID
}

// New creates a new props for a Connection actor.
func New(conn net.Conn, brokerPID *actor.PID) *actor.Props {
	return actor.PropsFromProducer(func() actor.Actor {
		return &Connection{
			conn:      conn,
			brokerPID: brokerPID,
		}
	})
}

// Receive is the message handler for the Connection actor.
func (c *Connection) Receive(context actor.Context) {
	switch msg := context.Message().(type) {
	case *actor.Started:
		go c.handleConnection(context)
	case *broker.ForwardedPublish:
		c.handleForwardedPublish(msg)
	case *actor.Stopping:
		c.conn.Close()
	}
}

// handleConnection is the main loop for a single client connection.
func (c *Connection) handleConnection(context actor.Context) {
	defer context.Stop(context.Self())

	if _, err := mqtt.DecodeConnect(c.conn); err != nil {
		return
	}
	if err := mqtt.EncodeConnack(c.conn, &mqtt.ConnackPacket{ReturnCode: mqtt.CodeAccepted}); err != nil {
		return
	}

	for {
		fh, err := mqtt.DecodeFixedHeader(c.conn)
		if err != nil {
			return
		}

		switch fh.PacketType {
		case mqtt.TypeSUBSCRIBE:
			packet, _ := mqtt.DecodeSubscribe(fh, c.conn)
			for _, topic := range packet.Topics {
				context.Send(c.brokerPID, &broker.Subscribe{Topic: topic, Sender: context.Self()})
			}
			mqtt.EncodeSuback(c.conn, &mqtt.SubackPacket{MessageID: packet.MessageID, ReturnCodes: []byte{0x00}})
		case mqtt.TypePUBLISH:
			packet, _ := mqtt.DecodePublish(fh, c.conn)
			context.Send(c.brokerPID, &broker.Publish{Topic: packet.TopicName, Payload: packet.Payload})
		case mqtt.TypePINGREQ:
			// Respond to PINGREQ with PINGRESP
		case mqtt.TypeDISCONNECT:
			return
		}
	}
}

func (c *Connection) handleForwardedPublish(msg *broker.ForwardedPublish) {
	packet := &mqtt.PublishPacket{
		TopicName: msg.Topic,
		Payload:   msg.Payload,
	}
	mqtt.EncodePublish(c.conn, packet)
}
