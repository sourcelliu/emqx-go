package mqtt

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
)

// FixedHeader represents the first part of any MQTT packet.
type FixedHeader struct {
	PacketType byte
	Flags      byte
	RemLength  int
}

// DecodeFixedHeader reads the first few bytes and decodes them into a FixedHeader struct.
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

// DecodeConnect reads from the provided reader and attempts to parse an MQTT CONNECT packet.
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

// DecodeSubscribe reads a SUBSCRIBE packet.
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

// DecodePublish reads a PUBLISH packet.
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

// EncodeConnack writes an MQTT CONNACK packet to the provided writer.
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

// EncodeSuback writes a SUBACK packet to the writer.
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

// EncodePublish writes a PUBLISH packet to the writer.
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

// readString reads a length-prefixed string as per the MQTT spec.
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
