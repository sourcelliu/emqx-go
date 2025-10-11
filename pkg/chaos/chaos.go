// Package chaos provides fault injection capabilities for chaos engineering
package chaos

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"time"
)

// FaultType represents different types of failures
type FaultType string

const (
	FaultTypePodKill       FaultType = "pod-kill"
	FaultTypeNetworkDelay  FaultType = "network-delay"
	FaultTypeNetworkLoss   FaultType = "network-loss"
	FaultTypeNetworkPartition FaultType = "network-partition"
	FaultTypeCPUStress     FaultType = "cpu-stress"
	FaultTypeMemoryStress  FaultType = "memory-stress"
	FaultTypeIOStress      FaultType = "io-stress"
	FaultTypeClockSkew     FaultType = "clock-skew"
)

// Injector manages fault injection
type Injector struct {
	mu               sync.RWMutex
	enabled          bool
	networkDelay     time.Duration
	networkLossRate  float64
	partitionedNodes map[string]bool
	cpuStressLevel   int // 0-100
	memoryStressLevel int // 0-100
	clockSkew        time.Duration
	stopChan         chan struct{}
}

// NewInjector creates a new fault injector
func NewInjector() *Injector {
	return &Injector{
		enabled:          false,
		partitionedNodes: make(map[string]bool),
		stopChan:         make(chan struct{}),
	}
}

// Enable enables fault injection
func (i *Injector) Enable() {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.enabled = true
}

// Disable disables fault injection
func (i *Injector) Disable() {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.enabled = false
	i.networkDelay = 0
	i.networkLossRate = 0
	i.partitionedNodes = make(map[string]bool)
	i.cpuStressLevel = 0
	i.memoryStressLevel = 0
	i.clockSkew = 0
}

// IsEnabled returns if fault injection is enabled
func (i *Injector) IsEnabled() bool {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.enabled
}

// InjectNetworkDelay injects network delay
func (i *Injector) InjectNetworkDelay(delay time.Duration) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.networkDelay = delay
	fmt.Printf("[CHAOS] Injecting network delay: %v\n", delay)
}

// GetNetworkDelay returns current network delay
func (i *Injector) GetNetworkDelay() time.Duration {
	i.mu.RLock()
	defer i.mu.RUnlock()
	if !i.enabled {
		return 0
	}
	return i.networkDelay
}

// InjectNetworkLoss injects packet loss
func (i *Injector) InjectNetworkLoss(lossRate float64) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.networkLossRate = lossRate
	fmt.Printf("[CHAOS] Injecting network loss: %.2f%%\n", lossRate*100)
}

// ShouldDropPacket returns true if packet should be dropped
func (i *Injector) ShouldDropPacket() bool {
	i.mu.RLock()
	defer i.mu.RUnlock()
	if !i.enabled || i.networkLossRate == 0 {
		return false
	}
	return rand.Float64() < i.networkLossRate
}

// InjectNetworkPartition simulates network partition
func (i *Injector) InjectNetworkPartition(nodeID string) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.partitionedNodes[nodeID] = true
	fmt.Printf("[CHAOS] Network partition injected for node: %s\n", nodeID)
}

// IsNodePartitioned checks if a node is partitioned
func (i *Injector) IsNodePartitioned(nodeID string) bool {
	i.mu.RLock()
	defer i.mu.RUnlock()
	if !i.enabled {
		return false
	}
	return i.partitionedNodes[nodeID]
}

// RemoveNetworkPartition removes network partition
func (i *Injector) RemoveNetworkPartition(nodeID string) {
	i.mu.Lock()
	defer i.mu.Unlock()
	delete(i.partitionedNodes, nodeID)
	fmt.Printf("[CHAOS] Network partition removed for node: %s\n", nodeID)
}

// InjectCPUStress injects CPU stress
func (i *Injector) InjectCPUStress(level int) {
	i.mu.Lock()
	defer i.mu.Unlock()
	if level < 0 {
		level = 0
	}
	if level > 100 {
		level = 100
	}
	i.cpuStressLevel = level
	fmt.Printf("[CHAOS] Injecting CPU stress: %d%%\n", level)
}

// StartCPUStress starts CPU stress in background
func (i *Injector) StartCPUStress(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(10 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-i.stopChan:
				return
			case <-ticker.C:
				i.mu.RLock()
				level := i.cpuStressLevel
				enabled := i.enabled
				i.mu.RUnlock()

				if !enabled || level == 0 {
					continue
				}

				// Busy loop to consume CPU
				duration := time.Duration(level) * time.Millisecond / 10
				start := time.Now()
				for time.Since(start) < duration {
					// Busy work
					_ = rand.Int()
				}
			}
		}
	}()
}

// InjectMemoryStress injects memory stress
func (i *Injector) InjectMemoryStress(level int) {
	i.mu.Lock()
	defer i.mu.Unlock()
	if level < 0 {
		level = 0
	}
	if level > 100 {
		level = 100
	}
	i.memoryStressLevel = level
	fmt.Printf("[CHAOS] Injecting memory stress: %d%%\n", level)
}

// GetMemoryStressLevel returns current memory stress level
func (i *Injector) GetMemoryStressLevel() int {
	i.mu.RLock()
	defer i.mu.RUnlock()
	if !i.enabled {
		return 0
	}
	return i.memoryStressLevel
}

// InjectClockSkew injects clock skew
func (i *Injector) InjectClockSkew(skew time.Duration) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.clockSkew = skew
	fmt.Printf("[CHAOS] Injecting clock skew: %v\n", skew)
}

// GetAdjustedTime returns time adjusted by clock skew
func (i *Injector) GetAdjustedTime() time.Time {
	i.mu.RLock()
	defer i.mu.RUnlock()
	if !i.enabled {
		return time.Now()
	}
	return time.Now().Add(i.clockSkew)
}

// Stop stops all background stress operations
func (i *Injector) Stop() {
	close(i.stopChan)
}

// Global injector instance
var globalInjector = NewInjector()

// GetGlobalInjector returns the global injector
func GetGlobalInjector() *Injector {
	return globalInjector
}

// ApplyNetworkDelay applies network delay if chaos is enabled
func ApplyNetworkDelay() {
	delay := globalInjector.GetNetworkDelay()
	if delay > 0 {
		time.Sleep(delay)
	}
}

// ShouldDropPacket checks if packet should be dropped
func ShouldDropPacket() bool {
	return globalInjector.ShouldDropPacket()
}

// IsNodePartitioned checks if node is partitioned
func IsNodePartitioned(nodeID string) bool {
	return globalInjector.IsNodePartitioned(nodeID)
}
