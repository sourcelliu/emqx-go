package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/turtacn/emqx-go/pkg/rules"
)

const (
	rulesBrokerURL = "tcp://localhost:1883"
	dashboardURL   = "http://localhost:18083"
	rulesAPIURL    = dashboardURL + "/api/rules"
)

func TestRuleEngineE2E(t *testing.T) {
	t.Log("Starting Rule Engine E2E test")

	// Test steps:
	// 1. Create a rule via API
	// 2. Publish messages to test rule matching
	// 3. Verify rule execution metrics
	// 4. Update rule
	// 5. Test republish rule
	// 6. Delete rule

	// Create a console action rule
	rule := rules.Rule{
		ID:          "test-console-rule",
		Name:        "Test Console Rule",
		Description: "E2E test rule for console action",
		SQL:         "topic = 'sensor/temperature'",
		Actions: []rules.Action{
			{
				Type: rules.ActionTypeConsole,
				Parameters: map[string]interface{}{
					"level":    "info",
					"template": "Temperature sensor data: ${payload} from client ${clientid}",
				},
			},
		},
		Status:   rules.RuleStatusEnabled,
		Priority: 1,
	}

	// Create rule via API
	t.Log("Creating rule via Dashboard API")
	ruleID := createRuleViaAPI(t, rule)
	defer deleteRuleViaAPI(t, ruleID)

	// Test message publishing and rule execution
	t.Log("Testing rule execution with MQTT messages")
	testRuleExecution(t, ruleID)

	// Test rule update
	t.Log("Testing rule update")
	testRuleUpdate(t, ruleID)

	// Test republish rule
	t.Log("Testing republish rule")
	testRepublishRule(t)
}

func createRuleViaAPI(t *testing.T, rule rules.Rule) string {
	ruleJSON, err := json.Marshal(rule)
	require.NoError(t, err, "Failed to marshal rule")

	resp, err := http.Post(rulesAPIURL, "application/json", bytes.NewBuffer(ruleJSON))
	require.NoError(t, err, "Failed to create rule via API")
	defer resp.Body.Close()

	require.Equal(t, http.StatusCreated, resp.StatusCode, "Rule creation should return 201")

	// Verify rule was created by getting it back
	getRespURL := fmt.Sprintf("%s/%s", rulesAPIURL, rule.ID)
	getResp, err := http.Get(getRespURL)
	require.NoError(t, err, "Failed to get rule via API")
	defer getResp.Body.Close()

	require.Equal(t, http.StatusOK, getResp.StatusCode, "Rule retrieval should return 200")

	var retrievedRule rules.Rule
	err = json.NewDecoder(getResp.Body).Decode(&retrievedRule)
	require.NoError(t, err, "Failed to decode retrieved rule")

	assert.Equal(t, rule.ID, retrievedRule.ID, "Rule ID should match")
	assert.Equal(t, rule.Name, retrievedRule.Name, "Rule name should match")
	assert.Equal(t, rule.SQL, retrievedRule.SQL, "Rule SQL should match")

	t.Logf("Successfully created rule: %s", rule.ID)
	return rule.ID
}

func deleteRuleViaAPI(t *testing.T, ruleID string) {
	deleteURL := fmt.Sprintf("%s/%s", rulesAPIURL, ruleID)
	req, err := http.NewRequest("DELETE", deleteURL, nil)
	require.NoError(t, err, "Failed to create delete request")

	client := &http.Client{}
	resp, err := client.Do(req)
	require.NoError(t, err, "Failed to delete rule via API")
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode, "Rule deletion should return 200")
	t.Logf("Successfully deleted rule: %s", ruleID)
}

func testRuleExecution(t *testing.T, ruleID string) {
	// Setup MQTT client for publishing
	publisher := setupMQTTClient("rule-test-publisher", t)
	defer publisher.Disconnect(250)

	// Publish matching message
	matchingTopic := "sensor/temperature"
	testPayload := `{"temperature": 25.5, "humidity": 60, "timestamp": "2023-12-01T10:00:00Z"}`

	t.Logf("Publishing test message to topic: %s", matchingTopic)
	pubToken := publisher.Publish(matchingTopic, 1, false, testPayload)
	require.True(t, pubToken.Wait(), "Failed to publish message")
	require.NoError(t, pubToken.Error(), "Publish error")

	// Wait for rule processing
	time.Sleep(500 * time.Millisecond)

	// Check rule metrics
	t.Log("Checking rule execution metrics")
	metricsURL := fmt.Sprintf("%s/%s", rulesAPIURL, ruleID)
	resp, err := http.Get(metricsURL)
	require.NoError(t, err, "Failed to get rule metrics")
	defer resp.Body.Close()

	var rule rules.Rule
	err = json.NewDecoder(resp.Body).Decode(&rule)
	require.NoError(t, err, "Failed to decode rule with metrics")

	assert.Greater(t, rule.Metrics.MatchedCount, int64(0), "Rule should have matched at least one message")
	assert.Greater(t, rule.Metrics.ExecutedCount, int64(0), "Rule should have executed at least once")
	assert.Equal(t, int64(0), rule.Metrics.FailedCount, "Rule should have no failures")

	t.Logf("Rule metrics - Matched: %d, Executed: %d, Failed: %d",
		rule.Metrics.MatchedCount, rule.Metrics.ExecutedCount, rule.Metrics.FailedCount)

	// Publish non-matching message
	nonMatchingTopic := "sensor/humidity"
	t.Logf("Publishing non-matching message to topic: %s", nonMatchingTopic)
	pubToken = publisher.Publish(nonMatchingTopic, 1, false, testPayload)
	require.True(t, pubToken.Wait(), "Failed to publish non-matching message")
	require.NoError(t, pubToken.Error(), "Publish error for non-matching message")

	// Wait and check that metrics didn't change
	time.Sleep(200 * time.Millisecond)

	resp2, err := http.Get(metricsURL)
	require.NoError(t, err, "Failed to get rule metrics after non-matching message")
	defer resp2.Body.Close()

	var rule2 rules.Rule
	err = json.NewDecoder(resp2.Body).Decode(&rule2)
	require.NoError(t, err, "Failed to decode rule after non-matching message")

	assert.Equal(t, rule.Metrics.MatchedCount, rule2.Metrics.MatchedCount,
		"Matched count should not change for non-matching message")
}

func testRuleUpdate(t *testing.T, ruleID string) {
	// Update rule to match different topic
	updatedRule := rules.Rule{
		ID:          ruleID,
		Name:        "Updated Test Rule",
		Description: "Updated E2E test rule",
		SQL:         "topic LIKE 'sensor/%' AND payload.temperature > 20",
		Actions: []rules.Action{
			{
				Type: rules.ActionTypeConsole,
				Parameters: map[string]interface{}{
					"level":    "warn",
					"template": "High temperature alert: ${payload.temperature}°C from ${topic}",
				},
			},
		},
		Status:   rules.RuleStatusEnabled,
		Priority: 2,
	}

	// Update via API
	updateURL := fmt.Sprintf("%s/%s", rulesAPIURL, ruleID)
	ruleJSON, err := json.Marshal(updatedRule)
	require.NoError(t, err, "Failed to marshal updated rule")

	req, err := http.NewRequest("PUT", updateURL, bytes.NewBuffer(ruleJSON))
	require.NoError(t, err, "Failed to create update request")
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	require.NoError(t, err, "Failed to update rule via API")
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode, "Rule update should return 200")

	// Verify update
	getResp, err := http.Get(updateURL)
	require.NoError(t, err, "Failed to get updated rule")
	defer getResp.Body.Close()

	var retrievedRule rules.Rule
	err = json.NewDecoder(getResp.Body).Decode(&retrievedRule)
	require.NoError(t, err, "Failed to decode updated rule")

	assert.Equal(t, updatedRule.Name, retrievedRule.Name, "Updated rule name should match")
	assert.Equal(t, updatedRule.SQL, retrievedRule.SQL, "Updated rule SQL should match")
	assert.Equal(t, updatedRule.Priority, retrievedRule.Priority, "Updated rule priority should match")

	t.Log("Successfully updated rule")
}

func testRepublishRule(t *testing.T) {
	// Create a republish rule
	republishRule := rules.Rule{
		ID:          "test-republish-rule",
		Name:        "Test Republish Rule",
		Description: "E2E test rule for republish action",
		SQL:         "topic = 'input/data'",
		Actions: []rules.Action{
			{
				Type: rules.ActionTypeRepublish,
				Parameters: map[string]interface{}{
					"topic":            "output/processed",
					"qos":              1,
					"payload_template": `{"original_topic": "${topic}", "processed_at": "${timestamp}", "data": ${payload}}`,
				},
			},
		},
		Status:   rules.RuleStatusEnabled,
		Priority: 1,
	}

	// Create republish rule
	ruleID := createRuleViaAPI(t, republishRule)
	defer deleteRuleViaAPI(t, ruleID)

	// Setup subscriber for republished messages
	subscriber := setupMQTTClient("republish-test-subscriber", t)
	defer subscriber.Disconnect(250)

	// Setup publisher
	publisher := setupMQTTClient("republish-test-publisher", t)
	defer publisher.Disconnect(250)

	// Channel to receive republished message
	messageReceived := make(chan string, 1)

	// Subscribe to output topic
	outputTopic := "output/processed"
	token := subscriber.Subscribe(outputTopic, 1, func(client mqtt.Client, msg mqtt.Message) {
		receivedMsg := string(msg.Payload())
		t.Logf("Received republished message: %s", receivedMsg)
		messageReceived <- receivedMsg
	})

	require.True(t, token.Wait(), "Failed to subscribe to output topic")
	require.NoError(t, token.Error(), "Subscribe error")

	// Wait for subscription to be established
	time.Sleep(200 * time.Millisecond)

	// Publish to input topic
	inputTopic := "input/data"
	inputPayload := `{"sensor": "temp1", "value": 23.5}`

	t.Logf("Publishing to input topic: %s", inputTopic)
	pubToken := publisher.Publish(inputTopic, 1, false, inputPayload)
	require.True(t, pubToken.Wait(), "Failed to publish to input topic")
	require.NoError(t, pubToken.Error(), "Publish error")

	// Wait for republished message
	select {
	case receivedMsg := <-messageReceived:
		t.Logf("Successfully received republished message: %s", receivedMsg)

		// Verify the message contains expected fields
		var msgData map[string]interface{}
		err := json.Unmarshal([]byte(receivedMsg), &msgData)
		require.NoError(t, err, "Failed to parse republished message JSON")

		assert.Equal(t, "input/data", msgData["original_topic"], "Original topic should be preserved")
		assert.NotNil(t, msgData["processed_at"], "Processed timestamp should be present")
		assert.NotNil(t, msgData["data"], "Original data should be present")

	case <-time.After(5 * time.Second):
		t.Fatal("Timeout waiting for republished message")
	}
}

func testRuleDisableEnable(t *testing.T) {
	// Create a rule
	rule := rules.Rule{
		ID:          "test-disable-enable-rule",
		Name:        "Test Disable/Enable Rule",
		Description: "E2E test for rule disable/enable",
		SQL:         "topic = 'test/disable'",
		Actions: []rules.Action{
			{
				Type: rules.ActionTypeConsole,
				Parameters: map[string]interface{}{
					"level":    "info",
					"template": "Test message: ${payload}",
				},
			},
		},
		Status:   rules.RuleStatusEnabled,
		Priority: 1,
	}

	ruleID := createRuleViaAPI(t, rule)
	defer deleteRuleViaAPI(t, ruleID)

	// Disable rule
	disableURL := fmt.Sprintf("%s/%s/disable", rulesAPIURL, ruleID)
	req, err := http.NewRequest("POST", disableURL, nil)
	require.NoError(t, err, "Failed to create disable request")

	client := &http.Client{}
	resp, err := client.Do(req)
	require.NoError(t, err, "Failed to disable rule")
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode, "Rule disable should return 200")

	// Verify rule is disabled
	getRespURL := fmt.Sprintf("%s/%s", rulesAPIURL, ruleID)
	getResp, err := http.Get(getRespURL)
	require.NoError(t, err, "Failed to get rule status")
	defer getResp.Body.Close()

	var disabledRule rules.Rule
	err = json.NewDecoder(getResp.Body).Decode(&disabledRule)
	require.NoError(t, err, "Failed to decode disabled rule")

	assert.Equal(t, rules.RuleStatusDisabled, disabledRule.Status, "Rule should be disabled")

	// Enable rule
	enableURL := fmt.Sprintf("%s/%s/enable", rulesAPIURL, ruleID)
	req, err = http.NewRequest("POST", enableURL, nil)
	require.NoError(t, err, "Failed to create enable request")

	resp, err = client.Do(req)
	require.NoError(t, err, "Failed to enable rule")
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode, "Rule enable should return 200")

	// Verify rule is enabled
	getResp, err = http.Get(getRespURL)
	require.NoError(t, err, "Failed to get rule status after enable")
	defer getResp.Body.Close()

	var enabledRule rules.Rule
	err = json.NewDecoder(getResp.Body).Decode(&enabledRule)
	require.NoError(t, err, "Failed to decode enabled rule")

	assert.Equal(t, rules.RuleStatusEnabled, enabledRule.Status, "Rule should be enabled")

	t.Log("Successfully tested rule disable/enable")
}

func TestRuleEngineCompleteE2E(t *testing.T) {
	t.Log("Starting comprehensive Rule Engine E2E test")

	// Run all sub-tests
	t.Run("BasicRuleExecution", TestRuleEngineE2E)
	t.Run("RuleDisableEnable", testRuleDisableEnable)
}

func setupMQTTClient(clientID string, t *testing.T) mqtt.Client {
	opts := mqtt.NewClientOptions()
	opts.AddBroker(rulesBrokerURL)
	opts.SetClientID(clientID)
	opts.SetKeepAlive(60 * time.Second)
	opts.SetDefaultPublishHandler(func(client mqtt.Client, msg mqtt.Message) {
		t.Logf("Unexpected message on client %s: %s", clientID, msg.Payload())
	})
	opts.SetPingTimeout(1 * time.Second)
	opts.SetConnectTimeout(5 * time.Second)

	client := mqtt.NewClient(opts)
	token := client.Connect()
	require.True(t, token.Wait(), "Failed to connect to MQTT broker")
	require.NoError(t, token.Error(), "Connection error")

	t.Logf("Successfully connected MQTT client: %s", clientID)
	return client
}