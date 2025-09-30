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

package topic

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/turtacn/emqx-go/pkg/actor"
)

func TestStore(t *testing.T) {
	s := NewStore()
	assert.NotNil(t, s)

	mb1 := actor.NewMailbox(10)
	mb2 := actor.NewMailbox(10)

	// Test Subscribe
	s.Subscribe("test/topic", mb1, 1)
	s.Subscribe("test/topic", mb2, 2)

	subs := s.GetSubscribers("test/topic")
	assert.Len(t, subs, 2)

	// Check that we have the correct mailboxes and QoS levels
	var foundMB1, foundMB2 bool
	for _, sub := range subs {
		if sub.Mailbox == mb1 {
			foundMB1 = true
			assert.Equal(t, byte(1), sub.QoS)
		} else if sub.Mailbox == mb2 {
			foundMB2 = true
			assert.Equal(t, byte(2), sub.QoS)
		}
	}
	assert.True(t, foundMB1)
	assert.True(t, foundMB2)

	// Test GetSubscribers for unknown topic
	subs = s.GetSubscribers("unknown/topic")
	assert.Len(t, subs, 0)

	// Test Unsubscribe
	s.Unsubscribe("test/topic", mb1)
	subs = s.GetSubscribers("test/topic")
	assert.Len(t, subs, 1)
	assert.Equal(t, mb2, subs[0].Mailbox)

	// Test Unsubscribe last subscriber
	s.Unsubscribe("test/topic", mb2)
	subs = s.GetSubscribers("test/topic")
	assert.Len(t, subs, 0)
	_, ok := s.subscriptions["test/topic"]
	assert.False(t, ok)

	// Test Unsubscribe from non-existent topic
	s.Unsubscribe("non/existent", mb1)
}