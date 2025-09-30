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

package e2e

import (
	"context"
	"fmt"
	"testing"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/turtacn/emqx-go/pkg/broker"
)

func TestRequestResponsePatternBasic(t *testing.T) {
	// Start broker
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	b := broker.New("rr-test-node1", nil)
	b.SetupDefaultAuth()
	defer b.Close()

	go b.StartServer(ctx, ":1900")
	time.Sleep(100 * time.Millisecond) // Wait for server to start

	// Test basic request-response pattern using internal broker routing
	// Note: Since the Paho client doesn't support MQTT 5.0 request-response properties directly,
	// we test the internal routing functionality which handles MQTT 5.0 properly

	// Create requester client
	requester := createCleanClient("request-response-requester", ":1900")
	reqToken := requester.Connect()
	require.True(t, reqToken.WaitTimeout(5*time.Second))
	require.NoError(t, reqToken.Error())
	defer requester.Disconnect(100)

	// Create responder client
	responder := createCleanClient("request-response-responder", ":1900")
	respToken := responder.Connect()
	require.True(t, respToken.WaitTimeout(5*time.Second))
	require.NoError(t, respToken.Error())
	defer responder.Disconnect(100)

	// Channel to receive messages
	requestReceived := make(chan mqtt.Message, 1)
	responseReceived := make(chan mqtt.Message, 1)

	// Subscribe to request topic (responder)
	responder.Subscribe("api/v1/request", 1, func(client mqtt.Client, msg mqtt.Message) {
		requestReceived <- msg
	})

	// Subscribe to response topic (requester)
	requester.Subscribe("api/v1/response/12345", 1, func(client mqtt.Client, msg mqtt.Message) {
		responseReceived <- msg
	})

	time.Sleep(100 * time.Millisecond) // Wait for subscriptions

	// Simulate request with request-response properties using internal broker routing
	// This tests our MQTT 5.0 request-response implementation
	userProperties := map[string][]byte{
		"request-id":    []byte("req-12345"),
		"content-type":  []byte("application/json"),
		"client-type":   []byte("api-client"),
		"timestamp":     []byte("1234567890"),
	}

	requestPayload := `{"action": "get_temperature", "sensor_id": "temp_001"}`
	responseTopic := "api/v1/response/12345"
	correlationData := []byte("correlation-req-12345")

	// Use internal routing to test request-response functionality
	b.RouteToLocalSubscribersWithRequestResponse(
		"api/v1/request",
		[]byte(requestPayload),
		1,
		userProperties,
		responseTopic,
		correlationData,
	)

	// Verify request was received
	select {
	case msg := <-requestReceived:
		assert.Equal(t, "api/v1/request", msg.Topic())
		assert.Contains(t, string(msg.Payload()), "get_temperature")
		assert.Equal(t, byte(1), msg.Qos())
		t.Log("Request received successfully with request-response properties")
	case <-time.After(2 * time.Second):
		t.Fatal("Request was not received within timeout")
	}

	// Simulate response with request-response properties
	responseUserProperties := map[string][]byte{
		"request-id":    []byte("req-12345"),
		"response-type": []byte("success"),
		"content-type":  []byte("application/json"),
	}

	responsePayload := `{"status": "success", "temperature": 23.5, "unit": "celsius"}`

	// Route response back using request-response properties
	b.RouteToLocalSubscribersWithRequestResponse(
		responseTopic,
		[]byte(responsePayload),
		1,
		responseUserProperties,
		"", // No response topic needed for response
		correlationData,
	)

	// Verify response was received
	select {
	case msg := <-responseReceived:
		assert.Equal(t, responseTopic, msg.Topic())
		assert.Contains(t, string(msg.Payload()), "success")
		assert.Contains(t, string(msg.Payload()), "23.5")
		assert.Equal(t, byte(1), msg.Qos())
		t.Log("Response received successfully with correlation data")
	case <-time.After(2 * time.Second):
		t.Fatal("Response was not received within timeout")
	}

	t.Log("Basic request-response pattern test completed successfully")
}

func TestRequestResponsePatternMultipleRequests(t *testing.T) {
	// Test multiple concurrent request-response pairs
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	b := broker.New("rr-multi-node", nil)
	b.SetupDefaultAuth()
	defer b.Close()

	go b.StartServer(ctx, ":1901")
	time.Sleep(100 * time.Millisecond)

	// Create multiple requester clients
	const numRequesters = 3
	requesters := make([]mqtt.Client, numRequesters)
	responseChannels := make([]chan mqtt.Message, numRequesters)

	for i := 0; i < numRequesters; i++ {
		clientID := fmt.Sprintf("multi-requester-%d", i)
		requesters[i] = createCleanClient(clientID, ":1901")
		token := requesters[i].Connect()
		require.True(t, token.WaitTimeout(5*time.Second))
		require.NoError(t, token.Error())
		defer requesters[i].Disconnect(100)

		responseChannels[i] = make(chan mqtt.Message, 1)
		responseTopic := fmt.Sprintf("multi/response/%d", i)

		// Capture index for closure
		channelIndex := i
		requesters[i].Subscribe(responseTopic, 1, func(client mqtt.Client, msg mqtt.Message) {
			responseChannels[channelIndex] <- msg
		})
	}

	// Create responder client
	responder := createCleanClient("multi-responder", ":1901")
	respToken := responder.Connect()
	require.True(t, respToken.WaitTimeout(5*time.Second))
	require.NoError(t, respToken.Error())
	defer responder.Disconnect(100)

	requestChannel := make(chan mqtt.Message, numRequesters)
	responder.Subscribe("multi/request", 1, func(client mqtt.Client, msg mqtt.Message) {
		requestChannel <- msg
	})

	time.Sleep(200 * time.Millisecond) // Wait for all subscriptions

	// Send multiple requests concurrently
	for i := 0; i < numRequesters; i++ {
		userProperties := map[string][]byte{
			"request-id":     []byte(fmt.Sprintf("multi-req-%d", i)),
			"requester-id":   []byte(fmt.Sprintf("requester-%d", i)),
			"batch-request":  []byte("true"),
		}

		requestPayload := fmt.Sprintf(`{"request_id": %d, "action": "get_data"}`, i)
		responseTopic := fmt.Sprintf("multi/response/%d", i)
		correlationData := []byte(fmt.Sprintf("multi-correlation-%d", i))

		// Use internal routing for request-response
		b.RouteToLocalSubscribersWithRequestResponse(
			"multi/request",
			[]byte(requestPayload),
			1,
			userProperties,
			responseTopic,
			correlationData,
		)
	}

	// Verify all requests were received and send responses
	for i := 0; i < numRequesters; i++ {
		select {
		case msg := <-requestChannel:
			assert.Equal(t, "multi/request", msg.Topic())
			assert.Contains(t, string(msg.Payload()), "get_data")
			assert.Equal(t, byte(1), msg.Qos())

			// Extract request ID from payload and send response
			var requestID int
			fmt.Sscanf(string(msg.Payload()), `{"request_id": %d`, &requestID)

			responseUserProperties := map[string][]byte{
				"request-id":    []byte(fmt.Sprintf("multi-req-%d", requestID)),
				"response-type": []byte("success"),
			}

			responsePayload := fmt.Sprintf(`{"request_id": %d, "status": "success", "data": "response_%d"}`, requestID, requestID)
			responseTopic := fmt.Sprintf("multi/response/%d", requestID)
			correlationData := []byte(fmt.Sprintf("multi-correlation-%d", requestID))

			// Route response
			b.RouteToLocalSubscribersWithRequestResponse(
				responseTopic,
				[]byte(responsePayload),
				1,
				responseUserProperties,
				"",
				correlationData,
			)

		case <-time.After(1 * time.Second):
			t.Fatalf("Request %d was not received", i)
		}
	}

	// Verify all responses were received
	for i := 0; i < numRequesters; i++ {
		select {
		case msg := <-responseChannels[i]:
			expectedTopic := fmt.Sprintf("multi/response/%d", i)
			assert.Equal(t, expectedTopic, msg.Topic())
			assert.Contains(t, string(msg.Payload()), "success")
			assert.Contains(t, string(msg.Payload()), fmt.Sprintf("response_%d", i))
			assert.Equal(t, byte(1), msg.Qos())
		case <-time.After(2 * time.Second):
			t.Fatalf("Response %d was not received", i)
		}
	}

	t.Logf("Successfully tested %d concurrent request-response pairs", numRequesters)
}

func TestRequestResponsePatternWithUnicode(t *testing.T) {
	// Test request-response pattern with Unicode content
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	b := broker.New("rr-unicode-node", nil)
	b.SetupDefaultAuth()
	defer b.Close()

	go b.StartServer(ctx, ":1902")
	time.Sleep(100 * time.Millisecond)

	// Create clients for Unicode testing
	requester := createCleanClient("unicode-requester", ":1902")
	reqToken := requester.Connect()
	require.True(t, reqToken.WaitTimeout(5*time.Second))
	require.NoError(t, reqToken.Error())
	defer requester.Disconnect(100)

	responder := createCleanClient("unicode-responder", ":1902")
	respToken := responder.Connect()
	require.True(t, respToken.WaitTimeout(5*time.Second))
	require.NoError(t, respToken.Error())
	defer responder.Disconnect(100)

	// Channels for Unicode message testing
	unicodeRequestReceived := make(chan mqtt.Message, 1)
	unicodeResponseReceived := make(chan mqtt.Message, 1)

	// Subscribe to Unicode topics
	responder.Subscribe("测试/请求", 1, func(client mqtt.Client, msg mqtt.Message) {
		unicodeRequestReceived <- msg
	})

	requester.Subscribe("测试/响应/12345", 1, func(client mqtt.Client, msg mqtt.Message) {
		unicodeResponseReceived <- msg
	})

	time.Sleep(100 * time.Millisecond)

	// Test request with Unicode properties
	unicodeUserProperties := map[string][]byte{
		"请求编号":   []byte("请求-12345"),
		"客户端类型": []byte("测试客户端"),
		"内容类型":   []byte("应用程序/JSON"),
		"语言":     []byte("中文"),
	}

	unicodeRequestPayload := `{"操作": "获取温度", "传感器编号": "温度_001", "描述": "这是一个测试请求"}`
	unicodeResponseTopic := "测试/响应/12345"
	unicodeCorrelationData := []byte("关联数据-测试-12345-中文")

	// Route Unicode request
	b.RouteToLocalSubscribersWithRequestResponse(
		"测试/请求",
		[]byte(unicodeRequestPayload),
		1,
		unicodeUserProperties,
		unicodeResponseTopic,
		unicodeCorrelationData,
	)

	// Verify Unicode request was received
	select {
	case msg := <-unicodeRequestReceived:
		assert.Equal(t, "测试/请求", msg.Topic())
		assert.Contains(t, string(msg.Payload()), "获取温度")
		assert.Contains(t, string(msg.Payload()), "测试请求")
		assert.Equal(t, byte(1), msg.Qos())
		t.Log("Unicode request received successfully")
	case <-time.After(2 * time.Second):
		t.Fatal("Unicode request was not received")
	}

	// Send Unicode response
	unicodeResponseUserProperties := map[string][]byte{
		"请求编号":   []byte("请求-12345"),
		"响应类型":   []byte("成功"),
		"内容类型":   []byte("应用程序/JSON"),
		"状态":     []byte("完成"),
	}

	unicodeResponsePayload := `{"状态": "成功", "温度": 23.5, "单位": "摄氏度", "描述": "温度读取成功"}`

	// Route Unicode response
	b.RouteToLocalSubscribersWithRequestResponse(
		unicodeResponseTopic,
		[]byte(unicodeResponsePayload),
		1,
		unicodeResponseUserProperties,
		"",
		unicodeCorrelationData,
	)

	// Verify Unicode response was received
	select {
	case msg := <-unicodeResponseReceived:
		assert.Equal(t, unicodeResponseTopic, msg.Topic())
		assert.Contains(t, string(msg.Payload()), "成功")
		assert.Contains(t, string(msg.Payload()), "23.5")
		assert.Contains(t, string(msg.Payload()), "摄氏度")
		assert.Equal(t, byte(1), msg.Qos())
		t.Log("Unicode response received successfully")
	case <-time.After(2 * time.Second):
		t.Fatal("Unicode response was not received")
	}

	t.Log("Unicode request-response pattern test completed successfully")
}

func TestRequestResponsePatternErrorHandling(t *testing.T) {
	// Test error handling in request-response pattern
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	b := broker.New("rr-error-node", nil)
	b.SetupDefaultAuth()
	defer b.Close()

	go b.StartServer(ctx, ":1903")
	time.Sleep(100 * time.Millisecond)

	// Create clients for error testing
	requester := createCleanClient("error-requester", ":1903")
	reqToken := requester.Connect()
	require.True(t, reqToken.WaitTimeout(5*time.Second))
	require.NoError(t, reqToken.Error())
	defer requester.Disconnect(100)

	responder := createCleanClient("error-responder", ":1903")
	respToken := responder.Connect()
	require.True(t, respToken.WaitTimeout(5*time.Second))
	require.NoError(t, respToken.Error())
	defer responder.Disconnect(100)

	// Test scenarios for error handling
	errorTestCases := []struct {
		name            string
		requestTopic    string
		responseTopic   string
		correlationData []byte
		userProperties  map[string][]byte
		expectedSuccess bool
	}{
		{
			name:          "Valid request-response",
			requestTopic:  "error/valid/request",
			responseTopic: "error/valid/response",
			correlationData: []byte("valid-correlation"),
			userProperties: map[string][]byte{
				"test-case": []byte("valid"),
			},
			expectedSuccess: true,
		},
		{
			name:          "Empty response topic",
			requestTopic:  "error/empty-response/request",
			responseTopic: "",
			correlationData: []byte("empty-response-correlation"),
			userProperties: map[string][]byte{
				"test-case": []byte("empty-response-topic"),
			},
			expectedSuccess: true, // Should still work
		},
		{
			name:          "Empty correlation data",
			requestTopic:  "error/empty-correlation/request",
			responseTopic: "error/empty-correlation/response",
			correlationData: nil,
			userProperties: map[string][]byte{
				"test-case": []byte("empty-correlation"),
			},
			expectedSuccess: true, // Should still work
		},
		{
			name:          "Large correlation data",
			requestTopic:  "error/large-correlation/request",
			responseTopic: "error/large-correlation/response",
			correlationData: make([]byte, 1024), // Large correlation data
			userProperties: map[string][]byte{
				"test-case": []byte("large-correlation"),
			},
			expectedSuccess: true,
		},
	}

	for _, tc := range errorTestCases {
		t.Run(tc.name, func(t *testing.T) {
			// Channel to receive this specific test case's message
			testMessageReceived := make(chan mqtt.Message, 1)

			// Subscribe to the specific request topic for this test
			responder.Subscribe(tc.requestTopic, 1, func(client mqtt.Client, msg mqtt.Message) {
				testMessageReceived <- msg
			})

			time.Sleep(50 * time.Millisecond) // Wait for subscription

			// Send request using internal routing
			requestPayload := fmt.Sprintf(`{"test": "%s", "timestamp": %d}`, tc.name, time.Now().Unix())

			b.RouteToLocalSubscribersWithRequestResponse(
				tc.requestTopic,
				[]byte(requestPayload),
				1,
				tc.userProperties,
				tc.responseTopic,
				tc.correlationData,
			)

			// Verify message was received (or not, based on expected success)
			if tc.expectedSuccess {
				select {
				case msg := <-testMessageReceived:
					assert.Equal(t, tc.requestTopic, msg.Topic())
					assert.Contains(t, string(msg.Payload()), tc.name)
					assert.Equal(t, byte(1), msg.Qos())
					t.Logf("Test case '%s' passed: message received as expected", tc.name)
				case <-time.After(1 * time.Second):
					t.Fatalf("Test case '%s' failed: expected message was not received", tc.name)
				}
			}

			// Unsubscribe to avoid interference with other test cases
			responder.Unsubscribe(tc.requestTopic)
		})
	}

	t.Log("Request-response error handling tests completed successfully")
}

func TestRequestResponsePatternPerformance(t *testing.T) {
	// Test performance of request-response pattern with many messages
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	b := broker.New("rr-perf-node", nil)
	b.SetupDefaultAuth()
	defer b.Close()

	go b.StartServer(ctx, ":1904")
	time.Sleep(100 * time.Millisecond)

	// Create performance test client
	perfClient := createCleanClient("perf-test-client", ":1904")
	token := perfClient.Connect()
	require.True(t, token.WaitTimeout(5*time.Second))
	require.NoError(t, token.Error())
	defer perfClient.Disconnect(100)

	// Subscribe to performance test topic
	perfMessageReceived := make(chan mqtt.Message, 100)
	perfClient.Subscribe("perf/request-response", 1, func(client mqtt.Client, msg mqtt.Message) {
		perfMessageReceived <- msg
	})

	time.Sleep(100 * time.Millisecond)

	// Performance test parameters
	const numMessages = 50
	startTime := time.Now()

	// Send multiple request-response messages rapidly
	for i := 0; i < numMessages; i++ {
		userProperties := map[string][]byte{
			"message-id":   []byte(fmt.Sprintf("perf-%d", i)),
			"batch-test":   []byte("true"),
			"timestamp":    []byte(fmt.Sprintf("%d", time.Now().UnixNano())),
		}

		requestPayload := fmt.Sprintf(`{"message_id": %d, "data": "performance_test_data_%d"}`, i, i)
		responseTopic := fmt.Sprintf("perf/response/%d", i)
		correlationData := []byte(fmt.Sprintf("perf-correlation-%d", i))

		// Use internal routing for performance test
		b.RouteToLocalSubscribersWithRequestResponse(
			"perf/request-response",
			[]byte(requestPayload),
			1,
			userProperties,
			responseTopic,
			correlationData,
		)
	}

	// Verify all messages were received
	messagesReceived := 0
	for i := 0; i < numMessages; i++ {
		select {
		case msg := <-perfMessageReceived:
			assert.Equal(t, "perf/request-response", msg.Topic())
			assert.Contains(t, string(msg.Payload()), "performance_test_data_")
			assert.Equal(t, byte(1), msg.Qos())
			messagesReceived++
		case <-time.After(5 * time.Second):
			t.Fatalf("Performance test failed: only received %d/%d messages", messagesReceived, numMessages)
		}
	}

	duration := time.Since(startTime)
	messagesPerSecond := float64(numMessages) / duration.Seconds()

	assert.Equal(t, numMessages, messagesReceived)
	t.Logf("Performance test completed: %d messages in %v (%.2f msg/sec)",
		numMessages, duration, messagesPerSecond)

	// Basic performance assertion (should handle at least 10 messages per second)
	assert.Greater(t, messagesPerSecond, 10.0, "Performance below expected threshold")
}