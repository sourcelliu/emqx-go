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

// package metrics provides Prometheus metrics for the application.
package metrics

import (
	"log"
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	// ConnectionsTotal is a counter for the total number of connections.
	ConnectionsTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "emqx_go_connections_total",
		Help: "The total number of connections made to the broker.",
	})

	// SupervisorRestartsTotal is a counter for the total number of supervisor restarts.
	SupervisorRestartsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "emqx_go_supervisor_restarts_total",
		Help: "The total number of times a supervised actor has been restarted.",
	},
		[]string{"actor_id"},
	)
)

// Serve starts an HTTP server to expose the Prometheus metrics.
func Serve(addr string) {
	http.Handle("/metrics", promhttp.Handler())
	log.Printf("Metrics server listening on %s", addr)
	if err := http.ListenAndServe(addr, nil); err != nil {
		logFatalf("Metrics server failed: %v", err)
	}
}

// logFatalf can be replaced by tests to prevent process exit.
var logFatalf = log.Fatalf