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

// Package persistent provides will message management for MQTT clients
package persistent

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"
)

// WillMessagePublisher defines the interface for publishing will messages
type WillMessagePublisher interface {
	PublishWillMessage(ctx context.Context, willMsg *WillMessage, clientID string) error
}

// WillMessageManager manages will message publishing and delay timers
type WillMessageManager struct {
	publisher WillMessagePublisher
	timers    map[string]*willTimer
	mu        sync.RWMutex
}

// willTimer represents a scheduled will message
type willTimer struct {
	timer    *time.Timer
	willMsg  *WillMessage
	clientID string
	timerID  string
}

// NewWillMessageManager creates a new will message manager
func NewWillMessageManager(publisher WillMessagePublisher) *WillMessageManager {
	return &WillMessageManager{
		publisher: publisher,
		timers:    make(map[string]*willTimer),
	}
}

// PublishWillMessage publishes a will message immediately or schedules it for delayed publishing
func (wmm *WillMessageManager) PublishWillMessage(ctx context.Context, willMsg *WillMessage, clientID string) error {
	if willMsg == nil {
		return fmt.Errorf("will message is nil for client %s", clientID)
	}

	if willMsg.DelayInterval > 0 {
		return wmm.scheduleDelayedWillMessage(ctx, willMsg, clientID)
	}

	return wmm.publishImmediately(ctx, willMsg, clientID)
}

// publishImmediately publishes a will message without delay
func (wmm *WillMessageManager) publishImmediately(ctx context.Context, willMsg *WillMessage, clientID string) error {
	log.Printf("[INFO] Publishing will message for client %s to topic %s", clientID, willMsg.Topic)

	if err := wmm.publisher.PublishWillMessage(ctx, willMsg, clientID); err != nil {
		log.Printf("[ERROR] Failed to publish will message for client %s: %v", clientID, err)
		return err
	}

	log.Printf("[INFO] Successfully published will message for client %s", clientID)
	return nil
}

// scheduleDelayedWillMessage schedules a will message for delayed publishing
func (wmm *WillMessageManager) scheduleDelayedWillMessage(ctx context.Context, willMsg *WillMessage, clientID string) error {
	wmm.mu.Lock()
	defer wmm.mu.Unlock()

	timerID := fmt.Sprintf("%s-%d", clientID, time.Now().UnixNano())

	timer := time.AfterFunc(willMsg.DelayInterval, func() {
		// Remove timer from map when it fires
		wmm.mu.Lock()
		delete(wmm.timers, timerID)
		wmm.mu.Unlock()

		// Publish the will message
		if err := wmm.publishImmediately(context.Background(), willMsg, clientID); err != nil {
			log.Printf("[ERROR] Failed to publish delayed will message for client %s: %v", clientID, err)
		}
	})

	wmm.timers[timerID] = &willTimer{
		timer:    timer,
		willMsg:  willMsg,
		clientID: clientID,
		timerID:  timerID,
	}

	log.Printf("[INFO] Scheduled will message for client %s with delay %v (timerID: %s)",
		clientID, willMsg.DelayInterval, timerID)

	return nil
}

// CancelWillMessage cancels a scheduled will message for a client
func (wmm *WillMessageManager) CancelWillMessage(clientID string) {
	wmm.mu.Lock()
	defer wmm.mu.Unlock()

	// Find and cancel all timers for this client
	cancelled := 0
	for timerID, wTimer := range wmm.timers {
		if wTimer.clientID == clientID {
			wTimer.timer.Stop()
			delete(wmm.timers, timerID)
			cancelled++
		}
	}

	if cancelled > 0 {
		log.Printf("[INFO] Cancelled %d scheduled will messages for client %s", cancelled, clientID)
	}
}

// GetScheduledWillMessages returns information about scheduled will messages
func (wmm *WillMessageManager) GetScheduledWillMessages() map[string]WillTimerInfo {
	wmm.mu.RLock()
	defer wmm.mu.RUnlock()

	info := make(map[string]WillTimerInfo)
	for timerID, wTimer := range wmm.timers {
		info[timerID] = WillTimerInfo{
			ClientID:      wTimer.clientID,
			Topic:         wTimer.willMsg.Topic,
			DelayInterval: wTimer.willMsg.DelayInterval,
		}
	}

	return info
}

// WillTimerInfo contains information about a scheduled will message
type WillTimerInfo struct {
	ClientID      string        `json:"client_id"`
	Topic         string        `json:"topic"`
	DelayInterval time.Duration `json:"delay_interval"`
}

// Close stops all scheduled will messages and cleans up resources
func (wmm *WillMessageManager) Close() {
	wmm.mu.Lock()
	defer wmm.mu.Unlock()

	for timerID, wTimer := range wmm.timers {
		wTimer.timer.Stop()
		delete(wmm.timers, timerID)
	}

	log.Printf("[INFO] Will message manager closed, cancelled %d scheduled messages", len(wmm.timers))
}