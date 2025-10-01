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

package connector

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// BaseConnector provides common functionality for all connector implementations
type BaseConnector struct {
	config      ConnectorConfig
	state       ConnectorState
	status      ConnectorStatus
	metrics     ConnectorMetrics
	mu          sync.RWMutex
	ctx         context.Context
	cancel      context.CancelFunc
	healthTicker *time.Ticker
	retryCount  int64
	lastError   error
	startTime   *time.Time
}

// NewBaseConnector creates a new base connector
func NewBaseConnector(config ConnectorConfig) *BaseConnector {
	ctx, cancel := context.WithCancel(context.Background())

	now := time.Now()
	if config.CreatedAt.IsZero() {
		config.CreatedAt = now
	}
	config.UpdatedAt = now

	return &BaseConnector{
		config: config,
		state:  StateCreated,
		status: ConnectorStatus{
			ID:         config.ID,
			State:      StateCreated,
			UpdateTime: now,
			Metrics:    ConnectorMetrics{},
			HealthStatus: HealthStatus{
				Healthy:       false,
				LastCheckTime: now,
			},
		},
		metrics: ConnectorMetrics{},
		ctx:     ctx,
		cancel:  cancel,
	}
}

// GetConfig returns the connector configuration
func (bc *BaseConnector) GetConfig() ConnectorConfig {
	bc.mu.RLock()
	defer bc.mu.RUnlock()
	return bc.config
}

// Status returns the current connector status
func (bc *BaseConnector) Status() ConnectorStatus {
	bc.mu.RLock()
	defer bc.mu.RUnlock()

	status := bc.status
	status.Metrics = bc.metrics
	status.UpdateTime = time.Now()
	status.RetryCount = int(atomic.LoadInt64(&bc.retryCount))

	return status
}

// GetMetrics returns current metrics
func (bc *BaseConnector) GetMetrics() ConnectorMetrics {
	bc.mu.RLock()
	defer bc.mu.RUnlock()
	return bc.metrics
}

// ResetMetrics resets all metrics
func (bc *BaseConnector) ResetMetrics() {
	bc.mu.Lock()
	defer bc.mu.Unlock()

	bc.metrics = ConnectorMetrics{}
}

// IsRunning returns true if the connector is in running state
func (bc *BaseConnector) IsRunning() bool {
	bc.mu.RLock()
	defer bc.mu.RUnlock()
	return bc.state == StateRunning
}

// UpdateConfig updates the connector configuration
func (bc *BaseConnector) UpdateConfig(ctx context.Context, config ConnectorConfig) error {
	bc.mu.Lock()
	defer bc.mu.Unlock()

	if config.ID != bc.config.ID {
		return fmt.Errorf("cannot change connector ID")
	}

	if config.Type != bc.config.Type {
		return fmt.Errorf("cannot change connector type")
	}

	config.CreatedAt = bc.config.CreatedAt
	config.UpdatedAt = time.Now()

	bc.config = config
	bc.status.UpdateTime = time.Now()

	return nil
}

// setState updates the connector state
func (bc *BaseConnector) setState(state ConnectorState) {
	bc.mu.Lock()
	defer bc.mu.Unlock()

	bc.state = state
	bc.status.State = state
	bc.status.UpdateTime = time.Now()
}

// setError records an error
func (bc *BaseConnector) setError(err error) {
	bc.mu.Lock()
	defer bc.mu.Unlock()

	if err != nil {
		bc.lastError = err
		bc.status.LastError = err.Error()
		now := time.Now()
		bc.status.LastErrorTime = &now

		atomic.AddInt64(&bc.metrics.ErrorCount, 1)
		atomic.AddInt64(&bc.retryCount, 1)
	}
}

// recordSuccess records a successful operation
func (bc *BaseConnector) recordSuccess(latency time.Duration) {
	bc.mu.Lock()
	defer bc.mu.Unlock()

	atomic.AddInt64(&bc.metrics.SuccessCount, 1)

	// Update latency metrics
	if bc.metrics.MinLatency == 0 || latency < bc.metrics.MinLatency {
		bc.metrics.MinLatency = latency
	}
	if latency > bc.metrics.MaxLatency {
		bc.metrics.MaxLatency = latency
	}

	// Simple moving average for latency
	if bc.metrics.AvgLatency == 0 {
		bc.metrics.AvgLatency = latency
	} else {
		bc.metrics.AvgLatency = (bc.metrics.AvgLatency + latency) / 2
	}

	now := time.Now()
	bc.metrics.LastMessageTime = &now
}

// recordSentMessage increments the sent message counter
func (bc *BaseConnector) recordSentMessage() {
	atomic.AddInt64(&bc.metrics.MessagesSent, 1)
}

// recordReceivedMessage increments the received message counter
func (bc *BaseConnector) recordReceivedMessage() {
	atomic.AddInt64(&bc.metrics.MessagesReceived, 1)
}

// recordDroppedMessage increments the dropped message counter
func (bc *BaseConnector) recordDroppedMessage() {
	atomic.AddInt64(&bc.metrics.MessagesDropped, 1)
}

// startHealthCheck starts the health check routine
func (bc *BaseConnector) startHealthCheck(healthCheckFunc func(context.Context) error) {
	if !bc.config.HealthCheck.Enabled {
		return
	}

	if bc.healthTicker != nil {
		bc.healthTicker.Stop()
	}

	interval := bc.config.HealthCheck.Interval
	if interval == 0 {
		interval = 30 * time.Second // Default interval
	}

	bc.healthTicker = time.NewTicker(interval)

	go func() {
		defer bc.healthTicker.Stop()

		for {
			select {
			case <-bc.ctx.Done():
				return
			case <-bc.healthTicker.C:
				bc.performHealthCheck(healthCheckFunc)
			}
		}
	}()
}

// performHealthCheck performs a health check
func (bc *BaseConnector) performHealthCheck(healthCheckFunc func(context.Context) error) {
	timeout := bc.config.HealthCheck.Timeout
	if timeout == 0 {
		timeout = 10 * time.Second // Default timeout
	}

	ctx, cancel := context.WithTimeout(bc.ctx, timeout)
	defer cancel()

	now := time.Now()
	err := healthCheckFunc(ctx)

	bc.mu.Lock()
	defer bc.mu.Unlock()

	bc.status.HealthStatus.LastCheckTime = now
	bc.status.HealthStatus.CheckCount++

	if err != nil {
		bc.status.HealthStatus.Healthy = false
		bc.status.HealthStatus.FailureCount++
		bc.status.HealthStatus.Message = err.Error()

		// Check if we've exceeded max failures
		if bc.config.HealthCheck.MaxFails > 0 &&
		   bc.status.HealthStatus.FailureCount >= int64(bc.config.HealthCheck.MaxFails) {
			bc.setState(StateFailed)
		}
	} else {
		bc.status.HealthStatus.Healthy = true
		bc.status.HealthStatus.Message = ""

		// Reset failure count on successful check
		bc.status.HealthStatus.FailureCount = 0
	}
}

// startRetryMechanism starts the retry mechanism for failed operations
func (bc *BaseConnector) startRetryMechanism(retryFunc func() error) {
	if bc.config.Retry.MaxAttempts <= 0 {
		return
	}

	go func() {
		attempts := 0
		backoff := bc.config.Retry.InitialBackoff
		if backoff == 0 {
			backoff = 1 * time.Second
		}

		for attempts < bc.config.Retry.MaxAttempts {
			select {
			case <-bc.ctx.Done():
				return
			default:
			}

			if bc.state != StateFailed && bc.state != StateReconnecting {
				return // Stop retrying if not in failed state
			}

			attempts++
			bc.setState(StateReconnecting)

			// Set next retry time
			nextRetry := time.Now().Add(backoff)
			bc.mu.Lock()
			bc.status.NextRetryTime = &nextRetry
			bc.mu.Unlock()

			// Wait for backoff period
			select {
			case <-bc.ctx.Done():
				return
			case <-time.After(backoff):
			}

			if err := retryFunc(); err == nil {
				// Success! Reset retry count and return
				atomic.StoreInt64(&bc.retryCount, 0)
				bc.setState(StateRunning)
				return
			}

			// Calculate next backoff
			multiplier := bc.config.Retry.BackoffMultiplier
			if multiplier <= 0 {
				multiplier = 2.0
			}
			backoff = time.Duration(float64(backoff) * multiplier)

			if bc.config.Retry.MaxBackoff > 0 && backoff > bc.config.Retry.MaxBackoff {
				backoff = bc.config.Retry.MaxBackoff
			}
		}

		// Max attempts reached
		bc.setState(StateFailed)
	}()
}

// Close closes the base connector
func (bc *BaseConnector) Close() error {
	bc.cancel()

	if bc.healthTicker != nil {
		bc.healthTicker.Stop()
	}

	bc.setState(StateStopped)
	return nil
}

// Context returns the connector context
func (bc *BaseConnector) Context() context.Context {
	return bc.ctx
}

// DefaultHealthCheckConfig returns default health check configuration
func DefaultHealthCheckConfig() HealthCheckConfig {
	return HealthCheckConfig{
		Enabled:  true,
		Interval: 30 * time.Second,
		Timeout:  10 * time.Second,
		MaxFails: 3,
	}
}

// DefaultRetryConfig returns default retry configuration
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxAttempts:       3,
		InitialBackoff:    1 * time.Second,
		MaxBackoff:        60 * time.Second,
		BackoffMultiplier: 2.0,
	}
}

// DefaultPoolConfig returns default pool configuration
func DefaultPoolConfig() PoolConfig {
	return PoolConfig{
		MaxSize:     10,
		MinSize:     1,
		MaxIdle:     5 * time.Minute,
		MaxLifetime: 30 * time.Minute,
	}
}