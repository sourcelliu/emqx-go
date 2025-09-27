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

package session

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/mochi-mqtt/server/v2/packets"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/turtacn/emqx-go/pkg/actor"
	"github.com/turtacn/emqx-go/tests/testutil"
)

func TestNew(t *testing.T) {
	conn := &bytes.Buffer{}
	s := New("test-client", conn)
	assert.NotNil(t, s)
	assert.Equal(t, "test-client", s.ID)
	assert.Equal(t, conn, s.conn)
}

func TestSession_Start(t *testing.T) {
	conn := &bytes.Buffer{}
	s := New("test-client", conn)
	mb := actor.NewMailbox(10)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		err := s.Start(ctx, mb)
		// Expect a context canceled error on clean shutdown
		assert.ErrorIs(t, err, context.Canceled)
	}()

	// Send a publish message to the session actor
	pubMsg := Publish{
		Topic:   "test/topic",
		Payload: []byte("hello"),
	}
	mb.Send(pubMsg)

	// Give the actor a moment to process the message
	time.Sleep(100 * time.Millisecond)

	// Verify that the connection received the encoded PUBLISH packet
	pk, err := testutil.ReadPacket(conn)
	require.NoError(t, err)
	require.Equal(t, packets.Publish, pk.FixedHeader.Type)
	assert.Equal(t, "test/topic", pk.TopicName)
	assert.Equal(t, []byte("hello"), pk.Payload)
}