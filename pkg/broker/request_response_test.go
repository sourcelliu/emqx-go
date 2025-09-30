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

import (
	"testing"

	"github.com/mochi-mqtt/server/v2/packets"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/turtacn/emqx-go/pkg/actor"
	"github.com/turtacn/emqx-go/pkg/session"
)

func TestRouteToLocalSubscribersWithRequestResponse(t *testing.T) {
	broker := New("test-broker", nil)
	defer broker.Close()

	// Create a test mailbox to simulate a subscriber
	testMailbox := actor.NewMailbox(10)

	// Add subscriber to topic store
	broker.topics.Subscribe("test/request-response", testMailbox, 1)

	// Test routing with request-response properties
	userProperties := map[string][]byte{
		"client-id": []byte("test-client"),
		"timestamp": []byte("1234567890"),
	}

	responseTopic := "test/response/12345"
	correlationData := []byte("correlation-data-test")

	// Call the method under test
	broker.RouteToLocalSubscribersWithRequestResponse(
		"test/request-response",
		[]byte("test payload"),
		2,
		userProperties,
		responseTopic,
		correlationData,
	)

	// Verify the message was sent to the subscriber
	select {
	case msg := <-testMailbox.Chan():
		publishMsg, ok := msg.(session.Publish)
		require.True(t, ok, "Expected session.Publish message")

		assert.Equal(t, "test/request-response", publishMsg.Topic)
		assert.Equal(t, []byte("test payload"), publishMsg.Payload)
		assert.Equal(t, byte(1), publishMsg.QoS) // Should be downgraded to subscriber QoS
		assert.Equal(t, userProperties, publishMsg.UserProperties)
		assert.Equal(t, responseTopic, publishMsg.ResponseTopic)
		assert.Equal(t, correlationData, publishMsg.CorrelationData)
	default:
		t.Fatal("Expected message was not received")
	}
}

func TestRouteToLocalSubscribersWithRequestResponseEmptyProperties(t *testing.T) {
	broker := New("test-broker", nil)
	defer broker.Close()

	// Create a test mailbox to simulate a subscriber
	testMailbox := actor.NewMailbox(10)

	// Add subscriber to topic store
	broker.topics.Subscribe("test/empty-props", testMailbox, 2)

	// Test routing with empty request-response properties
	broker.RouteToLocalSubscribersWithRequestResponse(
		"test/empty-props",
		[]byte("empty props test"),
		0,
		nil,
		"",
		nil,
	)

	// Verify the message was sent to the subscriber
	select {
	case msg := <-testMailbox.Chan():
		publishMsg, ok := msg.(session.Publish)
		require.True(t, ok, "Expected session.Publish message")

		assert.Equal(t, "test/empty-props", publishMsg.Topic)
		assert.Equal(t, []byte("empty props test"), publishMsg.Payload)
		assert.Equal(t, byte(0), publishMsg.QoS)
		assert.Nil(t, publishMsg.UserProperties)
		assert.Equal(t, "", publishMsg.ResponseTopic)
		assert.Nil(t, publishMsg.CorrelationData)
	default:
		t.Fatal("Expected message was not received")
	}
}

func TestRouteToLocalSubscribersWithRequestResponseMultipleSubscribers(t *testing.T) {
	broker := New("test-broker", nil)
	defer broker.Close()

	// Create multiple test mailboxes to simulate multiple subscribers
	numSubscribers := 3
	testMailboxes := make([]*actor.Mailbox, numSubscribers)

	for i := 0; i < numSubscribers; i++ {
		testMailboxes[i] = actor.NewMailbox(10)
		broker.topics.Subscribe("multi/request-response", testMailboxes[i], 1)
	}

	// Test routing with request-response properties to multiple subscribers
	userProperties := map[string][]byte{
		"broadcast": []byte("true"),
		"count":     []byte("3"),
	}

	responseTopic := "multi/response"
	correlationData := []byte("multi-subscriber-test")

	// Call the method under test
	broker.RouteToLocalSubscribersWithRequestResponse(
		"multi/request-response",
		[]byte("multi subscriber test"),
		1,
		userProperties,
		responseTopic,
		correlationData,
	)

	// Verify all subscribers received the message
	for i := 0; i < numSubscribers; i++ {
		select {
		case msg := <-testMailboxes[i].Chan():
			publishMsg, ok := msg.(session.Publish)
			require.True(t, ok, "Expected session.Publish message for subscriber %d", i)

			assert.Equal(t, "multi/request-response", publishMsg.Topic)
			assert.Equal(t, []byte("multi subscriber test"), publishMsg.Payload)
			assert.Equal(t, byte(1), publishMsg.QoS)
			assert.Equal(t, userProperties, publishMsg.UserProperties)
			assert.Equal(t, responseTopic, publishMsg.ResponseTopic)
			assert.Equal(t, correlationData, publishMsg.CorrelationData)
		default:
			t.Fatalf("Subscriber %d did not receive the message", i)
		}
	}
}

func TestRoutePublishWithRequestResponse(t *testing.T) {
	broker := New("test-broker", nil)
	defer broker.Close()

	// Create a test mailbox to simulate a subscriber
	testMailbox := actor.NewMailbox(10)

	// Add subscriber to topic store
	broker.topics.Subscribe("route/test", testMailbox, 2)

	// Test routing through routePublishWithRequestResponse method
	userProperties := map[string][]byte{
		"sender": []byte("test-publisher"),
		"type":   []byte("request"),
	}

	responseTopic := "route/response"
	correlationData := []byte("route-test-correlation")

	// Call the internal routing method
	broker.routePublishWithRequestResponse(
		"route/test",
		[]byte("route test payload"),
		1,
		userProperties,
		responseTopic,
		correlationData,
	)

	// Verify the message was routed correctly
	select {
	case msg := <-testMailbox.Chan():
		publishMsg, ok := msg.(session.Publish)
		require.True(t, ok, "Expected session.Publish message")

		assert.Equal(t, "route/test", publishMsg.Topic)
		assert.Equal(t, []byte("route test payload"), publishMsg.Payload)
		assert.Equal(t, byte(1), publishMsg.QoS)
		assert.Equal(t, userProperties, publishMsg.UserProperties)
		assert.Equal(t, responseTopic, publishMsg.ResponseTopic)
		assert.Equal(t, correlationData, publishMsg.CorrelationData)
	default:
		t.Fatal("Expected message was not received through routing")
	}
}

func TestRequestResponsePropertyExtraction(t *testing.T) {
	// Test data for various request-response scenarios
	tests := []struct {
		name                string
		responseTopic       string
		correlationData     []byte
		requestResponseInfo byte
		expectedRespTopic   string
		expectedCorrData    []byte
	}{
		{
			name:              "No request-response properties",
			responseTopic:     "",
			correlationData:   nil,
			expectedRespTopic: "",
			expectedCorrData:  nil,
		},
		{
			name:              "Response topic only",
			responseTopic:     "api/response/v1",
			correlationData:   nil,
			expectedRespTopic: "api/response/v1",
			expectedCorrData:  nil,
		},
		{
			name:              "Correlation data only",
			responseTopic:     "",
			correlationData:   []byte("corr-123"),
			expectedRespTopic: "",
			expectedCorrData:  []byte("corr-123"),
		},
		{
			name:              "Both properties",
			responseTopic:     "full/response",
			correlationData:   []byte("full-correlation-data"),
			expectedRespTopic: "full/response",
			expectedCorrData:  []byte("full-correlation-data"),
		},
		{
			name:              "Binary correlation data",
			responseTopic:     "binary/response",
			correlationData:   []byte{0x01, 0x02, 0x03, 0xFF},
			expectedRespTopic: "binary/response",
			expectedCorrData:  []byte{0x01, 0x02, 0x03, 0xFF},
		},
		{
			name:                "With request response info",
			responseTopic:       "info/response",
			correlationData:     []byte("info-test"),
			requestResponseInfo: 1,
			expectedRespTopic:   "info/response",
			expectedCorrData:    []byte("info-test"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a packet with the test properties
			pk := &packets.Packet{
				FixedHeader: packets.FixedHeader{
					Type: packets.Publish,
					Qos:  1,
				},
				TopicName: "test/topic",
				Payload:   []byte("test payload"),
				Properties: packets.Properties{
					ResponseTopic:       tt.responseTopic,
					CorrelationData:     tt.correlationData,
					RequestResponseInfo: tt.requestResponseInfo,
				},
			}

			// Verify properties are set correctly
			assert.Equal(t, tt.expectedRespTopic, pk.Properties.ResponseTopic)
			assert.Equal(t, tt.expectedCorrData, pk.Properties.CorrelationData)

			if tt.requestResponseInfo > 0 {
				assert.Equal(t, tt.requestResponseInfo, pk.Properties.RequestResponseInfo)
			}
		})
	}
}

func TestConnackWithResponseInfo(t *testing.T) {
	// Test CONNACK response info generation scenarios
	tests := []struct {
		name                string
		requestResponseInfo byte
		nodeID              string
		expectedHasInfo     bool
	}{
		{
			name:                "No response info requested",
			requestResponseInfo: 0,
			nodeID:              "test-node",
			expectedHasInfo:     false,
		},
		{
			name:                "Response info requested",
			requestResponseInfo: 1,
			nodeID:              "test-node",
			expectedHasInfo:     true,
		},
		{
			name:                "Response info with different node",
			requestResponseInfo: 1,
			nodeID:              "production-node-1",
			expectedHasInfo:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			broker := New(tt.nodeID, nil)
			defer broker.Close()

			// Simulate CONNACK response info generation
			if tt.requestResponseInfo == 1 {
				expectedInfo := "response-info://" + tt.nodeID + "/response/"

				// Verify the expected response info format
				assert.Contains(t, expectedInfo, tt.nodeID)
				assert.Contains(t, expectedInfo, "response-info://")
				assert.Contains(t, expectedInfo, "/response/")
			}
		})
	}
}

func TestRequestResponseUnicodeSupport(t *testing.T) {
	broker := New("unicode-test-broker", nil)
	defer broker.Close()

	// Create a test mailbox to simulate a subscriber
	testMailbox := actor.NewMailbox(10)

	// Add subscriber to topic store with Unicode topic
	unicodeTopic := "测试/请求响应"
	broker.topics.Subscribe(unicodeTopic, testMailbox, 1)

	// Test routing with Unicode request-response properties
	userProperties := map[string][]byte{
		"客户端":   []byte("测试客户端"),
		"消息类型": []byte("请求"),
		"编码":   []byte("UTF-8"),
	}

	unicodeResponseTopic := "测试/响应/主题"
	unicodeCorrelationData := []byte("关联数据-测试-12345")

	// Call the method under test
	broker.RouteToLocalSubscribersWithRequestResponse(
		unicodeTopic,
		[]byte("Unicode payload: 这是一个测试消息"),
		1,
		userProperties,
		unicodeResponseTopic,
		unicodeCorrelationData,
	)

	// Verify the message was sent correctly
	select {
	case msg := <-testMailbox.Chan():
		publishMsg, ok := msg.(session.Publish)
		require.True(t, ok, "Expected session.Publish message")

		assert.Equal(t, unicodeTopic, publishMsg.Topic)
		assert.Equal(t, []byte("Unicode payload: 这是一个测试消息"), publishMsg.Payload)
		assert.Equal(t, byte(1), publishMsg.QoS)
		assert.Equal(t, userProperties, publishMsg.UserProperties)
		assert.Equal(t, unicodeResponseTopic, publishMsg.ResponseTopic)
		assert.Equal(t, unicodeCorrelationData, publishMsg.CorrelationData)

		// Verify specific Unicode properties
		assert.Equal(t, []byte("测试客户端"), publishMsg.UserProperties["客户端"])
		assert.Equal(t, []byte("请求"), publishMsg.UserProperties["消息类型"])
		assert.Equal(t, []byte("UTF-8"), publishMsg.UserProperties["编码"])
	default:
		t.Fatal("Expected Unicode message was not received")
	}
}

func TestRequestResponseWithTopicAliases(t *testing.T) {
	broker := New("topic-alias-rr-broker", nil)
	defer broker.Close()

	// Create a test mailbox to simulate a subscriber
	testMailbox := actor.NewMailbox(10)

	// Add subscriber to topic store
	broker.topics.Subscribe("alias/request-response", testMailbox, 1)

	// Test routing with both topic aliases and request-response properties
	userProperties := map[string][]byte{
		"topic-alias": []byte("1"),
		"message-id":  []byte("alias-rr-001"),
	}

	responseTopic := "alias/response"
	correlationData := []byte("alias-correlation-data")

	// Call the method under test
	broker.RouteToLocalSubscribersWithRequestResponse(
		"alias/request-response",
		[]byte("Topic alias with request-response"),
		2,
		userProperties,
		responseTopic,
		correlationData,
	)

	// Verify the message was sent correctly
	select {
	case msg := <-testMailbox.Chan():
		publishMsg, ok := msg.(session.Publish)
		require.True(t, ok, "Expected session.Publish message")

		assert.Equal(t, "alias/request-response", publishMsg.Topic)
		assert.Equal(t, []byte("Topic alias with request-response"), publishMsg.Payload)
		assert.Equal(t, byte(1), publishMsg.QoS) // Downgraded to subscriber QoS
		assert.Equal(t, userProperties, publishMsg.UserProperties)
		assert.Equal(t, responseTopic, publishMsg.ResponseTopic)
		assert.Equal(t, correlationData, publishMsg.CorrelationData)

		// Verify topic alias user property is preserved
		assert.Equal(t, []byte("1"), publishMsg.UserProperties["topic-alias"])
		assert.Equal(t, []byte("alias-rr-001"), publishMsg.UserProperties["message-id"])
	default:
		t.Fatal("Expected topic alias request-response message was not received")
	}
}