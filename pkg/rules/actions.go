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
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/turtacn/emqx-go/pkg/connector"
)

// ConsoleActionExecutor logs messages to console
type ConsoleActionExecutor struct{}

func NewConsoleActionExecutor() *ConsoleActionExecutor {
	return &ConsoleActionExecutor{}
}

func (e *ConsoleActionExecutor) Execute(ctx context.Context, ruleCtx *RuleContext, params map[string]interface{}) (*ActionResult, error) {
	start := time.Now()

	// Get log level (default: info)
	level := "info"
	if l, ok := params["level"].(string); ok {
		level = l
	}

	// Get message template
	template := "${topic}: ${payload}"
	if t, ok := params["template"].(string); ok {
		template = t
	}

	// Replace variables in template
	message := e.replaceVariables(template, ruleCtx)

	// Log message (in a real implementation, this would use a proper logger)
	logMessage := fmt.Sprintf("[%s] %s", strings.ToUpper(level), message)
	fmt.Println(logMessage)

	return &ActionResult{
		Success:   true,
		Latency:   time.Since(start),
		Timestamp: time.Now(),
		Output:    logMessage,
	}, nil
}

func (e *ConsoleActionExecutor) Validate(params map[string]interface{}) error {
	// Optional validation for level
	if level, ok := params["level"]; ok {
		if levelStr, ok := level.(string); ok {
			validLevels := []string{"debug", "info", "warn", "error"}
			for _, valid := range validLevels {
				if levelStr == valid {
					return nil
				}
			}
			return fmt.Errorf("invalid log level: %s", levelStr)
		}
		return fmt.Errorf("level must be a string")
	}
	return nil
}

func (e *ConsoleActionExecutor) GetSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"level": map[string]interface{}{
				"type":        "string",
				"description": "Log level",
				"enum":        []string{"debug", "info", "warn", "error"},
				"default":     "info",
			},
			"template": map[string]interface{}{
				"type":        "string",
				"description": "Message template with variables like ${topic}, ${payload}",
				"default":     "${topic}: ${payload}",
			},
		},
	}
}

func (e *ConsoleActionExecutor) replaceVariables(template string, ctx *RuleContext) string {
	result := template
	result = strings.ReplaceAll(result, "${topic}", ctx.Topic)
	result = strings.ReplaceAll(result, "${payload}", string(ctx.Payload))
	result = strings.ReplaceAll(result, "${clientid}", ctx.ClientID)
	result = strings.ReplaceAll(result, "${username}", ctx.Username)
	result = strings.ReplaceAll(result, "${qos}", fmt.Sprintf("%d", ctx.QoS))
	result = strings.ReplaceAll(result, "${timestamp}", ctx.Timestamp.Format(time.RFC3339))
	return result
}

// HTTPActionExecutor sends HTTP requests
type HTTPActionExecutor struct {
	client *http.Client
}

func NewHTTPActionExecutor() *HTTPActionExecutor {
	return &HTTPActionExecutor{
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (e *HTTPActionExecutor) Execute(ctx context.Context, ruleCtx *RuleContext, params map[string]interface{}) (*ActionResult, error) {
	start := time.Now()

	// Get required parameters
	url, ok := params["url"].(string)
	if !ok {
		return &ActionResult{
			Success:   false,
			Error:     "url parameter is required",
			Latency:   time.Since(start),
			Timestamp: time.Now(),
		}, fmt.Errorf("url parameter is required")
	}

	method := "POST"
	if m, ok := params["method"].(string); ok {
		method = strings.ToUpper(m)
	}

	// Prepare request body
	var requestBody io.Reader
	bodyTemplate := "${payload}"
	if b, ok := params["body"].(string); ok {
		bodyTemplate = b
	}

	body := e.replaceVariables(bodyTemplate, ruleCtx)
	requestBody = strings.NewReader(body)

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, method, url, requestBody)
	if err != nil {
		return &ActionResult{
			Success:   false,
			Error:     fmt.Sprintf("failed to create request: %v", err),
			Latency:   time.Since(start),
			Timestamp: time.Now(),
		}, err
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	if headers, ok := params["headers"].(map[string]interface{}); ok {
		for key, value := range headers {
			if valueStr, ok := value.(string); ok {
				req.Header.Set(key, e.replaceVariables(valueStr, ruleCtx))
			}
		}
	}

	// Send request
	resp, err := e.client.Do(req)
	if err != nil {
		return &ActionResult{
			Success:   false,
			Error:     fmt.Sprintf("HTTP request failed: %v", err),
			Latency:   time.Since(start),
			Timestamp: time.Now(),
		}, err
	}
	defer resp.Body.Close()

	// Read response
	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 400 {
		return &ActionResult{
			Success:   false,
			Error:     fmt.Sprintf("HTTP %d: %s", resp.StatusCode, string(respBody)),
			Latency:   time.Since(start),
			Timestamp: time.Now(),
		}, fmt.Errorf("HTTP request failed with status %d", resp.StatusCode)
	}

	return &ActionResult{
		Success:   true,
		Latency:   time.Since(start),
		Timestamp: time.Now(),
		Output: map[string]interface{}{
			"status_code": resp.StatusCode,
			"response":    string(respBody),
		},
	}, nil
}

func (e *HTTPActionExecutor) Validate(params map[string]interface{}) error {
	// URL is required
	if _, ok := params["url"].(string); !ok {
		return fmt.Errorf("url parameter is required")
	}

	// Validate method if provided
	if method, ok := params["method"]; ok {
		if methodStr, ok := method.(string); ok {
			validMethods := []string{"GET", "POST", "PUT", "DELETE", "PATCH"}
			methodUpper := strings.ToUpper(methodStr)
			for _, valid := range validMethods {
				if methodUpper == valid {
					return nil
				}
			}
			return fmt.Errorf("invalid HTTP method: %s", methodStr)
		}
		return fmt.Errorf("method must be a string")
	}

	return nil
}

func (e *HTTPActionExecutor) GetSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"url": map[string]interface{}{
				"type":        "string",
				"description": "HTTP endpoint URL",
				"format":      "uri",
			},
			"method": map[string]interface{}{
				"type":        "string",
				"description": "HTTP method",
				"enum":        []string{"GET", "POST", "PUT", "DELETE", "PATCH"},
				"default":     "POST",
			},
			"headers": map[string]interface{}{
				"type":        "object",
				"description": "HTTP headers",
				"additionalProperties": map[string]interface{}{
					"type": "string",
				},
			},
			"body": map[string]interface{}{
				"type":        "string",
				"description": "Request body template",
				"default":     "${payload}",
			},
		},
		"required": []string{"url"},
	}
}

func (e *HTTPActionExecutor) replaceVariables(template string, ctx *RuleContext) string {
	result := template
	result = strings.ReplaceAll(result, "${topic}", ctx.Topic)
	result = strings.ReplaceAll(result, "${payload}", string(ctx.Payload))
	result = strings.ReplaceAll(result, "${clientid}", ctx.ClientID)
	result = strings.ReplaceAll(result, "${username}", ctx.Username)
	result = strings.ReplaceAll(result, "${qos}", fmt.Sprintf("%d", ctx.QoS))
	result = strings.ReplaceAll(result, "${timestamp}", ctx.Timestamp.Format(time.RFC3339))
	return result
}

// ConnectorActionExecutor sends messages to connectors
type ConnectorActionExecutor struct {
	connManager *connector.ConnectorManager
}

func NewConnectorActionExecutor(connManager *connector.ConnectorManager) *ConnectorActionExecutor {
	return &ConnectorActionExecutor{
		connManager: connManager,
	}
}

func (e *ConnectorActionExecutor) Execute(ctx context.Context, ruleCtx *RuleContext, params map[string]interface{}) (*ActionResult, error) {
	start := time.Now()

	// Get connector ID
	connectorID, ok := params["connector_id"].(string)
	if !ok {
		return &ActionResult{
			Success:   false,
			Error:     "connector_id parameter is required",
			Latency:   time.Since(start),
			Timestamp: time.Now(),
		}, fmt.Errorf("connector_id parameter is required")
	}

	// Get connector
	conn, err := e.connManager.GetConnector(connectorID)
	if err != nil {
		return &ActionResult{
			Success:   false,
			Error:     fmt.Sprintf("connector not found: %v", err),
			Latency:   time.Since(start),
			Timestamp: time.Now(),
		}, err
	}

	// Prepare message
	message := &connector.Message{
		ID:        fmt.Sprintf("rule-%d", time.Now().UnixNano()),
		Topic:     ruleCtx.Topic,
		Payload:   ruleCtx.Payload,
		Headers:   ruleCtx.Headers,
		Timestamp: ruleCtx.Timestamp,
		Metadata:  ruleCtx.Metadata,
	}

	// Override topic if specified
	if topic, ok := params["topic"].(string); ok {
		message.Topic = e.replaceVariables(topic, ruleCtx)
	}

	// Transform payload if template specified
	if payloadTemplate, ok := params["payload_template"].(string); ok {
		transformedPayload := e.replaceVariables(payloadTemplate, ruleCtx)
		message.Payload = []byte(transformedPayload)
	}

	// Send message
	result, err := conn.Send(ctx, message)
	if err != nil {
		return &ActionResult{
			Success:   false,
			Error:     fmt.Sprintf("failed to send to connector: %v", err),
			Latency:   time.Since(start),
			Timestamp: time.Now(),
		}, err
	}

	if !result.Success {
		return &ActionResult{
			Success:   false,
			Error:     result.Error,
			Latency:   time.Since(start),
			Timestamp: time.Now(),
		}, fmt.Errorf("connector send failed: %s", result.Error)
	}

	return &ActionResult{
		Success:   true,
		Latency:   time.Since(start),
		Timestamp: time.Now(),
		Output:    result,
	}, nil
}

func (e *ConnectorActionExecutor) Validate(params map[string]interface{}) error {
	// Connector ID is required
	if _, ok := params["connector_id"].(string); !ok {
		return fmt.Errorf("connector_id parameter is required")
	}
	return nil
}

func (e *ConnectorActionExecutor) GetSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"connector_id": map[string]interface{}{
				"type":        "string",
				"description": "ID of the connector to send message to",
			},
			"topic": map[string]interface{}{
				"type":        "string",
				"description": "Override topic (optional)",
			},
			"payload_template": map[string]interface{}{
				"type":        "string",
				"description": "Payload transformation template (optional)",
			},
		},
		"required": []string{"connector_id"},
	}
}

func (e *ConnectorActionExecutor) replaceVariables(template string, ctx *RuleContext) string {
	result := template
	result = strings.ReplaceAll(result, "${topic}", ctx.Topic)
	result = strings.ReplaceAll(result, "${payload}", string(ctx.Payload))
	result = strings.ReplaceAll(result, "${clientid}", ctx.ClientID)
	result = strings.ReplaceAll(result, "${username}", ctx.Username)
	result = strings.ReplaceAll(result, "${qos}", fmt.Sprintf("%d", ctx.QoS))
	result = strings.ReplaceAll(result, "${timestamp}", ctx.Timestamp.Format(time.RFC3339))
	return result
}

// RepublishActionExecutor republishes messages to different topics
type RepublishActionExecutor struct {
	republishCallback func(topic string, qos int, payload []byte) error
	mu                sync.RWMutex
}

func NewRepublishActionExecutor() *RepublishActionExecutor {
	return &RepublishActionExecutor{}
}

func (e *RepublishActionExecutor) SetRepublishCallback(callback func(topic string, qos int, payload []byte) error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.republishCallback = callback
}

func (e *RepublishActionExecutor) Execute(ctx context.Context, ruleCtx *RuleContext, params map[string]interface{}) (*ActionResult, error) {
	start := time.Now()

	// Get target topic
	topic, ok := params["topic"].(string)
	if !ok {
		return &ActionResult{
			Success:   false,
			Error:     "topic parameter is required",
			Latency:   time.Since(start),
			Timestamp: time.Now(),
		}, fmt.Errorf("topic parameter is required")
	}

	// Replace variables in topic
	targetTopic := e.replaceVariables(topic, ruleCtx)

	// Get QoS (default: original QoS)
	qos := ruleCtx.QoS
	if q, ok := params["qos"].(float64); ok {
		qos = int(q)
	} else if q, ok := params["qos"].(int); ok {
		qos = q
	}

	// Get payload (can be transformed)
	payload := ruleCtx.Payload
	if payloadTemplate, ok := params["payload_template"].(string); ok {
		transformedPayload := e.replaceVariables(payloadTemplate, ruleCtx)
		payload = []byte(transformedPayload)
	}

	// Republish using callback
	e.mu.RLock()
	callback := e.republishCallback
	e.mu.RUnlock()

	if callback != nil {
		err := callback(targetTopic, qos, payload)
		if err != nil {
			return &ActionResult{
				Success:   false,
				Error:     fmt.Sprintf("republish failed: %v", err),
				Latency:   time.Since(start),
				Timestamp: time.Now(),
			}, err
		}
	}

	return &ActionResult{
		Success:   true,
		Latency:   time.Since(start),
		Timestamp: time.Now(),
		Output: map[string]interface{}{
			"target_topic": targetTopic,
			"qos":          qos,
			"payload_size": len(payload),
		},
	}, nil
}

func (e *RepublishActionExecutor) Validate(params map[string]interface{}) error {
	// Topic is required
	if _, ok := params["topic"].(string); !ok {
		return fmt.Errorf("topic parameter is required")
	}

	// Validate QoS if provided
	if qos, ok := params["qos"]; ok {
		var qosInt int
		switch v := qos.(type) {
		case int:
			qosInt = v
		case float64:
			qosInt = int(v)
		default:
			return fmt.Errorf("qos must be an integer")
		}
		if qosInt < 0 || qosInt > 2 {
			return fmt.Errorf("qos must be 0, 1, or 2")
		}
	}

	return nil
}

func (e *RepublishActionExecutor) GetSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"topic": map[string]interface{}{
				"type":        "string",
				"description": "Target topic for republishing",
			},
			"qos": map[string]interface{}{
				"type":        "integer",
				"description": "QoS level for republished message",
				"enum":        []int{0, 1, 2},
				"default":     0,
			},
			"payload_template": map[string]interface{}{
				"type":        "string",
				"description": "Payload transformation template (optional)",
			},
		},
		"required": []string{"topic"},
	}
}

func (e *RepublishActionExecutor) replaceVariables(template string, ctx *RuleContext) string {
	result := template
	result = strings.ReplaceAll(result, "${topic}", ctx.Topic)
	result = strings.ReplaceAll(result, "${payload}", string(ctx.Payload))
	result = strings.ReplaceAll(result, "${clientid}", ctx.ClientID)
	result = strings.ReplaceAll(result, "${username}", ctx.Username)
	result = strings.ReplaceAll(result, "${qos}", fmt.Sprintf("%d", ctx.QoS))
	result = strings.ReplaceAll(result, "${timestamp}", ctx.Timestamp.Format(time.RFC3339))
	return result
}

// DiscardActionExecutor discards messages (stops processing)
type DiscardActionExecutor struct{}

func NewDiscardActionExecutor() *DiscardActionExecutor {
	return &DiscardActionExecutor{}
}

func (e *DiscardActionExecutor) Execute(ctx context.Context, ruleCtx *RuleContext, params map[string]interface{}) (*ActionResult, error) {
	start := time.Now()

	reason := "Message discarded by rule"
	if r, ok := params["reason"].(string); ok {
		reason = r
	}

	return &ActionResult{
		Success:   true,
		Latency:   time.Since(start),
		Timestamp: time.Now(),
		Output: map[string]interface{}{
			"action": "discard",
			"reason": reason,
		},
	}, nil
}

func (e *DiscardActionExecutor) Validate(params map[string]interface{}) error {
	// No validation needed for discard action
	return nil
}

func (e *DiscardActionExecutor) GetSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"reason": map[string]interface{}{
				"type":        "string",
				"description": "Reason for discarding the message",
				"default":     "Message discarded by rule",
			},
		},
	}
}