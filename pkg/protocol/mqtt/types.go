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

// package mqtt provides basic types and parsing functions for the MQTT protocol.
// It includes definitions for various MQTT packet types and functions to encode
// and decode them from a network connection.
package mqtt

// Control Packet Types as defined in the MQTT 3.1.1 specification.
const (
	_               byte = iota // Reserved
	TypeCONNECT                 // Client request to connect to Server
	TypeCONNACK                 // Connect acknowledgment
	TypePUBLISH                 // Publish message
	TypePUBACK                  // Publish acknowledgment
	TypePUBREC                  // Publish received (assured delivery part 1)
	TypePUBREL                  // Publish release (assured delivery part 2)
	TypePUBCOMP                 // Publish complete (assured delivery part 3)
	TypeSUBSCRIBE               // Client subscribe request
	TypeSUBACK                  // Subscribe acknowledgment
	TypeUNSUBSCRIBE             // Unsubscribe request
	TypeUNSUBACK                // Unsubscribe acknowledgment
	TypePINGREQ                 // PING request
	TypePINGRESP                // PING response
	TypeDISCONNECT              // Client is disconnecting
	_                           // Reserved
)

// CONNACK Return Codes indicate the result of a connection request.
const (
	CodeAccepted byte = 0 // Connection accepted
)

// ConnectPacket represents an MQTT CONNECT packet.
// It is the first packet sent by a client to the server.
type ConnectPacket struct {
	// ProtocolName is the name of the protocol, e.g., "MQTT".
	ProtocolName string
	// ProtocolVersion is the version of the MQTT protocol used by the client.
	ProtocolVersion byte
	// CleanSession indicates whether the client wants to start a new session or
	// resume an existing one.
	CleanSession bool
	// KeepAlive is the time interval in seconds within which the client must
	// send a message to the server.
	KeepAlive uint16
	// ClientID is the unique identifier of the client.
	ClientID string
}

// ConnackPacket represents an MQTT CONNACK packet.
// It is the response sent by the server to a CONNECT packet.
type ConnackPacket struct {
	// SessionPresent indicates whether the server is resuming a previous session.
	SessionPresent bool
	// ReturnCode is the result of the connection attempt.
	ReturnCode byte
}

// PublishPacket represents an MQTT PUBLISH packet.
// It is used to send a message from a client to the server or from the server
// to a client.
type PublishPacket struct {
	// TopicName is the topic to which the message is published.
	TopicName string
	// Payload is the content of the message.
	Payload []byte
}

// SubscribePacket represents an MQTT SUBSCRIBE packet.
// It is sent by a client to the server to subscribe to one or more topics.
type SubscribePacket struct {
	// MessageID is the unique identifier for the subscribe request.
	MessageID uint16
	// Topics is a list of topic filters to subscribe to.
	Topics []string
	// QoSs is a list of requested QoS levels for each topic filter.
	QoSs []byte
}

// SubackPacket represents an MQTT SUBACK packet.
// It is sent by the server in response to a SUBSCRIBE packet.
type SubackPacket struct {
	// MessageID is the identifier of the corresponding SUBSCRIBE packet.
	MessageID uint16
	// ReturnCodes is a list of return codes, one for each topic filter in the
	// SUBSCRIBE packet, indicating the result of the subscription.
	ReturnCodes []byte
}
