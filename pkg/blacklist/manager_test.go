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

package blacklist

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewBlacklistManager(t *testing.T) {
	manager := NewBlacklistManager()
	assert.NotNil(t, manager)
	assert.NotNil(t, manager.entries)
	assert.NotNil(t, manager.clientIDEntries)
	assert.NotNil(t, manager.usernameEntries)
	assert.NotNil(t, manager.ipAddressEntries)
	assert.NotNil(t, manager.topicEntries)
	assert.NotNil(t, manager.patternEntries)
}

func TestBlacklistManager_AddEntry(t *testing.T) {
	manager := NewBlacklistManager()

	testCases := []struct {
		name      string
		entry     *BlacklistEntry
		expectErr bool
		errMsg    string
	}{
		{
			name: "valid client ID entry",
			entry: &BlacklistEntry{
				ID:     "test-client-1",
				Type:   ClientIDBlacklist,
				Value:  "malicious-client",
				Action: ActionDeny,
			},
			expectErr: false,
		},
		{
			name: "valid username entry",
			entry: &BlacklistEntry{
				ID:     "test-user-1",
				Type:   UsernameBlacklist,
				Value:  "baduser",
				Action: ActionDeny,
			},
			expectErr: false,
		},
		{
			name: "valid IP address entry",
			entry: &BlacklistEntry{
				ID:     "test-ip-1",
				Type:   IPAddressBlacklist,
				Value:  "192.168.1.100",
				Action: ActionDeny,
			},
			expectErr: false,
		},
		{
			name: "valid topic entry",
			entry: &BlacklistEntry{
				ID:     "test-topic-1",
				Type:   TopicBlacklist,
				Value:  "forbidden/topic",
				Action: ActionDeny,
			},
			expectErr: false,
		},
		{
			name: "valid pattern entry",
			entry: &BlacklistEntry{
				ID:      "test-pattern-1",
				Type:    ClientIDBlacklist,
				Pattern: "bot_.*",
				Action:  ActionDeny,
			},
			expectErr: false,
		},
		{
			name: "empty ID",
			entry: &BlacklistEntry{
				Type:   ClientIDBlacklist,
				Value:  "test",
				Action: ActionDeny,
			},
			expectErr: true,
			errMsg:    "entry ID is required",
		},
		{
			name: "invalid type",
			entry: &BlacklistEntry{
				ID:     "test-invalid",
				Type:   BlacklistType("invalid"),
				Value:  "test",
				Action: ActionDeny,
			},
			expectErr: true,
		},
		{
			name: "invalid action",
			entry: &BlacklistEntry{
				ID:     "test-invalid-action",
				Type:   ClientIDBlacklist,
				Value:  "test",
				Action: BlacklistAction("invalid"),
			},
			expectErr: true,
		},
		{
			name: "no value or pattern",
			entry: &BlacklistEntry{
				ID:     "test-no-value",
				Type:   ClientIDBlacklist,
				Action: ActionDeny,
			},
			expectErr: true,
			errMsg:    "either value or pattern is required",
		},
		{
			name: "invalid pattern",
			entry: &BlacklistEntry{
				ID:      "test-invalid-pattern",
				Type:    ClientIDBlacklist,
				Pattern: "[invalid",
				Action:  ActionDeny,
			},
			expectErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := manager.AddEntry(tc.entry)
			if tc.expectErr {
				require.Error(t, err)
				if tc.errMsg != "" {
					assert.Contains(t, err.Error(), tc.errMsg)
				}
			} else {
				require.NoError(t, err)
				// Verify entry was added
				retrieved, err := manager.GetEntry(tc.entry.ID)
				require.NoError(t, err)
				assert.Equal(t, tc.entry.ID, retrieved.ID)
				assert.Equal(t, tc.entry.Type, retrieved.Type)
				assert.Equal(t, tc.entry.Value, retrieved.Value)
				assert.Equal(t, tc.entry.Pattern, retrieved.Pattern)
				assert.Equal(t, tc.entry.Action, retrieved.Action)
			}
		})
	}
}

func TestBlacklistManager_AddDuplicateEntry(t *testing.T) {
	manager := NewBlacklistManager()

	entry := &BlacklistEntry{
		ID:     "duplicate-test",
		Type:   ClientIDBlacklist,
		Value:  "test-client",
		Action: ActionDeny,
	}

	// Add first time - should succeed
	err := manager.AddEntry(entry)
	require.NoError(t, err)

	// Add second time - should fail
	err = manager.AddEntry(entry)
	require.Error(t, err)
	assert.Equal(t, ErrEntryAlreadyExists, err)
}

func TestBlacklistManager_UpdateEntry(t *testing.T) {
	manager := NewBlacklistManager()

	// Add initial entry
	entry := &BlacklistEntry{
		ID:          "update-test",
		Type:        ClientIDBlacklist,
		Value:       "original-value",
		Action:      ActionDeny,
		Reason:      "original reason",
		Description: "original description",
		Enabled:     true,
	}
	err := manager.AddEntry(entry)
	require.NoError(t, err)

	// Update entry
	updates := &BlacklistEntry{
		ID:          "update-test",
		Value:       "updated-value",
		Action:      ActionLog,
		Reason:      "updated reason",
		Description: "updated description",
		Enabled:     false,
	}

	err = manager.UpdateEntry("update-test", updates)
	require.NoError(t, err)

	// Verify updates
	updated, err := manager.GetEntry("update-test")
	require.NoError(t, err)
	assert.Equal(t, "updated-value", updated.Value)
	assert.Equal(t, ActionLog, updated.Action)
	assert.Equal(t, "updated reason", updated.Reason)
	assert.Equal(t, "updated description", updated.Description)
	assert.False(t, updated.Enabled)
}

func TestBlacklistManager_UpdateNonexistentEntry(t *testing.T) {
	manager := NewBlacklistManager()

	updates := &BlacklistEntry{
		Value: "test",
	}

	err := manager.UpdateEntry("nonexistent", updates)
	require.Error(t, err)
	assert.Equal(t, ErrEntryNotFound, err)
}

func TestBlacklistManager_RemoveEntry(t *testing.T) {
	manager := NewBlacklistManager()

	// Add entry
	entry := &BlacklistEntry{
		ID:     "remove-test",
		Type:   ClientIDBlacklist,
		Value:  "test-client",
		Action: ActionDeny,
	}
	err := manager.AddEntry(entry)
	require.NoError(t, err)

	// Remove entry
	err = manager.RemoveEntry("remove-test")
	require.NoError(t, err)

	// Verify entry is gone
	_, err = manager.GetEntry("remove-test")
	require.Error(t, err)
	assert.Equal(t, ErrEntryNotFound, err)
}

func TestBlacklistManager_RemoveNonexistentEntry(t *testing.T) {
	manager := NewBlacklistManager()

	err := manager.RemoveEntry("nonexistent")
	require.Error(t, err)
	assert.Equal(t, ErrEntryNotFound, err)
}

func TestBlacklistManager_ListEntries(t *testing.T) {
	manager := NewBlacklistManager()

	// Add test entries
	entries := []*BlacklistEntry{
		{
			ID:      "client-1",
			Type:    ClientIDBlacklist,
			Value:   "client1",
			Action:  ActionDeny,
			Enabled: true,
		},
		{
			ID:      "client-2",
			Type:    ClientIDBlacklist,
			Value:   "client2",
			Action:  ActionDeny,
			Enabled: false,
		},
		{
			ID:      "user-1",
			Type:    UsernameBlacklist,
			Value:   "user1",
			Action:  ActionDeny,
			Enabled: true,
		},
	}

	for _, entry := range entries {
		err := manager.AddEntry(entry)
		require.NoError(t, err)
	}

	// Test listing all entries
	allEntries := manager.ListEntries("", nil)
	assert.Len(t, allEntries, 3)

	// Test filtering by type
	clientEntries := manager.ListEntries(ClientIDBlacklist, nil)
	assert.Len(t, clientEntries, 2)

	// Test filtering by enabled status
	enabledTrue := true
	enabledEntries := manager.ListEntries("", &enabledTrue)
	assert.Len(t, enabledEntries, 2)

	enabledFalse := false
	disabledEntries := manager.ListEntries("", &enabledFalse)
	assert.Len(t, disabledEntries, 1)
}

func TestBlacklistManager_CheckClientConnection(t *testing.T) {
	manager := NewBlacklistManager()

	// Add blacklist entries
	entries := []*BlacklistEntry{
		{
			ID:      "blocked-client",
			Type:    ClientIDBlacklist,
			Value:   "malicious-client",
			Action:  ActionDeny,
			Enabled: true,
		},
		{
			ID:      "blocked-user",
			Type:    UsernameBlacklist,
			Value:   "baduser",
			Action:  ActionDeny,
			Enabled: true,
		},
		{
			ID:      "blocked-ip",
			Type:    IPAddressBlacklist,
			Value:   "192.168.1.100",
			Action:  ActionDeny,
			Enabled: true,
		},
		{
			ID:      "disabled-entry",
			Type:    ClientIDBlacklist,
			Value:   "disabled-client",
			Action:  ActionDeny,
			Enabled: false,
		},
	}

	for _, entry := range entries {
		err := manager.AddEntry(entry)
		require.NoError(t, err)
	}

	testCases := []struct {
		name         string
		clientInfo   ClientInfo
		expectAllowed bool
	}{
		{
			name: "allowed connection",
			clientInfo: ClientInfo{
				ClientID:  "good-client",
				Username:  "gooduser",
				IPAddress: "192.168.1.10",
			},
			expectAllowed: true,
		},
		{
			name: "blocked by client ID",
			clientInfo: ClientInfo{
				ClientID:  "malicious-client",
				Username:  "gooduser",
				IPAddress: "192.168.1.10",
			},
			expectAllowed: false,
		},
		{
			name: "blocked by username",
			clientInfo: ClientInfo{
				ClientID:  "good-client",
				Username:  "baduser",
				IPAddress: "192.168.1.10",
			},
			expectAllowed: false,
		},
		{
			name: "blocked by IP address",
			clientInfo: ClientInfo{
				ClientID:  "good-client",
				Username:  "gooduser",
				IPAddress: "192.168.1.100",
			},
			expectAllowed: false,
		},
		{
			name: "disabled entry should not block",
			clientInfo: ClientInfo{
				ClientID:  "disabled-client",
				Username:  "gooduser",
				IPAddress: "192.168.1.10",
			},
			expectAllowed: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			allowed, entry, err := manager.CheckClientConnection(tc.clientInfo)
			require.NoError(t, err)
			assert.Equal(t, tc.expectAllowed, allowed)
			if !tc.expectAllowed {
				assert.NotNil(t, entry)
			}
		})
	}
}

func TestBlacklistManager_CheckClientPatterns(t *testing.T) {
	manager := NewBlacklistManager()

	// Add pattern-based entries
	entries := []*BlacklistEntry{
		{
			ID:      "bot-pattern",
			Type:    ClientIDBlacklist,
			Pattern: "bot_.*",
			Action:  ActionDeny,
			Enabled: true,
		},
		{
			ID:      "test-user-pattern",
			Type:    UsernameBlacklist,
			Pattern: "test.*",
			Action:  ActionDeny,
			Enabled: true,
		},
	}

	for _, entry := range entries {
		err := manager.AddEntry(entry)
		require.NoError(t, err)
	}

	testCases := []struct {
		name         string
		clientInfo   ClientInfo
		expectAllowed bool
	}{
		{
			name: "client ID matches bot pattern",
			clientInfo: ClientInfo{
				ClientID:  "bot_crawler",
				Username:  "gooduser",
				IPAddress: "192.168.1.10",
			},
			expectAllowed: false,
		},
		{
			name: "username matches test pattern",
			clientInfo: ClientInfo{
				ClientID:  "good-client",
				Username:  "testuser123",
				IPAddress: "192.168.1.10",
			},
			expectAllowed: false,
		},
		{
			name: "no pattern match",
			clientInfo: ClientInfo{
				ClientID:  "legitimate-client",
				Username:  "realuser",
				IPAddress: "192.168.1.10",
			},
			expectAllowed: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			allowed, entry, err := manager.CheckClientConnection(tc.clientInfo)
			require.NoError(t, err)
			assert.Equal(t, tc.expectAllowed, allowed)
			if !tc.expectAllowed {
				assert.NotNil(t, entry)
			}
		})
	}
}

func TestBlacklistManager_CheckTopicAccess(t *testing.T) {
	manager := NewBlacklistManager()

	// Add topic blacklist entries
	entries := []*BlacklistEntry{
		{
			ID:      "forbidden-topic",
			Type:    TopicBlacklist,
			Value:   "secret/data",
			Action:  ActionDeny,
			Enabled: true,
		},
		{
			ID:      "pattern-topic",
			Type:    TopicBlacklist,
			Pattern: "admin/.*",
			Action:  ActionDeny,
			Enabled: true,
		},
	}

	for _, entry := range entries {
		err := manager.AddEntry(entry)
		require.NoError(t, err)
	}

	testCases := []struct {
		name         string
		topicAccess  TopicAccess
		expectAllowed bool
	}{
		{
			name: "allowed topic",
			topicAccess: TopicAccess{
				Topic:  "sensor/temperature",
				Action: "publish",
			},
			expectAllowed: true,
		},
		{
			name: "blocked exact topic",
			topicAccess: TopicAccess{
				Topic:  "secret/data",
				Action: "publish",
			},
			expectAllowed: false,
		},
		{
			name: "blocked by pattern",
			topicAccess: TopicAccess{
				Topic:  "admin/users",
				Action: "subscribe",
			},
			expectAllowed: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			allowed, entry, err := manager.CheckTopicAccess(tc.topicAccess)
			require.NoError(t, err)
			assert.Equal(t, tc.expectAllowed, allowed)
			if !tc.expectAllowed {
				assert.NotNil(t, entry)
			}
		})
	}
}

func TestBlacklistManager_CleanupExpiredEntries(t *testing.T) {
	manager := NewBlacklistManager()

	// Add entries with different expiration times
	now := time.Now()
	past := now.Add(-time.Hour)
	future := now.Add(time.Hour)

	entries := []*BlacklistEntry{
		{
			ID:        "expired-entry",
			Type:      ClientIDBlacklist,
			Value:     "expired-client",
			Action:    ActionDeny,
			Enabled:   true,
			ExpiresAt: &past,
		},
		{
			ID:        "valid-entry",
			Type:      ClientIDBlacklist,
			Value:     "valid-client",
			Action:    ActionDeny,
			Enabled:   true,
			ExpiresAt: &future,
		},
		{
			ID:      "no-expiry-entry",
			Type:    ClientIDBlacklist,
			Value:   "permanent-client",
			Action:  ActionDeny,
			Enabled: true,
		},
	}

	for _, entry := range entries {
		err := manager.AddEntry(entry)
		require.NoError(t, err)
	}

	// Verify all entries exist before cleanup
	// Note: ListEntries filters expired entries, so we'll only see 2 valid entries
	allEntries := manager.ListEntries("", nil)
	assert.Len(t, allEntries, 2) // Only non-expired entries

	// Check that we can see all entries in the entries map (including expired)
	assert.Len(t, manager.entries, 3)

	// Cleanup expired entries
	removedCount := manager.CleanupExpiredEntries()
	assert.Equal(t, 1, removedCount)

	// Verify only non-expired entries remain
	remainingEntries := manager.ListEntries("", nil)
	assert.Len(t, remainingEntries, 2)

	// Verify expired entry is gone
	_, err := manager.GetEntry("expired-entry")
	require.Error(t, err)
	assert.Equal(t, ErrEntryNotFound, err)
}

func TestBlacklistManager_GetStats(t *testing.T) {
	manager := NewBlacklistManager()

	// Add various entries
	entries := []*BlacklistEntry{
		{
			ID:      "client-1",
			Type:    ClientIDBlacklist,
			Value:   "client1",
			Action:  ActionDeny,
			Enabled: true,
		},
		{
			ID:      "client-2",
			Type:    ClientIDBlacklist,
			Value:   "client2",
			Action:  ActionLog,
			Enabled: true,
		},
		{
			ID:      "user-1",
			Type:    UsernameBlacklist,
			Value:   "user1",
			Action:  ActionDeny,
			Enabled: true,
		},
	}

	for _, entry := range entries {
		err := manager.AddEntry(entry)
		require.NoError(t, err)
	}

	stats := manager.GetStats()
	assert.Equal(t, 3, stats.TotalEntries)
	assert.Equal(t, 2, stats.EntriesByType[ClientIDBlacklist])
	assert.Equal(t, 1, stats.EntriesByType[UsernameBlacklist])
	assert.Equal(t, 2, stats.EntriesByAction[ActionDeny])
	assert.Equal(t, 1, stats.EntriesByAction[ActionLog])
}

func TestBlacklistManager_MQTTTopicFilterMatching(t *testing.T) {
	manager := NewBlacklistManager()

	testCases := []struct {
		name     string
		topic    string
		filter   string
		expected bool
	}{
		{
			name:     "exact match",
			topic:    "sensor/temperature",
			filter:   "sensor/temperature",
			expected: true,
		},
		{
			name:     "single level wildcard",
			topic:    "sensor/temperature",
			filter:   "sensor/+",
			expected: true,
		},
		{
			name:     "multi level wildcard",
			topic:    "sensor/temperature/room1",
			filter:   "sensor/#",
			expected: true,
		},
		{
			name:     "no match",
			topic:    "device/status",
			filter:   "sensor/+",
			expected: false,
		},
		{
			name:     "complex pattern match",
			topic:    "sensor/temp/room1/value",
			filter:   "sensor/+/+/value",
			expected: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := manager.matchMQTTTopicFilter(tc.topic, tc.filter)
			assert.Equal(t, tc.expected, result)
		})
	}
}