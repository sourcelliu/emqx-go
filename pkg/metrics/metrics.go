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

// Package metrics defines and exposes Prometheus metrics for monitoring the
// application's health and performance. It provides a central place for all
// metric definitions and includes an HTTP server to expose them for scraping.
// This package implements EMQX-compatible monitoring metrics and statistics.
package metrics

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// MetricsManager manages all broker metrics and statistics
type MetricsManager struct {
	mu sync.RWMutex

	// Connection metrics
	connectionsCount    int64
	connectionsMax      int64
	connectionsTotal    int64
	disconnectionsTotal int64

	// Session metrics
	sessionsCount     int64
	sessionsMax       int64
	sessionsPersistent int64
	sessionsTransient  int64

	// Subscription metrics
	subscriptionsCount int64
	subscriptionsMax   int64
	subscriptionsShared int64

	// Message metrics
	messagesReceived   int64
	messagesSent       int64
	messagesDropped    int64
	messagesRetained   int64
	messagesQos0       int64
	messagesQos1       int64
	messagesQos2       int64

	// Route metrics
	routesCount int64
	routesMax   int64

	// Topic metrics
	topicsCount int64
	topicsMax   int64

	// Packet metrics
	packetsReceived   int64
	packetsSent       int64
	packetsConnect    int64
	packetsDisconnect int64
	packetsPublish    int64
	packetsSubscribe  int64
	packetsUnsubscribe int64

	// Error metrics
	errorsTotal       int64
	authFailures      int64
	aclDenied         int64

	// Cluster metrics
	nodesRunning      int64
	nodesStopped      int64

	// Timestamps
	startTime     time.Time
	uptime        time.Duration
}

var (
	// Global metrics manager instance
	DefaultManager = NewMetricsManager()

	// Legacy Prometheus metrics for backward compatibility
	ConnectionsTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "emqx_go_connections_total",
		Help: "The total number of connections made to the broker.",
	})

	SupervisorRestartsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "emqx_go_supervisor_restarts_total",
		Help: "The total number of times a supervised actor has been restarted.",
	},
		[]string{"actor_id"},
	)

	// New EMQX-compatible Prometheus metrics
	ConnectionsGauge = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "emqx_connections_count",
		Help: "Current number of active connections",
	})

	SessionsGauge = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "emqx_sessions_count",
		Help: "Current number of active sessions",
	})

	SubscriptionsGauge = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "emqx_subscriptions_count",
		Help: "Current number of subscriptions",
	})

	MessagesReceivedCounter = promauto.NewCounter(prometheus.CounterOpts{
		Name: "emqx_messages_received_total",
		Help: "Total number of received messages",
	})

	MessagesSentCounter = promauto.NewCounter(prometheus.CounterOpts{
		Name: "emqx_messages_sent_total",
		Help: "Total number of sent messages",
	})

	MessagesDroppedCounter = promauto.NewCounter(prometheus.CounterOpts{
		Name: "emqx_messages_dropped_total",
		Help: "Total number of dropped messages",
	})

	PacketsReceivedCounter = promauto.NewCounter(prometheus.CounterOpts{
		Name: "emqx_packets_received_total",
		Help: "Total number of received packets",
	})

	PacketsSentCounter = promauto.NewCounter(prometheus.CounterOpts{
		Name: "emqx_packets_sent_total",
		Help: "Total number of sent packets",
	})

	UptimeGauge = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "emqx_uptime_seconds",
		Help: "Broker uptime in seconds",
	})
)

// NewMetricsManager creates a new metrics manager instance
func NewMetricsManager() *MetricsManager {
	return &MetricsManager{
		startTime: time.Now(),
	}
}

// Statistics represents the current statistics snapshot
type Statistics struct {
	Connections struct {
		Count int64 `json:"count"`
		Max   int64 `json:"max"`
	} `json:"connections"`

	Sessions struct {
		Count      int64 `json:"count"`
		Max        int64 `json:"max"`
		Persistent int64 `json:"persistent"`
		Transient  int64 `json:"transient"`
	} `json:"sessions"`

	Subscriptions struct {
		Count  int64 `json:"count"`
		Max    int64 `json:"max"`
		Shared int64 `json:"shared"`
	} `json:"subscriptions"`

	Routes struct {
		Count int64 `json:"count"`
		Max   int64 `json:"max"`
	} `json:"routes"`

	Topics struct {
		Count int64 `json:"count"`
		Max   int64 `json:"max"`
	} `json:"topics"`

	Messages struct {
		Received int64 `json:"received"`
		Sent     int64 `json:"sent"`
		Dropped  int64 `json:"dropped"`
		Retained int64 `json:"retained"`
		Qos0     int64 `json:"qos0"`
		Qos1     int64 `json:"qos1"`
		Qos2     int64 `json:"qos2"`
	} `json:"messages"`

	Packets struct {
		Received    int64 `json:"received"`
		Sent        int64 `json:"sent"`
		Connect     int64 `json:"connect"`
		Disconnect  int64 `json:"disconnect"`
		Publish     int64 `json:"publish"`
		Subscribe   int64 `json:"subscribe"`
		Unsubscribe int64 `json:"unsubscribe"`
	} `json:"packets"`

	Errors struct {
		Total        int64 `json:"total"`
		AuthFailures int64 `json:"auth_failures"`
		AclDenied    int64 `json:"acl_denied"`
	} `json:"errors"`

	Node struct {
		Uptime    int64  `json:"uptime"`
		StartTime string `json:"start_time"`
	} `json:"node"`
}

// GetStatistics returns current statistics snapshot
func (m *MetricsManager) GetStatistics() Statistics {
	m.mu.RLock()
	defer m.mu.RUnlock()

	m.uptime = time.Since(m.startTime)

	return Statistics{
		Connections: struct {
			Count int64 `json:"count"`
			Max   int64 `json:"max"`
		}{
			Count: atomic.LoadInt64(&m.connectionsCount),
			Max:   atomic.LoadInt64(&m.connectionsMax),
		},
		Sessions: struct {
			Count      int64 `json:"count"`
			Max        int64 `json:"max"`
			Persistent int64 `json:"persistent"`
			Transient  int64 `json:"transient"`
		}{
			Count:      atomic.LoadInt64(&m.sessionsCount),
			Max:        atomic.LoadInt64(&m.sessionsMax),
			Persistent: atomic.LoadInt64(&m.sessionsPersistent),
			Transient:  atomic.LoadInt64(&m.sessionsTransient),
		},
		Subscriptions: struct {
			Count  int64 `json:"count"`
			Max    int64 `json:"max"`
			Shared int64 `json:"shared"`
		}{
			Count:  atomic.LoadInt64(&m.subscriptionsCount),
			Max:    atomic.LoadInt64(&m.subscriptionsMax),
			Shared: atomic.LoadInt64(&m.subscriptionsShared),
		},
		Routes: struct {
			Count int64 `json:"count"`
			Max   int64 `json:"max"`
		}{
			Count: atomic.LoadInt64(&m.routesCount),
			Max:   atomic.LoadInt64(&m.routesMax),
		},
		Topics: struct {
			Count int64 `json:"count"`
			Max   int64 `json:"max"`
		}{
			Count: atomic.LoadInt64(&m.topicsCount),
			Max:   atomic.LoadInt64(&m.topicsMax),
		},
		Messages: struct {
			Received int64 `json:"received"`
			Sent     int64 `json:"sent"`
			Dropped  int64 `json:"dropped"`
			Retained int64 `json:"retained"`
			Qos0     int64 `json:"qos0"`
			Qos1     int64 `json:"qos1"`
			Qos2     int64 `json:"qos2"`
		}{
			Received: atomic.LoadInt64(&m.messagesReceived),
			Sent:     atomic.LoadInt64(&m.messagesSent),
			Dropped:  atomic.LoadInt64(&m.messagesDropped),
			Retained: atomic.LoadInt64(&m.messagesRetained),
			Qos0:     atomic.LoadInt64(&m.messagesQos0),
			Qos1:     atomic.LoadInt64(&m.messagesQos1),
			Qos2:     atomic.LoadInt64(&m.messagesQos2),
		},
		Packets: struct {
			Received    int64 `json:"received"`
			Sent        int64 `json:"sent"`
			Connect     int64 `json:"connect"`
			Disconnect  int64 `json:"disconnect"`
			Publish     int64 `json:"publish"`
			Subscribe   int64 `json:"subscribe"`
			Unsubscribe int64 `json:"unsubscribe"`
		}{
			Received:    atomic.LoadInt64(&m.packetsReceived),
			Sent:        atomic.LoadInt64(&m.packetsSent),
			Connect:     atomic.LoadInt64(&m.packetsConnect),
			Disconnect:  atomic.LoadInt64(&m.packetsDisconnect),
			Publish:     atomic.LoadInt64(&m.packetsPublish),
			Subscribe:   atomic.LoadInt64(&m.packetsSubscribe),
			Unsubscribe: atomic.LoadInt64(&m.packetsUnsubscribe),
		},
		Errors: struct {
			Total        int64 `json:"total"`
			AuthFailures int64 `json:"auth_failures"`
			AclDenied    int64 `json:"acl_denied"`
		}{
			Total:        atomic.LoadInt64(&m.errorsTotal),
			AuthFailures: atomic.LoadInt64(&m.authFailures),
			AclDenied:    atomic.LoadInt64(&m.aclDenied),
		},
		Node: struct {
			Uptime    int64  `json:"uptime"`
			StartTime string `json:"start_time"`
		}{
			Uptime:    int64(m.uptime.Seconds()),
			StartTime: m.startTime.Format(time.RFC3339),
		},
	}
}

// Connection metrics methods
func (m *MetricsManager) IncConnections() {
	count := atomic.AddInt64(&m.connectionsCount, 1)
	atomic.AddInt64(&m.connectionsTotal, 1)

	// Update max if necessary
	for {
		max := atomic.LoadInt64(&m.connectionsMax)
		if count <= max || atomic.CompareAndSwapInt64(&m.connectionsMax, max, count) {
			break
		}
	}

	// Update Prometheus metrics
	ConnectionsGauge.Inc()
	ConnectionsTotal.Inc()
}

func (m *MetricsManager) DecConnections() {
	atomic.AddInt64(&m.connectionsCount, -1)
	atomic.AddInt64(&m.disconnectionsTotal, 1)
	ConnectionsGauge.Dec()
}

// Session metrics methods
func (m *MetricsManager) IncSessions(persistent bool) {
	count := atomic.AddInt64(&m.sessionsCount, 1)
	if persistent {
		atomic.AddInt64(&m.sessionsPersistent, 1)
	} else {
		atomic.AddInt64(&m.sessionsTransient, 1)
	}

	// Update max if necessary
	for {
		max := atomic.LoadInt64(&m.sessionsMax)
		if count <= max || atomic.CompareAndSwapInt64(&m.sessionsMax, max, count) {
			break
		}
	}

	SessionsGauge.Inc()
}

func (m *MetricsManager) DecSessions(persistent bool) {
	atomic.AddInt64(&m.sessionsCount, -1)
	if persistent {
		atomic.AddInt64(&m.sessionsPersistent, -1)
	} else {
		atomic.AddInt64(&m.sessionsTransient, -1)
	}
	SessionsGauge.Dec()
}

// Subscription metrics methods
func (m *MetricsManager) IncSubscriptions(shared bool) {
	count := atomic.AddInt64(&m.subscriptionsCount, 1)
	if shared {
		atomic.AddInt64(&m.subscriptionsShared, 1)
	}

	// Update max if necessary
	for {
		max := atomic.LoadInt64(&m.subscriptionsMax)
		if count <= max || atomic.CompareAndSwapInt64(&m.subscriptionsMax, max, count) {
			break
		}
	}

	SubscriptionsGauge.Inc()
}

func (m *MetricsManager) DecSubscriptions(shared bool) {
	atomic.AddInt64(&m.subscriptionsCount, -1)
	if shared {
		atomic.AddInt64(&m.subscriptionsShared, -1)
	}
	SubscriptionsGauge.Dec()
}

// Message metrics methods
func (m *MetricsManager) IncMessagesReceived(qos byte) {
	atomic.AddInt64(&m.messagesReceived, 1)
	switch qos {
	case 0:
		atomic.AddInt64(&m.messagesQos0, 1)
	case 1:
		atomic.AddInt64(&m.messagesQos1, 1)
	case 2:
		atomic.AddInt64(&m.messagesQos2, 1)
	}
	MessagesReceivedCounter.Inc()
}

func (m *MetricsManager) IncMessagesSent(qos byte) {
	atomic.AddInt64(&m.messagesSent, 1)
	MessagesSentCounter.Inc()
}

func (m *MetricsManager) IncMessagesDropped() {
	atomic.AddInt64(&m.messagesDropped, 1)
	MessagesDroppedCounter.Inc()
}

func (m *MetricsManager) IncMessagesRetained() {
	atomic.AddInt64(&m.messagesRetained, 1)
}

// Packet metrics methods
func (m *MetricsManager) IncPacketsReceived(packetType string) {
	atomic.AddInt64(&m.packetsReceived, 1)
	switch packetType {
	case "CONNECT":
		atomic.AddInt64(&m.packetsConnect, 1)
	case "DISCONNECT":
		atomic.AddInt64(&m.packetsDisconnect, 1)
	case "PUBLISH":
		atomic.AddInt64(&m.packetsPublish, 1)
	case "SUBSCRIBE":
		atomic.AddInt64(&m.packetsSubscribe, 1)
	case "UNSUBSCRIBE":
		atomic.AddInt64(&m.packetsUnsubscribe, 1)
	}
	PacketsReceivedCounter.Inc()
}

func (m *MetricsManager) IncPacketsSent() {
	atomic.AddInt64(&m.packetsSent, 1)
	PacketsSentCounter.Inc()
}

// Error metrics methods
func (m *MetricsManager) IncErrors() {
	atomic.AddInt64(&m.errorsTotal, 1)
}

func (m *MetricsManager) IncAuthFailures() {
	atomic.AddInt64(&m.authFailures, 1)
	atomic.AddInt64(&m.errorsTotal, 1)
}

func (m *MetricsManager) IncAclDenied() {
	atomic.AddInt64(&m.aclDenied, 1)
	atomic.AddInt64(&m.errorsTotal, 1)
}

// Route and topic metrics methods
func (m *MetricsManager) SetRoutes(count int64) {
	atomic.StoreInt64(&m.routesCount, count)
	for {
		max := atomic.LoadInt64(&m.routesMax)
		if count <= max || atomic.CompareAndSwapInt64(&m.routesMax, max, count) {
			break
		}
	}
}

func (m *MetricsManager) SetTopics(count int64) {
	atomic.StoreInt64(&m.topicsCount, count)
	for {
		max := atomic.LoadInt64(&m.topicsMax)
		if count <= max || atomic.CompareAndSwapInt64(&m.topicsMax, max, count) {
			break
		}
	}
}

// UpdatePrometheusMetrics updates all Prometheus metrics based on current state
func (m *MetricsManager) UpdatePrometheusMetrics() {
	stats := m.GetStatistics()

	ConnectionsGauge.Set(float64(stats.Connections.Count))
	SessionsGauge.Set(float64(stats.Sessions.Count))
	SubscriptionsGauge.Set(float64(stats.Subscriptions.Count))
	UptimeGauge.Set(float64(stats.Node.Uptime))
}

// statsHandler handles /api/v5/stats endpoint
func statsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	stats := DefaultManager.GetStatistics()
	if err := json.NewEncoder(w).Encode(stats); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

// metricsHandler handles /api/v5/metrics endpoint for Prometheus format
func metricsHandler(w http.ResponseWriter, r *http.Request) {
	DefaultManager.UpdatePrometheusMetrics()
	promhttp.Handler().ServeHTTP(w, r)
}

// Serve starts a new HTTP server in a blocking fashion to expose the defined
// Prometheus metrics. The metrics are available at the "/metrics" endpoint.
// Additionally provides EMQX-compatible API endpoints.
//
// - addr: The network address to listen on (e.g., ":8081").
func Serve(addr string) {
	// Prometheus metrics endpoint
	http.Handle("/metrics", http.HandlerFunc(metricsHandler))

	// EMQX-compatible API endpoints
	http.HandleFunc("/api/v5/stats", statsHandler)
	http.HandleFunc("/api/v5/prometheus/stats", metricsHandler)

	log.Printf("Metrics server listening on %s", addr)
	log.Printf("Available endpoints:")
	log.Printf("  - /metrics (Prometheus format)")
	log.Printf("  - /api/v5/stats (EMQX statistics)")
	log.Printf("  - /api/v5/prometheus/stats (Prometheus format)")

	if err := http.ListenAndServe(addr, nil); err != nil {
		logFatalf("Metrics server failed: %v", err)
	}
}

// logFatalf can be replaced by tests to prevent process exit.
var logFatalf = log.Fatalf