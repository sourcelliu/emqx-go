#!/bin/bash
#
# EMQX-Go Chaos Engineering Game Day Runner
#
# Automated Game Day scenario execution with:
# - Pre-flight checks
# - Progressive fault injection
# - Real-time monitoring
# - Automated rollback
# - Post-mortem report generation
#

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
PURPLE='\033[0;35m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

# Configuration
GAME_DAY_DATE=$(date +%Y%m%d-%H%M%S)
RESULTS_DIR="$PROJECT_DIR/gameday-${GAME_DAY_DATE}"
DASHBOARD_PORT=8889
DASHBOARD_PID=""

# Game Day Phases
declare -a PHASES=(
    "warmup:Warm-up and Baseline:baseline:60"
    "network:Network Resilience:network-delay,network-loss:90"
    "resource:Resource Pressure:cpu-stress:120"
    "combined:Combined Failures:combined-network:120"
    "extreme:Extreme Conditions:cascade-failure:180"
)

print_header() {
    echo ""
    echo -e "${PURPLE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo -e "${PURPLE}$1${NC}"
    echo -e "${PURPLE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo ""
}

print_phase() {
    echo ""
    echo -e "${CYAN}╔══════════════════════════════════════════════════════════╗${NC}"
    echo -e "${CYAN}║${NC}  $1"
    echo -e "${CYAN}╚══════════════════════════════════════════════════════════╝${NC}"
    echo ""
}

log_info() {
    echo -e "${BLUE}ℹ${NC}  $1"
}

log_success() {
    echo -e "${GREEN}✓${NC}  $1"
}

log_warning() {
    echo -e "${YELLOW}⚠${NC}  $1"
}

log_error() {
    echo -e "${RED}✗${NC}  $1"
}

cleanup() {
    log_info "Cleaning up..."

    # Stop dashboard
    if [ -n "$DASHBOARD_PID" ] && kill -0 "$DASHBOARD_PID" 2>/dev/null; then
        log_info "Stopping dashboard (PID: $DASHBOARD_PID)..."
        kill "$DASHBOARD_PID" 2>/dev/null || true
        wait "$DASHBOARD_PID" 2>/dev/null || true
    fi

    # Stop any cluster processes
    pkill -f "bin/emqx-go" 2>/dev/null || true

    log_success "Cleanup completed"
}

trap cleanup EXIT INT TERM

preflight_checks() {
    print_phase "🔍 Pre-flight Checks"

    local checks_passed=true

    # Check binaries
    log_info "Checking required binaries..."
    if [ ! -f "$PROJECT_DIR/bin/emqx-go" ]; then
        log_error "emqx-go binary not found"
        checks_passed=false
    else
        log_success "emqx-go binary found"
    fi

    if [ ! -f "$PROJECT_DIR/bin/chaos-test-runner" ]; then
        log_error "chaos-test-runner binary not found"
        checks_passed=false
    else
        log_success "chaos-test-runner binary found"
    fi

    if [ ! -f "$PROJECT_DIR/bin/chaos-dashboard" ]; then
        log_warning "chaos-dashboard binary not found (optional)"
    else
        log_success "chaos-dashboard binary found"
    fi

    # Check ports
    log_info "Checking port availability..."
    for port in 8082 8084 8086 1883 1884 1885 $DASHBOARD_PORT; do
        if lsof -Pi :$port -sTCP:LISTEN -t >/dev/null 2>&1; then
            log_error "Port $port is already in use"
            checks_passed=false
        fi
    done
    log_success "All required ports are available"

    # Check Python
    if command -v python3 >/dev/null 2>&1; then
        log_success "Python3 available"
    else
        log_warning "Python3 not found (analysis tools unavailable)"
    fi

    # Check disk space
    local available_space=$(df -h "$PROJECT_DIR" | awk 'NR==2 {print $4}')
    log_info "Available disk space: $available_space"

    if [ "$checks_passed" = false ]; then
        log_error "Pre-flight checks failed. Please fix the issues above."
        exit 1
    fi

    log_success "All pre-flight checks passed ✈️"
}

start_monitoring_dashboard() {
    print_phase "📊 Starting Monitoring Dashboard"

    if [ ! -f "$PROJECT_DIR/bin/chaos-dashboard" ]; then
        log_warning "Dashboard binary not found, skipping..."
        return
    fi

    log_info "Starting dashboard on port $DASHBOARD_PORT..."

    cd "$PROJECT_DIR"
    ./bin/chaos-dashboard -port "$DASHBOARD_PORT" > "$RESULTS_DIR/dashboard.log" 2>&1 &
    DASHBOARD_PID=$!

    sleep 2

    if kill -0 "$DASHBOARD_PID" 2>/dev/null; then
        log_success "Dashboard started (PID: $DASHBOARD_PID)"
        log_info "Access at: ${GREEN}http://localhost:$DASHBOARD_PORT${NC}"
    else
        log_error "Failed to start dashboard"
        DASHBOARD_PID=""
    fi
}

run_phase() {
    local phase_id=$1
    local phase_name=$2
    local scenarios=$3
    local duration=$4

    print_phase "🎯 Phase: $phase_name"

    local phase_dir="$RESULTS_DIR/phase-$phase_id"
    mkdir -p "$phase_dir"

    log_info "Duration per scenario: ${duration}s"
    log_info "Scenarios: $scenarios"

    IFS=',' read -ra SCENARIO_LIST <<< "$scenarios"

    local phase_start=$(date +%s)

    for scenario in "${SCENARIO_LIST[@]}"; do
        log_info "Running scenario: ${YELLOW}$scenario${NC}"

        cd "$PROJECT_DIR"

        # Run chaos test
        if ./bin/chaos-test-runner -scenario "$scenario" -duration "$duration" > "$phase_dir/$scenario.log" 2>&1; then
            log_success "Scenario $scenario completed"

            # Move generated report
            if [ -f "chaos-test-report-${scenario}.md" ]; then
                mv "chaos-test-report-${scenario}.md" "$phase_dir/"
            fi
        else
            log_error "Scenario $scenario failed"
        fi

        # Collect metrics
        log_info "Collecting metrics..."
        for port in 8082 8084 8086; do
            curl -s "http://localhost:$port/metrics" > "$phase_dir/metrics-$port-$scenario.txt" 2>/dev/null || true
        done

        # Brief pause between scenarios
        if [ "$scenario" != "${SCENARIO_LIST[-1]}" ]; then
            log_info "Cooling down for 10 seconds..."
            sleep 10
        fi
    done

    local phase_end=$(date +%s)
    local phase_duration=$((phase_end - phase_start))

    log_success "Phase '$phase_name' completed in ${phase_duration}s"
}

generate_reports() {
    print_phase "📝 Generating Reports"

    # Generate summary
    log_info "Creating summary report..."

    cat > "$RESULTS_DIR/GAME_DAY_SUMMARY.md" << EOF
# Game Day Summary Report

**Date**: $(date)
**Duration**: $(($(date +%s) - GAME_DAY_START))s

## Overview

This Game Day tested the resilience of EMQX-Go under various chaos scenarios.

## Phases Executed

EOF

    for phase_info in "${PHASES[@]}"; do
        IFS=':' read -r phase_id phase_name scenarios duration <<< "$phase_info"

        echo "### Phase: $phase_name" >> "$RESULTS_DIR/GAME_DAY_SUMMARY.md"
        echo "" >> "$RESULTS_DIR/GAME_DAY_SUMMARY.md"
        echo "- **Scenarios**: $scenarios" >> "$RESULTS_DIR/GAME_DAY_SUMMARY.md"
        echo "- **Duration**: ${duration}s per scenario" >> "$RESULTS_DIR/GAME_DAY_SUMMARY.md"
        echo "" >> "$RESULTS_DIR/GAME_DAY_SUMMARY.md"

        # Count passed/failed
        local phase_dir="$RESULTS_DIR/phase-$phase_id"
        if [ -d "$phase_dir" ]; then
            local total=$(find "$phase_dir" -name "*.md" | wc -l)
            local passed=$(grep -l "✅ PASS" "$phase_dir"/*.md 2>/dev/null | wc -l)
            echo "- **Results**: $passed/$total scenarios passed" >> "$RESULTS_DIR/GAME_DAY_SUMMARY.md"
            echo "" >> "$RESULTS_DIR/GAME_DAY_SUMMARY.md"
        fi
    done

    log_success "Summary report created"

    # Generate HTML report if Python available
    if command -v python3 >/dev/null 2>&1 && [ -f "$SCRIPT_DIR/generate-html-report.py" ]; then
        log_info "Generating HTML report..."

        # Collect all markdown reports
        find "$RESULTS_DIR" -name "*.md" -not -name "GAME_DAY_SUMMARY.md" -exec cp {} "$RESULTS_DIR/" \;

        if python3 "$SCRIPT_DIR/generate-html-report.py" "$RESULTS_DIR" > /dev/null 2>&1; then
            log_success "HTML report generated"
        else
            log_warning "HTML report generation failed"
        fi
    fi

    # Generate analysis
    if command -v python3 >/dev/null 2>&1 && [ -f "$SCRIPT_DIR/analyze-chaos-results.py" ]; then
        log_info "Running results analysis..."
        python3 "$SCRIPT_DIR/analyze-chaos-results.py" "$RESULTS_DIR" > "$RESULTS_DIR/analysis.txt" 2>&1 || true
        log_success "Analysis completed"
    fi
}

print_final_summary() {
    print_header "🎉 Game Day Completed!"

    echo ""
    echo -e "${GREEN}╔══════════════════════════════════════════════════════════╗${NC}"
    echo -e "${GREEN}║${NC}                   GAME DAY RESULTS                       ${GREEN}║${NC}"
    echo -e "${GREEN}╚══════════════════════════════════════════════════════════╝${NC}"
    echo ""

    # Count totals
    local total_tests=$(find "$RESULTS_DIR" -name "chaos-test-report-*.md" | wc -l)
    local passed_tests=$(grep -l "✅ PASS" "$RESULTS_DIR"/phase-*/chaos-test-report-*.md 2>/dev/null | wc -l)

    echo -e "  ${BLUE}Total Tests:${NC}      $total_tests"
    echo -e "  ${GREEN}Passed:${NC}           $passed_tests"
    echo -e "  ${RED}Failed:${NC}           $((total_tests - passed_tests))"
    echo -e "  ${CYAN}Success Rate:${NC}     $(awk "BEGIN {printf \"%.1f\", ($passed_tests/$total_tests)*100}")%"
    echo ""
    echo -e "  ${PURPLE}Results Directory:${NC} $RESULTS_DIR"
    echo ""

    if [ -f "$RESULTS_DIR/chaos-test-report.html" ]; then
        echo -e "  ${CYAN}📊 HTML Report:${NC} ${GREEN}file://$RESULTS_DIR/chaos-test-report.html${NC}"
    fi

    if [ -f "$RESULTS_DIR/GAME_DAY_SUMMARY.md" ]; then
        echo -e "  ${CYAN}📝 Summary:${NC} $RESULTS_DIR/GAME_DAY_SUMMARY.md"
    fi

    echo ""
    echo -e "${GREEN}╚══════════════════════════════════════════════════════════╝${NC}"
    echo ""
}

main() {
    GAME_DAY_START=$(date +%s)

    print_header "🔥 EMQX-Go Chaos Engineering Game Day 🔥"

    log_info "Game Day ID: ${GAME_DAY_DATE}"
    log_info "Results will be saved to: $RESULTS_DIR"

    # Create results directory
    mkdir -p "$RESULTS_DIR"

    # Pre-flight checks
    preflight_checks

    # Start monitoring
    start_monitoring_dashboard

    # Execute phases
    for phase_info in "${PHASES[@]}"; do
        IFS=':' read -r phase_id phase_name scenarios duration <<< "$phase_info"
        run_phase "$phase_id" "$phase_name" "$scenarios" "$duration"
    done

    # Generate reports
    generate_reports

    # Print summary
    print_final_summary

    log_success "Game Day completed successfully! 🎉"
}

# Show usage
if [ "$1" = "--help" ] || [ "$1" = "-h" ]; then
    echo "EMQX-Go Chaos Engineering Game Day Runner"
    echo ""
    echo "Usage: $0"
    echo ""
    echo "This script runs a complete Game Day exercise with multiple phases:"
    echo ""
    echo "Phases:"
    for phase_info in "${PHASES[@]}"; do
        IFS=':' read -r phase_id phase_name scenarios duration <<< "$phase_info"
        echo "  - $phase_name: $scenarios (${duration}s each)"
    done
    echo ""
    echo "Results will be saved to: gameday-<timestamp>/"
    echo ""
    exit 0
fi

main "$@"
