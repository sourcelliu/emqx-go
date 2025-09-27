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

// Package mqtt provides low-level parsing and encoding of MQTT control packets.
// It is designed to work directly with network I/O streams and focuses on
// correctness and adherence to the MQTT v3.1.1 specification.
package mqtt

// Control Packet Types are defined in section 2.2.1 of the MQTT v3.1.1
// specification. The first 4 bits of the fixed header define the packet type.
const (
	_               byte = iota // 0: Reserved
	TypeCONNECT                 // 1: Client request to connect to Server
	TypeCONNACK                 // 2: Connect acknowledgment
	TypePUBLISH                 // 3: Publish message
	TypePUBACK                  // 4: Publish acknowledgment
	TypePUBREC                  // 5: Publish received (assured delivery part 1)
	TypePUBREL                  // 6: Publish release (assured delivery part 2)
	TypePUBCOMP                 // 7: Publish complete (assured delivery part 3)
	TypeSUBSCRIBE               // 8: Client subscribe request
	TypeSUBACK                  // 9: Subscribe acknowledgment
	TypeUNSUBSCRIBE             // 10: Unsubscribe request
	TypeUNSUBACK                // 11: Unsubscribe acknowledgment
	TypePINGREQ                 // 12: PING request
	TypePINGRESP                // 13: PING response
	TypeDISCONNECT              // 14: Client is disconnecting
	_                           // 15: Reserved
)

// CONNACK Return Codes, defined in section 3.2.2.3 of the MQTT v3.1.1 spec,
// indicate the result of a connection request.
const (
	// CodeAccepted means the connection was accepted by the server.
	CodeAccepted byte = 0
)

// ConnectPacket represents an MQTT CONNECT packet. This is the first packet
// sent by a client after establishing a network connection with the server.
type ConnectPacket struct {
	// ProtocolName is the name of the protocol, which must be "MQTT".
	ProtocolName string
	// ProtocolVersion is the version of the MQTT protocol. For MQTT v3.1.1,
	// this value is 4.
	ProtocolVersion byte
	// CleanSession is a flag that indicates whether the client and server should
	// discard any previous session information upon connection.
	CleanSession bool
	// KeepAlive is the maximum time interval in seconds that is allowed to
	// elapse between messages sent by the client. It enables the server to
	// detect that the client is no longer connected.
	KeepAlive uint16
	// ClientID is the unique identifier that the client provides to the server.
	// It must be unique per server.
	ClientID string
}

// ConnackPacket represents an MQTT CONNACK packet. This is the response sent
// by the server to a client's CONNECT request.
type ConnackPacket struct {
	// SessionPresent is a flag that informs the client whether the server is
	// resuming a previous session. This is only relevant for clients that
	// connect with CleanSession set to false.
	SessionPresent bool
	// ReturnCode indicates the outcome of the connection attempt. A value of 0
	// (CodeAccepted) means success.
	ReturnCode byte
}

// PublishPacket represents an MQTT PUBLISH packet. It is used to transport an
// application message from a publisher to the broker, and from the broker to
// subscribers.
type PublishPacket struct {
	// TopicName is the topic to which the message is being published.
	TopicName string
	// Payload is the actual content of the message. It is a byte slice and can
	// contain any binary data.
	Payload []byte
}

// SubscribePacket represents an MQTT SUBSCRIBE packet. It is sent by a client
// to the server to create one or more subscriptions.
type SubscribePacket struct {
	// MessageID is a unique identifier for the subscription request. The server
	// will include this ID in the corresponding SUBACK packet.
	MessageID uint16
	// Topics is a list of topic filters that the client wants to subscribe to.
	Topics []string
	// QoSs is a list of requested Quality of Service levels, corresponding to
	// each topic filter in the Topics list.
	QoSs []byte
}

// SubackPacket represents an MQTT SUBACK packet. It is sent by the server in
// response to a SUBSCRIBE packet to confirm the receipt and processing of the
// request.
type SubackPacket struct {
	// MessageID is the identifier from the corresponding SUBSCRIBE packet, used
	// to correlate the acknowledgment.
	MessageID uint16
	// ReturnCodes is a list of return codes, one for each topic filter from the
	// SUBSCRIBE request. Each code indicates whether the subscription was
	// accepted and at what QoS level, or if it was rejected.
	ReturnCodes []byte
}
