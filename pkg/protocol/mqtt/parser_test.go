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

package mqtt

import (
	"bytes"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDecodeFixedHeader(t *testing.T) {
	// PINGREQ packet with RemLength 0
	data := []byte{0xC0, 0x00}
	r := bytes.NewReader(data)
	fh, err := DecodeFixedHeader(r)
	require.NoError(t, err)
	require.NotNil(t, fh)
	assert.Equal(t, byte(0x0C), fh.PacketType)
	assert.Equal(t, byte(0x00), fh.Flags)
	assert.Equal(t, 0, fh.RemLength)
}

func TestDecodeConnect(t *testing.T) {
	// Valid CONNECT packet
	payload := []byte{
		0x00, 0x04, 'M', 'Q', 'T', 'T', // Protocol Name
		0x04,       // Protocol Level
		0x02,       // Connect Flags (Clean Session)
		0x00, 0x3C, // Keep Alive
		0x00, 0x06, 't', 'e', 's', 't', 'i', 'd', // Client ID
	}
	header := []byte{TypeCONNECT << 4, byte(len(payload))}
	packetBytes := append(header, payload...)

	r := bytes.NewReader(packetBytes)
	packet, err := DecodeConnect(r)
	require.NoError(t, err)
	assert.Equal(t, "MQTT", packet.ProtocolName)
	assert.Equal(t, byte(4), packet.ProtocolVersion)
	assert.True(t, packet.CleanSession)
	assert.Equal(t, uint16(60), packet.KeepAlive)
	assert.Equal(t, "testid", packet.ClientID)

	// Test with a non-CONNECT packet type in the header
	header[0] = TypePUBLISH << 4
	r = bytes.NewReader(append(header, payload...))
	_, err = DecodeConnect(r)
	assert.Error(t, err)
}

func TestEncodeConnack(t *testing.T) {
	var buf bytes.Buffer
	p := &ConnackPacket{
		SessionPresent: true,
		ReturnCode:     0,
	}
	err := EncodeConnack(&buf, p)
	require.NoError(t, err)
	expected := []byte{
		TypeCONNACK << 4, 2, // Header
		1, 0, // Variable Header (SessionPresent=true, ReturnCode=0)
	}
	assert.Equal(t, expected, buf.Bytes())
}

func TestDecodeSubscribe(t *testing.T) {
	payload := []byte{
		0x00, 0x0A, // Message ID
		0x00, 0x03, 'a', '/', 'b', // Topic
		0x01, // QoS
	}
	fh := &FixedHeader{PacketType: TypeSUBSCRIBE, RemLength: len(payload)}
	r := bytes.NewReader(payload)
	packet, err := DecodeSubscribe(fh, r)
	require.NoError(t, err)
	assert.Equal(t, uint16(10), packet.MessageID)
	assert.Equal(t, []string{"a/b"}, packet.Topics)
	assert.Equal(t, []byte{1}, packet.QoSs)
}

func TestEncodeSuback(t *testing.T) {
	var buf bytes.Buffer
	p := &SubackPacket{
		MessageID:   123,
		ReturnCodes: []byte{0x00, 0x01, 0x80},
	}
	err := EncodeSuback(&buf, p)
	require.NoError(t, err)

	r := bytes.NewReader(buf.Bytes())
	fh, err := DecodeFixedHeader(r)
	require.NoError(t, err)
	assert.Equal(t, TypeSUBACK, fh.PacketType)
	assert.Equal(t, 2+len(p.ReturnCodes), fh.RemLength)

	remBytes, err := io.ReadAll(r)
	require.NoError(t, err)
	assert.Equal(t, byte(0), remBytes[0])   // Message ID MSB
	assert.Equal(t, byte(123), remBytes[1]) // Message ID LSB
	assert.Equal(t, []byte{0x00, 0x01, 0x80}, remBytes[2:])
}

func TestDecodePublish(t *testing.T) {
	payload := []byte{
		0x00, 0x07, 't', 'o', 'p', 'i', 'c', '/', '1', // Topic Name
		'h', 'e', 'l', 'l', 'o', // Payload
	}
	fh := &FixedHeader{PacketType: TypePUBLISH, RemLength: len(payload)}
	r := bytes.NewReader(payload)
	packet, err := DecodePublish(fh, r)
	require.NoError(t, err)
	assert.Equal(t, "topic/1", packet.TopicName)
	assert.Equal(t, []byte("hello"), packet.Payload)
}

func TestEncodePublish(t *testing.T) {
	var buf bytes.Buffer
	p := &PublishPacket{
		TopicName: "a/b",
		Payload:   []byte("world"),
	}
	err := EncodePublish(&buf, p)
	require.NoError(t, err)

	r := bytes.NewReader(buf.Bytes())
	fh, err := DecodeFixedHeader(r)
	require.NoError(t, err)
	assert.Equal(t, TypePUBLISH, fh.PacketType)

	remBytes, err := io.ReadAll(r)
	require.NoError(t, err)

	// Manually decode to verify
	topicLen := int(remBytes[0])<<8 | int(remBytes[1])
	assert.Equal(t, 3, topicLen)
	assert.Equal(t, "a/b", string(remBytes[2:2+topicLen]))
	assert.Equal(t, []byte("world"), remBytes[2+topicLen:])
}