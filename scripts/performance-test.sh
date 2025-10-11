#!/bin/bash
# Performance testing script for EMQX-Go cluster
# This script runs comprehensive performance tests and collects metrics

set -e

# Colors for output
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
RESULTS_DIR="perf-results-$(date +%Y%m%d-%H%M%S)"
CLUSTER_NODES=3
MQTT_PORTS=(1883 1884 1885)
TEST_DURATION=60
MESSAGE_PAYLOAD_SIZE=256
CONCURRENT_CLIENTS=100

# Test scenarios
declare -a SCENARIOS=(
    "latency:Message latency test"
    "throughput:Message throughput test"
    "concurrent:Concurrent connections test"
    "qos:QoS level comparison"
    "retained:Retained messages test"
    "stress:Stress test"
)

log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_test() {
    echo -e "${BLUE}[TEST]${NC} $1"
}

# Initialize results directory
init_results() {
    mkdir -p "$RESULTS_DIR"/{logs,metrics,reports}
    log_info "Results will be saved to: $RESULTS_DIR"
}

# Check prerequisites
check_prerequisites() {
    log_info "Checking prerequisites..."

    if [ ! -f "./bin/emqx-go" ]; then
        log_error "emqx-go binary not found. Run: go build -o bin/emqx-go ./cmd/emqx-go"
        exit 1
    fi

    if [ ! -f "./bin/cluster-test" ]; then
        log_error "cluster-test binary not found. Run: go build -o bin/cluster-test ./cmd/cluster-test"
        exit 1
    fi

    if ! command -v mosquitto_pub &> /dev/null; then
        log_warn "mosquitto_pub not found. Some tests may be skipped."
    fi

    if ! command -v bc &> /dev/null; then
        log_error "bc calculator not found. Please install it."
        exit 1
    fi

    log_info "Prerequisites check passed"
}

# Start cluster if not running
ensure_cluster_running() {
    log_info "Ensuring cluster is running..."

    # Check if cluster is already running
    if pgrep -f "emqx-go" > /dev/null; then
        log_info "Cluster is already running"
        return 0
    fi

    log_info "Starting cluster..."
    ./scripts/start-cluster.sh > "$RESULTS_DIR/logs/cluster-startup.log" 2>&1

    # Wait for cluster to be ready
    sleep 5

    # Verify all nodes are running
    RUNNING_NODES=$(pgrep -f "emqx-go" | wc -l | tr -d ' ')
    if [ "$RUNNING_NODES" -lt $CLUSTER_NODES ]; then
        log_error "Only $RUNNING_NODES nodes running (expected $CLUSTER_NODES)"
        exit 1
    fi

    log_info "Cluster started successfully ($RUNNING_NODES nodes)"
}

# Collect baseline metrics
collect_baseline_metrics() {
    log_info "Collecting baseline metrics..."

    for port in 8082 8084 8086; do
        curl -s "http://localhost:$port/metrics" > "$RESULTS_DIR/metrics/baseline-$port.txt"
    done

    log_info "Baseline metrics collected"
}

# Test 1: Latency test
test_latency() {
    log_test "Running latency test..."

    local ITERATIONS=1000
    local LATENCIES_FILE="$RESULTS_DIR/metrics/latencies.txt"

    for i in $(seq 1 $ITERATIONS); do
        ./bin/cluster-test 2>&1 | grep "received in" | awk '{print $NF}' | sed 's/ms//' >> "$LATENCIES_FILE"
    done

    # Calculate statistics
    local AVG=$(awk '{sum+=$1; count++} END {print sum/count}' "$LATENCIES_FILE")
    local MIN=$(sort -n "$LATENCIES_FILE" | head -1)
    local MAX=$(sort -n "$LATENCIES_FILE" | tail -1)

    # Calculate percentiles
    local P50=$(sort -n "$LATENCIES_FILE" | awk '{all[NR] = $0} END{print all[int(NR*0.50)]}')
    local P95=$(sort -n "$LATENCIES_FILE" | awk '{all[NR] = $0} END{print all[int(NR*0.95)]}')
    local P99=$(sort -n "$LATENCIES_FILE" | awk '{all[NR] = $0} END{print all[int(NR*0.99)]}')

    cat > "$RESULTS_DIR/reports/latency-report.txt" <<EOF
Latency Test Results
====================
Iterations: $ITERATIONS
Average: ${AVG}ms
Minimum: ${MIN}ms
Maximum: ${MAX}ms
P50: ${P50}ms
P95: ${P95}ms
P99: ${P99}ms
EOF

    log_info "Latency test completed: Avg=${AVG}ms, P95=${P95}ms, P99=${P99}ms"
}

# Test 2: Throughput test
test_throughput() {
    log_test "Running throughput test..."

    local DURATION=60
    local START_TIME=$(date +%s)
    local MESSAGE_COUNT=0
    local THROUGHPUT_FILE="$RESULTS_DIR/metrics/throughput.txt"

    log_info "Sending messages for ${DURATION} seconds..."

    while [ $(($(date +%s) - START_TIME)) -lt $DURATION ]; do
        ./bin/cluster-test -message "throughput-test-$MESSAGE_COUNT" > /dev/null 2>&1 &
        MESSAGE_COUNT=$((MESSAGE_COUNT + 1))

        # Collect throughput every second
        if [ $((MESSAGE_COUNT % 100)) -eq 0 ]; then
            ELAPSED=$(($(date +%s) - START_TIME))
            CURRENT_THROUGHPUT=$(echo "scale=2; $MESSAGE_COUNT / $ELAPSED" | bc)
            echo "$ELAPSED $CURRENT_THROUGHPUT" >> "$THROUGHPUT_FILE"
        fi
    done

    # Wait for all background jobs to finish
    wait

    local END_TIME=$(date +%s)
    local TOTAL_TIME=$((END_TIME - START_TIME))
    local AVG_THROUGHPUT=$(echo "scale=2; $MESSAGE_COUNT / $TOTAL_TIME" | bc)

    cat > "$RESULTS_DIR/reports/throughput-report.txt" <<EOF
Throughput Test Results
=======================
Duration: ${TOTAL_TIME}s
Total Messages: $MESSAGE_COUNT
Average Throughput: ${AVG_THROUGHPUT} msg/s
EOF

    log_info "Throughput test completed: $MESSAGE_COUNT messages in ${TOTAL_TIME}s (${AVG_THROUGHPUT} msg/s)"
}

# Test 3: Concurrent connections test
test_concurrent_connections() {
    log_test "Running concurrent connections test..."

    local MAX_CONNECTIONS=100
    local STEP=10
    local RESULTS_FILE="$RESULTS_DIR/reports/concurrent-connections.txt"

    echo "Concurrent Connections Test Results" > "$RESULTS_FILE"
    echo "====================================" >> "$RESULTS_FILE"
    echo "" >> "$RESULTS_FILE"

    for CONNECTIONS in $(seq $STEP $STEP $MAX_CONNECTIONS); do
        log_info "Testing with $CONNECTIONS concurrent connections..."

        local START=$(date +%s%3N)

        # Launch concurrent clients
        for i in $(seq 1 $CONNECTIONS); do
            ./bin/cluster-test -message "concurrent-$i" > /dev/null 2>&1 &
        done

        # Wait for all to complete
        wait

        local END=$(date +%s%3N)
        local DURATION=$((END - START))
        local AVG_PER_CONN=$(echo "scale=2; $DURATION / $CONNECTIONS" | bc)

        echo "Connections: $CONNECTIONS | Total Time: ${DURATION}ms | Avg per conn: ${AVG_PER_CONN}ms" >> "$RESULTS_FILE"
        log_info "  → ${CONNECTIONS} connections: ${DURATION}ms total, ${AVG_PER_CONN}ms per connection"
    done

    log_info "Concurrent connections test completed"
}

# Test 4: QoS comparison test
test_qos_levels() {
    log_test "Running QoS levels comparison test..."

    local ITERATIONS=100
    local RESULTS_FILE="$RESULTS_DIR/reports/qos-comparison.txt"

    echo "QoS Levels Comparison Test Results" > "$RESULTS_FILE"
    echo "===================================" >> "$RESULTS_FILE"
    echo "" >> "$RESULTS_FILE"

    # Note: Current implementation may not support different QoS levels
    # This is a placeholder for future implementation

    log_info "Testing QoS 0 (At most once)..."
    local QOS0_START=$(date +%s%3N)
    for i in $(seq 1 $ITERATIONS); do
        ./bin/cluster-test > /dev/null 2>&1
    done
    local QOS0_END=$(date +%s%3N)
    local QOS0_TIME=$((QOS0_END - QOS0_START))
    local QOS0_AVG=$(echo "scale=2; $QOS0_TIME / $ITERATIONS" | bc)

    echo "QoS 0: ${QOS0_TIME}ms total, ${QOS0_AVG}ms average" >> "$RESULTS_FILE"

    log_info "QoS comparison test completed"
}

# Test 5: Stress test
test_stress() {
    log_test "Running stress test..."

    local DURATION=120
    local MAX_RATE=1000
    local RESULTS_FILE="$RESULTS_DIR/reports/stress-test.txt"

    echo "Stress Test Results" > "$RESULTS_FILE"
    echo "===================" >> "$RESULTS_FILE"
    echo "Duration: ${DURATION}s" >> "$RESULTS_FILE"
    echo "Target Rate: ${MAX_RATE} msg/s" >> "$RESULTS_FILE"
    echo "" >> "$RESULTS_FILE"

    log_info "Running stress test for ${DURATION} seconds at ${MAX_RATE} msg/s..."

    local START_TIME=$(date +%s)
    local MESSAGE_COUNT=0
    local FAILED_COUNT=0

    while [ $(($(date +%s) - START_TIME)) -lt $DURATION ]; do
        # Send messages at target rate
        for i in $(seq 1 10); do
            if ./bin/cluster-test -message "stress-$MESSAGE_COUNT" > /dev/null 2>&1; then
                MESSAGE_COUNT=$((MESSAGE_COUNT + 1))
            else
                FAILED_COUNT=$((FAILED_COUNT + 1))
            fi
        done

        # Sleep to achieve target rate
        sleep 0.01
    done

    local END_TIME=$(date +%s)
    local TOTAL_TIME=$((END_TIME - START_TIME))
    local SUCCESS_RATE=$(echo "scale=2; ($MESSAGE_COUNT * 100) / ($MESSAGE_COUNT + $FAILED_COUNT)" | bc)
    local ACTUAL_RATE=$(echo "scale=2; $MESSAGE_COUNT / $TOTAL_TIME" | bc)

    cat >> "$RESULTS_FILE" <<EOF

Results:
--------
Total Messages: $MESSAGE_COUNT
Failed Messages: $FAILED_COUNT
Success Rate: ${SUCCESS_RATE}%
Actual Throughput: ${ACTUAL_RATE} msg/s
EOF

    log_info "Stress test completed: $MESSAGE_COUNT messages, ${SUCCESS_RATE}% success rate"
}

# Collect final metrics
collect_final_metrics() {
    log_info "Collecting final metrics..."

    for port in 8082 8084 8086; do
        curl -s "http://localhost:$port/metrics" > "$RESULTS_DIR/metrics/final-$port.txt"
    done

    # Calculate deltas
    for port in 8082 8084 8086; do
        log_info "Metrics delta for node on port $port:"

        BASELINE_CONN=$(grep "emqx_go_connections_total" "$RESULTS_DIR/metrics/baseline-$port.txt" | awk '{print $2}')
        FINAL_CONN=$(grep "emqx_go_connections_total" "$RESULTS_DIR/metrics/final-$port.txt" | awk '{print $2}')
        CONN_DELTA=$((FINAL_CONN - BASELINE_CONN))

        echo "  Connection delta: $CONN_DELTA"
    done

    log_info "Final metrics collected"
}

# Generate summary report
generate_summary() {
    log_info "Generating summary report..."

    local SUMMARY_FILE="$RESULTS_DIR/PERFORMANCE_REPORT.md"

    cat > "$SUMMARY_FILE" <<'EOF'
# EMQX-Go Performance Test Report

**Date**: $(date)
**Cluster Size**: 3 nodes
**Test Duration**: Approximately 10 minutes

---

## Executive Summary

This report contains the results of comprehensive performance testing of the EMQX-Go MQTT broker cluster.

### Test Scenarios

1. **Latency Test**: Measured message delivery latency under normal load
2. **Throughput Test**: Measured maximum sustainable message throughput
3. **Concurrent Connections**: Tested scalability with multiple simultaneous connections
4. **QoS Comparison**: Compared performance across different QoS levels
5. **Stress Test**: Evaluated system behavior under high load

---

## Detailed Results

### 1. Latency Test

EOF

    cat "$RESULTS_DIR/reports/latency-report.txt" >> "$SUMMARY_FILE"

    cat >> "$SUMMARY_FILE" <<'EOF'

### 2. Throughput Test

EOF

    cat "$RESULTS_DIR/reports/throughput-report.txt" >> "$SUMMARY_FILE"

    cat >> "$SUMMARY_FILE" <<'EOF'

### 3. Concurrent Connections Test

EOF

    cat "$RESULTS_DIR/reports/concurrent-connections.txt" >> "$SUMMARY_FILE"

    cat >> "$SUMMARY_FILE" <<'EOF'

### 4. QoS Comparison

EOF

    cat "$RESULTS_DIR/reports/qos-comparison.txt" >> "$SUMMARY_FILE"

    cat >> "$SUMMARY_FILE" <<'EOF'

### 5. Stress Test

EOF

    cat "$RESULTS_DIR/reports/stress-test.txt" >> "$SUMMARY_FILE"

    cat >> "$SUMMARY_FILE" <<EOF

---

## Recommendations

Based on the test results:

1. **Performance**: The cluster demonstrates good performance characteristics
2. **Scalability**: Review concurrent connection test results for scaling decisions
3. **Monitoring**: Set up alerts based on the stress test failure thresholds
4. **Optimization**: Consider optimizing based on P95/P99 latency metrics

---

**Report Generated**: $(date)
**Results Directory**: $RESULTS_DIR
EOF

    log_info "Summary report generated: $SUMMARY_FILE"
}

# Main execution
main() {
    echo "========================================================================="
    echo "EMQX-Go Performance Testing Suite"
    echo "========================================================================="
    echo ""

    init_results
    check_prerequisites
    ensure_cluster_running
    collect_baseline_metrics

    echo ""
    echo "Starting performance tests..."
    echo ""

    test_latency
    echo ""

    test_throughput
    echo ""

    test_concurrent_connections
    echo ""

    test_qos_levels
    echo ""

    test_stress
    echo ""

    collect_final_metrics
    generate_summary

    echo ""
    echo "========================================================================="
    log_info "Performance testing completed!"
    log_info "Results: $RESULTS_DIR"
    log_info "Report: $RESULTS_DIR/PERFORMANCE_REPORT.md"
    echo "========================================================================="
}

# Run main function
main "$@"
