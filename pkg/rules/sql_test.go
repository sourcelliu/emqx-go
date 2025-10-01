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
	"testing"
	"time"
)

func TestSimpleSQLParser_Parse(t *testing.T) {
	parser := NewSimpleSQLParser()

	tests := []struct {
		name    string
		sql     string
		wantErr bool
	}{
		{
			name:    "simple WHERE clause",
			sql:     "topic = 'sensor/temperature'",
			wantErr: false,
		},
		{
			name:    "SELECT with WHERE",
			sql:     "SELECT * FROM t WHERE topic LIKE 'sensor/%'",
			wantErr: false,
		},
		{
			name:    "complex condition with AND",
			sql:     "topic = 'sensor/temp' AND payload.temperature > 25",
			wantErr: false,
		},
		{
			name:    "complex condition with OR",
			sql:     "topic = 'sensor/temp' OR topic = 'sensor/humid'",
			wantErr: false,
		},
		{
			name:    "nested conditions",
			sql:     "(topic = 'sensor/temp' AND payload.value > 20) OR (topic = 'alarm/fire' AND payload.level = 'critical')",
			wantErr: false,
		},
		{
			name:    "NULL check",
			sql:     "payload.value IS NOT NULL",
			wantErr: false,
		},
		{
			name:    "empty SQL",
			sql:     "",
			wantErr: false,
		},
		{
			name:    "SELECT without WHERE",
			sql:     "SELECT * FROM t",
			wantErr: false,
		},
		{
			name:    "invalid SQL",
			sql:     "INVALID SQL SYNTAX",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			condition, err := parser.Parse(tt.sql)
			if (err != nil) != tt.wantErr {
				t.Errorf("Parse() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && condition == nil {
				t.Errorf("Parse() returned nil condition for valid SQL")
			}
		})
	}
}

func TestSimpleSQLParser_Validate(t *testing.T) {
	parser := NewSimpleSQLParser()

	tests := []struct {
		name    string
		sql     string
		wantErr bool
	}{
		{
			name:    "valid simple condition",
			sql:     "topic = 'test/topic'",
			wantErr: false,
		},
		{
			name:    "valid complex condition",
			sql:     "topic LIKE 'sensor/%' AND payload.temperature > 25.5",
			wantErr: false,
		},
		{
			name:    "empty SQL",
			sql:     "",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := parser.Validate(tt.sql)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestComparisonCondition_Evaluate(t *testing.T) {
	ruleCtx := &RuleContext{
		Topic:     "sensor/temperature",
		QoS:       1,
		Payload:   []byte(`{"temperature": 26.5, "humidity": 60}`),
		Headers:   map[string]string{"device": "sensor1"},
		ClientID:  "client123",
		Username:  "user1",
		Timestamp: time.Now(),
		Metadata:  map[string]interface{}{"location": "room1"},
	}

	tests := []struct {
		name      string
		condition *ComparisonCondition
		want      bool
		wantErr   bool
	}{
		{
			name: "topic equals",
			condition: &ComparisonCondition{
				Field:    "topic",
				Operator: "=",
				Value:    "sensor/temperature",
			},
			want:    true,
			wantErr: false,
		},
		{
			name: "topic not equals",
			condition: &ComparisonCondition{
				Field:    "topic",
				Operator: "!=",
				Value:    "sensor/humidity",
			},
			want:    true,
			wantErr: false,
		},
		{
			name: "qos comparison",
			condition: &ComparisonCondition{
				Field:    "qos",
				Operator: ">",
				Value:    int64(0),
			},
			want:    true,
			wantErr: false,
		},
		{
			name: "clientid equals",
			condition: &ComparisonCondition{
				Field:    "clientid",
				Operator: "=",
				Value:    "client123",
			},
			want:    true,
			wantErr: false,
		},
		{
			name: "username equals",
			condition: &ComparisonCondition{
				Field:    "username",
				Operator: "=",
				Value:    "user1",
			},
			want:    true,
			wantErr: false,
		},
		{
			name: "topic LIKE match",
			condition: &ComparisonCondition{
				Field:    "topic",
				Operator: "LIKE",
				Value:    "sensor/%",
			},
			want:    true,
			wantErr: false,
		},
		{
			name: "topic LIKE no match",
			condition: &ComparisonCondition{
				Field:    "topic",
				Operator: "LIKE",
				Value:    "alarm/%",
			},
			want:    false,
			wantErr: false,
		},
		{
			name: "JSON payload field",
			condition: &ComparisonCondition{
				Field:    "payload.temperature",
				Operator: ">",
				Value:    float64(25),
			},
			want:    true,
			wantErr: false,
		},
		{
			name: "header field",
			condition: &ComparisonCondition{
				Field:    "headers.device",
				Operator: "=",
				Value:    "sensor1",
			},
			want:    true,
			wantErr: false,
		},
		{
			name: "metadata field",
			condition: &ComparisonCondition{
				Field:    "metadata.location",
				Operator: "=",
				Value:    "room1",
			},
			want:    true,
			wantErr: false,
		},
		{
			name: "non-existent field",
			condition: &ComparisonCondition{
				Field:    "nonexistent",
				Operator: "=",
				Value:    "value",
			},
			want:    false,
			wantErr: true,
		},
		{
			name: "IS NULL on null field",
			condition: &ComparisonCondition{
				Field:    "headers.nonexistent",
				Operator: "IS NULL",
				Value:    nil,
			},
			want:    true,
			wantErr: false,
		},
		{
			name: "IS NOT NULL on existing field",
			condition: &ComparisonCondition{
				Field:    "clientid",
				Operator: "IS NOT NULL",
				Value:    nil,
			},
			want:    true,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.condition.Evaluate(ruleCtx)
			if (err != nil) != tt.wantErr {
				t.Errorf("ComparisonCondition.Evaluate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("ComparisonCondition.Evaluate() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAndCondition_Evaluate(t *testing.T) {
	ruleCtx := &RuleContext{
		Topic:   "sensor/temperature",
		QoS:     1,
		Payload: []byte(`{"temperature": 26.5}`),
	}

	condition := &AndCondition{
		Conditions: []SQLCondition{
			&ComparisonCondition{
				Field:    "topic",
				Operator: "=",
				Value:    "sensor/temperature",
			},
			&ComparisonCondition{
				Field:    "qos",
				Operator: ">=",
				Value:    int64(1),
			},
		},
	}

	result, err := condition.Evaluate(ruleCtx)
	if err != nil {
		t.Errorf("AndCondition.Evaluate() error = %v", err)
	}
	if !result {
		t.Errorf("AndCondition.Evaluate() = %v, want %v", result, true)
	}

	// Test with one false condition
	condition.Conditions[1] = &ComparisonCondition{
		Field:    "qos",
		Operator: ">",
		Value:    int64(1),
	}

	result, err = condition.Evaluate(ruleCtx)
	if err != nil {
		t.Errorf("AndCondition.Evaluate() error = %v", err)
	}
	if result {
		t.Errorf("AndCondition.Evaluate() = %v, want %v", result, false)
	}
}

func TestOrCondition_Evaluate(t *testing.T) {
	ruleCtx := &RuleContext{
		Topic:   "sensor/temperature",
		QoS:     0,
		Payload: []byte(`{"temperature": 26.5}`),
	}

	condition := &OrCondition{
		Conditions: []SQLCondition{
			&ComparisonCondition{
				Field:    "topic",
				Operator: "=",
				Value:    "sensor/humidity", // false
			},
			&ComparisonCondition{
				Field:    "qos",
				Operator: ">=",
				Value:    int64(0), // true
			},
		},
	}

	result, err := condition.Evaluate(ruleCtx)
	if err != nil {
		t.Errorf("OrCondition.Evaluate() error = %v", err)
	}
	if !result {
		t.Errorf("OrCondition.Evaluate() = %v, want %v", result, true)
	}

	// Test with all false conditions
	condition.Conditions[1] = &ComparisonCondition{
		Field:    "qos",
		Operator: ">",
		Value:    int64(0),
	}

	result, err = condition.Evaluate(ruleCtx)
	if err != nil {
		t.Errorf("OrCondition.Evaluate() error = %v", err)
	}
	if result {
		t.Errorf("OrCondition.Evaluate() = %v, want %v", result, false)
	}
}

func TestNotCondition_Evaluate(t *testing.T) {
	ruleCtx := &RuleContext{
		Topic: "sensor/temperature",
	}

	condition := &NotCondition{
		Condition: &ComparisonCondition{
			Field:    "topic",
			Operator: "=",
			Value:    "sensor/humidity",
		},
	}

	result, err := condition.Evaluate(ruleCtx)
	if err != nil {
		t.Errorf("NotCondition.Evaluate() error = %v", err)
	}
	if !result {
		t.Errorf("NotCondition.Evaluate() = %v, want %v", result, true)
	}
}

func TestTrueCondition_Evaluate(t *testing.T) {
	condition := &TrueCondition{}
	result, err := condition.Evaluate(&RuleContext{})
	if err != nil {
		t.Errorf("TrueCondition.Evaluate() error = %v", err)
	}
	if !result {
		t.Errorf("TrueCondition.Evaluate() = %v, want %v", result, true)
	}
}

// Test string representations
func TestCondition_String(t *testing.T) {
	tests := []struct {
		name      string
		condition SQLCondition
		expected  string
	}{
		{
			name:      "TrueCondition",
			condition: &TrueCondition{},
			expected:  "TRUE",
		},
		{
			name: "ComparisonCondition",
			condition: &ComparisonCondition{
				Field:    "topic",
				Operator: "=",
				Value:    "test",
			},
			expected: "topic = 'test'",
		},
		{
			name: "AndCondition",
			condition: &AndCondition{
				Conditions: []SQLCondition{
					&ComparisonCondition{Field: "a", Operator: "=", Value: "1"},
					&ComparisonCondition{Field: "b", Operator: "=", Value: "2"},
				},
			},
			expected: "(a = '1' AND b = '2')",
		},
		{
			name: "OrCondition",
			condition: &OrCondition{
				Conditions: []SQLCondition{
					&ComparisonCondition{Field: "a", Operator: "=", Value: "1"},
					&ComparisonCondition{Field: "b", Operator: "=", Value: "2"},
				},
			},
			expected: "(a = '1' OR b = '2')",
		},
		{
			name: "NotCondition",
			condition: &NotCondition{
				Condition: &ComparisonCondition{Field: "a", Operator: "=", Value: "1"},
			},
			expected: "NOT a = '1'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.condition.String()
			if result != tt.expected {
				t.Errorf("String() = %v, want %v", result, tt.expected)
			}
		})
	}
}