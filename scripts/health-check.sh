#!/usr/bin/env bash
# Cluster health check script for EMQX-Go
# Performs comprehensive health checks on the cluster

set -e

# Colors
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

# Configuration
MQTT_PORTS=(1883 1884 1885)
GRPC_PORTS=(8081 8083 8085)
METRICS_PORTS=(8082 8084 8086)
DASHBOARD_PORTS=(18083 18084 18085)
EXPECTED_NODES=3

# Health check results (using simple variables instead of associative arrays for macOS compatibility)
HEALTH_STATUS_PROCESSES=""
HEALTH_STATUS_PORTS=""
HEALTH_STATUS_METRICS=""
HEALTH_STATUS_CONNECTIVITY=""
HEALTH_STATUS_LOGS=""

HEALTH_MESSAGE_PROCESSES=""
HEALTH_MESSAGE_PORTS=""
HEALTH_MESSAGE_METRICS=""
HEALTH_MESSAGE_CONNECTIVITY=""
HEALTH_MESSAGE_LOGS=""

log_pass() {
    echo -e "${GREEN}✓${NC} $1"
}

log_fail() {
    echo -e "${RED}✗${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}⚠${NC} $1"
}

log_info() {
    echo -e "${BLUE}ℹ${NC} $1"
}

# Check if a port is listening
check_port() {
    local port=$1
    nc -z localhost "$port" > /dev/null 2>&1
}

# Check cluster processes
check_processes() {
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "1. Process Health Check"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

    local running_nodes=$(pgrep -f "emqx-go" | wc -l | tr -d ' ')

    if [ "$running_nodes" -eq $EXPECTED_NODES ]; then
        log_pass "All $EXPECTED_NODES nodes are running"
        HEALTH_STATUS[processes]="OK"
    elif [ "$running_nodes" -gt 0 ]; then
        log_warn "Only $running_nodes out of $EXPECTED_NODES nodes are running"
        HEALTH_STATUS[processes]="DEGRADED"
        HEALTH_MESSAGES[processes]="Cluster is degraded: $running_nodes/$EXPECTED_NODES nodes"
    else
        log_fail "No nodes are running"
        HEALTH_STATUS[processes]="CRITICAL"
        HEALTH_MESSAGES[processes]="Cluster is down: 0/$EXPECTED_NODES nodes"
        return 1
    fi

    # Show running PIDs
    log_info "Running processes:"
    pgrep -f "emqx-go" | while read pid; do
        local cmd=$(ps -p $pid -o command= | cut -c1-80)
        echo "  PID $pid: $cmd"
    done

    echo ""
}

# Check network ports
check_ports() {
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "2. Network Ports Health Check"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

    local all_ports_ok=true

    # Check MQTT ports
    log_info "Checking MQTT ports..."
    for port in "${MQTT_PORTS[@]}"; do
        if check_port "$port"; then
            log_pass "MQTT port $port is listening"
        else
            log_fail "MQTT port $port is NOT listening"
            all_ports_ok=false
        fi
    done

    # Check gRPC ports
    log_info "Checking gRPC ports..."
    for port in "${GRPC_PORTS[@]}"; do
        if check_port "$port"; then
            log_pass "gRPC port $port is listening"
        else
            log_fail "gRPC port $port is NOT listening"
            all_ports_ok=false
        fi
    done

    # Check metrics ports
    log_info "Checking metrics ports..."
    for port in "${METRICS_PORTS[@]}"; do
        if check_port "$port"; then
            log_pass "Metrics port $port is listening"
        else
            log_fail "Metrics port $port is NOT listening"
            all_ports_ok=false
        fi
    done

    # Check dashboard ports
    log_info "Checking dashboard ports..."
    for port in "${DASHBOARD_PORTS[@]}"; do
        if check_port "$port"; then
            log_pass "Dashboard port $port is listening"
        else
            log_warn "Dashboard port $port is NOT listening"
        fi
    done

    if [ "$all_ports_ok" = true ]; then
        HEALTH_STATUS[ports]="OK"
    else
        HEALTH_STATUS[ports]="CRITICAL"
        HEALTH_MESSAGES[ports]="Some critical ports are not listening"
    fi

    echo ""
}

# Check Prometheus metrics
check_metrics() {
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "3. Metrics Health Check"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

    local metrics_ok=true
    local total_connections=0
    local total_sessions=0
    local total_subscriptions=0
    local total_dropped=0

    for port in "${METRICS_PORTS[@]}"; do
        if ! check_port "$port"; then
            log_fail "Cannot reach metrics endpoint on port $port"
            metrics_ok=false
            continue
        fi

        local metrics=$(curl -s "http://localhost:$port/metrics")

        if [ -z "$metrics" ]; then
            log_fail "Empty metrics response from port $port"
            metrics_ok=false
            continue
        fi

        # Extract key metrics
        local connections=$(echo "$metrics" | grep "^emqx_connections_count " | awk '{print $2}')
        local sessions=$(echo "$metrics" | grep "^emqx_sessions_count " | awk '{print $2}')
        local subscriptions=$(echo "$metrics" | grep "^emqx_subscriptions_count " | awk '{print $2}')
        local dropped=$(echo "$metrics" | grep "^emqx_messages_dropped_total " | awk '{print $2}')
        local uptime=$(echo "$metrics" | grep "^emqx_uptime_seconds " | awk '{print $2}')

        log_pass "Node on port $port:"
        echo "     Connections: ${connections:-0}"
        echo "     Sessions: ${sessions:-0}"
        echo "     Subscriptions: ${subscriptions:-0}"
        echo "     Dropped Messages: ${dropped:-0}"
        echo "     Uptime: ${uptime:-0}s"

        total_connections=$((total_connections + ${connections:-0}))
        total_sessions=$((total_sessions + ${sessions:-0}))
        total_subscriptions=$((total_subscriptions + ${subscriptions:-0}))
        total_dropped=$((total_dropped + ${dropped:-0}))
    done

    echo ""
    log_info "Cluster Totals:"
    echo "   Total Connections: $total_connections"
    echo "   Total Sessions: $total_sessions"
    echo "   Total Subscriptions: $total_subscriptions"
    echo "   Total Dropped Messages: $total_dropped"

    # Check for issues
    if [ $total_dropped -gt 100 ]; then
        log_warn "High number of dropped messages: $total_dropped"
        HEALTH_STATUS[metrics]="WARNING"
        HEALTH_MESSAGES[metrics]="High dropped message count: $total_dropped"
    elif [ "$metrics_ok" = true ]; then
        HEALTH_STATUS[metrics]="OK"
    else
        HEALTH_STATUS[metrics]="CRITICAL"
        HEALTH_MESSAGES[metrics]="Cannot fetch metrics from all nodes"
    fi

    echo ""
}

# Check cluster connectivity
check_cluster_connectivity() {
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "4. Cluster Connectivity Check"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

    if [ ! -f "./bin/cluster-test" ]; then
        log_warn "cluster-test binary not found, skipping connectivity test"
        HEALTH_STATUS[connectivity]="UNKNOWN"
        echo ""
        return
    fi

    log_info "Running cross-node messaging test..."

    if timeout 10s ./bin/cluster-test > /dev/null 2>&1; then
        log_pass "Cross-node messaging is working"
        HEALTH_STATUS[connectivity]="OK"
    else
        log_fail "Cross-node messaging test failed"
        HEALTH_STATUS[connectivity]="CRITICAL"
        HEALTH_MESSAGES[connectivity]="Cross-node communication is broken"
    fi

    echo ""
}

# Check log files for errors
check_logs() {
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "5. Log Health Check"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

    if [ ! -d "./logs" ]; then
        log_warn "Logs directory not found"
        HEALTH_STATUS[logs]="UNKNOWN"
        echo ""
        return
    fi

    local error_count=0
    local warn_count=0

    for logfile in ./logs/*.log; do
        if [ -f "$logfile" ]; then
            local errors=$(grep -i "fatal\|panic\|error" "$logfile" | grep -v "INFO" | grep -v "already in use" | wc -l | tr -d ' ')
            local warnings=$(grep -i "warn" "$logfile" | grep -v "INFO" | wc -l | tr -d ' ')

            error_count=$((error_count + errors))
            warn_count=$((warn_count + warnings))

            if [ $errors -gt 0 ]; then
                log_fail "$(basename $logfile): $errors errors found"
            elif [ $warnings -gt 0 ]; then
                log_warn "$(basename $logfile): $warnings warnings found"
            else
                log_pass "$(basename $logfile): No critical issues"
            fi
        fi
    done

    if [ $error_count -gt 0 ]; then
        HEALTH_STATUS[logs]="WARNING"
        HEALTH_MESSAGES[logs]="Found $error_count errors in logs"
    else
        HEALTH_STATUS[logs]="OK"
    fi

    echo ""
}

# Generate health report
generate_health_report() {
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "Health Check Summary"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo ""

    local overall_status="OK"

    for check in processes ports metrics connectivity logs; do
        local status="${HEALTH_STATUS[$check]:-UNKNOWN}"
        local message="${HEALTH_MESSAGES[$check]:-}"

        case $status in
            "OK")
                log_pass "$check: $status"
                ;;
            "WARNING"|"DEGRADED")
                log_warn "$check: $status ${message:+- $message}"
                if [ "$overall_status" = "OK" ]; then
                    overall_status="WARNING"
                fi
                ;;
            "CRITICAL")
                log_fail "$check: $status ${message:+- $message}"
                overall_status="CRITICAL"
                ;;
            *)
                log_info "$check: $status"
                ;;
        esac
    done

    echo ""
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

    case $overall_status in
        "OK")
            echo -e "${GREEN}Overall Status: HEALTHY${NC}"
            echo "All systems are operational."
            return 0
            ;;
        "WARNING")
            echo -e "${YELLOW}Overall Status: DEGRADED${NC}"
            echo "Cluster is functional but some issues detected."
            return 1
            ;;
        "CRITICAL")
            echo -e "${RED}Overall Status: UNHEALTHY${NC}"
            echo "Critical issues detected. Immediate action required."
            return 2
            ;;
    esac
}

# Main execution
main() {
    echo ""
    echo "╔═══════════════════════════════════════════════════════════════╗"
    echo "║          EMQX-Go Cluster Health Check                         ║"
    echo "╚═══════════════════════════════════════════════════════════════╝"
    echo ""
    echo "Timestamp: $(date)"
    echo ""

    check_processes || true
    check_ports || true
    check_metrics || true
    check_cluster_connectivity || true
    check_logs || true

    generate_health_report
    local exit_code=$?

    echo ""
    echo "Health check completed at $(date)"
    echo ""

    exit $exit_code
}

# Run main function
main "$@"
