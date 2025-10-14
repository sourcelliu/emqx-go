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

// Package persistent provides persistent session management for MQTT clients
// with support for offline message queuing, will messages, and QoS acknowledgments.
package persistent

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/turtacn/emqx-go/pkg/storage"
)

// PersistentSession represents a durable MQTT client session that survives
// disconnections when CleanSession=false
type PersistentSession struct {
	// Basic session information
	ClientID        string            `json:"client_id"`
	CleanSession    bool              `json:"clean_session"`
	Connected       bool              `json:"connected"`
	CreatedAt       time.Time         `json:"created_at"`
	LastActivity    time.Time         `json:"last_activity"`
	ExpiryInterval  time.Duration     `json:"expiry_interval"`

	// Subscriptions: topic -> QoS
	Subscriptions   map[string]byte   `json:"subscriptions"`

	// Packet ID management
	PacketIDCounter uint16            `json:"packet_id_counter"`

	// Inflight messages waiting for acknowledgment
	InflightQoS1    map[uint16]*InflightMessage `json:"inflight_qos1"`
	InflightQoS2    map[uint16]*InflightMessage `json:"inflight_qos2"`

	// Offline message queue
	MessageQueue    []*QueuedMessage  `json:"message_queue"`

	// Will message configuration
	WillMessage     *WillMessage      `json:"will_message,omitempty"`

	// Synchronization
	mu              sync.RWMutex      `json:"-"`
}

// InflightMessage represents a message waiting for acknowledgment
type InflightMessage struct {
	PacketID    uint16            `json:"packet_id"`
	Topic       string            `json:"topic"`
	Payload     []byte            `json:"payload"`
	QoS         byte              `json:"qos"`
	Retain      bool              `json:"retain"`
	Timestamp   time.Time         `json:"timestamp"`
	RetryCount  int               `json:"retry_count"`
	Headers     map[string]string `json:"headers,omitempty"`
}

// QueuedMessage represents an offline message waiting to be delivered
type QueuedMessage struct {
	Topic       string            `json:"topic"`
	Payload     []byte            `json:"payload"`
	QoS         byte              `json:"qos"`
	Retain      bool              `json:"retain"`
	Timestamp   time.Time         `json:"timestamp"`
	ExpiryTime  *time.Time        `json:"expiry_time,omitempty"`
	Headers     map[string]string `json:"headers,omitempty"`
}

// WillMessage represents the last will and testament message
type WillMessage struct {
	Topic         string        `json:"topic"`
	Payload       []byte        `json:"payload"`
	QoS           byte          `json:"qos"`
	Retain        bool          `json:"retain"`
	DelayInterval time.Duration `json:"delay_interval"`
}

// Config defines configuration for persistent session management
type Config struct {
	// Maximum number of messages to queue for offline clients
	MaxOfflineMessages  uint64        `json:"max_offline_messages"`

	// Maximum time to keep messages in offline queue
	MessageExpiryInterval time.Duration `json:"message_expiry_interval"`

	// Default session expiry interval when not specified by client
	DefaultSessionExpiry time.Duration `json:"default_session_expiry"`

	// Interval for retrying unacknowledged QoS 1/2 messages
	RetryInterval       time.Duration `json:"retry_interval"`

	// Maximum number of retry attempts for QoS 1/2 messages
	MaxRetryAttempts    int           `json:"max_retry_attempts"`

	// Cleanup interval for expired sessions and messages
	CleanupInterval     time.Duration `json:"cleanup_interval"`

	// Maximum number of inflight messages per session
	MaxInflightMessages uint16        `json:"max_inflight_messages"`
}

// DefaultConfig returns a default configuration for persistent sessions
func DefaultConfig() *Config {
	return &Config{
		MaxOfflineMessages:    10000,
		MessageExpiryInterval: 24 * time.Hour,
		DefaultSessionExpiry:  24 * time.Hour,
		RetryInterval:         30 * time.Second,
		MaxRetryAttempts:      3,
		CleanupInterval:       5 * time.Minute,
		MaxInflightMessages:   100,
	}
}

// SessionManager manages persistent sessions for MQTT clients
type SessionManager struct {
	store   storage.Store
	config  *Config

	// Active sessions (connected clients)
	activeSessions map[string]*PersistentSession

	// Will message management
	willManager *WillMessageManager

	// Cleanup and retry management
	cleanupTicker *time.Ticker
	retryTicker   *time.Ticker
	stopCleanup   chan struct{}
	stopRetry     chan struct{}
	closed        bool

	mu sync.RWMutex
}

// NewSessionManager creates a new persistent session manager
func NewSessionManager(store storage.Store, config *Config) *SessionManager {
	if config == nil {
		config = DefaultConfig()
	}

	sm := &SessionManager{
		store:          store,
		config:         config,
		activeSessions: make(map[string]*PersistentSession),
		stopCleanup:    make(chan struct{}),
		stopRetry:      make(chan struct{}),
	}

	// Initialize will message manager with a placeholder publisher
	// The actual publisher will be set by the broker
	sm.willManager = NewWillMessageManager(nil)

	// Start background routines
	sm.startCleanupRoutine()
	sm.startRetryRoutine()

	return sm
}

// SetWillMessagePublisher sets the publisher for will messages
func (sm *SessionManager) SetWillMessagePublisher(publisher WillMessagePublisher) {
	sm.willManager.publisher = publisher
}

// CreateOrResumeSession creates a new session or resumes an existing persistent session
func (sm *SessionManager) CreateOrResumeSession(clientID string, cleanSession bool, expiryInterval time.Duration) (*PersistentSession, error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	now := time.Now()

	// Check if session already exists in active sessions
	if activeSession, exists := sm.activeSessions[clientID]; exists {
		activeSession.mu.Lock()
		activeSession.Connected = true
		activeSession.LastActivity = now
		if cleanSession {
			// Clean session requested, clear all state and update clean session flag
			activeSession.CleanSession = true
			activeSession.Subscriptions = make(map[string]byte)
			activeSession.MessageQueue = nil
			activeSession.InflightQoS1 = make(map[uint16]*InflightMessage)
			activeSession.InflightQoS2 = make(map[uint16]*InflightMessage)
			activeSession.WillMessage = nil
		}
		activeSession.mu.Unlock()

		log.Printf("[INFO] Resumed active session for client %s (cleanSession: %t)", clientID, cleanSession)
		return activeSession, nil
	}

	// Try to load persistent session from storage
	if !cleanSession {
		if session, err := sm.loadSession(clientID); err == nil {
			// Resume existing persistent session
			session.Connected = true
			session.LastActivity = now
			if expiryInterval > 0 {
				session.ExpiryInterval = expiryInterval
			}

			sm.activeSessions[clientID] = session
			log.Printf("[INFO] Resumed persistent session for client %s (offline messages: %d, subscriptions: %d)",
				clientID, len(session.MessageQueue), len(session.Subscriptions))
			return session, nil
		}
	}

	// Create new session
	session := &PersistentSession{
		ClientID:        clientID,
		CleanSession:    cleanSession,
		Connected:       true,
		CreatedAt:       now,
		LastActivity:    now,
		ExpiryInterval:  expiryInterval,
		Subscriptions:   make(map[string]byte),
		PacketIDCounter: 1,
		InflightQoS1:    make(map[uint16]*InflightMessage),
		InflightQoS2:    make(map[uint16]*InflightMessage),
		MessageQueue:    nil,
	}

	if session.ExpiryInterval == 0 {
		session.ExpiryInterval = sm.config.DefaultSessionExpiry
	}

	sm.activeSessions[clientID] = session

	// Save to persistent storage if not clean session
	if !cleanSession {
		if err := sm.saveSession(session); err != nil {
			log.Printf("[ERROR] Failed to save new persistent session for %s: %v", clientID, err)
		}
	}

	log.Printf("[INFO] Created new session for client %s (cleanSession: %t)", clientID, cleanSession)
	return session, nil
}

// DisconnectSession marks a session as disconnected and handles will message
func (sm *SessionManager) DisconnectSession(clientID string, graceful bool) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	session, exists := sm.activeSessions[clientID]
	if !exists {
		return fmt.Errorf("session not found for client %s", clientID)
	}

	session.mu.Lock()
	defer session.mu.Unlock()

	session.Connected = false
	session.LastActivity = time.Now()

	// Handle will message if disconnection was not graceful
	if !graceful && session.WillMessage != nil {
		log.Printf("[INFO] Triggering will message for client %s due to abnormal disconnection", clientID)

		// Publish will message using the will manager
		ctx := context.Background()
		if err := sm.willManager.PublishWillMessage(ctx, session.WillMessage, clientID); err != nil {
			log.Printf("[ERROR] Failed to publish will message for client %s: %v", clientID, err)
		}
	} else if graceful {
		// Cancel any scheduled will messages for graceful disconnection
		sm.willManager.CancelWillMessage(clientID)
		log.Printf("[INFO] Cancelled scheduled will messages for client %s (graceful disconnect)", clientID)
	}

	// Clear will message after disconnection (graceful or after publishing)
	session.WillMessage = nil

	// If clean session, remove from active sessions and storage
	if session.CleanSession {
		delete(sm.activeSessions, clientID)
		sm.deleteSession(clientID)
		log.Printf("[INFO] Removed clean session for client %s", clientID)
	} else {
		// Save persistent session state
		if err := sm.saveSession(session); err != nil {
			log.Printf("[ERROR] Failed to save session state for %s: %v", clientID, err)
		}
		log.Printf("[INFO] Saved persistent session for client %s (offline messages: %d)", clientID, len(session.MessageQueue))
	}

	return nil
}

// AddSubscription adds a subscription to the session
func (sm *SessionManager) AddSubscription(clientID, topic string, qos byte) error {
	sm.mu.RLock()
	session, exists := sm.activeSessions[clientID]
	sm.mu.RUnlock()

	if !exists {
		return fmt.Errorf("session not found for client %s", clientID)
	}

	session.mu.Lock()
	defer session.mu.Unlock()

	session.Subscriptions[topic] = qos
	session.LastActivity = time.Now()

	// Save to storage if persistent session
	if !session.CleanSession {
		if err := sm.saveSession(session); err != nil {
			log.Printf("[ERROR] Failed to save subscription for %s: %v", clientID, err)
		}
	}

	log.Printf("[INFO] Added subscription for client %s: topic=%s, qos=%d", clientID, topic, qos)
	return nil
}

// RemoveSubscription removes a subscription from the session
func (sm *SessionManager) RemoveSubscription(clientID, topic string) error {
	sm.mu.RLock()
	session, exists := sm.activeSessions[clientID]
	sm.mu.RUnlock()

	if !exists {
		return fmt.Errorf("session not found for client %s", clientID)
	}

	session.mu.Lock()
	defer session.mu.Unlock()

	delete(session.Subscriptions, topic)
	session.LastActivity = time.Now()

	// Save to storage if persistent session
	if !session.CleanSession {
		if err := sm.saveSession(session); err != nil {
			log.Printf("[ERROR] Failed to save unsubscription for %s: %v", clientID, err)
		}
	}

	log.Printf("[INFO] Removed subscription for client %s: topic=%s", clientID, topic)
	return nil
}

// SetWillMessage sets the will message for a session
func (sm *SessionManager) SetWillMessage(clientID, topic string, payload []byte, qos byte, retain bool, delayInterval time.Duration) error {
	sm.mu.RLock()
	session, exists := sm.activeSessions[clientID]
	sm.mu.RUnlock()

	if !exists {
		return fmt.Errorf("session not found for client %s", clientID)
	}

	session.mu.Lock()
	defer session.mu.Unlock()

	session.WillMessage = &WillMessage{
		Topic:         topic,
		Payload:       payload,
		QoS:           qos,
		Retain:        retain,
		DelayInterval: delayInterval,
	}

	log.Printf("[INFO] Set will message for client %s: topic=%s", clientID, topic)
	return nil
}

// QueueMessage queues a message for offline delivery
func (sm *SessionManager) QueueMessage(clientID, topic string, payload []byte, qos byte, retain bool) error {
	sm.mu.RLock()
	session, exists := sm.activeSessions[clientID]
	sm.mu.RUnlock()

	if !exists {
		return fmt.Errorf("session not found for client %s", clientID)
	}

	session.mu.Lock()
	defer session.mu.Unlock()

	// Don't queue messages for connected clients
	if session.Connected {
		return nil
	}

	// Don't queue messages for clean sessions
	if session.CleanSession {
		return nil
	}

	// Check queue size limit
	if uint64(len(session.MessageQueue)) >= sm.config.MaxOfflineMessages {
		// Remove oldest message to make room
		session.MessageQueue = session.MessageQueue[1:]
		log.Printf("[WARN] Offline message queue full for client %s, dropping oldest message", clientID)
	}

	// Create queued message
	msg := &QueuedMessage{
		Topic:     topic,
		Payload:   payload,
		QoS:       qos,
		Retain:    retain,
		Timestamp: time.Now(),
	}

	// Set expiry time if configured
	if sm.config.MessageExpiryInterval > 0 {
		expiryTime := msg.Timestamp.Add(sm.config.MessageExpiryInterval)
		msg.ExpiryTime = &expiryTime
	}

	session.MessageQueue = append(session.MessageQueue, msg)

	// Save to storage
	if err := sm.saveSession(session); err != nil {
		log.Printf("[ERROR] Failed to save queued message for %s: %v", clientID, err)
	}

	log.Printf("[INFO] Queued offline message for client %s: topic=%s, queue_size=%d", clientID, topic, len(session.MessageQueue))
	return nil
}

// NextPacketID generates the next packet ID for QoS 1/2 messages
func (sm *SessionManager) NextPacketID(clientID string) (uint16, error) {
	sm.mu.RLock()
	session, exists := sm.activeSessions[clientID]
	sm.mu.RUnlock()

	if !exists {
		return 0, fmt.Errorf("session not found for client %s", clientID)
	}

	session.mu.Lock()
	defer session.mu.Unlock()

	// Increment packet ID (skip 0)
	session.PacketIDCounter++
	if session.PacketIDCounter == 0 {
		session.PacketIDCounter = 1
	}

	return session.PacketIDCounter, nil
}

// Close shuts down the session manager
func (sm *SessionManager) Close() error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	// Check if already closed
	if sm.closed {
		return nil
	}
	sm.closed = true

	if sm.cleanupTicker != nil {
		sm.cleanupTicker.Stop()
		close(sm.stopCleanup)
	}
	if sm.retryTicker != nil {
		sm.retryTicker.Stop()
		close(sm.stopRetry)
	}

	// Close will message manager
	if sm.willManager != nil {
		sm.willManager.Close()
	}

	// Save all active sessions
	for _, session := range sm.activeSessions {
		if !session.CleanSession {
			if err := sm.saveSession(session); err != nil {
				log.Printf("[ERROR] Failed to save session %s during shutdown: %v", session.ClientID, err)
			}
		}
	}

	return nil
}

// Private helper methods

func (sm *SessionManager) saveSession(session *PersistentSession) error {
	key := fmt.Sprintf("session:%s", session.ClientID)
	data, err := json.Marshal(session)
	if err != nil {
		return fmt.Errorf("failed to marshal session: %w", err)
	}
	return sm.store.Set(key, data)
}

func (sm *SessionManager) loadSession(clientID string) (*PersistentSession, error) {
	key := fmt.Sprintf("session:%s", clientID)
	data, err := sm.store.Get(key)
	if err != nil {
		return nil, err
	}

	var session PersistentSession
	if err := json.Unmarshal(data.([]byte), &session); err != nil {
		return nil, fmt.Errorf("failed to unmarshal session: %w", err)
	}

	return &session, nil
}

func (sm *SessionManager) deleteSession(clientID string) error {
	key := fmt.Sprintf("session:%s", clientID)
	return sm.store.Delete(key)
}

func (sm *SessionManager) startCleanupRoutine() {
	sm.cleanupTicker = time.NewTicker(sm.config.CleanupInterval)

	go func() {
		for {
			select {
			case <-sm.cleanupTicker.C:
				sm.cleanupExpiredSessions()
			case <-sm.stopCleanup:
				return
			}
		}
	}()

	log.Printf("[INFO] Started session cleanup routine with interval: %v", sm.config.CleanupInterval)
}

func (sm *SessionManager) startRetryRoutine() {
	sm.retryTicker = time.NewTicker(sm.config.RetryInterval)

	go func() {
		for {
			select {
			case <-sm.retryTicker.C:
				sm.retryInflightMessages()
			case <-sm.stopRetry:
				return
			}
		}
	}()

	log.Printf("[INFO] Started message retry routine with interval: %v", sm.config.RetryInterval)
}

func (sm *SessionManager) cleanupExpiredSessions() {
	now := time.Now()
	sm.mu.Lock()
	defer sm.mu.Unlock()

	for clientID, session := range sm.activeSessions {
		session.mu.Lock()

		// Remove expired offline sessions
		if !session.Connected && now.Sub(session.LastActivity) > session.ExpiryInterval {
			delete(sm.activeSessions, clientID)
			sm.deleteSession(clientID)
			log.Printf("[INFO] Cleaned up expired session for client %s", clientID)
			session.mu.Unlock()
			continue
		}

		// Clean up expired messages in queue
		var validMessages []*QueuedMessage
		for _, msg := range session.MessageQueue {
			if msg.ExpiryTime == nil || now.Before(*msg.ExpiryTime) {
				validMessages = append(validMessages, msg)
			}
		}

		if len(validMessages) != len(session.MessageQueue) {
			session.MessageQueue = validMessages
			if !session.CleanSession {
				sm.saveSession(session)
			}
			log.Printf("[INFO] Cleaned up expired messages for client %s", clientID)
		}

		session.mu.Unlock()
	}
}

func (sm *SessionManager) retryInflightMessages() {
	// TODO: Implement retry logic for QoS 1/2 messages
	// This would integrate with the broker's publish mechanism
	log.Printf("[DEBUG] Retry routine executed (implementation pending)")
}