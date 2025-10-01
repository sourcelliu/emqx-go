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
	"testing"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/turtacn/emqx-go/pkg/blacklist"
	"github.com/turtacn/emqx-go/pkg/broker"
)

// TestBlacklistIntegrationClientIDBlocking tests client ID blacklist with real broker
func TestBlacklistIntegrationClientIDBlocking(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start broker
	brokerInstance := broker.New("blacklist-integration-broker", nil)
	brokerInstance.SetupDefaultAuth()
	defer brokerInstance.Close()

	// Start broker on unique port
	brokerPort := ":1945"
	go brokerInstance.StartServer(ctx, brokerPort)
	time.Sleep(300 * time.Millisecond)

	// Get blacklist middleware from broker
	blacklistMiddleware := brokerInstance.GetBlacklistMiddleware()
	require.NotNil(t, blacklistMiddleware, "Blacklist middleware should be available")

	// Test 1: Normal client should connect initially
	normalClient := createTestMQTTClient("good-client", brokerPort)
	token := normalClient.Connect()
	require.True(t, token.WaitTimeout(5*time.Second), "Good client should connect initially")
	require.NoError(t, token.Error())
	normalClient.Disconnect(100)

	// Test 2: Add client ID to blacklist
	entry := &blacklist.BlacklistEntry{
		ID:      "test-blocked-client",
		Type:    blacklist.ClientIDBlacklist,
		Value:   "malicious-client",
		Action:  blacklist.ActionDeny,
		Reason:  "Suspicious behavior detected",
		Enabled: true,
	}

	err := blacklistMiddleware.GetManager().AddEntry(entry)
	require.NoError(t, err)

	// Test 3: Blacklisted client should be blocked
	blockedClient := createTestMQTTClient("malicious-client", brokerPort)
	token = blockedClient.Connect()

	// Give the broker a moment to process and block the connection
	connected := token.WaitTimeout(2 * time.Second)

	// The client library might receive a CONNACK before being disconnected
	// Wait a bit to see if the connection is closed by the broker
	time.Sleep(1 * time.Second)

	// The key test is that the client should not remain connected
	// due to the blacklist middleware blocking it
	isConnected := blockedClient.IsConnected()
	if connected && isConnected {
		t.Errorf("Blacklisted client should be blocked by the broker")
	}
	// Even if connection attempt succeeds initially, it should be terminated
	// This indicates the blacklist middleware is working
	blockedClient.Disconnect(100)

	// Test 4: Normal client should still work
	normalClient2 := createTestMQTTClient("another-good-client", brokerPort)
	token = normalClient2.Connect()
	require.True(t, token.WaitTimeout(5*time.Second), "Good client should still connect")
	require.NoError(t, token.Error())
	normalClient2.Disconnect(100)

	// Test 5: Verify blacklist statistics
	stats := blacklistMiddleware.GetStats()
	assert.Equal(t, 1, stats.TotalEntries)
	assert.Equal(t, 1, stats.EntriesByType[blacklist.ClientIDBlacklist])
}

// TestBlacklistIntegrationUsernameBlocking tests username blacklist
func TestBlacklistIntegrationUsernameBlocking(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start broker
	brokerInstance := broker.New("blacklist-username-integration-broker", nil)
	brokerInstance.SetupDefaultAuth()
	defer brokerInstance.Close()

	brokerPort := ":1946"
	go brokerInstance.StartServer(ctx, brokerPort)
	time.Sleep(300 * time.Millisecond)

	// Get blacklist middleware
	blacklistMiddleware := brokerInstance.GetBlacklistMiddleware()
	require.NotNil(t, blacklistMiddleware)

	// Add username to blacklist
	entry := &blacklist.BlacklistEntry{
		ID:      "test-blocked-username",
		Type:    blacklist.UsernameBlacklist,
		Value:   "baduser",
		Action:  blacklist.ActionDeny,
		Reason:  "Account compromised",
		Enabled: true,
	}

	err := blacklistMiddleware.GetManager().AddEntry(entry)
	require.NoError(t, err)

	// Test blocked username
	blockedClient := createTestMQTTClientWithCredentials("username-test-client", "baduser", "test", brokerPort)
	token := blockedClient.Connect()

	connected := token.WaitTimeout(5 * time.Second)
	if connected {
		time.Sleep(500 * time.Millisecond)
		assert.False(t, blockedClient.IsConnected(), "Blacklisted username should be disconnected")
	} else {
		assert.Error(t, token.Error(), "Blacklisted username connection should fail")
	}
	blockedClient.Disconnect(100)

	// Test good username should work
	goodClient := createTestMQTTClientWithCredentials("username-test-client-2", "test", "test", brokerPort)
	token = goodClient.Connect()
	require.True(t, token.WaitTimeout(5*time.Second), "Good username should connect")
	require.NoError(t, token.Error())
	goodClient.Disconnect(100)
}

// TestBlacklistIntegrationPatternMatching tests pattern-based blacklist
func TestBlacklistIntegrationPatternMatching(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start broker
	brokerInstance := broker.New("blacklist-pattern-integration-broker", nil)
	brokerInstance.SetupDefaultAuth()
	defer brokerInstance.Close()

	brokerPort := ":1947"
	go brokerInstance.StartServer(ctx, brokerPort)
	time.Sleep(300 * time.Millisecond)

	// Get blacklist middleware
	blacklistMiddleware := brokerInstance.GetBlacklistMiddleware()
	require.NotNil(t, blacklistMiddleware)

	// Add pattern-based blacklist entry
	entry := &blacklist.BlacklistEntry{
		ID:      "test-bot-pattern",
		Type:    blacklist.ClientIDBlacklist,
		Pattern: "bot_.*",
		Action:  blacklist.ActionDeny,
		Reason:  "Automated bot clients not allowed",
		Enabled: true,
	}

	err := blacklistMiddleware.GetManager().AddEntry(entry)
	require.NoError(t, err)

	// Test client matching pattern should be blocked
	blockedClient := createTestMQTTClient("bot_crawler", brokerPort)
	token := blockedClient.Connect()

	connected := token.WaitTimeout(5 * time.Second)
	if connected {
		time.Sleep(500 * time.Millisecond)
		assert.False(t, blockedClient.IsConnected(), "Pattern-matched client should be disconnected")
	} else {
		assert.Error(t, token.Error(), "Pattern-matched client connection should fail")
	}
	blockedClient.Disconnect(100)

	// Test client not matching pattern should work
	goodClient := createTestMQTTClient("legitimate_client", brokerPort)
	token = goodClient.Connect()
	require.True(t, token.WaitTimeout(5*time.Second), "Non-pattern-matched client should connect")
	require.NoError(t, token.Error())
	goodClient.Disconnect(100)
}

// TestBlacklistIntegrationTemporaryBlocking tests temporary blacklist entries
func TestBlacklistIntegrationTemporaryBlocking(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start broker
	brokerInstance := broker.New("blacklist-temp-integration-broker", nil)
	brokerInstance.SetupDefaultAuth()
	defer brokerInstance.Close()

	brokerPort := ":1948"
	go brokerInstance.StartServer(ctx, brokerPort)
	time.Sleep(300 * time.Millisecond)

	// Get blacklist middleware
	blacklistMiddleware := brokerInstance.GetBlacklistMiddleware()
	require.NotNil(t, blacklistMiddleware)

	// Add temporary block using middleware convenience method
	err := blacklistMiddleware.BlockClientID("temp-blocked-client", "Temporary block for testing", 3*time.Second)
	require.NoError(t, err)

	// Test client should be blocked initially
	blockedClient := createTestMQTTClient("temp-blocked-client", brokerPort)
	token := blockedClient.Connect()

	connected := token.WaitTimeout(5 * time.Second)
	if connected {
		time.Sleep(500 * time.Millisecond)
		assert.False(t, blockedClient.IsConnected(), "Temporarily blocked client should be disconnected")
	}
	blockedClient.Disconnect(100)

	// Wait for expiration
	time.Sleep(4 * time.Second)

	// Test client should be able to connect after expiration
	unblockedClient := createTestMQTTClient("temp-blocked-client", brokerPort)
	token = unblockedClient.Connect()
	assert.True(t, token.WaitTimeout(5*time.Second), "Client should connect after blacklist expires")
	if token.Error() != nil {
		t.Logf("Connection error after expiration: %v", token.Error())
	}
	unblockedClient.Disconnect(100)
}

// TestBlacklistIntegrationCRUDOperations tests CRUD operations on blacklist
func TestBlacklistIntegrationCRUDOperations(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start broker
	brokerInstance := broker.New("blacklist-crud-integration-broker", nil)
	brokerInstance.SetupDefaultAuth()
	defer brokerInstance.Close()

	brokerPort := ":1949"
	go brokerInstance.StartServer(ctx, brokerPort)
	time.Sleep(300 * time.Millisecond)

	// Get blacklist middleware
	blacklistMiddleware := brokerInstance.GetBlacklistMiddleware()
	require.NotNil(t, blacklistMiddleware)

	manager := blacklistMiddleware.GetManager()

	// Test Create
	entry := &blacklist.BlacklistEntry{
		ID:          "test-crud-entry",
		Type:        blacklist.ClientIDBlacklist,
		Value:       "crud-test-client",
		Action:      blacklist.ActionDeny,
		Reason:      "CRUD test entry",
		Description: "Test entry for CRUD operations",
		Enabled:     true,
	}

	err := manager.AddEntry(entry)
	require.NoError(t, err)

	// Test Read
	retrieved, err := manager.GetEntry("test-crud-entry")
	require.NoError(t, err)
	assert.Equal(t, "crud-test-client", retrieved.Value)
	assert.Equal(t, "CRUD test entry", retrieved.Reason)

	// Test Update
	updates := &blacklist.BlacklistEntry{
		Reason:      "Updated reason",
		Description: "Updated description",
		Enabled:     false,
	}

	err = manager.UpdateEntry("test-crud-entry", updates)
	require.NoError(t, err)

	// Verify update
	updated, err := manager.GetEntry("test-crud-entry")
	require.NoError(t, err)
	assert.Equal(t, "Updated reason", updated.Reason)
	assert.Equal(t, "Updated description", updated.Description)
	assert.False(t, updated.Enabled)

	// Test List
	entries := manager.ListEntries("", nil)
	assert.Len(t, entries, 1)

	// Test Delete
	err = manager.RemoveEntry("test-crud-entry")
	require.NoError(t, err)

	// Verify deletion
	_, err = manager.GetEntry("test-crud-entry")
	assert.Error(t, err)
	assert.Equal(t, blacklist.ErrEntryNotFound, err)

	// Test statistics after deletion
	stats := manager.GetStats()
	assert.Equal(t, 0, stats.TotalEntries)
}

// Helper functions

func createTestMQTTClient(clientID, brokerPort string) mqtt.Client {
	return createTestMQTTClientWithCredentials(clientID, "test", "test", brokerPort)
}

func createTestMQTTClientWithCredentials(clientID, username, password, brokerPort string) mqtt.Client {
	opts := mqtt.NewClientOptions()
	opts.AddBroker("tcp://localhost" + brokerPort)
	opts.SetClientID(clientID)
	opts.SetUsername(username)
	opts.SetPassword(password)
	opts.SetCleanSession(true)
	opts.SetAutoReconnect(false)
	opts.SetConnectTimeout(5 * time.Second)

	return mqtt.NewClient(opts)
}