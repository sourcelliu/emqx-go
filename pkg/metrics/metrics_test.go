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

package metrics

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMetrics(t *testing.T) {
	assert.NotNil(t, ConnectionsTotal)
	assert.NotNil(t, SupervisorRestartsTotal)
}

func TestServe(t *testing.T) {
	// Find an available port
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := listener.Addr().String()

	// We need to run the server in a goroutine as it's a blocking call.
	// We also need to replace the fatal logging with something that won't exit the test.
	originalLogFatalf := logFatalf
	defer func() { logFatalf = originalLogFatalf }()

	serverErrChan := make(chan error, 1)
	logFatalf = func(format string, v ...interface{}) {
		// Instead of exiting, send the error to a channel
		serverErrChan <- fmt.Errorf(format, v...)
	}

	go func() {
		// Use the listener for the server to ensure we have control over the port
		// and can shut it down.
		server := &http.Server{}
		http.DefaultServeMux = http.NewServeMux() // Reset handler to avoid test pollution
		http.Handle("/metrics", promhttp.Handler())
		_ = server.Serve(listener)
	}()

	// Give the server a moment to start
	time.Sleep(100 * time.Millisecond)

	// Trigger the metrics so they appear in the output
	ConnectionsTotal.Inc()
	SupervisorRestartsTotal.WithLabelValues("test-actor").Inc()

	// Make a request to the metrics endpoint
	resp, err := http.Get("http://" + addr + "/metrics")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	// Check for our custom metrics
	assert.Contains(t, string(body), "emqx_go_connections_total")
	assert.Contains(t, string(body), "emqx_go_supervisor_restarts_total")

	// Shutdown the server by closing the listener
	require.NoError(t, listener.Close())

	// Check if the server goroutine exited with an error
	select {
	case err := <-serverErrChan:
		// We expect an error from closing the listener, but it shouldn't be a fatal one
		// from starting up.
		if err != nil && !strings.Contains(err.Error(), "use of closed network connection") {
			t.Fatalf("server failed unexpectedly: %v", err)
		}
	case <-time.After(1 * time.Second):
		// No error, which is also fine.
	}
}