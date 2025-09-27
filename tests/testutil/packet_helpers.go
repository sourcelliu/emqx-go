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

package testutil

import (
	"bufio"
	"bytes"
	"fmt"
	"io"

	"github.com/mochi-mqtt/server/v2/packets"
)

// ReadPacket reads a full MQTT packet from a connection.
func ReadPacket(r io.Reader) (*packets.Packet, error) {
	reader := bufio.NewReader(r)
	fh := new(packets.FixedHeader)
	b, err := reader.ReadByte()
	if err != nil {
		return nil, err
	}
	err = fh.Decode(b)
	if err != nil {
		return nil, err
	}
	rem, _, err := packets.DecodeLength(reader)
	if err != nil {
		return nil, err
	}
	fh.Remaining = rem

	buf := make([]byte, fh.Remaining)
	if fh.Remaining > 0 {
		_, err = io.ReadFull(reader, buf)
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
	case packets.Suback:
		err = pk.SubackDecode(buf)
	case packets.Pingreq:
		err = pk.PingreqDecode(buf)
	case packets.Disconnect:
		err = pk.DisconnectDecode(buf)
	case packets.Connack:
		err = pk.ConnackDecode(buf)
	}
	if err != nil {
		return nil, err
	}

	return pk, nil
}

// WritePacket encodes and writes a packet to a connection.
func WritePacket(w io.Writer, pk *packets.Packet) error {
	var buf bytes.Buffer
	var err error
	switch pk.FixedHeader.Type {
	case packets.Connack:
		err = pk.ConnackEncode(&buf)
	case packets.Suback:
		err = pk.SubackEncode(&buf)
	case packets.Pingresp:
		err = pk.PingrespEncode(&buf)
	case packets.Publish:
		err = pk.PublishEncode(&buf)
	case packets.Connect:
		err = pk.ConnectEncode(&buf)
	case packets.Subscribe:
		err = pk.SubscribeEncode(&buf)
	case packets.Disconnect:
		err = pk.DisconnectEncode(&buf)
	default:
		return fmt.Errorf("unsupported packet type for writing: %v", pk.FixedHeader.Type)
	}

	if err != nil {
		return err
	}
	_, err = w.Write(buf.Bytes())
	return err
}