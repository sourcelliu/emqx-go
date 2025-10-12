#!/bin/bash
# Advanced chaos testing with real workload verification
# This script runs chaos tests and measures actual message delivery

set -e

GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Configuration
TEST_DURATION=${1:-30}
RESULTS_DIR="chaos-results-$(date +%Y%m%d-%H%M%S)"
mkdir -p "$RESULTS_DIR"

log_info "Starting advanced chaos testing..."
log_info "Results will be saved to: $RESULTS_DIR"

# Start cluster
log_info "Starting cluster..."
./scripts/start-cluster.sh
sleep 5

# Verify cluster is healthy
log_info "Verifying cluster health..."
if ! ./scripts/health-check-simple.sh > "$RESULTS_DIR/pre-test-health.txt" 2>&1; then
    log_error "Cluster is not healthy"
    exit 1
fi

log_info "Cluster is healthy, starting chaos tests..."

# Test scenarios
declare -a SCENARIOS=(
    "baseline:No fault injection"
    "network-delay:100ms network delay"
    "high-network-delay:500ms network delay"
    "network-loss:10% packet loss"
    "high-network-loss:30% packet loss"
    "combined-network:100ms delay + 10% loss"
    "cpu-stress:80% CPU stress"
    "extreme-cpu-stress:95% CPU stress"
    "clock-skew:5 second clock skew"
    "cascade-failure:Multiple faults combined"
)

# Run each scenario
for scenario_info in "${SCENARIOS[@]}"; do
    scenario="${scenario_info%%:*}"
    description="${scenario_info#*:}"

    log_info "================================================"
    log_info "Running scenario: $scenario"
    log_info "Description: $description"
    log_info "================================================"

    # Create scenario directory
    scenario_dir="$RESULTS_DIR/$scenario"
    mkdir -p "$scenario_dir"

    # Run chaos test
    log_info "Executing chaos injection..."
    ./bin/chaos-test-runner -scenario "$scenario" -duration "$TEST_DURATION" \
        > "$scenario_dir/chaos-test.log" 2>&1 &
    CHAOS_PID=$!

    # Wait for chaos to be injected
    sleep 2

    # Run actual workload test
    log_info "Running workload verification..."
    ./bin/cluster-test > "$scenario_dir/cluster-test.log" 2>&1 || true

    # Collect metrics during chaos
    log_info "Collecting metrics..."
    for port in 8082 8084 8086; do
        curl -s "http://localhost:$port/metrics" > "$scenario_dir/metrics-$port.txt" 2>&1 || true
    done

    # Wait for chaos test to complete
    wait $CHAOS_PID || true

    # Check cluster health after
    ./scripts/health-check-simple.sh > "$scenario_dir/post-test-health.txt" 2>&1 || true

    log_info "Scenario $scenario completed"

    # Wait between scenarios
    sleep 3
done

# Stop cluster
log_info "Stopping cluster..."
./scripts/stop-cluster.sh
sleep 2

# Generate summary report
log_info "Generating summary report..."

cat > "$RESULTS_DIR/SUMMARY.md" << 'EOF'
# Advanced Chaos Testing Summary

**Execution Time**: $(date)
**Test Duration per Scenario**: $TEST_DURATION seconds

---

## Test Scenarios

EOF

# Add scenario results
for scenario_info in "${SCENARIOS[@]}"; do
    scenario="${scenario_info%%:*}"
    description="${scenario_info#*:}"
    scenario_dir="$RESULTS_DIR/$scenario"

    echo "### Scenario: $scenario" >> "$RESULTS_DIR/SUMMARY.md"
    echo "" >> "$RESULTS_DIR/SUMMARY.md"
    echo "**Description**: $description" >> "$RESULTS_DIR/SUMMARY.md"
    echo "" >> "$RESULTS_DIR/SUMMARY.md"

    # Check if cluster-test succeeded
    if grep -q "SUCCESS" "$scenario_dir/cluster-test.log" 2>/dev/null; then
        echo "**Cluster Test**: ✅ PASS" >> "$RESULTS_DIR/SUMMARY.md"
    else
        echo "**Cluster Test**: ❌ FAIL" >> "$RESULTS_DIR/SUMMARY.md"
    fi

    # Extract key metrics
    if [ -f "$scenario_dir/metrics-8082.txt" ]; then
        connections=$(grep "^emqx_connections_count" "$scenario_dir/metrics-8082.txt" 2>/dev/null | awk '{print $2}' || echo "0")
        messages=$(grep "^emqx_messages_received_total" "$scenario_dir/metrics-8082.txt" 2>/dev/null | awk '{print $2}' || echo "0")
        echo "**Metrics**: Connections=$connections, Messages=$messages" >> "$RESULTS_DIR/SUMMARY.md"
    fi

    echo "" >> "$RESULTS_DIR/SUMMARY.md"
    echo "---" >> "$RESULTS_DIR/SUMMARY.md"
    echo "" >> "$RESULTS_DIR/SUMMARY.md"
done

log_info "================================================"
log_info "Advanced chaos testing completed!"
log_info "Results saved to: $RESULTS_DIR"
log_info "Summary report: $RESULTS_DIR/SUMMARY.md"
log_info "================================================"

# Display summary
cat "$RESULTS_DIR/SUMMARY.md"
