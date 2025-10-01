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

func TestRuleEngine_CreateRule(t *testing.T) {
	connManager := connector.NewConnectorManager()
	engine := NewRuleEngine(connManager)

	rule := Rule{
		ID:          "test-rule",
		Name:        "Test Rule",
		Description: "A test rule",
		SQL:         "topic = 'test/topic'",
		Actions: []Action{
			{
				Type: ActionTypeConsole,
				Parameters: map[string]interface{}{
					"level": "info",
				},
			},
		},
		Status:   RuleStatusEnabled,
		Priority: 1,
	}

	err := engine.CreateRule(rule)
	if err != nil {
		t.Errorf("CreateRule() error = %v", err)
	}

	// Test duplicate rule creation
	err = engine.CreateRule(rule)
	if err == nil {
		t.Error("CreateRule() should return error for duplicate rule ID")
	}
}

func TestRuleEngine_CreateRule_InvalidSQL(t *testing.T) {
	connManager := connector.NewConnectorManager()
	engine := NewRuleEngine(connManager)

	rule := Rule{
		ID:     "invalid-sql-rule",
		Name:   "Invalid SQL Rule",
		SQL:    "INVALID SQL SYNTAX",
		Status: RuleStatusEnabled,
	}

	err := engine.CreateRule(rule)
	if err == nil {
		t.Error("CreateRule() should return error for invalid SQL")
	}
}

func TestRuleEngine_CreateRule_InvalidAction(t *testing.T) {
	connManager := connector.NewConnectorManager()
	engine := NewRuleEngine(connManager)

	rule := Rule{
		ID:   "invalid-action-rule",
		Name: "Invalid Action Rule",
		SQL:  "topic = 'test'",
		Actions: []Action{
			{
				Type: "invalid_action_type",
			},
		},
		Status: RuleStatusEnabled,
	}

	err := engine.CreateRule(rule)
	if err == nil {
		t.Error("CreateRule() should return error for invalid action type")
	}
}

func TestRuleEngine_GetRule(t *testing.T) {
	connManager := connector.NewConnectorManager()
	engine := NewRuleEngine(connManager)

	rule := Rule{
		ID:     "test-rule",
		Name:   "Test Rule",
		SQL:    "topic = 'test/topic'",
		Status: RuleStatusEnabled,
	}

	err := engine.CreateRule(rule)
	if err != nil {
		t.Fatalf("CreateRule() error = %v", err)
	}

	retrievedRule, err := engine.GetRule("test-rule")
	if err != nil {
		t.Errorf("GetRule() error = %v", err)
	}

	if retrievedRule.ID != rule.ID {
		t.Errorf("GetRule() ID = %v, want %v", retrievedRule.ID, rule.ID)
	}
	if retrievedRule.Name != rule.Name {
		t.Errorf("GetRule() Name = %v, want %v", retrievedRule.Name, rule.Name)
	}
}

func TestRuleEngine_GetRule_NotFound(t *testing.T) {
	connManager := connector.NewConnectorManager()
	engine := NewRuleEngine(connManager)

	_, err := engine.GetRule("nonexistent-rule")
	if err == nil {
		t.Error("GetRule() should return error for nonexistent rule")
	}
}

func TestRuleEngine_UpdateRule(t *testing.T) {
	connManager := connector.NewConnectorManager()
	engine := NewRuleEngine(connManager)

	originalRule := Rule{
		ID:     "test-rule",
		Name:   "Original Rule",
		SQL:    "topic = 'test/topic'",
		Status: RuleStatusEnabled,
	}

	err := engine.CreateRule(originalRule)
	if err != nil {
		t.Fatalf("CreateRule() error = %v", err)
	}

	updatedRule := Rule{
		ID:     "test-rule",
		Name:   "Updated Rule",
		SQL:    "topic LIKE 'test/%'",
		Status: RuleStatusEnabled,
	}

	err = engine.UpdateRule("test-rule", updatedRule)
	if err != nil {
		t.Errorf("UpdateRule() error = %v", err)
	}

	retrievedRule, err := engine.GetRule("test-rule")
	if err != nil {
		t.Fatalf("GetRule() error = %v", err)
	}

	if retrievedRule.Name != updatedRule.Name {
		t.Errorf("UpdateRule() Name = %v, want %v", retrievedRule.Name, updatedRule.Name)
	}
	if retrievedRule.SQL != updatedRule.SQL {
		t.Errorf("UpdateRule() SQL = %v, want %v", retrievedRule.SQL, updatedRule.SQL)
	}
}

func TestRuleEngine_UpdateRule_NotFound(t *testing.T) {
	connManager := connector.NewConnectorManager()
	engine := NewRuleEngine(connManager)

	rule := Rule{
		ID:   "nonexistent-rule",
		Name: "Test Rule",
		SQL:  "topic = 'test'",
	}

	err := engine.UpdateRule("nonexistent-rule", rule)
	if err == nil {
		t.Error("UpdateRule() should return error for nonexistent rule")
	}
}

func TestRuleEngine_DeleteRule(t *testing.T) {
	connManager := connector.NewConnectorManager()
	engine := NewRuleEngine(connManager)

	rule := Rule{
		ID:     "test-rule",
		Name:   "Test Rule",
		SQL:    "topic = 'test/topic'",
		Status: RuleStatusEnabled,
	}

	err := engine.CreateRule(rule)
	if err != nil {
		t.Fatalf("CreateRule() error = %v", err)
	}

	err = engine.DeleteRule("test-rule")
	if err != nil {
		t.Errorf("DeleteRule() error = %v", err)
	}

	_, err = engine.GetRule("test-rule")
	if err == nil {
		t.Error("GetRule() should return error after rule deletion")
	}
}

func TestRuleEngine_DeleteRule_NotFound(t *testing.T) {
	connManager := connector.NewConnectorManager()
	engine := NewRuleEngine(connManager)

	err := engine.DeleteRule("nonexistent-rule")
	if err == nil {
		t.Error("DeleteRule() should return error for nonexistent rule")
	}
}

func TestRuleEngine_EnableDisableRule(t *testing.T) {
	connManager := connector.NewConnectorManager()
	engine := NewRuleEngine(connManager)

	rule := Rule{
		ID:     "test-rule",
		Name:   "Test Rule",
		SQL:    "topic = 'test/topic'",
		Status: RuleStatusDisabled,
	}

	err := engine.CreateRule(rule)
	if err != nil {
		t.Fatalf("CreateRule() error = %v", err)
	}

	// Enable rule
	err = engine.EnableRule("test-rule")
	if err != nil {
		t.Errorf("EnableRule() error = %v", err)
	}

	retrievedRule, _ := engine.GetRule("test-rule")
	if retrievedRule.Status != RuleStatusEnabled {
		t.Errorf("EnableRule() Status = %v, want %v", retrievedRule.Status, RuleStatusEnabled)
	}

	// Disable rule
	err = engine.DisableRule("test-rule")
	if err != nil {
		t.Errorf("DisableRule() error = %v", err)
	}

	retrievedRule, _ = engine.GetRule("test-rule")
	if retrievedRule.Status != RuleStatusDisabled {
		t.Errorf("DisableRule() Status = %v, want %v", retrievedRule.Status, RuleStatusDisabled)
	}
}

func TestRuleEngine_ListRules(t *testing.T) {
	connManager := connector.NewConnectorManager()
	engine := NewRuleEngine(connManager)

	rules := []Rule{
		{
			ID:     "rule1",
			Name:   "Rule 1",
			SQL:    "topic = 'test1'",
			Status: RuleStatusEnabled,
		},
		{
			ID:     "rule2",
			Name:   "Rule 2",
			SQL:    "topic = 'test2'",
			Status: RuleStatusDisabled,
		},
	}

	for _, rule := range rules {
		err := engine.CreateRule(rule)
		if err != nil {
			t.Fatalf("CreateRule() error = %v", err)
		}
	}

	listedRules := engine.ListRules()
	if len(listedRules) != len(rules) {
		t.Errorf("ListRules() count = %v, want %v", len(listedRules), len(rules))
	}
}

func TestRuleEngine_ProcessMessage(t *testing.T) {
	connManager := connector.NewConnectorManager()
	engine := NewRuleEngine(connManager)

	// Create a test rule
	rule := Rule{
		ID:   "test-rule",
		Name: "Test Rule",
		SQL:  "topic = 'sensor/temperature'",
		Actions: []Action{
			{
				Type: ActionTypeConsole,
				Parameters: map[string]interface{}{
					"level":    "info",
					"template": "Temperature reading: ${payload}",
				},
			},
		},
		Status:   RuleStatusEnabled,
		Priority: 1,
	}

	err := engine.CreateRule(rule)
	if err != nil {
		t.Fatalf("CreateRule() error = %v", err)
	}

	// Process a matching message
	ruleCtx := &RuleContext{
		Topic:     "sensor/temperature",
		QoS:       1,
		Payload:   []byte("25.5"),
		Headers:   map[string]string{},
		ClientID:  "sensor1",
		Username:  "user1",
		Timestamp: time.Now(),
		Metadata:  map[string]interface{}{},
	}

	ctx := context.Background()
	err = engine.ProcessMessage(ctx, ruleCtx)
	if err != nil {
		t.Errorf("ProcessMessage() error = %v", err)
	}

	// Check that metrics were updated
	metrics, err := engine.GetRuleMetrics("test-rule")
	if err != nil {
		t.Errorf("GetRuleMetrics() error = %v", err)
	}

	if metrics.MatchedCount != 1 {
		t.Errorf("ProcessMessage() MatchedCount = %v, want %v", metrics.MatchedCount, 1)
	}
	if metrics.ExecutedCount != 1 {
		t.Errorf("ProcessMessage() ExecutedCount = %v, want %v", metrics.ExecutedCount, 1)
	}
}

func TestRuleEngine_ProcessMessage_NoMatch(t *testing.T) {
	connManager := connector.NewConnectorManager()
	engine := NewRuleEngine(connManager)

	// Create a test rule
	rule := Rule{
		ID:     "test-rule",
		Name:   "Test Rule",
		SQL:    "topic = 'sensor/temperature'",
		Status: RuleStatusEnabled,
	}

	err := engine.CreateRule(rule)
	if err != nil {
		t.Fatalf("CreateRule() error = %v", err)
	}

	// Process a non-matching message
	ruleCtx := &RuleContext{
		Topic:     "sensor/humidity", // Different topic
		QoS:       1,
		Payload:   []byte("60"),
		Headers:   map[string]string{},
		ClientID:  "sensor1",
		Username:  "user1",
		Timestamp: time.Now(),
		Metadata:  map[string]interface{}{},
	}

	ctx := context.Background()
	err = engine.ProcessMessage(ctx, ruleCtx)
	if err != nil {
		t.Errorf("ProcessMessage() error = %v", err)
	}

	// Check that metrics were not updated
	metrics, err := engine.GetRuleMetrics("test-rule")
	if err != nil {
		t.Errorf("GetRuleMetrics() error = %v", err)
	}

	if metrics.MatchedCount != 0 {
		t.Errorf("ProcessMessage() MatchedCount = %v, want %v", metrics.MatchedCount, 0)
	}
}

func TestRuleEngine_ProcessMessage_DisabledRule(t *testing.T) {
	connManager := connector.NewConnectorManager()
	engine := NewRuleEngine(connManager)

	// Create a disabled rule
	rule := Rule{
		ID:     "test-rule",
		Name:   "Test Rule",
		SQL:    "topic = 'sensor/temperature'",
		Status: RuleStatusDisabled,
	}

	err := engine.CreateRule(rule)
	if err != nil {
		t.Fatalf("CreateRule() error = %v", err)
	}

	// Process a matching message
	ruleCtx := &RuleContext{
		Topic:     "sensor/temperature",
		QoS:       1,
		Payload:   []byte("25.5"),
		Headers:   map[string]string{},
		ClientID:  "sensor1",
		Username:  "user1",
		Timestamp: time.Now(),
		Metadata:  map[string]interface{}{},
	}

	ctx := context.Background()
	err = engine.ProcessMessage(ctx, ruleCtx)
	if err != nil {
		t.Errorf("ProcessMessage() error = %v", err)
	}

	// Check that metrics were not updated for disabled rule
	metrics, err := engine.GetRuleMetrics("test-rule")
	if err != nil {
		t.Errorf("GetRuleMetrics() error = %v", err)
	}

	if metrics.MatchedCount != 0 {
		t.Errorf("ProcessMessage() MatchedCount = %v, want %v", metrics.MatchedCount, 0)
	}
}

func TestRuleEngine_GetActionTypes(t *testing.T) {
	connManager := connector.NewConnectorManager()
	engine := NewRuleEngine(connManager)

	actionTypes := engine.GetActionTypes()

	expectedTypes := []ActionType{
		ActionTypeConsole,
		ActionTypeHTTP,
		ActionTypeConnector,
		ActionTypeRepublish,
		ActionTypeDiscard,
	}

	if len(actionTypes) != len(expectedTypes) {
		t.Errorf("GetActionTypes() count = %v, want %v", len(actionTypes), len(expectedTypes))
	}

	for _, expectedType := range expectedTypes {
		found := false
		for _, actionType := range actionTypes {
			if actionType == expectedType {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("GetActionTypes() missing expected type: %v", expectedType)
		}
	}
}

func TestRuleEngine_GetActionExecutor(t *testing.T) {
	connManager := connector.NewConnectorManager()
	engine := NewRuleEngine(connManager)

	executor, err := engine.GetActionExecutor(ActionTypeConsole)
	if err != nil {
		t.Errorf("GetActionExecutor() error = %v", err)
	}
	if executor == nil {
		t.Error("GetActionExecutor() returned nil executor")
	}

	_, err = engine.GetActionExecutor("invalid_action_type")
	if err == nil {
		t.Error("GetActionExecutor() should return error for invalid action type")
	}
}

func TestRuleEngine_GetActionSchema(t *testing.T) {
	connManager := connector.NewConnectorManager()
	engine := NewRuleEngine(connManager)

	schema, err := engine.GetActionSchema(ActionTypeConsole)
	if err != nil {
		t.Errorf("GetActionSchema() error = %v", err)
	}
	if schema == nil {
		t.Error("GetActionSchema() returned nil schema")
	}

	_, err = engine.GetActionSchema("invalid_action_type")
	if err == nil {
		t.Error("GetActionSchema() should return error for invalid action type")
	}
}

func TestRuleEngine_RulePriority(t *testing.T) {
	connManager := connector.NewConnectorManager()
	engine := NewRuleEngine(connManager)

	// Create rules with different priorities
	rules := []Rule{
		{
			ID:       "low-priority",
			Name:     "Low Priority Rule",
			SQL:      "topic LIKE '%'",
			Status:   RuleStatusEnabled,
			Priority: 1,
			Actions: []Action{
				{
					Type: ActionTypeConsole,
					Parameters: map[string]interface{}{
						"template": "Low priority: ${topic}",
					},
				},
			},
		},
		{
			ID:       "high-priority",
			Name:     "High Priority Rule",
			SQL:      "topic LIKE '%'",
			Status:   RuleStatusEnabled,
			Priority: 10,
			Actions: []Action{
				{
					Type: ActionTypeConsole,
					Parameters: map[string]interface{}{
						"template": "High priority: ${topic}",
					},
				},
			},
		},
	}

	for _, rule := range rules {
		err := engine.CreateRule(rule)
		if err != nil {
			t.Fatalf("CreateRule() error = %v", err)
		}
	}

	// Process message - both rules should match, high priority should execute first
	ruleCtx := &RuleContext{
		Topic:     "test/topic",
		QoS:       1,
		Payload:   []byte("test"),
		Headers:   map[string]string{},
		ClientID:  "client1",
		Username:  "user1",
		Timestamp: time.Now(),
		Metadata:  map[string]interface{}{},
	}

	ctx := context.Background()
	err := engine.ProcessMessage(ctx, ruleCtx)
	if err != nil {
		t.Errorf("ProcessMessage() error = %v", err)
	}

	// Both rules should have been executed
	for _, ruleID := range []string{"low-priority", "high-priority"} {
		metrics, err := engine.GetRuleMetrics(ruleID)
		if err != nil {
			t.Errorf("GetRuleMetrics() error = %v", err)
		}
		if metrics.MatchedCount != 1 {
			t.Errorf("Rule %s MatchedCount = %v, want %v", ruleID, metrics.MatchedCount, 1)
		}
		if metrics.ExecutedCount != 1 {
			t.Errorf("Rule %s ExecutedCount = %v, want %v", ruleID, metrics.ExecutedCount, 1)
		}
	}
}

func TestRuleEngine_Close(t *testing.T) {
	connManager := connector.NewConnectorManager()
	engine := NewRuleEngine(connManager)

	err := engine.Close()
	if err != nil {
		t.Errorf("Close() error = %v", err)
	}
}