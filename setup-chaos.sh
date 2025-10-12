#!/bin/bash
#
# EMQX-Go Chaos Engineering - Quick Setup Script
#
# This script sets up the complete chaos engineering environment
#

set -e

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
PURPLE='\033[0;35m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$SCRIPT_DIR"

print_banner() {
    echo -e "${PURPLE}"
    echo "┌─────────────────────────────────────────────────────────┐"
    echo "│  🔥 EMQX-Go Chaos Engineering Setup                     │"
    echo "│                                                          │"
    echo "│  Quick setup for chaos testing environment              │"
    echo "└─────────────────────────────────────────────────────────┘"
    echo -e "${NC}"
}

print_step() {
    echo -e "\n${CYAN}▶ $1${NC}"
}

print_success() {
    echo -e "${GREEN}✓ $1${NC}"
}

print_error() {
    echo -e "${RED}✗ $1${NC}"
}

print_info() {
    echo -e "${BLUE}ℹ $1${NC}"
}

check_dependencies() {
    print_step "Checking dependencies..."

    local missing=0

    # Check Go
    if command -v go >/dev/null 2>&1; then
        GO_VERSION=$(go version | awk '{print $3}')
        print_success "Go found: $GO_VERSION"
    else
        print_error "Go not found. Please install Go 1.21 or later"
        missing=1
    fi

    # Check Python3
    if command -v python3 >/dev/null 2>&1; then
        PYTHON_VERSION=$(python3 --version)
        print_success "Python found: $PYTHON_VERSION"
    else
        print_error "Python3 not found"
        missing=1
    fi

    # Check Git
    if command -v git >/dev/null 2>&1; then
        print_success "Git found"
    else
        print_error "Git not found"
        missing=1
    fi

    # Check optional tools
    if command -v docker >/dev/null 2>&1; then
        print_success "Docker found (optional)"
    else
        print_info "Docker not found (optional for K8s deployment)"
    fi

    if [ $missing -eq 1 ]; then
        echo ""
        print_error "Missing required dependencies. Please install them first."
        exit 1
    fi
}

build_binaries() {
    print_step "Building binaries..."

    cd "$PROJECT_DIR"

    # Build main broker
    print_info "Building emqx-go..."
    if go build -o bin/emqx-go ./cmd/emqx-go; then
        print_success "emqx-go built successfully"
    else
        print_error "Failed to build emqx-go"
        exit 1
    fi

    # Build chaos test runner
    print_info "Building chaos-test-runner..."
    if go build -o bin/chaos-test-runner ./tests/chaos-test-runner; then
        print_success "chaos-test-runner built successfully"
    else
        print_error "Failed to build chaos-test-runner"
        exit 1
    fi

    # Build dashboard
    print_info "Building chaos-dashboard..."
    if go build -o bin/chaos-dashboard ./cmd/chaos-dashboard; then
        print_success "chaos-dashboard built successfully"
    else
        print_error "Failed to build chaos-dashboard"
        exit 1
    fi

    # Build chaos-cli
    print_info "Building chaos-cli..."
    if go build -o bin/chaos-cli ./cmd/chaos-cli; then
        print_success "chaos-cli built successfully"
    else
        print_error "Failed to build chaos-cli"
        exit 1
    fi

    # Build cluster test
    print_info "Building cluster-test..."
    if go build -o bin/cluster-test ./tests/cluster; then
        print_success "cluster-test built successfully"
    else
        print_error "Failed to build cluster-test"
        exit 1
    fi
}

setup_scripts() {
    print_step "Setting up scripts..."

    cd "$PROJECT_DIR/scripts"

    # Make scripts executable
    chmod +x *.sh 2>/dev/null || true
    chmod +x *.py 2>/dev/null || true

    print_success "Scripts are now executable"
}

create_directories() {
    print_step "Creating directories..."

    mkdir -p "$PROJECT_DIR/data"
    mkdir -p "$PROJECT_DIR/logs"
    mkdir -p "$PROJECT_DIR/.chaos"

    print_success "Directories created"
}

run_quick_test() {
    print_step "Running quick verification test..."

    cd "$PROJECT_DIR"

    print_info "Running baseline scenario (30 seconds)..."
    if ./bin/chaos-cli test baseline -d 30 >/dev/null 2>&1; then
        print_success "Baseline test passed!"
    else
        print_error "Baseline test failed (this is okay, cluster might not be running)"
    fi
}

print_next_steps() {
    echo ""
    echo -e "${GREEN}════════════════════════════════════════════════════════════${NC}"
    echo -e "${GREEN}✅ Setup Complete!${NC}"
    echo -e "${GREEN}════════════════════════════════════════════════════════════${NC}"
    echo ""
    echo -e "${CYAN}📚 Quick Start Guide:${NC}"
    echo ""
    echo -e "1. Check system status:"
    echo -e "   ${YELLOW}./bin/chaos-cli status${NC}"
    echo ""
    echo -e "2. Run a single test:"
    echo -e "   ${YELLOW}./bin/chaos-cli test baseline -d 30${NC}"
    echo ""
    echo -e "3. Start monitoring dashboard:"
    echo -e "   ${YELLOW}./bin/chaos-cli dashboard${NC}"
    echo -e "   ${BLUE}   Then open: http://localhost:8888${NC}"
    echo ""
    echo -e "4. Run all scenarios:"
    echo -e "   ${YELLOW}./bin/chaos-cli test all -d 30${NC}"
    echo ""
    echo -e "5. Run Game Day exercise:"
    echo -e "   ${YELLOW}./bin/chaos-cli gameday${NC}"
    echo ""
    echo -e "6. Generate HTML report:"
    echo -e "   ${YELLOW}./bin/chaos-cli report chaos-results-*/${NC}"
    echo ""
    echo -e "${CYAN}📖 Documentation:${NC}"
    echo -e "   • README: ./CHAOS_README.md"
    echo -e "   • Guide:  ./CHAOS_TESTING_GUIDE.md"
    echo -e "   • Best Practices: ./CHAOS_BEST_PRACTICES.md"
    echo ""
    echo -e "${CYAN}🔗 Available Commands:${NC}"
    echo -e "   ${YELLOW}./bin/chaos-cli --help${NC}  - Show all commands"
    echo ""
    echo -e "${GREEN}════════════════════════════════════════════════════════════${NC}"
}

main() {
    print_banner

    check_dependencies
    build_binaries
    setup_scripts
    create_directories
    # run_quick_test  # Commented out for now

    print_next_steps
}

main "$@"
