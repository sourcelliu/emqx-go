package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"
)

var (
	port     = flag.Int("port", 8888, "Dashboard server port")
	dataFile = flag.String("data", "", "Path to chaos results directory")
)

// MetricsData represents real-time chaos metrics
type MetricsData struct {
	Timestamp        time.Time              `json:"timestamp"`
	ActiveScenario   string                 `json:"active_scenario"`
	ClusterStatus    string                 `json:"cluster_status"`
	TotalConnections int                    `json:"total_connections"`
	MessagesReceived int64                  `json:"messages_received"`
	MessagesSent     int64                  `json:"messages_sent"`
	MessagesDropped  int64                  `json:"messages_dropped"`
	SuccessRate      float64                `json:"success_rate"`
	AverageLatency   float64                `json:"average_latency"`
	P99Latency       float64                `json:"p99_latency"`
	ActiveFaults     []string               `json:"active_faults"`
	NodeMetrics      map[string]NodeMetrics `json:"node_metrics"`
}

// NodeMetrics represents per-node metrics
type NodeMetrics struct {
	NodeID      string `json:"node_id"`
	Connections int    `json:"connections"`
	MessagesRx  int64  `json:"messages_rx"`
	MessagesTx  int64  `json:"messages_tx"`
	Status      string `json:"status"`
	Uptime      int64  `json:"uptime"`
}

// Dashboard manages the real-time dashboard
type Dashboard struct {
	mu          sync.RWMutex
	currentData MetricsData
	history     []MetricsData
	maxHistory  int
}

// NewDashboard creates a new dashboard
func NewDashboard() *Dashboard {
	return &Dashboard{
		maxHistory: 100,
		history:    make([]MetricsData, 0, 100),
		currentData: MetricsData{
			Timestamp:    time.Now(),
			NodeMetrics:  make(map[string]NodeMetrics),
			ActiveFaults: make([]string, 0),
		},
	}
}

// UpdateMetrics updates the current metrics
func (d *Dashboard) UpdateMetrics(data MetricsData) {
	d.mu.Lock()
	defer d.mu.Unlock()

	data.Timestamp = time.Now()
	d.currentData = data

	// Add to history
	d.history = append(d.history, data)
	if len(d.history) > d.maxHistory {
		d.history = d.history[1:]
	}
}

// GetCurrent returns current metrics
func (d *Dashboard) GetCurrent() MetricsData {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.currentData
}

// GetHistory returns metrics history
func (d *Dashboard) GetHistory() []MetricsData {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return append([]MetricsData{}, d.history...)
}

// HTML template for the dashboard
const dashboardHTML = `
<!DOCTYPE html>
<html>
<head>
    <title>EMQX-Go Chaos Engineering Dashboard</title>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <style>
        * { margin: 0; padding: 0; box-sizing: border-box; }
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Oxygen, Ubuntu, Cantarell, sans-serif;
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
            color: #333;
            padding: 20px;
        }
        .container {
            max-width: 1400px;
            margin: 0 auto;
        }
        .header {
            background: white;
            padding: 30px;
            border-radius: 10px;
            box-shadow: 0 10px 30px rgba(0,0,0,0.1);
            margin-bottom: 20px;
            text-align: center;
        }
        h1 {
            color: #667eea;
            font-size: 2.5em;
            margin-bottom: 10px;
        }
        .subtitle {
            color: #666;
            font-size: 1.1em;
        }
        .status-bar {
            display: flex;
            gap: 20px;
            margin-bottom: 20px;
            flex-wrap: wrap;
        }
        .status-card {
            flex: 1;
            min-width: 200px;
            background: white;
            padding: 20px;
            border-radius: 10px;
            box-shadow: 0 5px 15px rgba(0,0,0,0.1);
        }
        .status-card h3 {
            color: #667eea;
            margin-bottom: 10px;
            font-size: 0.9em;
            text-transform: uppercase;
            letter-spacing: 1px;
        }
        .status-card .value {
            font-size: 2em;
            font-weight: bold;
            color: #333;
        }
        .status-card .label {
            color: #999;
            font-size: 0.9em;
            margin-top: 5px;
        }
        .metrics-grid {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(300px, 1fr));
            gap: 20px;
            margin-bottom: 20px;
        }
        .metric-card {
            background: white;
            padding: 25px;
            border-radius: 10px;
            box-shadow: 0 5px 15px rgba(0,0,0,0.1);
        }
        .metric-card h2 {
            color: #667eea;
            margin-bottom: 20px;
            font-size: 1.3em;
        }
        .metric-row {
            display: flex;
            justify-content: space-between;
            padding: 10px 0;
            border-bottom: 1px solid #f0f0f0;
        }
        .metric-row:last-child {
            border-bottom: none;
        }
        .metric-label {
            color: #666;
            font-weight: 500;
        }
        .metric-value {
            color: #333;
            font-weight: bold;
        }
        .status-healthy { color: #10b981; }
        .status-warning { color: #f59e0b; }
        .status-critical { color: #ef4444; }
        .fault-badge {
            display: inline-block;
            background: #ef4444;
            color: white;
            padding: 5px 12px;
            border-radius: 20px;
            font-size: 0.85em;
            margin: 5px 5px 5px 0;
        }
        .node-card {
            background: #f9fafb;
            padding: 15px;
            border-radius: 8px;
            margin-bottom: 10px;
        }
        .node-header {
            display: flex;
            justify-content: space-between;
            align-items: center;
            margin-bottom: 10px;
        }
        .node-id {
            font-weight: bold;
            color: #667eea;
        }
        .auto-refresh {
            background: white;
            padding: 15px;
            border-radius: 10px;
            box-shadow: 0 5px 15px rgba(0,0,0,0.1);
            text-align: center;
            margin-bottom: 20px;
        }
        .refresh-button {
            background: #667eea;
            color: white;
            border: none;
            padding: 10px 30px;
            border-radius: 5px;
            cursor: pointer;
            font-size: 1em;
            margin: 0 10px;
        }
        .refresh-button:hover {
            background: #5568d3;
        }
        #lastUpdate {
            color: #666;
            margin-top: 10px;
        }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h1>🔥 EMQX-Go Chaos Engineering Dashboard</h1>
            <div class="subtitle">Real-time System Resilience Monitoring</div>
        </div>

        <div class="auto-refresh">
            <button class="refresh-button" onclick="refreshData()">🔄 Refresh Now</button>
            <button class="refresh-button" onclick="toggleAutoRefresh()">⏱️ <span id="autoRefreshStatus">Enable</span> Auto-Refresh</button>
            <div id="lastUpdate">Last updated: Never</div>
        </div>

        <div class="status-bar">
            <div class="status-card">
                <h3>Active Scenario</h3>
                <div class="value" id="activeScenario">-</div>
            </div>
            <div class="status-card">
                <h3>Cluster Status</h3>
                <div class="value" id="clusterStatus">-</div>
            </div>
            <div class="status-card">
                <h3>Success Rate</h3>
                <div class="value" id="successRate">-</div>
            </div>
            <div class="status-card">
                <h3>Total Connections</h3>
                <div class="value" id="totalConnections">-</div>
            </div>
        </div>

        <div class="metrics-grid">
            <div class="metric-card">
                <h2>📊 Message Metrics</h2>
                <div class="metric-row">
                    <span class="metric-label">Messages Sent</span>
                    <span class="metric-value" id="messagesSent">0</span>
                </div>
                <div class="metric-row">
                    <span class="metric-label">Messages Received</span>
                    <span class="metric-value" id="messagesReceived">0</span>
                </div>
                <div class="metric-row">
                    <span class="metric-label">Messages Dropped</span>
                    <span class="metric-value" id="messagesDropped">0</span>
                </div>
                <div class="metric-row">
                    <span class="metric-label">Drop Rate</span>
                    <span class="metric-value" id="dropRate">0%</span>
                </div>
            </div>

            <div class="metric-card">
                <h2>⚡ Performance Metrics</h2>
                <div class="metric-row">
                    <span class="metric-label">Average Latency</span>
                    <span class="metric-value" id="avgLatency">0ms</span>
                </div>
                <div class="metric-row">
                    <span class="metric-label">P99 Latency</span>
                    <span class="metric-value" id="p99Latency">0ms</span>
                </div>
                <div class="metric-row">
                    <span class="metric-label">Throughput</span>
                    <span class="metric-value" id="throughput">0 msg/s</span>
                </div>
            </div>

            <div class="metric-card">
                <h2>🔧 Active Faults</h2>
                <div id="activeFaults">
                    <span class="fault-badge">No active faults</span>
                </div>
            </div>
        </div>

        <div class="metric-card">
            <h2>🖥️ Cluster Nodes</h2>
            <div id="nodeMetrics"></div>
        </div>
    </div>

    <script>
        let autoRefreshInterval = null;

        function refreshData() {
            fetch('/api/metrics/current')
                .then(response => response.json())
                .then(data => {
                    updateDashboard(data);
                    document.getElementById('lastUpdate').textContent =
                        'Last updated: ' + new Date().toLocaleTimeString();
                })
                .catch(error => {
                    console.error('Error fetching data:', error);
                });
        }

        function updateDashboard(data) {
            // Update status cards
            document.getElementById('activeScenario').textContent = data.active_scenario || 'None';
            document.getElementById('clusterStatus').textContent = data.cluster_status || 'Unknown';
            document.getElementById('clusterStatus').className =
                'value status-' + (data.cluster_status === 'healthy' ? 'healthy' : 'warning');

            const successRate = (data.success_rate * 100).toFixed(2);
            document.getElementById('successRate').textContent = successRate + '%';
            document.getElementById('successRate').className =
                'value status-' + (data.success_rate > 0.99 ? 'healthy' :
                                   data.success_rate > 0.90 ? 'warning' : 'critical');

            document.getElementById('totalConnections').textContent = data.total_connections || 0;

            // Update message metrics
            document.getElementById('messagesSent').textContent = data.messages_sent || 0;
            document.getElementById('messagesReceived').textContent = data.messages_received || 0;
            document.getElementById('messagesDropped').textContent = data.messages_dropped || 0;

            const dropRate = data.messages_sent > 0 ?
                (data.messages_dropped / data.messages_sent * 100).toFixed(2) : 0;
            document.getElementById('dropRate').textContent = dropRate + '%';

            // Update performance metrics
            document.getElementById('avgLatency').textContent =
                (data.average_latency || 0).toFixed(2) + 'ms';
            document.getElementById('p99Latency').textContent =
                (data.p99_latency || 0).toFixed(2) + 'ms';

            const throughput = data.messages_received > 0 ?
                (data.messages_received / 60).toFixed(0) : 0;
            document.getElementById('throughput').textContent = throughput + ' msg/s';

            // Update active faults
            const faultsDiv = document.getElementById('activeFaults');
            if (data.active_faults && data.active_faults.length > 0) {
                faultsDiv.innerHTML = data.active_faults
                    .map(fault => '<span class="fault-badge">' + fault + '</span>')
                    .join('');
            } else {
                faultsDiv.innerHTML = '<span class="fault-badge" style="background: #10b981;">✓ No active faults</span>';
            }

            // Update node metrics
            const nodesDiv = document.getElementById('nodeMetrics');
            if (data.node_metrics && Object.keys(data.node_metrics).length > 0) {
                let html = '';
                for (const [nodeId, metrics] of Object.entries(data.node_metrics)) {
                    const statusClass = metrics.status === 'running' ? 'healthy' : 'critical';
                    const statusText = metrics.status === 'running' ? '✓ Running' : '✗ Down';
                    html += '<div class="node-card">';
                    html += '<div class="node-header">';
                    html += '<span class="node-id">' + nodeId + '</span>';
                    html += '<span class="status-' + statusClass + '">' + statusText + '</span>';
                    html += '</div>';
                    html += '<div class="metric-row">';
                    html += '<span class="metric-label">Connections</span>';
                    html += '<span class="metric-value">' + (metrics.connections || 0) + '</span>';
                    html += '</div>';
                    html += '<div class="metric-row">';
                    html += '<span class="metric-label">Messages RX/TX</span>';
                    html += '<span class="metric-value">' + (metrics.messages_rx || 0) + ' / ' + (metrics.messages_tx || 0) + '</span>';
                    html += '</div>';
                    html += '<div class="metric-row">';
                    html += '<span class="metric-label">Uptime</span>';
                    html += '<span class="metric-value">' + formatUptime(metrics.uptime || 0) + '</span>';
                    html += '</div>';
                    html += '</div>';
                }
                nodesDiv.innerHTML = html;
            } else {
                nodesDiv.innerHTML = '<div style="color: #999; text-align: center; padding: 20px;">No node data available</div>';
            }
        }

        function formatUptime(seconds) {
            const hours = Math.floor(seconds / 3600);
            const minutes = Math.floor((seconds % 3600) / 60);
            const secs = seconds % 60;
            return hours + 'h ' + minutes + 'm ' + secs + 's';
        }

        function toggleAutoRefresh() {
            if (autoRefreshInterval) {
                clearInterval(autoRefreshInterval);
                autoRefreshInterval = null;
                document.getElementById('autoRefreshStatus').textContent = 'Enable';
            } else {
                autoRefreshInterval = setInterval(refreshData, 2000);
                document.getElementById('autoRefreshStatus').textContent = 'Disable';
                refreshData();
            }
        }

        // Initial load
        refreshData();
    </script>
</body>
</html>
`

func main() {
	flag.Parse()

	dashboard := NewDashboard()

	// Serve static dashboard
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, dashboardHTML)
	})

	// API endpoint for current metrics
	http.HandleFunc("/api/metrics/current", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")

		// Get current metrics
		current := dashboard.GetCurrent()

		// If data file specified, try to load from there
		if *dataFile != "" {
			// TODO: Load from results directory
		}

		json.NewEncoder(w).Encode(current)
	})

	// API endpoint for metrics history
	http.HandleFunc("/api/metrics/history", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		json.NewEncoder(w).Encode(dashboard.GetHistory())
	})

	// API endpoint to update metrics (for testing)
	http.HandleFunc("/api/metrics/update", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var data MetricsData
		if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		dashboard.UpdateMetrics(data)
		w.WriteHeader(http.StatusOK)
	})

	addr := fmt.Sprintf(":%d", *port)
	log.Printf("Starting Chaos Engineering Dashboard on http://localhost%s", addr)
	log.Printf("Press Ctrl+C to stop")

	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatal(err)
	}
}
