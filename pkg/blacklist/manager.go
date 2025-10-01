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
	"errors"
	"net"
	"regexp"
	"strings"
	"sync"
	"time"
)

// BlacklistType represents the type of blacklist entry
type BlacklistType string

const (
	ClientIDBlacklist  BlacklistType = "clientid"
	UsernameBlacklist  BlacklistType = "username"
	IPAddressBlacklist BlacklistType = "ipaddress"
	TopicBlacklist     BlacklistType = "topic"
)

// BlacklistAction represents what action to take when blacklist matches
type BlacklistAction string

const (
	ActionDeny       BlacklistAction = "deny"
	ActionDisconnect BlacklistAction = "disconnect"
	ActionLog        BlacklistAction = "log"
)

// BlacklistEntry represents a single blacklist entry
type BlacklistEntry struct {
	ID          string          `json:"id"`
	Type        BlacklistType   `json:"type"`
	Value       string          `json:"value"`
	Pattern     string          `json:"pattern,omitempty"`    // For regex patterns
	Action      BlacklistAction `json:"action"`
	Reason      string          `json:"reason,omitempty"`
	Description string          `json:"description,omitempty"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
	ExpiresAt   *time.Time      `json:"expires_at,omitempty"` // Optional expiration
	Enabled     bool            `json:"enabled"`

	// Metadata
	CreatedBy string                 `json:"created_by,omitempty"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`

	// Compiled pattern for performance
	compiledPattern *regexp.Regexp `json:"-"`
}

// BlacklistManager manages all blacklist entries
type BlacklistManager struct {
	entries map[string]*BlacklistEntry
	mu      sync.RWMutex

	// Type-specific indexes for fast lookup
	clientIDEntries  map[string]*BlacklistEntry
	usernameEntries  map[string]*BlacklistEntry
	ipAddressEntries map[string]*BlacklistEntry
	topicEntries     map[string]*BlacklistEntry

	// Pattern-based entries (for regex patterns)
	patternEntries map[BlacklistType][]*BlacklistEntry

	// Statistics
	stats BlacklistStats
}

// BlacklistStats represents statistics for blacklist operations
type BlacklistStats struct {
	TotalEntries     int                          `json:"total_entries"`
	EntriesByType    map[BlacklistType]int        `json:"entries_by_type"`
	EntriesByAction  map[BlacklistAction]int      `json:"entries_by_action"`
	TotalBlocks      int64                        `json:"total_blocks"`
	BlocksByType     map[BlacklistType]int64      `json:"blocks_by_type"`
	RecentBlocks     []BlacklistBlock             `json:"recent_blocks"`
	LastUpdated      time.Time                    `json:"last_updated"`
}

// BlacklistBlock represents a blocked action
type BlacklistBlock struct {
	Timestamp   time.Time       `json:"timestamp"`
	Type        BlacklistType   `json:"type"`
	Value       string          `json:"value"`
	Action      BlacklistAction `json:"action"`
	EntryID     string          `json:"entry_id"`
	ClientInfo  ClientInfo      `json:"client_info"`
	Reason      string          `json:"reason"`
}

// ClientInfo represents client information for blacklist checking
type ClientInfo struct {
	ClientID    string `json:"client_id"`
	Username    string `json:"username"`
	IPAddress   string `json:"ip_address"`
	Protocol    string `json:"protocol"`
	ConnectedAt time.Time `json:"connected_at"`
}

// TopicAccess represents topic access information
type TopicAccess struct {
	Topic      string `json:"topic"`
	Action     string `json:"action"` // publish, subscribe, pubsub
	ClientInfo ClientInfo `json:"client_info"`
}

// Common errors
var (
	ErrEntryNotFound      = errors.New("blacklist entry not found")
	ErrEntryAlreadyExists = errors.New("blacklist entry already exists")
	ErrInvalidPattern     = errors.New("invalid regex pattern")
	ErrInvalidType        = errors.New("invalid blacklist type")
	ErrInvalidAction      = errors.New("invalid blacklist action")
)

// NewBlacklistManager creates a new blacklist manager
func NewBlacklistManager() *BlacklistManager {
	return &BlacklistManager{
		entries:          make(map[string]*BlacklistEntry),
		clientIDEntries:  make(map[string]*BlacklistEntry),
		usernameEntries:  make(map[string]*BlacklistEntry),
		ipAddressEntries: make(map[string]*BlacklistEntry),
		topicEntries:     make(map[string]*BlacklistEntry),
		patternEntries:   make(map[BlacklistType][]*BlacklistEntry),
		stats: BlacklistStats{
			EntriesByType:   make(map[BlacklistType]int),
			EntriesByAction: make(map[BlacklistAction]int),
			BlocksByType:    make(map[BlacklistType]int64),
			RecentBlocks:    make([]BlacklistBlock, 0, 100), // Keep last 100 blocks
			LastUpdated:     time.Now(),
		},
	}
}

// AddEntry adds a new blacklist entry
func (bm *BlacklistManager) AddEntry(entry *BlacklistEntry) error {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	// Validate entry
	if err := bm.validateEntry(entry); err != nil {
		return err
	}

	// Check if entry already exists
	if _, exists := bm.entries[entry.ID]; exists {
		return ErrEntryAlreadyExists
	}

	// Compile regex pattern if provided
	if entry.Pattern != "" {
		compiled, err := regexp.Compile(entry.Pattern)
		if err != nil {
			return ErrInvalidPattern
		}
		entry.compiledPattern = compiled
	}

	// Set timestamps
	now := time.Now()
	entry.CreatedAt = now
	entry.UpdatedAt = now

	// Add to main storage
	bm.entries[entry.ID] = entry

	// Add to type-specific indexes
	bm.addToTypeIndex(entry)

	// Update statistics
	bm.updateStatsOnAdd(entry)

	return nil
}

// UpdateEntry updates an existing blacklist entry
func (bm *BlacklistManager) UpdateEntry(id string, updates *BlacklistEntry) error {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	entry, exists := bm.entries[id]
	if !exists {
		return ErrEntryNotFound
	}

	// Remove from old indexes
	bm.removeFromTypeIndex(entry)

	// Update fields
	oldType := entry.Type
	if updates.Value != "" {
		entry.Value = updates.Value
	}
	if updates.Pattern != "" {
		entry.Pattern = updates.Pattern
		// Recompile pattern
		compiled, err := regexp.Compile(entry.Pattern)
		if err != nil {
			return ErrInvalidPattern
		}
		entry.compiledPattern = compiled
	}
	if updates.Action != "" {
		entry.Action = updates.Action
	}
	if updates.Reason != "" {
		entry.Reason = updates.Reason
	}
	if updates.Description != "" {
		entry.Description = updates.Description
	}
	entry.Enabled = updates.Enabled
	entry.UpdatedAt = time.Now()

	if updates.ExpiresAt != nil {
		entry.ExpiresAt = updates.ExpiresAt
	}

	// Add back to indexes
	bm.addToTypeIndex(entry)

	// Update statistics if type changed
	if oldType != entry.Type {
		bm.stats.EntriesByType[oldType]--
		bm.stats.EntriesByType[entry.Type]++
	}

	bm.stats.LastUpdated = time.Now()

	return nil
}

// RemoveEntry removes a blacklist entry
func (bm *BlacklistManager) RemoveEntry(id string) error {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	entry, exists := bm.entries[id]
	if !exists {
		return ErrEntryNotFound
	}

	// Remove from main storage
	delete(bm.entries, id)

	// Remove from type-specific indexes
	bm.removeFromTypeIndex(entry)

	// Update statistics
	bm.updateStatsOnRemove(entry)

	return nil
}

// GetEntry retrieves a specific blacklist entry
func (bm *BlacklistManager) GetEntry(id string) (*BlacklistEntry, error) {
	bm.mu.RLock()
	defer bm.mu.RUnlock()

	entry, exists := bm.entries[id]
	if !exists {
		return nil, ErrEntryNotFound
	}

	// Return a copy to prevent external modification
	entryCopy := *entry
	return &entryCopy, nil
}

// ListEntries returns all blacklist entries with optional filtering
func (bm *BlacklistManager) ListEntries(entryType BlacklistType, enabled *bool) []*BlacklistEntry {
	bm.mu.RLock()
	defer bm.mu.RUnlock()

	var result []*BlacklistEntry

	for _, entry := range bm.entries {
		// Filter by type if specified
		if entryType != "" && entry.Type != entryType {
			continue
		}

		// Filter by enabled status if specified
		if enabled != nil && entry.Enabled != *enabled {
			continue
		}

		// Check if entry has expired
		if entry.ExpiresAt != nil && time.Now().After(*entry.ExpiresAt) {
			continue
		}

		// Return a copy to prevent external modification
		entryCopy := *entry
		result = append(result, &entryCopy)
	}

	return result
}

// CheckClientConnection checks if a client connection should be allowed
func (bm *BlacklistManager) CheckClientConnection(clientInfo ClientInfo) (bool, *BlacklistEntry, error) {
	bm.mu.RLock()
	defer bm.mu.RUnlock()

	// Check Client ID blacklist
	if entry := bm.checkClientID(clientInfo.ClientID); entry != nil {
		bm.recordBlock(entry, ClientIDBlacklist, clientInfo.ClientID, clientInfo)
		return false, entry, nil
	}

	// Check Username blacklist
	if entry := bm.checkUsername(clientInfo.Username); entry != nil {
		bm.recordBlock(entry, UsernameBlacklist, clientInfo.Username, clientInfo)
		return false, entry, nil
	}

	// Check IP Address blacklist
	if entry := bm.checkIPAddress(clientInfo.IPAddress); entry != nil {
		bm.recordBlock(entry, IPAddressBlacklist, clientInfo.IPAddress, clientInfo)
		return false, entry, nil
	}

	return true, nil, nil
}

// CheckTopicAccess checks if topic access should be allowed
func (bm *BlacklistManager) CheckTopicAccess(topicAccess TopicAccess) (bool, *BlacklistEntry, error) {
	bm.mu.RLock()
	defer bm.mu.RUnlock()

	if entry := bm.checkTopic(topicAccess.Topic, topicAccess.Action); entry != nil {
		bm.recordBlock(entry, TopicBlacklist, topicAccess.Topic, topicAccess.ClientInfo)
		return false, entry, nil
	}

	return true, nil, nil
}

// GetStats returns current blacklist statistics
func (bm *BlacklistManager) GetStats() BlacklistStats {
	bm.mu.RLock()
	defer bm.mu.RUnlock()

	// Create a copy to prevent external modification
	stats := bm.stats
	stats.EntriesByType = make(map[BlacklistType]int)
	stats.EntriesByAction = make(map[BlacklistAction]int)
	stats.BlocksByType = make(map[BlacklistType]int64)

	for k, v := range bm.stats.EntriesByType {
		stats.EntriesByType[k] = v
	}
	for k, v := range bm.stats.EntriesByAction {
		stats.EntriesByAction[k] = v
	}
	for k, v := range bm.stats.BlocksByType {
		stats.BlocksByType[k] = v
	}

	// Copy recent blocks
	stats.RecentBlocks = make([]BlacklistBlock, len(bm.stats.RecentBlocks))
	copy(stats.RecentBlocks, bm.stats.RecentBlocks)

	return stats
}

// CleanupExpiredEntries removes expired blacklist entries
func (bm *BlacklistManager) CleanupExpiredEntries() int {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	var removedCount int
	now := time.Now()

	for id, entry := range bm.entries {
		if entry.ExpiresAt != nil && now.After(*entry.ExpiresAt) {
			delete(bm.entries, id)
			bm.removeFromTypeIndex(entry)
			bm.updateStatsOnRemove(entry)
			removedCount++
		}
	}

	return removedCount
}

// validateEntry validates a blacklist entry
func (bm *BlacklistManager) validateEntry(entry *BlacklistEntry) error {
	if entry.ID == "" {
		return errors.New("entry ID is required")
	}

	if entry.Type == "" {
		return ErrInvalidType
	}

	if entry.Type != ClientIDBlacklist && entry.Type != UsernameBlacklist &&
	   entry.Type != IPAddressBlacklist && entry.Type != TopicBlacklist {
		return ErrInvalidType
	}

	if entry.Action == "" {
		return ErrInvalidAction
	}

	if entry.Action != ActionDeny && entry.Action != ActionDisconnect && entry.Action != ActionLog {
		return ErrInvalidAction
	}

	if entry.Value == "" && entry.Pattern == "" {
		return errors.New("either value or pattern is required")
	}

	return nil
}

// addToTypeIndex adds entry to appropriate type-specific index
func (bm *BlacklistManager) addToTypeIndex(entry *BlacklistEntry) {
	if entry.Pattern != "" {
		// Add to pattern entries
		bm.patternEntries[entry.Type] = append(bm.patternEntries[entry.Type], entry)
	} else {
		// Add to direct value lookup
		switch entry.Type {
		case ClientIDBlacklist:
			bm.clientIDEntries[entry.Value] = entry
		case UsernameBlacklist:
			bm.usernameEntries[entry.Value] = entry
		case IPAddressBlacklist:
			bm.ipAddressEntries[entry.Value] = entry
		case TopicBlacklist:
			bm.topicEntries[entry.Value] = entry
		}
	}
}

// removeFromTypeIndex removes entry from type-specific index
func (bm *BlacklistManager) removeFromTypeIndex(entry *BlacklistEntry) {
	if entry.Pattern != "" {
		// Remove from pattern entries
		patterns := bm.patternEntries[entry.Type]
		for i, e := range patterns {
			if e.ID == entry.ID {
				bm.patternEntries[entry.Type] = append(patterns[:i], patterns[i+1:]...)
				break
			}
		}
	} else {
		// Remove from direct value lookup
		switch entry.Type {
		case ClientIDBlacklist:
			delete(bm.clientIDEntries, entry.Value)
		case UsernameBlacklist:
			delete(bm.usernameEntries, entry.Value)
		case IPAddressBlacklist:
			delete(bm.ipAddressEntries, entry.Value)
		case TopicBlacklist:
			delete(bm.topicEntries, entry.Value)
		}
	}
}

// updateStatsOnAdd updates statistics when adding an entry
func (bm *BlacklistManager) updateStatsOnAdd(entry *BlacklistEntry) {
	bm.stats.TotalEntries++
	bm.stats.EntriesByType[entry.Type]++
	bm.stats.EntriesByAction[entry.Action]++
	bm.stats.LastUpdated = time.Now()
}

// updateStatsOnRemove updates statistics when removing an entry
func (bm *BlacklistManager) updateStatsOnRemove(entry *BlacklistEntry) {
	bm.stats.TotalEntries--
	bm.stats.EntriesByType[entry.Type]--
	bm.stats.EntriesByAction[entry.Action]--
	bm.stats.LastUpdated = time.Now()
}

// checkClientID checks if a client ID is blacklisted
func (bm *BlacklistManager) checkClientID(clientID string) *BlacklistEntry {
	// Check direct value match
	if entry, exists := bm.clientIDEntries[clientID]; exists && entry.Enabled {
		if entry.ExpiresAt == nil || time.Now().Before(*entry.ExpiresAt) {
			return entry
		}
	}

	// Check pattern matches
	for _, entry := range bm.patternEntries[ClientIDBlacklist] {
		if entry.Enabled && entry.compiledPattern != nil {
			if entry.ExpiresAt == nil || time.Now().Before(*entry.ExpiresAt) {
				if entry.compiledPattern.MatchString(clientID) {
					return entry
				}
			}
		}
	}

	return nil
}

// checkUsername checks if a username is blacklisted
func (bm *BlacklistManager) checkUsername(username string) *BlacklistEntry {
	// Check direct value match
	if entry, exists := bm.usernameEntries[username]; exists && entry.Enabled {
		if entry.ExpiresAt == nil || time.Now().Before(*entry.ExpiresAt) {
			return entry
		}
	}

	// Check pattern matches
	for _, entry := range bm.patternEntries[UsernameBlacklist] {
		if entry.Enabled && entry.compiledPattern != nil {
			if entry.ExpiresAt == nil || time.Now().Before(*entry.ExpiresAt) {
				if entry.compiledPattern.MatchString(username) {
					return entry
				}
			}
		}
	}

	return nil
}

// checkIPAddress checks if an IP address is blacklisted
func (bm *BlacklistManager) checkIPAddress(ipAddress string) *BlacklistEntry {
	// Check direct value match
	if entry, exists := bm.ipAddressEntries[ipAddress]; exists && entry.Enabled {
		if entry.ExpiresAt == nil || time.Now().Before(*entry.ExpiresAt) {
			return entry
		}
	}

	// Check CIDR ranges and patterns
	for _, entry := range bm.patternEntries[IPAddressBlacklist] {
		if entry.Enabled {
			if entry.ExpiresAt == nil || time.Now().Before(*entry.ExpiresAt) {
				if bm.matchIPPattern(ipAddress, entry) {
					return entry
				}
			}
		}
	}

	return nil
}

// checkTopic checks if a topic is blacklisted
func (bm *BlacklistManager) checkTopic(topic, action string) *BlacklistEntry {
	// Check direct value match
	if entry, exists := bm.topicEntries[topic]; exists && entry.Enabled {
		if entry.ExpiresAt == nil || time.Now().Before(*entry.ExpiresAt) {
			return entry
		}
	}

	// Check pattern matches and topic filters
	for _, entry := range bm.patternEntries[TopicBlacklist] {
		if entry.Enabled {
			if entry.ExpiresAt == nil || time.Now().Before(*entry.ExpiresAt) {
				if bm.matchTopicPattern(topic, entry) {
					return entry
				}
			}
		}
	}

	return nil
}

// matchIPPattern matches IP address against various patterns
func (bm *BlacklistManager) matchIPPattern(ipAddress string, entry *BlacklistEntry) bool {
	// Try CIDR match first
	if strings.Contains(entry.Value, "/") {
		_, network, err := net.ParseCIDR(entry.Value)
		if err == nil {
			ip := net.ParseIP(ipAddress)
			if ip != nil && network.Contains(ip) {
				return true
			}
		}
	}

	// Try regex pattern
	if entry.compiledPattern != nil {
		return entry.compiledPattern.MatchString(ipAddress)
	}

	return false
}

// matchTopicPattern matches topic against MQTT topic filters and patterns
func (bm *BlacklistManager) matchTopicPattern(topic string, entry *BlacklistEntry) bool {
	// Try MQTT topic filter match (with + and # wildcards)
	if bm.matchMQTTTopicFilter(topic, entry.Value) {
		return true
	}

	// Try regex pattern
	if entry.compiledPattern != nil {
		return entry.compiledPattern.MatchString(topic)
	}

	return false
}

// matchMQTTTopicFilter implements MQTT topic filter matching
func (bm *BlacklistManager) matchMQTTTopicFilter(topic, filter string) bool {
	topicLevels := strings.Split(topic, "/")
	filterLevels := strings.Split(filter, "/")

	return bm.matchTopicLevels(topicLevels, filterLevels, 0, 0)
}

// matchTopicLevels recursively matches topic levels
func (bm *BlacklistManager) matchTopicLevels(topicLevels, filterLevels []string, topicIndex, filterIndex int) bool {
	// If we've consumed all filter levels
	if filterIndex >= len(filterLevels) {
		return topicIndex >= len(topicLevels)
	}

	// If we've consumed all topic levels but filter has more
	if topicIndex >= len(topicLevels) {
		// Only valid if remaining filter is just "#"
		return filterIndex == len(filterLevels)-1 && filterLevels[filterIndex] == "#"
	}

	filterLevel := filterLevels[filterIndex]

	// Multi-level wildcard
	if filterLevel == "#" {
		return true
	}

	// Single-level wildcard
	if filterLevel == "+" {
		return bm.matchTopicLevels(topicLevels, filterLevels, topicIndex+1, filterIndex+1)
	}

	// Exact match
	if filterLevel == topicLevels[topicIndex] {
		return bm.matchTopicLevels(topicLevels, filterLevels, topicIndex+1, filterIndex+1)
	}

	return false
}

// recordBlock records a blocked action for statistics
func (bm *BlacklistManager) recordBlock(entry *BlacklistEntry, blockType BlacklistType, value string, clientInfo ClientInfo) {
	block := BlacklistBlock{
		Timestamp:  time.Now(),
		Type:       blockType,
		Value:      value,
		Action:     entry.Action,
		EntryID:    entry.ID,
		ClientInfo: clientInfo,
		Reason:     entry.Reason,
	}

	// Update statistics (note: this is called within a read lock, so we need to be careful)
	// For now, we'll defer the stats update to avoid lock issues
	go func() {
		bm.mu.Lock()
		defer bm.mu.Unlock()

		bm.stats.TotalBlocks++
		bm.stats.BlocksByType[blockType]++

		// Add to recent blocks (keep only last 100)
		bm.stats.RecentBlocks = append(bm.stats.RecentBlocks, block)
		if len(bm.stats.RecentBlocks) > 100 {
			bm.stats.RecentBlocks = bm.stats.RecentBlocks[1:]
		}

		bm.stats.LastUpdated = time.Now()
	}()
}