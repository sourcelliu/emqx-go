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

package broker

import "github.com/asynkron/protoactor-go/actor"

// Subscribe is an internal message used to request a subscription to a topic.
// It is typically sent from a client-handling process to the central broker or
// topic manager.
type Subscribe struct {
	// Topic is the topic filter to which the client wants to subscribe.
	Topic string
	// Sender is the Process ID (PID) of the actor that should receive
	// messages published to this topic.
	Sender *actor.PID
}

// Publish is an internal message representing an MQTT PUBLISH packet. It is
// used to send a message to the broker for routing to subscribers.
type Publish struct {
	// Topic is the topic to which the message is being published.
	Topic string
	// Payload is the content of the message.
	Payload []byte
}

// ForwardedPublish is an internal message used to deliver a published message
// to a subscribed client. It is sent from the broker to the actor managing the
// client's session.
type ForwardedPublish struct {
	// Topic is the topic on which the message was published.
	Topic string
	// Payload is the content of the message to be delivered.
	Payload []byte
}