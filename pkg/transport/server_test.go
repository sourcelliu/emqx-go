package transport

import (
	"bytes"
	"io"
	"net"
	"testing"
	"time"
	"github.com/turtacn/emqx-go/pkg/broker"
	"github.com/turtacn/emqx-go/pkg/protocol/mqtt"
	"github.com/turtacn/emqx-go/pkg/supervisor"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestServer(t *testing.T) (*supervisor.Supervisor, *Server) {
	sup := supervisor.New()
	brokerPID := sup.Spawn(broker.New())
	server := NewServer(sup, brokerPID)
	err := server.Start(":0")
	require.NoError(t, err)
	return sup, server
}

func connect(t *testing.T, addr string, clientID string) net.Conn {
	conn, err := net.DialTimeout("tcp", addr, 1*time.Second)
	require.NoError(t, err)

	connectPacket := []byte{
		0x10, 0x00, // Header, placeholder length
		0x00, 0x04, 'M', 'Q', 'T', 'T',
		0x04, 0x02, 0x00, 0x3C,
	}
	clientIDBytes := []byte(clientID)
	connectPacket = append(connectPacket, byte(len(clientIDBytes)>>8), byte(len(clientIDBytes)&0xFF))
	connectPacket = append(connectPacket, clientIDBytes...)
	connectPacket[1] = byte(len(connectPacket) - 2)

	_, err = conn.Write(connectPacket)
	require.NoError(t, err)

	responseBytes := make([]byte, 4)
	_, err = io.ReadFull(conn, responseBytes)
	require.NoError(t, err, "Should receive a CONNACK")
	expectedConnack := []byte{0x20, 0x02, 0x00, 0x00}
	assert.Equal(t, expectedConnack, responseBytes)
	return conn
}

func TestServer_Integration_PubSub(t *testing.T) {
	sup, server := setupTestServer(t)
	defer sup.Stop()
	defer server.Stop()
	addr := server.Addr().String()

	subConn := connect(t, addr, "subscriber")
	defer subConn.Close()

	subPacketBytes := []byte{0x82, 0x0e, 0x00, 0x01, 0x00, 0x09, 't', 'e', 's', 't', '/', 't', 'o', 'p', 'i', 'c', 0x00}
	_, err := subConn.Write(subPacketBytes)
	require.NoError(t, err)
	subackBytes := make([]byte, 5)
	_, err = io.ReadFull(subConn, subackBytes)
	require.NoError(t, err)
	assert.Equal(t, []byte{0x90, 0x03, 0x00, 0x01, 0x00}, subackBytes)

	time.Sleep(50 * time.Millisecond)

	pubConn := connect(t, addr, "publisher")
	defer pubConn.Close()

	pubPacket := &mqtt.PublishPacket{TopicName: "test/topic", Payload: []byte("hello")}
	var pubBuf bytes.Buffer
	err = mqtt.EncodePublish(&pubBuf, pubPacket)
	require.NoError(t, err)
	_, err = pubConn.Write(pubBuf.Bytes())
	require.NoError(t, err)

	fh, err := mqtt.DecodeFixedHeader(subConn)
	require.NoError(t, err)
	receivedPacket, err := mqtt.DecodePublish(fh, subConn)
	require.NoError(t, err)
	assert.Equal(t, "test/topic", receivedPacket.TopicName)
	assert.Equal(t, []byte("hello"), receivedPacket.Payload)
}
