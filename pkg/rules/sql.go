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
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// SimpleSQLParser implements a basic SQL WHERE clause parser
type SimpleSQLParser struct{}

// NewSimpleSQLParser creates a new simple SQL parser
func NewSimpleSQLParser() *SimpleSQLParser {
	return &SimpleSQLParser{}
}

// Parse parses a SQL WHERE clause
func (p *SimpleSQLParser) Parse(sql string) (SQLCondition, error) {
	// Remove SELECT ... FROM ... and extract WHERE clause
	sql = strings.TrimSpace(sql)

	// Handle simple SELECT statements
	if strings.HasPrefix(strings.ToUpper(sql), "SELECT") {
		whereIndex := strings.Index(strings.ToUpper(sql), "WHERE")
		if whereIndex == -1 {
			// No WHERE clause, always true
			return &TrueCondition{}, nil
		}
		sql = strings.TrimSpace(sql[whereIndex+5:]) // Remove "WHERE "
	}

	if sql == "" {
		return &TrueCondition{}, nil
	}

	return p.parseExpression(sql)
}

// Validate validates a SQL WHERE clause
func (p *SimpleSQLParser) Validate(sql string) error {
	_, err := p.Parse(sql)
	return err
}

// parseExpression parses a SQL expression
func (p *SimpleSQLParser) parseExpression(expr string) (SQLCondition, error) {
	expr = strings.TrimSpace(expr)

	// Handle OR operations (lowest precedence)
	if orParts := p.splitByOperator(expr, "OR"); len(orParts) > 1 {
		conditions := make([]SQLCondition, len(orParts))
		for i, part := range orParts {
			cond, err := p.parseExpression(part)
			if err != nil {
				return nil, err
			}
			conditions[i] = cond
		}
		return &OrCondition{Conditions: conditions}, nil
	}

	// Handle AND operations
	if andParts := p.splitByOperator(expr, "AND"); len(andParts) > 1 {
		conditions := make([]SQLCondition, len(andParts))
		for i, part := range andParts {
			cond, err := p.parseExpression(part)
			if err != nil {
				return nil, err
			}
			conditions[i] = cond
		}
		return &AndCondition{Conditions: conditions}, nil
	}

	// Handle parentheses
	if strings.HasPrefix(expr, "(") && strings.HasSuffix(expr, ")") {
		return p.parseExpression(expr[1 : len(expr)-1])
	}

	// Handle NOT operation
	if strings.HasPrefix(strings.ToUpper(expr), "NOT ") {
		inner, err := p.parseExpression(expr[4:])
		if err != nil {
			return nil, err
		}
		return &NotCondition{Condition: inner}, nil
	}

	// Handle comparison operations
	return p.parseComparison(expr)
}

// parseComparison parses a comparison operation
func (p *SimpleSQLParser) parseComparison(expr string) (SQLCondition, error) {
	expr = strings.TrimSpace(expr)

	// Regex for matching comparison operations
	comparisonRegex := regexp.MustCompile(`^(.+?)\s*(=|!=|<>|<=|>=|<|>|LIKE|NOT LIKE|IN|NOT IN|IS NULL|IS NOT NULL)\s*(.*)$`)
	matches := comparisonRegex.FindStringSubmatch(expr)

	if len(matches) != 4 {
		return nil, fmt.Errorf("invalid comparison expression: %s", expr)
	}

	field := strings.TrimSpace(matches[1])
	operator := strings.ToUpper(strings.TrimSpace(matches[2]))
	value := strings.TrimSpace(matches[3])

	// Handle NULL checks
	if operator == "IS NULL" {
		return &ComparisonCondition{
			Field:    field,
			Operator: "IS NULL",
			Value:    nil,
		}, nil
	}

	if operator == "IS NOT NULL" {
		return &ComparisonCondition{
			Field:    field,
			Operator: "IS NOT NULL",
			Value:    nil,
		}, nil
	}

	// Parse value
	parsedValue, err := p.parseValue(value)
	if err != nil {
		return nil, fmt.Errorf("invalid value in comparison: %w", err)
	}

	return &ComparisonCondition{
		Field:    field,
		Operator: operator,
		Value:    parsedValue,
	}, nil
}

// parseValue parses a value from SQL
func (p *SimpleSQLParser) parseValue(value string) (interface{}, error) {
	value = strings.TrimSpace(value)

	// String literal
	if (strings.HasPrefix(value, "'") && strings.HasSuffix(value, "'")) ||
		(strings.HasPrefix(value, "\"") && strings.HasSuffix(value, "\"")) {
		return value[1 : len(value)-1], nil
	}

	// Number
	if intVal, err := strconv.ParseInt(value, 10, 64); err == nil {
		return intVal, nil
	}

	if floatVal, err := strconv.ParseFloat(value, 64); err == nil {
		return floatVal, nil
	}

	// Boolean
	if value == "true" || value == "TRUE" {
		return true, nil
	}
	if value == "false" || value == "FALSE" {
		return false, nil
	}

	// Field reference
	return &FieldReference{Field: value}, nil
}

// splitByOperator splits expression by operator while respecting parentheses
func (p *SimpleSQLParser) splitByOperator(expr string, operator string) []string {
	parts := []string{}
	current := ""
	parenLevel := 0
	i := 0

	for i < len(expr) {
		if expr[i] == '(' {
			parenLevel++
		} else if expr[i] == ')' {
			parenLevel--
		}

		if parenLevel == 0 && i <= len(expr)-len(operator) {
			if strings.ToUpper(expr[i:i+len(operator)]) == operator {
				// Check if it's a whole word (not part of another word)
				if (i == 0 || !isAlphaNumeric(expr[i-1])) &&
					(i+len(operator) >= len(expr) || !isAlphaNumeric(expr[i+len(operator)])) {
					parts = append(parts, strings.TrimSpace(current))
					current = ""
					i += len(operator)
					continue
				}
			}
		}

		current += string(expr[i])
		i++
	}

	if current != "" {
		parts = append(parts, strings.TrimSpace(current))
	}

	if len(parts) <= 1 {
		return []string{expr}
	}

	return parts
}

// isAlphaNumeric checks if a character is alphanumeric
func isAlphaNumeric(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_'
}

// SQL Condition implementations

// TrueCondition always evaluates to true
type TrueCondition struct{}

func (c *TrueCondition) Evaluate(ctx *RuleContext) (bool, error) {
	return true, nil
}

func (c *TrueCondition) String() string {
	return "TRUE"
}

// AndCondition represents AND logic
type AndCondition struct {
	Conditions []SQLCondition
}

func (c *AndCondition) Evaluate(ctx *RuleContext) (bool, error) {
	for _, condition := range c.Conditions {
		result, err := condition.Evaluate(ctx)
		if err != nil {
			return false, err
		}
		if !result {
			return false, nil
		}
	}
	return true, nil
}

func (c *AndCondition) String() string {
	parts := make([]string, len(c.Conditions))
	for i, cond := range c.Conditions {
		parts[i] = cond.String()
	}
	return "(" + strings.Join(parts, " AND ") + ")"
}

// OrCondition represents OR logic
type OrCondition struct {
	Conditions []SQLCondition
}

func (c *OrCondition) Evaluate(ctx *RuleContext) (bool, error) {
	for _, condition := range c.Conditions {
		result, err := condition.Evaluate(ctx)
		if err != nil {
			return false, err
		}
		if result {
			return true, nil
		}
	}
	return false, nil
}

func (c *OrCondition) String() string {
	parts := make([]string, len(c.Conditions))
	for i, cond := range c.Conditions {
		parts[i] = cond.String()
	}
	return "(" + strings.Join(parts, " OR ") + ")"
}

// NotCondition represents NOT logic
type NotCondition struct {
	Condition SQLCondition
}

func (c *NotCondition) Evaluate(ctx *RuleContext) (bool, error) {
	result, err := c.Condition.Evaluate(ctx)
	if err != nil {
		return false, err
	}
	return !result, nil
}

func (c *NotCondition) String() string {
	return "NOT " + c.Condition.String()
}

// ComparisonCondition represents comparison operations
type ComparisonCondition struct {
	Field    string
	Operator string
	Value    interface{}
}

func (c *ComparisonCondition) Evaluate(ctx *RuleContext) (bool, error) {
	fieldValue, err := c.getFieldValue(ctx, c.Field)
	if err != nil {
		return false, err
	}

	compareValue := c.Value
	if fieldRef, ok := c.Value.(*FieldReference); ok {
		compareValue, err = c.getFieldValue(ctx, fieldRef.Field)
		if err != nil {
			return false, err
		}
	}

	return c.compareValues(fieldValue, compareValue)
}

func (c *ComparisonCondition) getFieldValue(ctx *RuleContext, field string) (interface{}, error) {
	switch field {
	case "topic":
		return ctx.Topic, nil
	case "qos":
		return int64(ctx.QoS), nil
	case "payload":
		return string(ctx.Payload), nil
	case "clientid":
		return ctx.ClientID, nil
	case "username":
		return ctx.Username, nil
	case "timestamp":
		return ctx.Timestamp.Unix(), nil
	default:
		// Try to parse as JSON path
		if strings.HasPrefix(field, "payload.") {
			return c.getJSONField(ctx.Payload, field[8:])
		}
		if strings.HasPrefix(field, "headers.") {
			headerKey := field[8:]
			if value, exists := ctx.Headers[headerKey]; exists {
				return value, nil
			}
			return nil, nil
		}
		if strings.HasPrefix(field, "metadata.") {
			metaKey := field[9:]
			if value, exists := ctx.Metadata[metaKey]; exists {
				return value, nil
			}
			return nil, nil
		}
		return nil, fmt.Errorf("unknown field: %s", field)
	}
}

func (c *ComparisonCondition) getJSONField(payload []byte, path string) (interface{}, error) {
	var data interface{}
	if err := json.Unmarshal(payload, &data); err != nil {
		return nil, err
	}

	// Simple path resolution (supports only single level for now)
	if dataMap, ok := data.(map[string]interface{}); ok {
		return dataMap[path], nil
	}

	return nil, nil
}

func (c *ComparisonCondition) compareValues(fieldValue, compareValue interface{}) (bool, error) {
	switch c.Operator {
	case "=":
		return c.equals(fieldValue, compareValue), nil
	case "!=", "<>":
		return !c.equals(fieldValue, compareValue), nil
	case "<":
		result, err := c.lessThan(fieldValue, compareValue)
		return result, err
	case "<=":
		less, err1 := c.lessThan(fieldValue, compareValue)
		if err1 != nil {
			return false, err1
		}
		equals := c.equals(fieldValue, compareValue)
		return less || equals, nil
	case ">":
		result, err := c.greaterThan(fieldValue, compareValue)
		return result, err
	case ">=":
		greater, err1 := c.greaterThan(fieldValue, compareValue)
		if err1 != nil {
			return false, err1
		}
		equals := c.equals(fieldValue, compareValue)
		return greater || equals, nil
	case "LIKE":
		return c.like(fieldValue, compareValue)
	case "NOT LIKE":
		result, err := c.like(fieldValue, compareValue)
		return !result, err
	case "IS NULL":
		return fieldValue == nil, nil
	case "IS NOT NULL":
		return fieldValue != nil, nil
	default:
		return false, fmt.Errorf("unsupported operator: %s", c.Operator)
	}
}

func (c *ComparisonCondition) equals(a, b interface{}) bool {
	// Handle nil cases
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}

	// Convert to comparable types
	aStr := fmt.Sprintf("%v", a)
	bStr := fmt.Sprintf("%v", b)
	return aStr == bStr
}

func (c *ComparisonCondition) lessThan(a, b interface{}) (bool, error) {
	aFloat, aOk := c.toFloat64(a)
	bFloat, bOk := c.toFloat64(b)

	if aOk && bOk {
		return aFloat < bFloat, nil
	}

	// String comparison
	aStr := fmt.Sprintf("%v", a)
	bStr := fmt.Sprintf("%v", b)
	return aStr < bStr, nil
}

func (c *ComparisonCondition) greaterThan(a, b interface{}) (bool, error) {
	aFloat, aOk := c.toFloat64(a)
	bFloat, bOk := c.toFloat64(b)

	if aOk && bOk {
		return aFloat > bFloat, nil
	}

	// String comparison
	aStr := fmt.Sprintf("%v", a)
	bStr := fmt.Sprintf("%v", b)
	return aStr > bStr, nil
}

func (c *ComparisonCondition) like(a, b interface{}) (bool, error) {
	aStr := fmt.Sprintf("%v", a)
	bStr := fmt.Sprintf("%v", b)

	// Convert SQL LIKE pattern to regex
	pattern := strings.ReplaceAll(bStr, "%", ".*")
	pattern = strings.ReplaceAll(pattern, "_", ".")
	pattern = "^" + pattern + "$"

	matched, err := regexp.MatchString(pattern, aStr)
	return matched, err
}

func (c *ComparisonCondition) toFloat64(value interface{}) (float64, bool) {
	switch v := value.(type) {
	case float64:
		return v, true
	case float32:
		return float64(v), true
	case int:
		return float64(v), true
	case int32:
		return float64(v), true
	case int64:
		return float64(v), true
	case string:
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f, true
		}
	}
	return 0, false
}

func (c *ComparisonCondition) String() string {
	valueStr := fmt.Sprintf("%v", c.Value)
	if _, ok := c.Value.(string); ok {
		valueStr = "'" + valueStr + "'"
	}
	return fmt.Sprintf("%s %s %s", c.Field, c.Operator, valueStr)
}

// FieldReference represents a reference to a field
type FieldReference struct {
	Field string
}

func (f *FieldReference) String() string {
	return f.Field
}