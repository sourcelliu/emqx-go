package mqtt

// Control Packet Types
const (
	TypeCONNECT     byte = 1
	TypeCONNACK     byte = 2
	TypePUBLISH     byte = 3
	TypeSUBSCRIBE   byte = 8
	TypeSUBACK      byte = 9
	TypePINGREQ     byte = 12
	TypePINGRESP    byte = 13
	TypeDISCONNECT  byte = 14
)

// CONNACK Return Codes
const (
	CodeAccepted byte = 0
)

// ConnectPacket represents an MQTT CONNECT packet.
type ConnectPacket struct {
	ProtocolName    string
	ProtocolVersion byte
	CleanSession    bool
	KeepAlive       uint16
	ClientID        string
}

// ConnackPacket represents an MQTT CONNACK packet.
type ConnackPacket struct {
	SessionPresent bool
	ReturnCode     byte
}

// PublishPacket represents an MQTT PUBLISH packet.
type PublishPacket struct {
	TopicName string
	Payload   []byte
}

// SubscribePacket represents an MQTT SUBSCRIBE packet.
type SubscribePacket struct {
	MessageID uint16
	Topics    []string
	QoSs      []byte
}

// SubackPacket represents an MQTT SUBACK packet.
type SubackPacket struct {
	MessageID   uint16
	ReturnCodes []byte
}
