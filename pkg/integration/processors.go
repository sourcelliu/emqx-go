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

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// JSONProcessor processes JSON data transformations
type JSONProcessor struct {
	id              string
	name            string
	transformations []JSONTransformation
	config          map[string]interface{}
}

// JSONTransformation represents a JSON transformation rule
type JSONTransformation struct {
	Type        string                 `json:"type"`         // "map", "filter", "enrich", "extract"
	SourcePath  string                 `json:"source_path"`  // JSONPath expression
	TargetPath  string                 `json:"target_path"`  // Target field path
	Value       interface{}            `json:"value"`        // Static value or template
	Condition   string                 `json:"condition"`    // Condition expression
	Operation   string                 `json:"operation"`    // "add", "multiply", "concat", etc.
	DefaultValue interface{}           `json:"default_value"`
	Template    string                 `json:"template"`     // Template with variables
	Parameters  map[string]interface{} `json:"parameters"`
}

// NewJSONProcessor creates a new JSON data processor
func NewJSONProcessor(id, name string, transformations []JSONTransformation) *JSONProcessor {
	return &JSONProcessor{
		id:              id,
		name:            name,
		transformations: transformations,
		config:          make(map[string]interface{}),
	}
}

// ID returns the processor ID
func (jp *JSONProcessor) ID() string {
	return jp.id
}

// Name returns the processor name
func (jp *JSONProcessor) Name() string {
	return jp.name
}

// Process processes a message through JSON transformations
func (jp *JSONProcessor) Process(ctx context.Context, msg *Message) (*Message, error) {
	// Parse payload as JSON
	var payload map[string]interface{}
	if err := json.Unmarshal(msg.Payload, &payload); err != nil {
		// If not JSON, wrap in a JSON object
		payload = map[string]interface{}{
			"raw_data": string(msg.Payload),
		}
	}

	// Apply transformations
	for _, transform := range jp.transformations {
		if err := jp.applyTransformation(payload, transform, msg); err != nil {
			return nil, fmt.Errorf("transformation failed: %w", err)
		}
	}

	// Marshal back to JSON
	processedPayload, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal processed payload: %w", err)
	}

	// Create processed message
	processedMsg := &Message{
		ID:              msg.ID,
		Topic:           msg.Topic,
		Payload:         processedPayload,
		QoS:             msg.QoS,
		Headers:         msg.Headers,
		Metadata:        msg.Metadata,
		Timestamp:       msg.Timestamp,
		SourceType:      msg.SourceType,
		SourceID:        msg.SourceID,
		DestinationType: msg.DestinationType,
		DestinationID:   msg.DestinationID,
	}

	// Add processing metadata
	if processedMsg.Metadata == nil {
		processedMsg.Metadata = make(map[string]interface{})
	}
	processedMsg.Metadata["processed_by"] = jp.id
	processedMsg.Metadata["processed_at"] = time.Now()

	return processedMsg, nil
}

// applyTransformation applies a single transformation
func (jp *JSONProcessor) applyTransformation(payload map[string]interface{}, transform JSONTransformation, msg *Message) error {
	switch transform.Type {
	case "map":
		return jp.applyMapTransformation(payload, transform)
	case "filter":
		return jp.applyFilterTransformation(payload, transform)
	case "enrich":
		return jp.applyEnrichTransformation(payload, transform, msg)
	case "extract":
		return jp.applyExtractTransformation(payload, transform)
	case "template":
		return jp.applyTemplateTransformation(payload, transform, msg)
	default:
		return fmt.Errorf("unknown transformation type: %s", transform.Type)
	}
}

// applyMapTransformation maps values from source to target path
func (jp *JSONProcessor) applyMapTransformation(payload map[string]interface{}, transform JSONTransformation) error {
	sourceValue := jp.getValueByPath(payload, transform.SourcePath)
	if sourceValue == nil && transform.DefaultValue != nil {
		sourceValue = transform.DefaultValue
	}

	if sourceValue != nil {
		jp.setValueByPath(payload, transform.TargetPath, sourceValue)
	}

	return nil
}

// applyFilterTransformation filters fields based on condition
func (jp *JSONProcessor) applyFilterTransformation(payload map[string]interface{}, transform JSONTransformation) error {
	if transform.Condition != "" {
		if !jp.evaluateCondition(payload, transform.Condition) {
			// Remove field if condition is false
			jp.removeFieldByPath(payload, transform.TargetPath)
		}
	}

	return nil
}

// applyEnrichTransformation adds enrichment data
func (jp *JSONProcessor) applyEnrichTransformation(payload map[string]interface{}, transform JSONTransformation, msg *Message) error {
	var enrichValue interface{}

	if transform.Value != nil {
		enrichValue = transform.Value
	} else if transform.Template != "" {
		enrichValue = jp.applyTemplate(transform.Template, payload, msg)
	} else {
		// Add timestamp, message metadata, etc.
		switch transform.TargetPath {
		case "timestamp":
			enrichValue = time.Now().Unix()
		case "iso_timestamp":
			enrichValue = time.Now().Format(time.RFC3339)
		case "topic":
			enrichValue = msg.Topic
		case "source_id":
			enrichValue = msg.SourceID
		default:
			enrichValue = transform.DefaultValue
		}
	}

	if enrichValue != nil {
		jp.setValueByPath(payload, transform.TargetPath, enrichValue)
	}

	return nil
}

// applyExtractTransformation extracts nested values
func (jp *JSONProcessor) applyExtractTransformation(payload map[string]interface{}, transform JSONTransformation) error {
	sourceValue := jp.getValueByPath(payload, transform.SourcePath)
	if sourceValue == nil {
		return nil
	}

	// If source is a JSON string, parse it
	if jsonStr, ok := sourceValue.(string); ok {
		var parsed interface{}
		if err := json.Unmarshal([]byte(jsonStr), &parsed); err == nil {
			jp.setValueByPath(payload, transform.TargetPath, parsed)
		}
	}

	return nil
}

// applyTemplateTransformation applies template-based transformation
func (jp *JSONProcessor) applyTemplateTransformation(payload map[string]interface{}, transform JSONTransformation, msg *Message) error {
	if transform.Template == "" {
		return nil
	}

	result := jp.applyTemplate(transform.Template, payload, msg)
	jp.setValueByPath(payload, transform.TargetPath, result)

	return nil
}

// getValueByPath gets a value by JSON path
func (jp *JSONProcessor) getValueByPath(data map[string]interface{}, path string) interface{} {
	if path == "" {
		return data
	}

	parts := strings.Split(path, ".")
	current := interface{}(data)

	for _, part := range parts {
		switch v := current.(type) {
		case map[string]interface{}:
			current = v[part]
		default:
			return nil
		}
	}

	return current
}

// setValueByPath sets a value by JSON path
func (jp *JSONProcessor) setValueByPath(data map[string]interface{}, path string, value interface{}) {
	if path == "" {
		return
	}

	parts := strings.Split(path, ".")
	current := data

	// Navigate to parent
	for i := 0; i < len(parts)-1; i++ {
		part := parts[i]
		if _, exists := current[part]; !exists {
			current[part] = make(map[string]interface{})
		}
		if nested, ok := current[part].(map[string]interface{}); ok {
			current = nested
		} else {
			// Can't navigate further
			return
		}
	}

	// Set final value
	current[parts[len(parts)-1]] = value
}

// removeFieldByPath removes a field by JSON path
func (jp *JSONProcessor) removeFieldByPath(data map[string]interface{}, path string) {
	if path == "" {
		return
	}

	parts := strings.Split(path, ".")
	current := data

	// Navigate to parent
	for i := 0; i < len(parts)-1; i++ {
		part := parts[i]
		if nested, ok := current[part].(map[string]interface{}); ok {
			current = nested
		} else {
			return
		}
	}

	// Remove final field
	delete(current, parts[len(parts)-1])
}

// evaluateCondition evaluates a simple condition
func (jp *JSONProcessor) evaluateCondition(payload map[string]interface{}, condition string) bool {
	// Simple condition evaluation - could be enhanced with a proper expression engine
	// For now, support basic comparisons like "field > 10", "field == 'value'"

	parts := strings.Fields(condition)
	if len(parts) != 3 {
		return true // Invalid condition, assume true
	}

	fieldPath := parts[0]
	operator := parts[1]
	expectedValue := parts[2]

	actualValue := jp.getValueByPath(payload, fieldPath)
	if actualValue == nil {
		return false
	}

	return jp.compareValues(actualValue, operator, expectedValue)
}

// compareValues compares two values based on operator
func (jp *JSONProcessor) compareValues(actual interface{}, operator, expected string) bool {
	switch operator {
	case "==", "=":
		return fmt.Sprintf("%v", actual) == strings.Trim(expected, "'\"")
	case "!=":
		return fmt.Sprintf("%v", actual) != strings.Trim(expected, "'\"")
	case ">":
		return jp.compareNumeric(actual, expected, func(a, b float64) bool { return a > b })
	case "<":
		return jp.compareNumeric(actual, expected, func(a, b float64) bool { return a < b })
	case ">=":
		return jp.compareNumeric(actual, expected, func(a, b float64) bool { return a >= b })
	case "<=":
		return jp.compareNumeric(actual, expected, func(a, b float64) bool { return a <= b })
	case "contains":
		actualStr := fmt.Sprintf("%v", actual)
		expectedStr := strings.Trim(expected, "'\"")
		return strings.Contains(actualStr, expectedStr)
	case "matches":
		actualStr := fmt.Sprintf("%v", actual)
		pattern := strings.Trim(expected, "'\"")
		matched, _ := regexp.MatchString(pattern, actualStr)
		return matched
	default:
		return false
	}
}

// compareNumeric compares numeric values
func (jp *JSONProcessor) compareNumeric(actual interface{}, expected string, compare func(float64, float64) bool) bool {
	actualFloat, err1 := jp.toFloat64(actual)
	expectedFloat, err2 := strconv.ParseFloat(expected, 64)

	if err1 != nil || err2 != nil {
		return false
	}

	return compare(actualFloat, expectedFloat)
}

// toFloat64 converts interface{} to float64
func (jp *JSONProcessor) toFloat64(v interface{}) (float64, error) {
	switch val := v.(type) {
	case float64:
		return val, nil
	case float32:
		return float64(val), nil
	case int:
		return float64(val), nil
	case int32:
		return float64(val), nil
	case int64:
		return float64(val), nil
	case string:
		return strconv.ParseFloat(val, 64)
	default:
		return 0, fmt.Errorf("cannot convert %T to float64", v)
	}
}

// applyTemplate applies a template with variable substitution
func (jp *JSONProcessor) applyTemplate(template string, payload map[string]interface{}, msg *Message) string {
	result := template

	// Replace payload variables
	for key, value := range payload {
		placeholder := fmt.Sprintf("${%s}", key)
		result = strings.ReplaceAll(result, placeholder, fmt.Sprintf("%v", value))
	}

	// Replace message variables
	result = strings.ReplaceAll(result, "${topic}", msg.Topic)
	result = strings.ReplaceAll(result, "${source_id}", msg.SourceID)
	result = strings.ReplaceAll(result, "${timestamp}", msg.Timestamp.Format(time.RFC3339))
	result = strings.ReplaceAll(result, "${unix_timestamp}", fmt.Sprintf("%d", msg.Timestamp.Unix()))

	// Replace header variables
	for key, value := range msg.Headers {
		placeholder := fmt.Sprintf("${headers.%s}", key)
		result = strings.ReplaceAll(result, placeholder, value)
	}

	// Replace metadata variables
	for key, value := range msg.Metadata {
		placeholder := fmt.Sprintf("${metadata.%s}", key)
		result = strings.ReplaceAll(result, placeholder, fmt.Sprintf("%v", value))
	}

	return result
}

// GetConfiguration returns processor configuration
func (jp *JSONProcessor) GetConfiguration() map[string]interface{} {
	return map[string]interface{}{
		"id":              jp.id,
		"name":            jp.name,
		"transformations": jp.transformations,
		"config":          jp.config,
	}
}

// SetConfiguration updates processor configuration
func (jp *JSONProcessor) SetConfiguration(config map[string]interface{}) error {
	if id, ok := config["id"].(string); ok {
		jp.id = id
	}

	if name, ok := config["name"].(string); ok {
		jp.name = name
	}

	if transformsInterface, ok := config["transformations"]; ok {
		transformsBytes, err := json.Marshal(transformsInterface)
		if err != nil {
			return fmt.Errorf("failed to marshal transformations: %w", err)
		}

		var transformations []JSONTransformation
		if err := json.Unmarshal(transformsBytes, &transformations); err != nil {
			return fmt.Errorf("failed to unmarshal transformations: %w", err)
		}

		jp.transformations = transformations
	}

	if configMap, ok := config["config"].(map[string]interface{}); ok {
		jp.config = configMap
	}

	return nil
}

// PlainTextProcessor processes plain text messages
type PlainTextProcessor struct {
	id     string
	name   string
	config map[string]interface{}
}

// NewPlainTextProcessor creates a new plain text processor
func NewPlainTextProcessor(id, name string) *PlainTextProcessor {
	return &PlainTextProcessor{
		id:     id,
		name:   name,
		config: make(map[string]interface{}),
	}
}

// ID returns the processor ID
func (ptp *PlainTextProcessor) ID() string {
	return ptp.id
}

// Name returns the processor name
func (ptp *PlainTextProcessor) Name() string {
	return ptp.name
}

// Process processes a plain text message
func (ptp *PlainTextProcessor) Process(ctx context.Context, msg *Message) (*Message, error) {
	// Simple plain text processing - convert to JSON
	processedPayload, err := json.Marshal(map[string]interface{}{
		"raw_text":  string(msg.Payload),
		"length":    len(msg.Payload),
		"topic":     msg.Topic,
		"timestamp": msg.Timestamp.Format(time.RFC3339),
	})

	if err != nil {
		return nil, fmt.Errorf("failed to process plain text: %w", err)
	}

	processedMsg := &Message{
		ID:              msg.ID,
		Topic:           msg.Topic,
		Payload:         processedPayload,
		QoS:             msg.QoS,
		Headers:         msg.Headers,
		Metadata:        msg.Metadata,
		Timestamp:       msg.Timestamp,
		SourceType:      msg.SourceType,
		SourceID:        msg.SourceID,
		DestinationType: msg.DestinationType,
		DestinationID:   msg.DestinationID,
	}

	// Add processing metadata
	if processedMsg.Metadata == nil {
		processedMsg.Metadata = make(map[string]interface{})
	}
	processedMsg.Metadata["processed_by"] = ptp.id
	processedMsg.Metadata["processed_at"] = time.Now()

	return processedMsg, nil
}

// GetConfiguration returns processor configuration
func (ptp *PlainTextProcessor) GetConfiguration() map[string]interface{} {
	return map[string]interface{}{
		"id":     ptp.id,
		"name":   ptp.name,
		"config": ptp.config,
	}
}

// SetConfiguration updates processor configuration
func (ptp *PlainTextProcessor) SetConfiguration(config map[string]interface{}) error {
	if id, ok := config["id"].(string); ok {
		ptp.id = id
	}

	if name, ok := config["name"].(string); ok {
		ptp.name = name
	}

	if configMap, ok := config["config"].(map[string]interface{}); ok {
		ptp.config = configMap
	}

	return nil
}