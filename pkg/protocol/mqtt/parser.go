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
	"encoding/binary"
	"errors"
	"io"
)

// FixedHeader represents the fixed header present in all MQTT control packets.
// It contains the packet type, flags, and the remaining length of the packet.
type FixedHeader struct {
	// PacketType is the type of the MQTT control packet (e.g., CONNECT, PUBLISH).
	PacketType byte
	// Flags are specific to each packet type.
	Flags byte
	// RemLength is the length of the variable header and payload.
	RemLength int
}

// DecodeFixedHeader reads and decodes the fixed header from an io.Reader.
// It returns the decoded FixedHeader or an error if reading fails.
func DecodeFixedHeader(r io.Reader) (*FixedHeader, error) {
	headerByte := make([]byte, 1)
	if _, err := io.ReadFull(r, headerByte); err != nil {
		return nil, err
	}
	fh := &FixedHeader{
		PacketType: headerByte[0] >> 4,
		Flags:      headerByte[0] & 0x0F,
	}
	lenByte := make([]byte, 1)
	if _, err := io.ReadFull(r, lenByte); err != nil {
		return nil, errors.New("could not read remaining length")
	}
	fh.RemLength = int(lenByte[0])
	return fh, nil
}

// DecodeConnect reads and decodes a CONNECT packet from an io.Reader.
// It performs validation to ensure the packet is a valid CONNECT packet.
func DecodeConnect(r io.Reader) (*ConnectPacket, error) {
	fh, err := DecodeFixedHeader(r)
	if err != nil {
		return nil, err
	}
	if fh.PacketType != TypeCONNECT {
		return nil, errors.New("not a CONNECT packet")
	}
	buf := make([]byte, fh.RemLength)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, errors.New("could not read remaining packet bytes")
	}
	packet := &ConnectPacket{}
	var offset int
	protocolName, offset, err := readString(buf, offset)
	if err != nil || protocolName != "MQTT" {
		return nil, errors.New("invalid protocol name")
	}
	packet.ProtocolName = protocolName
	packet.ProtocolVersion = buf[offset]
	offset++
	connectFlags := buf[offset]
	packet.CleanSession = (connectFlags>>1)&1 == 1
	offset++
	packet.KeepAlive = binary.BigEndian.Uint16(buf[offset : offset+2])
	offset += 2
	clientID, _, err := readString(buf, offset)
	if err != nil {
		return nil, errors.New("could not read client id")
	}
	packet.ClientID = clientID
	return packet, nil
}

// DecodeSubscribe reads and decodes a SUBSCRIBE packet.
func DecodeSubscribe(fh *FixedHeader, r io.Reader) (*SubscribePacket, error) {
	buf := make([]byte, fh.RemLength)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, err
	}
	packet := &SubscribePacket{}
	var offset int
	packet.MessageID = binary.BigEndian.Uint16(buf[offset : offset+2])
	offset += 2
	for offset < len(buf) {
		topic, newOffset, err := readString(buf, offset)
		if err != nil {
			return nil, err
		}
		packet.Topics = append(packet.Topics, topic)
		offset = newOffset
		packet.QoSs = append(packet.QoSs, buf[offset])
		offset++
	}
	return packet, nil
}

// DecodePublish reads and decodes a PUBLISH packet.
func DecodePublish(fh *FixedHeader, r io.Reader) (*PublishPacket, error) {
	buf := make([]byte, fh.RemLength)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, err
	}
	packet := &PublishPacket{}
	var offset int
	topicName, offset, err := readString(buf, offset)
	if err != nil {
		return nil, err
	}
	packet.TopicName = topicName
	packet.Payload = buf[offset:]
	return packet, nil
}

// EncodeConnack encodes a ConnackPacket and writes it to an io.Writer.
func EncodeConnack(w io.Writer, p *ConnackPacket) error {
	header := []byte{TypeCONNACK << 4, 2}
	variableHeader := []byte{0, p.ReturnCode}
	if p.SessionPresent {
		variableHeader[0] = 1
	}
	if _, err := w.Write(header); err != nil {
		return err
	}
	return binary.Write(w, binary.BigEndian, variableHeader)
}

// EncodeSuback encodes a SubackPacket and writes it to an io.Writer.
func EncodeSuback(w io.Writer, p *SubackPacket) error {
	header := []byte{TypeSUBACK << 4, 0}
	remLength := 2 + len(p.ReturnCodes)
	header[1] = byte(remLength)
	if _, err := w.Write(header); err != nil {
		return err
	}
	msgIDBytes := make([]byte, 2)
	binary.BigEndian.PutUint16(msgIDBytes, p.MessageID)
	if _, err := w.Write(msgIDBytes); err != nil {
		return err
	}
	if _, err := w.Write(p.ReturnCodes); err != nil {
		return err
	}
	return nil
}

// EncodePublish encodes a PublishPacket and writes it to an io.Writer.
func EncodePublish(w io.Writer, p *PublishPacket) error {
	var vh bytes.Buffer
	topicBytes := []byte(p.TopicName)
	vh.WriteByte(byte(len(topicBytes) >> 8))
	vh.WriteByte(byte(len(topicBytes) & 0xFF))
	vh.Write(topicBytes)

	remLength := vh.Len() + len(p.Payload)
	header := []byte{TypePUBLISH << 4, byte(remLength)}
	if _, err := w.Write(header); err != nil {
		return err
	}
	if _, err := w.Write(vh.Bytes()); err != nil {
		return err
	}
	if _, err := w.Write(p.Payload); err != nil {
		return err
	}
	return nil
}

// readString is a helper function to read a UTF-8 encoded string from a byte
// slice, as per the MQTT specification. The string is prefixed with a 2-byte
// length.
func readString(b []byte, offset int) (string, int, error) {
	if len(b) < offset+2 {
		return "", 0, errors.New("buffer too short to read string length")
	}
	length := int(binary.BigEndian.Uint16(b[offset : offset+2]))
	offset += 2
	if len(b) < offset+length {
		return "", 0, errors.New("buffer too short to read string content")
	}
	str := string(b[offset : offset+length])
	return str, offset + length, nil
}
