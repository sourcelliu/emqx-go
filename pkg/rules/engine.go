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

// Package rules provides rule engine functionality for emqx-go.
// It implements a rule engine system similar to EMQX for processing and routing MQTT messages.
package rules

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/turtacn/emqx-go/pkg/connector"
)

// RuleStatus represents the current status of a rule
type RuleStatus string

const (
	RuleStatusEnabled  RuleStatus = "enabled"
	RuleStatusDisabled RuleStatus = "disabled"
	RuleStatusStopped  RuleStatus = "stopped"
	RuleStatusFailed   RuleStatus = "failed"
)

// ActionType represents different types of actions that can be executed
type ActionType string

const (
	ActionTypeConsole    ActionType = "console"
	ActionTypeHTTP       ActionType = "http"
	ActionTypeConnector  ActionType = "connector"
	ActionTypeRepublish  ActionType = "republish"
	ActionTypeDiscard    ActionType = "discard"
)

// Rule represents a rule in the rule engine
type Rule struct {
	ID          string                 `json:"id" yaml:"id"`
	Name        string                 `json:"name" yaml:"name"`
	Description string                 `json:"description,omitempty" yaml:"description,omitempty"`
	SQL         string                 `json:"sql" yaml:"sql"`
	Actions     []Action               `json:"actions" yaml:"actions"`
	Status      RuleStatus             `json:"status" yaml:"status"`
	Priority    int                    `json:"priority" yaml:"priority"`
	CreatedAt   time.Time              `json:"created_at" yaml:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at" yaml:"updated_at"`
	Metrics     RuleMetrics            `json:"metrics" yaml:"-"`
	Metadata    map[string]interface{} `json:"metadata,omitempty" yaml:"metadata,omitempty"`
}

// Action represents an action to be executed when a rule matches
type Action struct {
	Type       ActionType             `json:"type" yaml:"type"`
	Parameters map[string]interface{} `json:"parameters" yaml:"parameters"`
}

// RuleMetrics holds metrics for a rule
type RuleMetrics struct {
	MatchedCount  int64         `json:"matched_count"`
	ExecutedCount int64         `json:"executed_count"`
	FailedCount   int64         `json:"failed_count"`
	AvgLatency    time.Duration `json:"avg_latency"`
	LastExecution *time.Time    `json:"last_execution,omitempty"`
	LastError     string        `json:"last_error,omitempty"`
	LastErrorTime *time.Time    `json:"last_error_time,omitempty"`
}

// RuleContext represents the context available during rule execution
type RuleContext struct {
	Topic     string                 `json:"topic"`
	QoS       int                    `json:"qos"`
	Payload   []byte                 `json:"payload"`
	Headers   map[string]string      `json:"headers"`
	ClientID  string                 `json:"clientid"`
	Username  string                 `json:"username"`
	Timestamp time.Time              `json:"timestamp"`
	Metadata  map[string]interface{} `json:"metadata"`
}

// ActionResult represents the result of an action execution
type ActionResult struct {
	Success   bool          `json:"success"`
	Error     string        `json:"error,omitempty"`
	Latency   time.Duration `json:"latency"`
	Timestamp time.Time     `json:"timestamp"`
	Output    interface{}   `json:"output,omitempty"`
}

// RuleEngine manages and executes rules
type RuleEngine struct {
	rules       map[string]*Rule
	sqlParser   SQLParser
	actions     map[ActionType]ActionExecutor
	connManager *connector.ConnectorManager
	mu          sync.RWMutex
	ctx         context.Context
	cancel      context.CancelFunc
}

// SQLParser defines the interface for parsing SQL WHERE clauses
type SQLParser interface {
	Parse(sql string) (SQLCondition, error)
	Validate(sql string) error
}

// SQLCondition represents a parsed SQL condition
type SQLCondition interface {
	Evaluate(ctx *RuleContext) (bool, error)
	String() string
}

// ActionExecutor defines the interface for executing actions
type ActionExecutor interface {
	Execute(ctx context.Context, ruleCtx *RuleContext, params map[string]interface{}) (*ActionResult, error)
	Validate(params map[string]interface{}) error
	GetSchema() map[string]interface{}
}

// NewRuleEngine creates a new rule engine instance
func NewRuleEngine(connManager *connector.ConnectorManager) *RuleEngine {
	ctx, cancel := context.WithCancel(context.Background())

	engine := &RuleEngine{
		rules:       make(map[string]*Rule),
		sqlParser:   NewSimpleSQLParser(),
		actions:     make(map[ActionType]ActionExecutor),
		connManager: connManager,
		ctx:         ctx,
		cancel:      cancel,
	}

	// Register default action executors
	engine.RegisterAction(ActionTypeConsole, NewConsoleActionExecutor())
	engine.RegisterAction(ActionTypeHTTP, NewHTTPActionExecutor())
	engine.RegisterAction(ActionTypeConnector, NewConnectorActionExecutor(connManager))
	engine.RegisterAction(ActionTypeRepublish, NewRepublishActionExecutor())
	engine.RegisterAction(ActionTypeDiscard, NewDiscardActionExecutor())

	return engine
}

// RegisterAction registers an action executor
func (re *RuleEngine) RegisterAction(actionType ActionType, executor ActionExecutor) {
	re.mu.Lock()
	defer re.mu.Unlock()
	re.actions[actionType] = executor
}

// CreateRule creates a new rule
func (re *RuleEngine) CreateRule(rule Rule) error {
	re.mu.Lock()
	defer re.mu.Unlock()

	if _, exists := re.rules[rule.ID]; exists {
		return fmt.Errorf("rule with ID %s already exists", rule.ID)
	}

	// Validate SQL
	if err := re.sqlParser.Validate(rule.SQL); err != nil {
		return fmt.Errorf("invalid SQL: %w", err)
	}

	// Validate actions
	for i, action := range rule.Actions {
		executor, exists := re.actions[action.Type]
		if !exists {
			return fmt.Errorf("unknown action type: %s", action.Type)
		}
		if err := executor.Validate(action.Parameters); err != nil {
			return fmt.Errorf("invalid action %d: %w", i, err)
		}
	}

	now := time.Now()
	rule.CreatedAt = now
	rule.UpdatedAt = now
	rule.Metrics = RuleMetrics{}

	re.rules[rule.ID] = &rule
	return nil
}

// GetRule retrieves a rule by ID
func (re *RuleEngine) GetRule(id string) (*Rule, error) {
	re.mu.RLock()
	defer re.mu.RUnlock()

	rule, exists := re.rules[id]
	if !exists {
		return nil, fmt.Errorf("rule not found: %s", id)
	}

	// Return a copy to prevent external modification
	ruleCopy := *rule
	return &ruleCopy, nil
}

// UpdateRule updates an existing rule
func (re *RuleEngine) UpdateRule(id string, rule Rule) error {
	re.mu.Lock()
	defer re.mu.Unlock()

	existing, exists := re.rules[id]
	if !exists {
		return fmt.Errorf("rule not found: %s", id)
	}

	// Validate SQL
	if err := re.sqlParser.Validate(rule.SQL); err != nil {
		return fmt.Errorf("invalid SQL: %w", err)
	}

	// Validate actions
	for i, action := range rule.Actions {
		executor, exists := re.actions[action.Type]
		if !exists {
			return fmt.Errorf("unknown action type: %s", action.Type)
		}
		if err := executor.Validate(action.Parameters); err != nil {
			return fmt.Errorf("invalid action %d: %w", i, err)
		}
	}

	// Preserve creation time and metrics
	rule.CreatedAt = existing.CreatedAt
	rule.UpdatedAt = time.Now()
	rule.Metrics = existing.Metrics

	re.rules[id] = &rule
	return nil
}

// DeleteRule deletes a rule
func (re *RuleEngine) DeleteRule(id string) error {
	re.mu.Lock()
	defer re.mu.Unlock()

	if _, exists := re.rules[id]; !exists {
		return fmt.Errorf("rule not found: %s", id)
	}

	delete(re.rules, id)
	return nil
}

// ListRules returns all rules
func (re *RuleEngine) ListRules() map[string]*Rule {
	re.mu.RLock()
	defer re.mu.RUnlock()

	rules := make(map[string]*Rule)
	for id, rule := range re.rules {
		ruleCopy := *rule
		rules[id] = &ruleCopy
	}
	return rules
}

// EnableRule enables a rule
func (re *RuleEngine) EnableRule(id string) error {
	re.mu.Lock()
	defer re.mu.Unlock()

	rule, exists := re.rules[id]
	if !exists {
		return fmt.Errorf("rule not found: %s", id)
	}

	rule.Status = RuleStatusEnabled
	rule.UpdatedAt = time.Now()
	return nil
}

// DisableRule disables a rule
func (re *RuleEngine) DisableRule(id string) error {
	re.mu.Lock()
	defer re.mu.Unlock()

	rule, exists := re.rules[id]
	if !exists {
		return fmt.Errorf("rule not found: %s", id)
	}

	rule.Status = RuleStatusDisabled
	rule.UpdatedAt = time.Now()
	return nil
}

// ProcessMessage processes a message through all enabled rules
func (re *RuleEngine) ProcessMessage(ctx context.Context, ruleCtx *RuleContext) error {
	re.mu.RLock()
	enabledRules := make([]*Rule, 0)
	for _, rule := range re.rules {
		if rule.Status == RuleStatusEnabled {
			enabledRules = append(enabledRules, rule)
		}
	}
	re.mu.RUnlock()

	// Sort rules by priority (higher priority first)
	for i := 0; i < len(enabledRules)-1; i++ {
		for j := i + 1; j < len(enabledRules); j++ {
			if enabledRules[i].Priority < enabledRules[j].Priority {
				enabledRules[i], enabledRules[j] = enabledRules[j], enabledRules[i]
			}
		}
	}

	for _, rule := range enabledRules {
		if err := re.processRule(ctx, rule, ruleCtx); err != nil {
			// Log error but continue processing other rules
			continue
		}
	}

	return nil
}

// processRule processes a single rule
func (re *RuleEngine) processRule(ctx context.Context, rule *Rule, ruleCtx *RuleContext) error {
	start := time.Now()

	// Parse and evaluate SQL condition
	condition, err := re.sqlParser.Parse(rule.SQL)
	if err != nil {
		re.recordRuleError(rule, fmt.Sprintf("SQL parse error: %v", err))
		return err
	}

	matched, err := condition.Evaluate(ruleCtx)
	if err != nil {
		re.recordRuleError(rule, fmt.Sprintf("SQL evaluation error: %v", err))
		return err
	}

	if !matched {
		return nil
	}

	re.recordRuleMatch(rule)

	// Execute actions
	for _, action := range rule.Actions {
		executor, exists := re.actions[action.Type]
		if !exists {
			re.recordRuleError(rule, fmt.Sprintf("unknown action type: %s", action.Type))
			continue
		}

		result, err := executor.Execute(ctx, ruleCtx, action.Parameters)
		if err != nil {
			re.recordRuleError(rule, fmt.Sprintf("action execution error: %v", err))
			continue
		}

		if !result.Success {
			re.recordRuleError(rule, result.Error)
			continue
		}
	}

	re.recordRuleExecution(rule, time.Since(start))
	return nil
}

// recordRuleMatch records a rule match
func (re *RuleEngine) recordRuleMatch(rule *Rule) {
	re.mu.Lock()
	defer re.mu.Unlock()
	rule.Metrics.MatchedCount++
}

// recordRuleExecution records a successful rule execution
func (re *RuleEngine) recordRuleExecution(rule *Rule, latency time.Duration) {
	re.mu.Lock()
	defer re.mu.Unlock()

	rule.Metrics.ExecutedCount++
	now := time.Now()
	rule.Metrics.LastExecution = &now

	// Calculate average latency
	if rule.Metrics.ExecutedCount == 1 {
		rule.Metrics.AvgLatency = latency
	} else {
		rule.Metrics.AvgLatency = time.Duration(
			(int64(rule.Metrics.AvgLatency)*int64(rule.Metrics.ExecutedCount-1) + int64(latency)) / int64(rule.Metrics.ExecutedCount),
		)
	}
}

// recordRuleError records a rule execution error
func (re *RuleEngine) recordRuleError(rule *Rule, errorMsg string) {
	re.mu.Lock()
	defer re.mu.Unlock()

	rule.Metrics.FailedCount++
	rule.Metrics.LastError = errorMsg
	now := time.Now()
	rule.Metrics.LastErrorTime = &now
}

// GetRuleMetrics returns metrics for a specific rule
func (re *RuleEngine) GetRuleMetrics(id string) (*RuleMetrics, error) {
	re.mu.RLock()
	defer re.mu.RUnlock()

	rule, exists := re.rules[id]
	if !exists {
		return nil, fmt.Errorf("rule not found: %s", id)
	}

	metrics := rule.Metrics
	return &metrics, nil
}

// GetAllMetrics returns metrics for all rules
func (re *RuleEngine) GetAllMetrics() map[string]RuleMetrics {
	re.mu.RLock()
	defer re.mu.RUnlock()

	metrics := make(map[string]RuleMetrics)
	for id, rule := range re.rules {
		metrics[id] = rule.Metrics
	}
	return metrics
}

// GetActionTypes returns all registered action types
func (re *RuleEngine) GetActionTypes() []ActionType {
	re.mu.RLock()
	defer re.mu.RUnlock()

	types := make([]ActionType, 0, len(re.actions))
	for actionType := range re.actions {
		types = append(types, actionType)
	}
	return types
}

// GetActionExecutor returns the action executor for a specific type
func (re *RuleEngine) GetActionExecutor(actionType ActionType) (ActionExecutor, error) {
	re.mu.RLock()
	defer re.mu.RUnlock()

	executor, exists := re.actions[actionType]
	if !exists {
		return nil, fmt.Errorf("unknown action type: %s", actionType)
	}

	return executor, nil
}

// GetActionSchema returns the schema for a specific action type
func (re *RuleEngine) GetActionSchema(actionType ActionType) (map[string]interface{}, error) {
	re.mu.RLock()
	defer re.mu.RUnlock()

	executor, exists := re.actions[actionType]
	if !exists {
		return nil, fmt.Errorf("unknown action type: %s", actionType)
	}

	return executor.GetSchema(), nil
}

// Close gracefully shuts down the rule engine
func (re *RuleEngine) Close() error {
	re.cancel()
	return nil
}

// MarshalJSON implements custom JSON marshaling for Rule
func (r Rule) MarshalJSON() ([]byte, error) {
	type Alias Rule
	return json.Marshal(&struct {
		CreatedAt string `json:"created_at"`
		UpdatedAt string `json:"updated_at"`
		*Alias
	}{
		CreatedAt: r.CreatedAt.Format(time.RFC3339),
		UpdatedAt: r.UpdatedAt.Format(time.RFC3339),
		Alias:     (*Alias)(&r),
	})
}

// UnmarshalJSON implements custom JSON unmarshaling for Rule
func (r *Rule) UnmarshalJSON(data []byte) error {
	type Alias Rule
	aux := &struct {
		CreatedAt string `json:"created_at"`
		UpdatedAt string `json:"updated_at"`
		*Alias
	}{
		Alias: (*Alias)(r),
	}

	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}

	if aux.CreatedAt != "" {
		if t, err := time.Parse(time.RFC3339, aux.CreatedAt); err == nil {
			r.CreatedAt = t
		}
	}

	if aux.UpdatedAt != "" {
		if t, err := time.Parse(time.RFC3339, aux.UpdatedAt); err == nil {
			r.UpdatedAt = t
		}
	}

	return nil
}