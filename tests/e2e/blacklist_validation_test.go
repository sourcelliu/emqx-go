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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/turtacn/emqx-go/pkg/blacklist"
	"github.com/turtacn/emqx-go/pkg/broker"
)

// TestBlacklistFunctionalityValidation validates that blacklist functionality works
func TestBlacklistFunctionalityValidation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start broker
	brokerInstance := broker.New("blacklist-validation-broker", nil)
	brokerInstance.SetupDefaultAuth()
	defer brokerInstance.Close()

	brokerPort := ":1950"
	go brokerInstance.StartServer(ctx, brokerPort)
	time.Sleep(300 * time.Millisecond)

	// Get blacklist middleware
	blacklistMiddleware := brokerInstance.GetBlacklistMiddleware()
	require.NotNil(t, blacklistMiddleware, "Blacklist middleware should be available")

	// Test 1: Verify initial state - no blacklist entries
	stats := blacklistMiddleware.GetStats()
	assert.Equal(t, 0, stats.TotalEntries, "Should start with no blacklist entries")

	// Test 2: Normal client should connect successfully
	normalClient := createTestMQTTClient("normal-client", brokerPort)
	token := normalClient.Connect()
	require.True(t, token.WaitTimeout(5*time.Second), "Normal client should connect")
	require.NoError(t, token.Error(), "Normal client should have no connection error")
	assert.True(t, normalClient.IsConnected(), "Normal client should be connected")
	normalClient.Disconnect(100)

	// Test 3: Add client to blacklist
	entry := &blacklist.BlacklistEntry{
		ID:      "test-blacklist-client",
		Type:    blacklist.ClientIDBlacklist,
		Value:   "blocked-client",
		Action:  blacklist.ActionDeny,
		Reason:  "Test blacklist entry",
		Enabled: true,
	}

	err := blacklistMiddleware.GetManager().AddEntry(entry)
	require.NoError(t, err, "Should be able to add blacklist entry")

	// Test 4: Verify blacklist entry was added
	stats = blacklistMiddleware.GetStats()
	assert.Equal(t, 1, stats.TotalEntries, "Should have one blacklist entry")
	assert.Equal(t, 1, stats.EntriesByType[blacklist.ClientIDBlacklist], "Should have one client ID entry")

	// Test 5: Retrieve the entry
	retrieved, err := blacklistMiddleware.GetManager().GetEntry("test-blacklist-client")
	require.NoError(t, err, "Should be able to retrieve entry")
	assert.Equal(t, "blocked-client", retrieved.Value, "Entry value should match")
	assert.Equal(t, "Test blacklist entry", retrieved.Reason, "Entry reason should match")

	// Test 6: Test blacklist functionality via middleware
	clientInfo := blacklist.ClientInfo{
		ClientID:  "blocked-client",
		Username:  "test",
		IPAddress: "127.0.0.1",
		Protocol:  "mqtt",
	}

	allowed, blockedEntry, err := blacklistMiddleware.GetManager().CheckClientConnection(clientInfo)
	require.NoError(t, err, "Blacklist check should not error")
	assert.False(t, allowed, "Blocked client should not be allowed")
	assert.NotNil(t, blockedEntry, "Should return the blocking entry")
	assert.Equal(t, "blocked-client", blockedEntry.Value, "Blocking entry should match")

	// Test 7: Test that good client is still allowed
	goodClientInfo := blacklist.ClientInfo{
		ClientID:  "good-client",
		Username:  "test",
		IPAddress: "127.0.0.1",
		Protocol:  "mqtt",
	}

	allowed, blockedEntry, err = blacklistMiddleware.GetManager().CheckClientConnection(goodClientInfo)
	require.NoError(t, err, "Blacklist check should not error")
	assert.True(t, allowed, "Good client should be allowed")
	assert.Nil(t, blockedEntry, "Should not return blocking entry for allowed client")

	// Test 8: Test pattern-based blacklist
	patternEntry := &blacklist.BlacklistEntry{
		ID:      "test-pattern-entry",
		Type:    blacklist.ClientIDBlacklist,
		Pattern: "bot_.*",
		Action:  blacklist.ActionDeny,
		Reason:  "Block bot clients",
		Enabled: true,
	}

	err = blacklistMiddleware.GetManager().AddEntry(patternEntry)
	require.NoError(t, err, "Should be able to add pattern entry")

	// Test pattern matching
	botClientInfo := blacklist.ClientInfo{
		ClientID:  "bot_crawler",
		Username:  "test",
		IPAddress: "127.0.0.1",
		Protocol:  "mqtt",
	}

	allowed, blockedEntry, err = blacklistMiddleware.GetManager().CheckClientConnection(botClientInfo)
	require.NoError(t, err, "Pattern check should not error")
	assert.False(t, allowed, "Bot client should be blocked by pattern")
	assert.NotNil(t, blockedEntry, "Should return the pattern blocking entry")

	// Test 9: Test temporary blocking
	err = blacklistMiddleware.BlockClientID("temp-client", "Temporary block", 2*time.Second)
	require.NoError(t, err, "Should be able to add temporary block")

	tempClientInfo := blacklist.ClientInfo{
		ClientID:  "temp-client",
		Username:  "test",
		IPAddress: "127.0.0.1",
		Protocol:  "mqtt",
	}

	// Should be blocked initially
	allowed, _, err = blacklistMiddleware.GetManager().CheckClientConnection(tempClientInfo)
	require.NoError(t, err)
	assert.False(t, allowed, "Temp client should be blocked initially")

	// Wait for expiration
	time.Sleep(3 * time.Second)

	// Should be allowed after expiration
	allowed, _, err = blacklistMiddleware.GetManager().CheckClientConnection(tempClientInfo)
	require.NoError(t, err)
	assert.True(t, allowed, "Temp client should be allowed after expiration")

	// Test 10: Test CRUD operations
	// Update
	updates := &blacklist.BlacklistEntry{
		Reason:      "Updated reason",
		Description: "Updated description",
	}

	err = blacklistMiddleware.GetManager().UpdateEntry("test-blacklist-client", updates)
	require.NoError(t, err, "Should be able to update entry")

	updated, err := blacklistMiddleware.GetManager().GetEntry("test-blacklist-client")
	require.NoError(t, err)
	assert.Equal(t, "Updated reason", updated.Reason, "Reason should be updated")

	// Delete
	err = blacklistMiddleware.GetManager().RemoveEntry("test-blacklist-client")
	require.NoError(t, err, "Should be able to remove entry")

	_, err = blacklistMiddleware.GetManager().GetEntry("test-blacklist-client")
	assert.Error(t, err, "Entry should be gone after deletion")
	assert.Equal(t, blacklist.ErrEntryNotFound, err, "Should get not found error")

	t.Log("✅ All blacklist functionality tests passed!")
}

// TestBlacklistUsernameValidation tests username blacklist
func TestBlacklistUsernameValidation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start broker
	brokerInstance := broker.New("blacklist-username-validation-broker", nil)
	brokerInstance.SetupDefaultAuth()
	defer brokerInstance.Close()

	brokerPort := ":1951"
	go brokerInstance.StartServer(ctx, brokerPort)
	time.Sleep(300 * time.Millisecond)

	// Get blacklist middleware
	blacklistMiddleware := brokerInstance.GetBlacklistMiddleware()
	require.NotNil(t, blacklistMiddleware)

	// Add username to blacklist
	entry := &blacklist.BlacklistEntry{
		ID:      "blocked-username",
		Type:    blacklist.UsernameBlacklist,
		Value:   "blockeduser",
		Action:  blacklist.ActionDeny,
		Reason:  "Account compromised",
		Enabled: true,
	}

	err := blacklistMiddleware.GetManager().AddEntry(entry)
	require.NoError(t, err)

	// Test blocked username via middleware check
	blockedUserInfo := blacklist.ClientInfo{
		ClientID:  "any-client",
		Username:  "blockeduser",
		IPAddress: "127.0.0.1",
		Protocol:  "mqtt",
	}

	allowed, blockedEntry, err := blacklistMiddleware.GetManager().CheckClientConnection(blockedUserInfo)
	require.NoError(t, err)
	assert.False(t, allowed, "Blocked username should not be allowed")
	assert.NotNil(t, blockedEntry, "Should return blocking entry")
	assert.Equal(t, blacklist.UsernameBlacklist, blockedEntry.Type, "Should be username blacklist")

	// Test good username
	goodUserInfo := blacklist.ClientInfo{
		ClientID:  "any-client",
		Username:  "gooduser",
		IPAddress: "127.0.0.1",
		Protocol:  "mqtt",
	}

	allowed, blockedEntry, err = blacklistMiddleware.GetManager().CheckClientConnection(goodUserInfo)
	require.NoError(t, err)
	assert.True(t, allowed, "Good username should be allowed")
	assert.Nil(t, blockedEntry, "Should not return blocking entry")

	t.Log("✅ Username blacklist validation passed!")
}

// TestBlacklistTopicValidation tests topic blacklist
func TestBlacklistTopicValidation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start broker
	brokerInstance := broker.New("blacklist-topic-validation-broker", nil)
	brokerInstance.SetupDefaultAuth()
	defer brokerInstance.Close()

	brokerPort := ":1952"
	go brokerInstance.StartServer(ctx, brokerPort)
	time.Sleep(300 * time.Millisecond)

	// Get blacklist middleware
	blacklistMiddleware := brokerInstance.GetBlacklistMiddleware()
	require.NotNil(t, blacklistMiddleware)

	// Add topic to blacklist
	entry := &blacklist.BlacklistEntry{
		ID:      "blocked-topic",
		Type:    blacklist.TopicBlacklist,
		Value:   "forbidden/topic",
		Action:  blacklist.ActionDeny,
		Reason:  "Sensitive data",
		Enabled: true,
	}

	err := blacklistMiddleware.GetManager().AddEntry(entry)
	require.NoError(t, err)

	// Test blocked topic
	blockedTopicAccess := blacklist.TopicAccess{
		Topic:  "forbidden/topic",
		Action: "publish",
		ClientInfo: blacklist.ClientInfo{
			ClientID:  "test-client",
			Username:  "test",
			IPAddress: "127.0.0.1",
		},
	}

	allowed, blockedEntry, err := blacklistMiddleware.GetManager().CheckTopicAccess(blockedTopicAccess)
	require.NoError(t, err)
	assert.False(t, allowed, "Blocked topic should not be allowed")
	assert.NotNil(t, blockedEntry, "Should return blocking entry")
	assert.Equal(t, blacklist.TopicBlacklist, blockedEntry.Type, "Should be topic blacklist")

	// Test allowed topic
	allowedTopicAccess := blacklist.TopicAccess{
		Topic:  "allowed/topic",
		Action: "publish",
		ClientInfo: blacklist.ClientInfo{
			ClientID:  "test-client",
			Username:  "test",
			IPAddress: "127.0.0.1",
		},
	}

	allowed, blockedEntry, err = blacklistMiddleware.GetManager().CheckTopicAccess(allowedTopicAccess)
	require.NoError(t, err)
	assert.True(t, allowed, "Allowed topic should be allowed")
	assert.Nil(t, blockedEntry, "Should not return blocking entry")

	t.Log("✅ Topic blacklist validation passed!")
}

