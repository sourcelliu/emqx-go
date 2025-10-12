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
	"fmt"
	"log"
	"net"
	"time"
)

// BlacklistMiddleware provides blacklist checking functionality for the broker
type BlacklistMiddleware struct {
	manager *BlacklistManager
	config  *MiddlewareConfig
}

// MiddlewareConfig configures the blacklist middleware behavior
type MiddlewareConfig struct {
	// Enable/disable blacklist checking
	EnableClientIDCheck  bool `json:"enable_clientid_check"`
	EnableUsernameCheck  bool `json:"enable_username_check"`
	EnableIPAddressCheck bool `json:"enable_ipaddress_check"`
	EnableTopicCheck     bool `json:"enable_topic_check"`

	// Behavior settings
	LogBlocks       bool          `json:"log_blocks"`
	BlockTimeout    time.Duration `json:"block_timeout"`     // Timeout for temporary blocks
	CleanupInterval time.Duration `json:"cleanup_interval"`  // Cleanup expired entries interval
	MaxRecentBlocks int           `json:"max_recent_blocks"` // Maximum recent blocks to keep

	// Default actions
	DefaultAction       BlacklistAction `json:"default_action"`
	UnknownClientAction BlacklistAction `json:"unknown_client_action"`
}

// DefaultMiddlewareConfig returns default middleware configuration
func DefaultMiddlewareConfig() *MiddlewareConfig {
	return &MiddlewareConfig{
		EnableClientIDCheck:  true,
		EnableUsernameCheck:  true,
		EnableIPAddressCheck: true,
		EnableTopicCheck:     true,
		LogBlocks:            true,
		BlockTimeout:         time.Hour * 24, // 24 hours default
		CleanupInterval:      time.Hour,      // Cleanup every hour
		MaxRecentBlocks:      1000,
		DefaultAction:        ActionDeny,
		UnknownClientAction:  ActionLog,
	}
}

// NewBlacklistMiddleware creates a new blacklist middleware
func NewBlacklistMiddleware(manager *BlacklistManager, config *MiddlewareConfig) *BlacklistMiddleware {
	if config == nil {
		config = DefaultMiddlewareConfig()
	}

	middleware := &BlacklistMiddleware{
		manager: manager,
		config:  config,
	}

	// Start cleanup routine
	go middleware.startCleanupRoutine()

	return middleware
}

// CheckClientConnection validates a client connection against blacklists
func (bm *BlacklistMiddleware) CheckClientConnection(clientID, username, ipAddress, protocol string) (bool, string, error) {
	if bm.manager == nil {
		return true, "", nil
	}

	clientInfo := ClientInfo{
		ClientID:    clientID,
		Username:    username,
		IPAddress:   ipAddress,
		Protocol:    protocol,
		ConnectedAt: time.Now(),
	}

	// Check blacklists if enabled
	if bm.config.EnableClientIDCheck || bm.config.EnableUsernameCheck || bm.config.EnableIPAddressCheck {
		allowed, entry, err := bm.manager.CheckClientConnection(clientInfo)
		if err != nil {
			return false, "Internal blacklist check error", err
		}

		if !allowed && entry != nil {
			reason := fmt.Sprintf("Blocked by %s blacklist: %s", entry.Type, entry.Reason)

			if bm.config.LogBlocks {
				log.Printf("[BLACKLIST] Connection blocked - Client: %s, Username: %s, IP: %s, Reason: %s",
					clientID, username, ipAddress, reason)
			}

			return false, reason, nil
		}
	}

	return true, "", nil
}

// CheckTopicAccess validates topic access against blacklists
func (bm *BlacklistMiddleware) CheckTopicAccess(clientID, username, ipAddress, topic, action string) (bool, string, error) {
	if bm.manager == nil || !bm.config.EnableTopicCheck {
		return true, "", nil
	}

	topicAccess := TopicAccess{
		Topic:  topic,
		Action: action,
		ClientInfo: ClientInfo{
			ClientID:  clientID,
			Username:  username,
			IPAddress: ipAddress,
		},
	}

	allowed, entry, err := bm.manager.CheckTopicAccess(topicAccess)
	if err != nil {
		return false, "Internal blacklist check error", err
	}

	if !allowed && entry != nil {
		reason := fmt.Sprintf("Topic access blocked by blacklist: %s", entry.Reason)

		if bm.config.LogBlocks {
			log.Printf("[BLACKLIST] Topic access blocked - Client: %s, Topic: %s, Action: %s, Reason: %s",
				clientID, topic, action, reason)
		}

		return false, reason, nil
	}

	return true, "", nil
}

// CheckPublishAccess specifically checks publish access to a topic
func (bm *BlacklistMiddleware) CheckPublishAccess(clientID, username, ipAddress, topic string) (bool, string, error) {
	return bm.CheckTopicAccess(clientID, username, ipAddress, topic, "publish")
}

// CheckSubscribeAccess specifically checks subscribe access to a topic
func (bm *BlacklistMiddleware) CheckSubscribeAccess(clientID, username, ipAddress, topic string) (bool, string, error) {
	return bm.CheckTopicAccess(clientID, username, ipAddress, topic, "subscribe")
}

// AddTemporaryBlock adds a temporary blacklist entry
func (bm *BlacklistMiddleware) AddTemporaryBlock(blockType BlacklistType, value, reason string, duration time.Duration) error {
	if bm.manager == nil {
		return fmt.Errorf("blacklist manager not available")
	}

	expiresAt := time.Now().Add(duration)
	entry := &BlacklistEntry{
		ID:          fmt.Sprintf("temp_%s_%d", blockType, time.Now().UnixNano()),
		Type:        blockType,
		Value:       value,
		Action:      bm.config.DefaultAction,
		Reason:      reason,
		Description: fmt.Sprintf("Temporary block for %v", duration),
		ExpiresAt:   &expiresAt,
		Enabled:     true,
		CreatedBy:   "middleware",
		Metadata: map[string]interface{}{
			"temporary": true,
			"duration":  duration.String(),
		},
	}

	return bm.manager.AddEntry(entry)
}

// BlockClientID temporarily blocks a client ID
func (bm *BlacklistMiddleware) BlockClientID(clientID, reason string, duration time.Duration) error {
	return bm.AddTemporaryBlock(ClientIDBlacklist, clientID, reason, duration)
}

// BlockUsername temporarily blocks a username
func (bm *BlacklistMiddleware) BlockUsername(username, reason string, duration time.Duration) error {
	return bm.AddTemporaryBlock(UsernameBlacklist, username, reason, duration)
}

// BlockIPAddress temporarily blocks an IP address
func (bm *BlacklistMiddleware) BlockIPAddress(ipAddress, reason string, duration time.Duration) error {
	return bm.AddTemporaryBlock(IPAddressBlacklist, ipAddress, reason, duration)
}

// BlockTopic temporarily blocks a topic
func (bm *BlacklistMiddleware) BlockTopic(topic, reason string, duration time.Duration) error {
	return bm.AddTemporaryBlock(TopicBlacklist, topic, reason, duration)
}

// IsIPAddressValid validates IP address format
func (bm *BlacklistMiddleware) IsIPAddressValid(ipAddress string) bool {
	return net.ParseIP(ipAddress) != nil
}

// GetManager returns the underlying blacklist manager
func (bm *BlacklistMiddleware) GetManager() *BlacklistManager {
	return bm.manager
}

// UpdateConfig updates the middleware configuration
func (bm *BlacklistMiddleware) UpdateConfig(config *MiddlewareConfig) {
	if config != nil {
		bm.config = config
	}
}

// GetConfig returns the current middleware configuration
func (bm *BlacklistMiddleware) GetConfig() *MiddlewareConfig {
	// Return a copy to prevent external modification
	configCopy := *bm.config
	return &configCopy
}

// IsEnabled returns whether blacklist checking is enabled
func (bm *BlacklistMiddleware) IsEnabled() bool {
	return bm.manager != nil && (bm.config.EnableClientIDCheck ||
		bm.config.EnableUsernameCheck ||
		bm.config.EnableIPAddressCheck ||
		bm.config.EnableTopicCheck)
}

// GetStats returns blacklist statistics
func (bm *BlacklistMiddleware) GetStats() BlacklistStats {
	if bm.manager == nil {
		return BlacklistStats{}
	}
	return bm.manager.GetStats()
}

// startCleanupRoutine starts the cleanup routine for expired entries
func (bm *BlacklistMiddleware) startCleanupRoutine() {
	if bm.config.CleanupInterval <= 0 {
		return
	}

	ticker := time.NewTicker(bm.config.CleanupInterval)
	go func() {
		for range ticker.C {
			if bm.manager != nil {
				removed := bm.manager.CleanupExpiredEntries()
				if removed > 0 && bm.config.LogBlocks {
					log.Printf("[BLACKLIST] Cleanup completed, removed %d expired entries", removed)
				}
			}
		}
	}()
}

// LogBlock logs a blacklist block event
func (bm *BlacklistMiddleware) LogBlock(blockType BlacklistType, value, reason string, clientInfo ClientInfo) {
	if bm.config.LogBlocks {
		log.Printf("[BLACKLIST] %s blocked - Type: %s, Value: %s, Client: %s, Username: %s, IP: %s, Reason: %s",
			blockType, blockType, value, clientInfo.ClientID, clientInfo.Username, clientInfo.IPAddress, reason)
	}
}

// ValidateEntry validates a blacklist entry before adding
func (bm *BlacklistMiddleware) ValidateEntry(entry *BlacklistEntry) error {
	if entry == nil {
		return fmt.Errorf("entry cannot be nil")
	}

	// Validate IP address format for IP blacklist
	if entry.Type == IPAddressBlacklist && entry.Value != "" {
		// Check if it's a valid IP address or CIDR
		if net.ParseIP(entry.Value) == nil {
			if _, _, err := net.ParseCIDR(entry.Value); err != nil {
				return fmt.Errorf("invalid IP address or CIDR: %s", entry.Value)
			}
		}
	}

	// Validate topic format for topic blacklist
	if entry.Type == TopicBlacklist {
		if entry.Value == "" && entry.Pattern == "" {
			return fmt.Errorf("topic cannot be empty")
		}
		// Additional topic validation could be added here
	}

	return nil
}

// ExportBlacklist exports blacklist entries for backup or migration
func (bm *BlacklistMiddleware) ExportBlacklist() []*BlacklistEntry {
	if bm.manager == nil {
		return nil
	}

	return bm.manager.ListEntries("", nil)
}

// ImportBlacklist imports blacklist entries from backup or migration
func (bm *BlacklistMiddleware) ImportBlacklist(entries []*BlacklistEntry, overwrite bool) error {
	if bm.manager == nil {
		return fmt.Errorf("blacklist manager not available")
	}

	var errors []string

	for _, entry := range entries {
		if err := bm.ValidateEntry(entry); err != nil {
			errors = append(errors, fmt.Sprintf("validation failed for entry %s: %v", entry.ID, err))
			continue
		}

		// Check if entry exists
		if existing, _ := bm.manager.GetEntry(entry.ID); existing != nil {
			if !overwrite {
				errors = append(errors, fmt.Sprintf("entry %s already exists", entry.ID))
				continue
			}
			// Update existing entry
			if err := bm.manager.UpdateEntry(entry.ID, entry); err != nil {
				errors = append(errors, fmt.Sprintf("failed to update entry %s: %v", entry.ID, err))
			}
		} else {
			// Add new entry
			if err := bm.manager.AddEntry(entry); err != nil {
				errors = append(errors, fmt.Sprintf("failed to add entry %s: %v", entry.ID, err))
			}
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("import completed with errors: %v", errors)
	}

	return nil
}
