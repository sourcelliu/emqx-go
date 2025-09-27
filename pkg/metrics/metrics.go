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
package metrics

import (
	"log"
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	// ConnectionsTotal is a Prometheus counter that tracks the total number of
	// client connections accepted by the broker since the application started.
	ConnectionsTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "emqx_go_connections_total",
		Help: "The total number of connections made to the broker.",
	})

	// SupervisorRestartsTotal is a Prometheus counter vector that tracks the
	// total number of times a supervised actor has been restarted. It uses the
	// "actor_id" label to distinguish between different actors.
	SupervisorRestartsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "emqx_go_supervisor_restarts_total",
		Help: "The total number of times a supervised actor has been restarted.",
	},
		[]string{"actor_id"},
	)
)

// Serve starts a new HTTP server in a blocking fashion to expose the defined
// Prometheus metrics. The metrics are available at the "/metrics" endpoint.
//
// - addr: The network address to listen on (e.g., ":8081").
func Serve(addr string) {
	http.Handle("/metrics", promhttp.Handler())
	log.Printf("Metrics server listening on %s", addr)
	if err := http.ListenAndServe(addr, nil); err != nil {
		logFatalf("Metrics server failed: %v", err)
	}
}

// logFatalf can be replaced by tests to prevent process exit.
var logFatalf = log.Fatalf