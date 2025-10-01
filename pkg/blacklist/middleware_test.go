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

func TestDefaultMiddlewareConfig(t *testing.T) {
	config := DefaultMiddlewareConfig()
	assert.NotNil(t, config)
	assert.True(t, config.EnableClientIDCheck)
	assert.True(t, config.EnableUsernameCheck)
	assert.True(t, config.EnableIPAddressCheck)
	assert.True(t, config.EnableTopicCheck)
	assert.True(t, config.LogBlocks)
	assert.Equal(t, time.Hour*24, config.BlockTimeout)
	assert.Equal(t, time.Hour, config.CleanupInterval)
	assert.Equal(t, 1000, config.MaxRecentBlocks)
	assert.Equal(t, ActionDeny, config.DefaultAction)
	assert.Equal(t, ActionLog, config.UnknownClientAction)
}

func TestNewBlacklistMiddleware(t *testing.T) {
	manager := NewBlacklistManager()
	config := DefaultMiddlewareConfig()

	middleware := NewBlacklistMiddleware(manager, config)
	assert.NotNil(t, middleware)
	assert.Equal(t, manager, middleware.manager)
	assert.Equal(t, config, middleware.config)
}

func TestNewBlacklistMiddleware_NilConfig(t *testing.T) {
	manager := NewBlacklistManager()

	middleware := NewBlacklistMiddleware(manager, nil)
	assert.NotNil(t, middleware)
	assert.Equal(t, manager, middleware.manager)
	assert.NotNil(t, middleware.config)
	// Should use default config
	assert.True(t, middleware.config.EnableClientIDCheck)
}

func TestBlacklistMiddleware_CheckClientConnection(t *testing.T) {
	manager := NewBlacklistManager()
	config := DefaultMiddlewareConfig()
	middleware := NewBlacklistMiddleware(manager, config)

	// Add a blocked client
	entry := &BlacklistEntry{
		ID:      "blocked-client-1",
		Type:    ClientIDBlacklist,
		Value:   "malicious-client",
		Action:  ActionDeny,
		Reason:  "Suspicious activity",
		Enabled: true,
	}
	err := manager.AddEntry(entry)
	require.NoError(t, err)

	testCases := []struct {
		name         string
		clientID     string
		username     string
		ipAddress    string
		protocol     string
		expectAllowed bool
		expectReason string
	}{
		{
			name:         "allowed connection",
			clientID:     "good-client",
			username:     "gooduser",
			ipAddress:    "192.168.1.10",
			protocol:     "mqtt",
			expectAllowed: true,
		},
		{
			name:         "blocked connection",
			clientID:     "malicious-client",
			username:     "gooduser",
			ipAddress:    "192.168.1.10",
			protocol:     "mqtt",
			expectAllowed: false,
			expectReason: "Blocked by clientid blacklist: Suspicious activity",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			allowed, reason, err := middleware.CheckClientConnection(tc.clientID, tc.username, tc.ipAddress, tc.protocol)
			require.NoError(t, err)
			assert.Equal(t, tc.expectAllowed, allowed)
			if !tc.expectAllowed {
				assert.Equal(t, tc.expectReason, reason)
			}
		})
	}
}

func TestBlacklistMiddleware_CheckClientConnection_NilManager(t *testing.T) {
	config := DefaultMiddlewareConfig()
	middleware := NewBlacklistMiddleware(nil, config)

	// Should allow when manager is nil
	allowed, reason, err := middleware.CheckClientConnection("test", "user", "192.168.1.1", "mqtt")
	require.NoError(t, err)
	assert.True(t, allowed)
	assert.Empty(t, reason)
}

func TestBlacklistMiddleware_CheckTopicAccess(t *testing.T) {
	manager := NewBlacklistManager()
	config := DefaultMiddlewareConfig()
	middleware := NewBlacklistMiddleware(manager, config)

	// Add a blocked topic
	entry := &BlacklistEntry{
		ID:      "blocked-topic-1",
		Type:    TopicBlacklist,
		Value:   "forbidden/topic",
		Action:  ActionDeny,
		Reason:  "Restricted access",
		Enabled: true,
	}
	err := manager.AddEntry(entry)
	require.NoError(t, err)

	testCases := []struct {
		name         string
		topic        string
		action       string
		expectAllowed bool
	}{
		{
			name:         "allowed topic",
			topic:        "sensor/temperature",
			action:       "publish",
			expectAllowed: true,
		},
		{
			name:         "blocked topic",
			topic:        "forbidden/topic",
			action:       "publish",
			expectAllowed: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			allowed, reason, err := middleware.CheckTopicAccess("client1", "user1", "192.168.1.1", tc.topic, tc.action)
			require.NoError(t, err)
			assert.Equal(t, tc.expectAllowed, allowed)
			if !tc.expectAllowed {
				assert.Contains(t, reason, "Topic access blocked by blacklist")
			}
		})
	}
}

func TestBlacklistMiddleware_CheckTopicAccess_Disabled(t *testing.T) {
	manager := NewBlacklistManager()
	config := DefaultMiddlewareConfig()
	config.EnableTopicCheck = false
	middleware := NewBlacklistMiddleware(manager, config)

	// Add a blocked topic
	entry := &BlacklistEntry{
		ID:      "blocked-topic-1",
		Type:    TopicBlacklist,
		Value:   "forbidden/topic",
		Action:  ActionDeny,
		Reason:  "Restricted access",
		Enabled: true,
	}
	err := manager.AddEntry(entry)
	require.NoError(t, err)

	// Should allow even blocked topic when topic check is disabled
	allowed, reason, err := middleware.CheckTopicAccess("client1", "user1", "192.168.1.1", "forbidden/topic", "publish")
	require.NoError(t, err)
	assert.True(t, allowed)
	assert.Empty(t, reason)
}

func TestBlacklistMiddleware_CheckPublishAccess(t *testing.T) {
	manager := NewBlacklistManager()
	config := DefaultMiddlewareConfig()
	middleware := NewBlacklistMiddleware(manager, config)

	allowed, reason, err := middleware.CheckPublishAccess("client1", "user1", "192.168.1.1", "sensor/temperature")
	require.NoError(t, err)
	assert.True(t, allowed)
	assert.Empty(t, reason)
}

func TestBlacklistMiddleware_CheckSubscribeAccess(t *testing.T) {
	manager := NewBlacklistManager()
	config := DefaultMiddlewareConfig()
	middleware := NewBlacklistMiddleware(manager, config)

	allowed, reason, err := middleware.CheckSubscribeAccess("client1", "user1", "192.168.1.1", "sensor/+")
	require.NoError(t, err)
	assert.True(t, allowed)
	assert.Empty(t, reason)
}

func TestBlacklistMiddleware_AddTemporaryBlock(t *testing.T) {
	manager := NewBlacklistManager()
	config := DefaultMiddlewareConfig()
	middleware := NewBlacklistMiddleware(manager, config)

	err := middleware.AddTemporaryBlock(ClientIDBlacklist, "temp-client", "Temporary block for testing", time.Hour)
	require.NoError(t, err)

	// Verify the temporary block was added
	entries := manager.ListEntries(ClientIDBlacklist, nil)
	found := false
	for _, entry := range entries {
		if entry.Value == "temp-client" {
			found = true
			assert.True(t, entry.Enabled)
			assert.NotNil(t, entry.ExpiresAt)
			assert.Equal(t, "Temporary block for testing", entry.Reason)
			assert.True(t, entry.Metadata["temporary"].(bool))
			break
		}
	}
	assert.True(t, found, "Temporary block entry not found")
}

func TestBlacklistMiddleware_AddTemporaryBlock_NilManager(t *testing.T) {
	config := DefaultMiddlewareConfig()
	middleware := NewBlacklistMiddleware(nil, config)

	err := middleware.AddTemporaryBlock(ClientIDBlacklist, "temp-client", "Temporary block", time.Hour)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "blacklist manager not available")
}

func TestBlacklistMiddleware_BlockClientID(t *testing.T) {
	manager := NewBlacklistManager()
	config := DefaultMiddlewareConfig()
	middleware := NewBlacklistMiddleware(manager, config)

	err := middleware.BlockClientID("bad-client", "Detected malicious behavior", time.Hour*24)
	require.NoError(t, err)

	// Verify the client is now blocked
	allowed, reason, err := middleware.CheckClientConnection("bad-client", "user", "192.168.1.1", "mqtt")
	require.NoError(t, err)
	assert.False(t, allowed)
	assert.Contains(t, reason, "Detected malicious behavior")
}

func TestBlacklistMiddleware_BlockUsername(t *testing.T) {
	manager := NewBlacklistManager()
	config := DefaultMiddlewareConfig()
	middleware := NewBlacklistMiddleware(manager, config)

	err := middleware.BlockUsername("bad-user", "Suspicious login attempts", time.Hour*12)
	require.NoError(t, err)

	// Verify the username is now blocked
	allowed, reason, err := middleware.CheckClientConnection("client", "bad-user", "192.168.1.1", "mqtt")
	require.NoError(t, err)
	assert.False(t, allowed)
	assert.Contains(t, reason, "Suspicious login attempts")
}

func TestBlacklistMiddleware_BlockIPAddress(t *testing.T) {
	manager := NewBlacklistManager()
	config := DefaultMiddlewareConfig()
	middleware := NewBlacklistMiddleware(manager, config)

	err := middleware.BlockIPAddress("192.168.1.100", "Multiple failed authentication attempts", time.Hour*6)
	require.NoError(t, err)

	// Verify the IP is now blocked
	allowed, reason, err := middleware.CheckClientConnection("client", "user", "192.168.1.100", "mqtt")
	require.NoError(t, err)
	assert.False(t, allowed)
	assert.Contains(t, reason, "Multiple failed authentication attempts")
}

func TestBlacklistMiddleware_BlockTopic(t *testing.T) {
	manager := NewBlacklistManager()
	config := DefaultMiddlewareConfig()
	middleware := NewBlacklistMiddleware(manager, config)

	err := middleware.BlockTopic("admin/secrets", "Contains sensitive information", time.Hour*48)
	require.NoError(t, err)

	// Verify the topic is now blocked
	allowed, reason, err := middleware.CheckTopicAccess("client", "user", "192.168.1.1", "admin/secrets", "publish")
	require.NoError(t, err)
	assert.False(t, allowed)
	assert.Contains(t, reason, "Contains sensitive information")
}

func TestBlacklistMiddleware_IsIPAddressValid(t *testing.T) {
	manager := NewBlacklistManager()
	config := DefaultMiddlewareConfig()
	middleware := NewBlacklistMiddleware(manager, config)

	testCases := []struct {
		name      string
		ipAddress string
		expected  bool
	}{
		{"valid IPv4", "192.168.1.1", true},
		{"valid IPv6", "2001:db8::1", true},
		{"invalid IP", "not.an.ip", false},
		{"empty string", "", false},
		{"localhost", "127.0.0.1", true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := middleware.IsIPAddressValid(tc.ipAddress)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestBlacklistMiddleware_GetManager(t *testing.T) {
	manager := NewBlacklistManager()
	config := DefaultMiddlewareConfig()
	middleware := NewBlacklistMiddleware(manager, config)

	assert.Equal(t, manager, middleware.GetManager())
}

func TestBlacklistMiddleware_UpdateConfig(t *testing.T) {
	manager := NewBlacklistManager()
	config := DefaultMiddlewareConfig()
	middleware := NewBlacklistMiddleware(manager, config)

	newConfig := &MiddlewareConfig{
		EnableClientIDCheck:  false,
		EnableUsernameCheck:  false,
		EnableIPAddressCheck: false,
		EnableTopicCheck:     false,
		LogBlocks:           false,
	}

	middleware.UpdateConfig(newConfig)
	assert.Equal(t, newConfig, middleware.config)
}

func TestBlacklistMiddleware_UpdateConfig_Nil(t *testing.T) {
	manager := NewBlacklistManager()
	config := DefaultMiddlewareConfig()
	middleware := NewBlacklistMiddleware(manager, config)

	originalConfig := middleware.config
	middleware.UpdateConfig(nil)
	assert.Equal(t, originalConfig, middleware.config)
}

func TestBlacklistMiddleware_GetConfig(t *testing.T) {
	manager := NewBlacklistManager()
	config := DefaultMiddlewareConfig()
	middleware := NewBlacklistMiddleware(manager, config)

	retrievedConfig := middleware.GetConfig()
	assert.NotNil(t, retrievedConfig)
	assert.Equal(t, config.EnableClientIDCheck, retrievedConfig.EnableClientIDCheck)
	assert.Equal(t, config.EnableUsernameCheck, retrievedConfig.EnableUsernameCheck)
	assert.Equal(t, config.EnableIPAddressCheck, retrievedConfig.EnableIPAddressCheck)
	assert.Equal(t, config.EnableTopicCheck, retrievedConfig.EnableTopicCheck)

	// Verify it's a copy (modifying returned config shouldn't affect original)
	retrievedConfig.EnableClientIDCheck = false
	assert.True(t, middleware.config.EnableClientIDCheck)
}

func TestBlacklistMiddleware_IsEnabled(t *testing.T) {
	manager := NewBlacklistManager()

	testCases := []struct {
		name     string
		config   *MiddlewareConfig
		expected bool
	}{
		{
			name:     "all checks enabled",
			config:   DefaultMiddlewareConfig(),
			expected: true,
		},
		{
			name: "only client ID check enabled",
			config: &MiddlewareConfig{
				EnableClientIDCheck:  true,
				EnableUsernameCheck:  false,
				EnableIPAddressCheck: false,
				EnableTopicCheck:     false,
			},
			expected: true,
		},
		{
			name: "all checks disabled",
			config: &MiddlewareConfig{
				EnableClientIDCheck:  false,
				EnableUsernameCheck:  false,
				EnableIPAddressCheck: false,
				EnableTopicCheck:     false,
			},
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			middleware := NewBlacklistMiddleware(manager, tc.config)
			result := middleware.IsEnabled()
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestBlacklistMiddleware_IsEnabled_NilManager(t *testing.T) {
	config := DefaultMiddlewareConfig()
	middleware := NewBlacklistMiddleware(nil, config)

	result := middleware.IsEnabled()
	assert.False(t, result)
}

func TestBlacklistMiddleware_GetStats(t *testing.T) {
	manager := NewBlacklistManager()
	config := DefaultMiddlewareConfig()
	middleware := NewBlacklistMiddleware(manager, config)

	// Add some entries to generate stats
	entry := &BlacklistEntry{
		ID:      "test-entry",
		Type:    ClientIDBlacklist,
		Value:   "test-client",
		Action:  ActionDeny,
		Enabled: true,
	}
	err := manager.AddEntry(entry)
	require.NoError(t, err)

	stats := middleware.GetStats()
	assert.Equal(t, 1, stats.TotalEntries)
	assert.Equal(t, 1, stats.EntriesByType[ClientIDBlacklist])
	assert.Equal(t, 1, stats.EntriesByAction[ActionDeny])
}

func TestBlacklistMiddleware_GetStats_NilManager(t *testing.T) {
	config := DefaultMiddlewareConfig()
	middleware := NewBlacklistMiddleware(nil, config)

	stats := middleware.GetStats()
	assert.Equal(t, 0, stats.TotalEntries)
	assert.Equal(t, 0, len(stats.EntriesByType))
}

func TestBlacklistMiddleware_ValidateEntry(t *testing.T) {
	manager := NewBlacklistManager()
	config := DefaultMiddlewareConfig()
	middleware := NewBlacklistMiddleware(manager, config)

	testCases := []struct {
		name      string
		entry     *BlacklistEntry
		expectErr bool
		errMsg    string
	}{
		{
			name: "valid entry",
			entry: &BlacklistEntry{
				ID:     "valid-entry",
				Type:   ClientIDBlacklist,
				Value:  "test-client",
				Action: ActionDeny,
			},
			expectErr: false,
		},
		{
			name:      "nil entry",
			entry:     nil,
			expectErr: true,
			errMsg:    "entry cannot be nil",
		},
		{
			name: "invalid IP address",
			entry: &BlacklistEntry{
				ID:     "invalid-ip",
				Type:   IPAddressBlacklist,
				Value:  "not.an.ip",
				Action: ActionDeny,
			},
			expectErr: true,
			errMsg:    "invalid IP address or CIDR",
		},
		{
			name: "valid CIDR",
			entry: &BlacklistEntry{
				ID:     "valid-cidr",
				Type:   IPAddressBlacklist,
				Value:  "192.168.1.0/24",
				Action: ActionDeny,
			},
			expectErr: false,
		},
		{
			name: "empty topic",
			entry: &BlacklistEntry{
				ID:     "empty-topic",
				Type:   TopicBlacklist,
				Value:  "",
				Action: ActionDeny,
			},
			expectErr: true,
			errMsg:    "topic cannot be empty",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := middleware.ValidateEntry(tc.entry)
			if tc.expectErr {
				require.Error(t, err)
				if tc.errMsg != "" {
					assert.Contains(t, err.Error(), tc.errMsg)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestBlacklistMiddleware_ExportImportBlacklist(t *testing.T) {
	manager := NewBlacklistManager()
	config := DefaultMiddlewareConfig()
	middleware := NewBlacklistMiddleware(manager, config)

	// Add some entries
	entries := []*BlacklistEntry{
		{
			ID:      "export-test-1",
			Type:    ClientIDBlacklist,
			Value:   "client1",
			Action:  ActionDeny,
			Enabled: true,
		},
		{
			ID:      "export-test-2",
			Type:    UsernameBlacklist,
			Value:   "user1",
			Action:  ActionLog,
			Enabled: false,
		},
	}

	for _, entry := range entries {
		err := manager.AddEntry(entry)
		require.NoError(t, err)
	}

	// Export blacklist
	exported := middleware.ExportBlacklist()
	assert.Len(t, exported, 2)

	// Create new middleware and import
	newManager := NewBlacklistManager()
	newMiddleware := NewBlacklistMiddleware(newManager, config)

	err := newMiddleware.ImportBlacklist(exported, false)
	require.NoError(t, err)

	// Verify import
	importedEntries := newManager.ListEntries("", nil)
	assert.Len(t, importedEntries, 2)

	// Test import with overwrite
	err = newMiddleware.ImportBlacklist(exported, true)
	require.NoError(t, err)

	// Should still have 2 entries (overwritten)
	importedEntries = newManager.ListEntries("", nil)
	assert.Len(t, importedEntries, 2)
}

func TestBlacklistMiddleware_ExportBlacklist_NilManager(t *testing.T) {
	config := DefaultMiddlewareConfig()
	middleware := NewBlacklistMiddleware(nil, config)

	exported := middleware.ExportBlacklist()
	assert.Nil(t, exported)
}

func TestBlacklistMiddleware_ImportBlacklist_NilManager(t *testing.T) {
	config := DefaultMiddlewareConfig()
	middleware := NewBlacklistMiddleware(nil, config)

	entries := []*BlacklistEntry{
		{
			ID:     "test",
			Type:   ClientIDBlacklist,
			Value:  "client",
			Action: ActionDeny,
		},
	}

	err := middleware.ImportBlacklist(entries, false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "blacklist manager not available")
}