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

package rules

import (
	"context"
	"testing"
	"time"

	"github.com/turtacn/emqx-go/pkg/connector"
)

func TestConsoleActionExecutor_Execute(t *testing.T) {
	executor := NewConsoleActionExecutor()

	ruleCtx := &RuleContext{
		Topic:     "sensor/temperature",
		QoS:       1,
		Payload:   []byte("25.5"),
		ClientID:  "sensor1",
		Username:  "user1",
		Timestamp: time.Now(),
	}

	params := map[string]interface{}{
		"level":    "info",
		"template": "Temperature reading: ${payload} from ${clientid}",
	}

	result, err := executor.Execute(context.Background(), ruleCtx, params)
	if err != nil {
		t.Errorf("Execute() error = %v", err)
	}

	if !result.Success {
		t.Errorf("Execute() Success = %v, want %v", result.Success, true)
	}

	if result.Output == nil {
		t.Error("Execute() Output should not be nil")
	}
}

func TestConsoleActionExecutor_Validate(t *testing.T) {
	executor := NewConsoleActionExecutor()

	tests := []struct {
		name    string
		params  map[string]interface{}
		wantErr bool
	}{
		{
			name:    "valid parameters",
			params:  map[string]interface{}{"level": "info"},
			wantErr: false,
		},
		{
			name:    "invalid level",
			params:  map[string]interface{}{"level": "invalid"},
			wantErr: true,
		},
		{
			name:    "level not string",
			params:  map[string]interface{}{"level": 123},
			wantErr: true,
		},
		{
			name:    "no level (optional)",
			params:  map[string]interface{}{},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := executor.Validate(tt.params)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestConsoleActionExecutor_GetSchema(t *testing.T) {
	executor := NewConsoleActionExecutor()
	schema := executor.GetSchema()

	if schema["type"] != "object" {
		t.Errorf("GetSchema() type = %v, want %v", schema["type"], "object")
	}

	properties, ok := schema["properties"].(map[string]interface{})
	if !ok {
		t.Error("GetSchema() properties should be a map")
	}

	if _, exists := properties["level"]; !exists {
		t.Error("GetSchema() should include level property")
	}

	if _, exists := properties["template"]; !exists {
		t.Error("GetSchema() should include template property")
	}
}

func TestHTTPActionExecutor_Execute(t *testing.T) {
	executor := NewHTTPActionExecutor()

	ruleCtx := &RuleContext{
		Topic:     "sensor/temperature",
		QoS:       1,
		Payload:   []byte(`{"temperature": 25.5}`),
		ClientID:  "sensor1",
		Username:  "user1",
		Timestamp: time.Now(),
	}

	// Note: This will try to make a real HTTP request
	// In a real test environment, you might want to use a mock HTTP server
	params := map[string]interface{}{
		"url":    "http://httpbin.org/post",
		"method": "POST",
		"body":   `{"topic": "${topic}", "payload": ${payload}}`,
		"headers": map[string]interface{}{
			"Content-Type": "application/json",
			"X-Source":     "emqx-go-rules",
		},
	}

	result, err := executor.Execute(context.Background(), ruleCtx, params)
	if err != nil {
		// HTTP requests might fail in test environments, so we'll check the result structure
		t.Logf("Execute() error = %v (expected in test environment)", err)
	}

	if result == nil {
		t.Error("Execute() should return a result even on failure")
	}
}

func TestHTTPActionExecutor_Validate(t *testing.T) {
	executor := NewHTTPActionExecutor()

	tests := []struct {
		name    string
		params  map[string]interface{}
		wantErr bool
	}{
		{
			name:    "valid parameters",
			params:  map[string]interface{}{"url": "http://example.com"},
			wantErr: false,
		},
		{
			name:    "missing url",
			params:  map[string]interface{}{},
			wantErr: true,
		},
		{
			name:    "url not string",
			params:  map[string]interface{}{"url": 123},
			wantErr: true,
		},
		{
			name: "valid method",
			params: map[string]interface{}{
				"url":    "http://example.com",
				"method": "PUT",
			},
			wantErr: false,
		},
		{
			name: "invalid method",
			params: map[string]interface{}{
				"url":    "http://example.com",
				"method": "INVALID",
			},
			wantErr: true,
		},
		{
			name: "method not string",
			params: map[string]interface{}{
				"url":    "http://example.com",
				"method": 123,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := executor.Validate(tt.params)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestHTTPActionExecutor_GetSchema(t *testing.T) {
	executor := NewHTTPActionExecutor()
	schema := executor.GetSchema()

	if schema["type"] != "object" {
		t.Errorf("GetSchema() type = %v, want %v", schema["type"], "object")
	}

	_, ok := schema["properties"].(map[string]interface{})
	if !ok {
		t.Error("GetSchema() properties should be a map")
	}

	requiredFields := []string{"url"}
	required, ok := schema["required"].([]string)
	if !ok {
		t.Error("GetSchema() required should be a string slice")
	}

	for _, field := range requiredFields {
		found := false
		for _, req := range required {
			if req == field {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("GetSchema() should require field: %s", field)
		}
	}
}

func TestConnectorActionExecutor_Execute(t *testing.T) {
	connManager := connector.NewConnectorManager()
	executor := NewConnectorActionExecutor(connManager)

	ruleCtx := &RuleContext{
		Topic:     "sensor/temperature",
		QoS:       1,
		Payload:   []byte(`{"temperature": 25.5}`),
		ClientID:  "sensor1",
		Username:  "user1",
		Timestamp: time.Now(),
		Headers:   map[string]string{},
		Metadata:  map[string]interface{}{},
	}

	params := map[string]interface{}{
		"connector_id": "nonexistent-connector",
	}

	// This should fail because the connector doesn't exist
	result, err := executor.Execute(context.Background(), ruleCtx, params)
	if err == nil {
		t.Error("Execute() should return error for nonexistent connector")
	}

	if result == nil || result.Success {
		t.Error("Execute() should return unsuccessful result for nonexistent connector")
	}
}

func TestConnectorActionExecutor_Validate(t *testing.T) {
	connManager := connector.NewConnectorManager()
	executor := NewConnectorActionExecutor(connManager)

	tests := []struct {
		name    string
		params  map[string]interface{}
		wantErr bool
	}{
		{
			name:    "valid parameters",
			params:  map[string]interface{}{"connector_id": "test-connector"},
			wantErr: false,
		},
		{
			name:    "missing connector_id",
			params:  map[string]interface{}{},
			wantErr: true,
		},
		{
			name:    "connector_id not string",
			params:  map[string]interface{}{"connector_id": 123},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := executor.Validate(tt.params)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestRepublishActionExecutor_Execute(t *testing.T) {
	executor := NewRepublishActionExecutor()

	// Set up republish callback
	var republishedTopic string
	var republishedQoS int
	var republishedPayload []byte

	executor.SetRepublishCallback(func(topic string, qos int, payload []byte) error {
		republishedTopic = topic
		republishedQoS = qos
		republishedPayload = payload
		return nil
	})

	ruleCtx := &RuleContext{
		Topic:     "sensor/temperature",
		QoS:       1,
		Payload:   []byte(`{"temperature": 25.5}`),
		ClientID:  "sensor1",
		Username:  "user1",
		Timestamp: time.Now(),
	}

	params := map[string]interface{}{
		"topic":            "processed/temperature",
		"qos":              2,
		"payload_template": `{"original_topic": "${topic}", "value": ${payload}}`,
	}

	result, err := executor.Execute(context.Background(), ruleCtx, params)
	if err != nil {
		t.Errorf("Execute() error = %v", err)
	}

	if !result.Success {
		t.Errorf("Execute() Success = %v, want %v", result.Success, true)
	}

	if republishedTopic != "processed/temperature" {
		t.Errorf("Republished topic = %v, want %v", republishedTopic, "processed/temperature")
	}

	if republishedQoS != 2 {
		t.Errorf("Republished QoS = %v, want %v", republishedQoS, 2)
	}

	expectedPayload := `{"original_topic": "sensor/temperature", "value": {"temperature": 25.5}}`
	if string(republishedPayload) != expectedPayload {
		t.Errorf("Republished payload = %v, want %v", string(republishedPayload), expectedPayload)
	}
}

func TestRepublishActionExecutor_Validate(t *testing.T) {
	executor := NewRepublishActionExecutor()

	tests := []struct {
		name    string
		params  map[string]interface{}
		wantErr bool
	}{
		{
			name:    "valid parameters",
			params:  map[string]interface{}{"topic": "test/topic"},
			wantErr: false,
		},
		{
			name:    "missing topic",
			params:  map[string]interface{}{},
			wantErr: true,
		},
		{
			name:    "topic not string",
			params:  map[string]interface{}{"topic": 123},
			wantErr: true,
		},
		{
			name: "valid qos",
			params: map[string]interface{}{
				"topic": "test/topic",
				"qos":   1,
			},
			wantErr: false,
		},
		{
			name: "invalid qos range",
			params: map[string]interface{}{
				"topic": "test/topic",
				"qos":   3,
			},
			wantErr: true,
		},
		{
			name: "invalid qos type",
			params: map[string]interface{}{
				"topic": "test/topic",
				"qos":   "invalid",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := executor.Validate(tt.params)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestDiscardActionExecutor_Execute(t *testing.T) {
	executor := NewDiscardActionExecutor()

	ruleCtx := &RuleContext{
		Topic:     "sensor/temperature",
		QoS:       1,
		Payload:   []byte("25.5"),
		ClientID:  "sensor1",
		Username:  "user1",
		Timestamp: time.Now(),
	}

	params := map[string]interface{}{
		"reason": "Temperature out of range",
	}

	result, err := executor.Execute(context.Background(), ruleCtx, params)
	if err != nil {
		t.Errorf("Execute() error = %v", err)
	}

	if !result.Success {
		t.Errorf("Execute() Success = %v, want %v", result.Success, true)
	}

	output, ok := result.Output.(map[string]interface{})
	if !ok {
		t.Error("Execute() Output should be a map")
	}

	if output["action"] != "discard" {
		t.Errorf("Execute() Output action = %v, want %v", output["action"], "discard")
	}

	if output["reason"] != "Temperature out of range" {
		t.Errorf("Execute() Output reason = %v, want %v", output["reason"], "Temperature out of range")
	}
}

func TestDiscardActionExecutor_Validate(t *testing.T) {
	executor := NewDiscardActionExecutor()

	// Discard action should always validate successfully
	err := executor.Validate(map[string]interface{}{})
	if err != nil {
		t.Errorf("Validate() error = %v", err)
	}

	err = executor.Validate(map[string]interface{}{"reason": "test"})
	if err != nil {
		t.Errorf("Validate() error = %v", err)
	}
}

// Test variable replacement functionality
func TestActionExecutor_ReplaceVariables(t *testing.T) {
	executor := NewConsoleActionExecutor()

	ruleCtx := &RuleContext{
		Topic:     "sensor/temperature",
		QoS:       1,
		Payload:   []byte(`{"temperature": 25.5}`),
		ClientID:  "sensor1",
		Username:  "user1",
		Timestamp: time.Now(),
	}

	template := "Topic: ${topic}, Client: ${clientid}, User: ${username}, QoS: ${qos}, Payload: ${payload}, Time: ${timestamp}"
	result := executor.replaceVariables(template, ruleCtx)

	expectedSubstrings := []string{
		"Topic: sensor/temperature",
		"Client: sensor1",
		"User: user1",
		"QoS: 1",
		"Payload: {\"temperature\": 25.5}",
	}

	for _, expected := range expectedSubstrings {
		if !contains(result, expected) {
			t.Errorf("replaceVariables() result should contain: %s, got: %s", expected, result)
		}
	}
}

// Helper function to check if string contains substring
func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}