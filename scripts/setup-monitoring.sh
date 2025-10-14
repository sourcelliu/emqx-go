#!/bin/bash
# Setup monitoring stack for EMQX-Go
# This script automatically installs and configures Prometheus, Grafana, and AlertManager

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
PROMETHEUS_VERSION="2.45.0"
GRAFANA_VERSION="10.0.3"
ALERTMANAGER_VERSION="0.26.0"
INSTALL_DIR="/opt/monitoring"
DATA_DIR="/var/lib/monitoring"
CONFIG_DIR="/etc/monitoring"

# Detect OS
detect_os() {
    if [[ "$OSTYPE" == "darwin"* ]]; then
        OS="macos"
    elif [[ -f /etc/os-release ]]; then
        . /etc/os-release
        OS=$ID
    else
        log_error "Unsupported operating system"
        exit 1
    fi
    log_info "Detected OS: $OS"
}

# Install Prometheus
install_prometheus() {
    log_info "Installing Prometheus..."

    if [[ "$OS" == "macos" ]]; then
        if ! command -v brew &> /dev/null; then
            log_error "Homebrew not found. Please install it first."
            exit 1
        fi
        brew install prometheus
        log_info "Prometheus installed via Homebrew"
    else
        # Linux installation
        cd /tmp
        wget "https://github.com/prometheus/prometheus/releases/download/v${PROMETHEUS_VERSION}/prometheus-${PROMETHEUS_VERSION}.linux-amd64.tar.gz"
        tar -xzf "prometheus-${PROMETHEUS_VERSION}.linux-amd64.tar.gz"

        sudo mkdir -p "$INSTALL_DIR/prometheus"
        sudo cp "prometheus-${PROMETHEUS_VERSION}.linux-amd64/prometheus" "$INSTALL_DIR/prometheus/"
        sudo cp "prometheus-${PROMETHEUS_VERSION}.linux-amd64/promtool" "$INSTALL_DIR/prometheus/"
        sudo cp -r "prometheus-${PROMETHEUS_VERSION}.linux-amd64/consoles" "$INSTALL_DIR/prometheus/"
        sudo cp -r "prometheus-${PROMETHEUS_VERSION}.linux-amd64/console_libraries" "$INSTALL_DIR/prometheus/"

        sudo mkdir -p "$DATA_DIR/prometheus"
        sudo mkdir -p "$CONFIG_DIR/prometheus"

        rm -rf "prometheus-${PROMETHEUS_VERSION}.linux-amd64"*

        log_info "Prometheus installed to $INSTALL_DIR/prometheus"
    fi
}

# Configure Prometheus
configure_prometheus() {
    log_info "Configuring Prometheus..."

    local config_file
    if [[ "$OS" == "macos" ]]; then
        config_file="/usr/local/etc/prometheus.yml"
    else
        config_file="$CONFIG_DIR/prometheus/prometheus.yml"
    fi

    sudo tee "$config_file" > /dev/null << 'EOF'
global:
  scrape_interval: 15s
  evaluation_interval: 15s
  external_labels:
    cluster: 'emqx-go'
    environment: 'production'

# Load alert rules
rule_files:
  - "prometheus-alerts.yml"

# Alertmanager configuration
alerting:
  alertmanagers:
    - static_configs:
        - targets:
          - localhost:9093

# Scrape configurations
scrape_configs:
  # EMQX-Go nodes
  - job_name: 'emqx-go'
    static_configs:
      - targets:
          - 'localhost:8082'
          - 'localhost:8084'
          - 'localhost:8086'
        labels:
          service: 'emqx-go'

  # Prometheus itself
  - job_name: 'prometheus'
    static_configs:
      - targets:
          - 'localhost:9090'

  # Node Exporter (if installed)
  - job_name: 'node'
    static_configs:
      - targets:
          - 'localhost:9100'
EOF

    # Copy alert rules
    if [[ "$OS" == "macos" ]]; then
        sudo cp monitoring/prometheus-alerts.yml /usr/local/etc/
    else
        sudo cp monitoring/prometheus-alerts.yml "$CONFIG_DIR/prometheus/"
    fi

    log_info "Prometheus configured at $config_file"
}

# Install Grafana
install_grafana() {
    log_info "Installing Grafana..."

    if [[ "$OS" == "macos" ]]; then
        brew install grafana
        log_info "Grafana installed via Homebrew"
    elif [[ "$OS" == "ubuntu" ]] || [[ "$OS" == "debian" ]]; then
        sudo apt-get install -y apt-transport-https software-properties-common
        wget -q -O - https://packages.grafana.com/gpg.key | sudo apt-key add -
        echo "deb https://packages.grafana.com/oss/deb stable main" | sudo tee /etc/apt/sources.list.d/grafana.list
        sudo apt-get update
        sudo apt-get install -y grafana
        log_info "Grafana installed via APT"
    elif [[ "$OS" == "centos" ]] || [[ "$OS" == "rhel" ]]; then
        sudo tee /etc/yum.repos.d/grafana.repo > /dev/null << 'EOF'
[grafana]
name=grafana
baseurl=https://packages.grafana.com/oss/rpm
repo_gpgcheck=1
enabled=1
gpgcheck=1
gpgkey=https://packages.grafana.com/gpg.key
sslverify=1
sslcacert=/etc/pki/tls/certs/ca-bundle.crt
EOF
        sudo yum install -y grafana
        log_info "Grafana installed via YUM"
    fi
}

# Configure Grafana
configure_grafana() {
    log_info "Configuring Grafana..."

    # Configure Grafana datasource
    local provisioning_dir
    if [[ "$OS" == "macos" ]]; then
        provisioning_dir="/usr/local/etc/grafana/provisioning"
    else
        provisioning_dir="/etc/grafana/provisioning"
    fi

    sudo mkdir -p "$provisioning_dir/datasources"
    sudo mkdir -p "$provisioning_dir/dashboards"

    # Create datasource configuration
    sudo tee "$provisioning_dir/datasources/prometheus.yml" > /dev/null << 'EOF'
apiVersion: 1

datasources:
  - name: Prometheus
    type: prometheus
    access: proxy
    url: http://localhost:9090
    isDefault: true
    editable: true
EOF

    # Create dashboard provisioning
    sudo tee "$provisioning_dir/dashboards/emqx-go.yml" > /dev/null << 'EOF'
apiVersion: 1

providers:
  - name: 'EMQX-Go'
    orgId: 1
    folder: ''
    type: file
    disableDeletion: false
    updateIntervalSeconds: 10
    allowUiUpdates: true
    options:
      path: /var/lib/grafana/dashboards
EOF

    # Copy dashboard
    local dashboard_dir
    if [[ "$OS" == "macos" ]]; then
        dashboard_dir="/usr/local/var/lib/grafana/dashboards"
    else
        dashboard_dir="/var/lib/grafana/dashboards"
    fi

    sudo mkdir -p "$dashboard_dir"
    sudo cp monitoring/grafana-dashboard.json "$dashboard_dir/"

    log_info "Grafana configured at $provisioning_dir"
}

# Install AlertManager
install_alertmanager() {
    log_info "Installing AlertManager..."

    if [[ "$OS" == "macos" ]]; then
        brew install alertmanager
        log_info "AlertManager installed via Homebrew"
    else
        cd /tmp
        wget "https://github.com/prometheus/alertmanager/releases/download/v${ALERTMANAGER_VERSION}/alertmanager-${ALERTMANAGER_VERSION}.linux-amd64.tar.gz"
        tar -xzf "alertmanager-${ALERTMANAGER_VERSION}.linux-amd64.tar.gz"

        sudo mkdir -p "$INSTALL_DIR/alertmanager"
        sudo cp "alertmanager-${ALERTMANAGER_VERSION}.linux-amd64/alertmanager" "$INSTALL_DIR/alertmanager/"
        sudo cp "alertmanager-${ALERTMANAGER_VERSION}.linux-amd64/amtool" "$INSTALL_DIR/alertmanager/"

        sudo mkdir -p "$DATA_DIR/alertmanager"
        sudo mkdir -p "$CONFIG_DIR/alertmanager"

        rm -rf "alertmanager-${ALERTMANAGER_VERSION}.linux-amd64"*

        log_info "AlertManager installed to $INSTALL_DIR/alertmanager"
    fi
}

# Configure AlertManager
configure_alertmanager() {
    log_info "Configuring AlertManager..."

    local config_file
    if [[ "$OS" == "macos" ]]; then
        config_file="/usr/local/etc/alertmanager.yml"
    else
        config_file="$CONFIG_DIR/alertmanager/alertmanager.yml"
    fi

    sudo tee "$config_file" > /dev/null << 'EOF'
global:
  resolve_timeout: 5m

route:
  group_by: ['alertname', 'cluster', 'service']
  group_wait: 10s
  group_interval: 10s
  repeat_interval: 12h
  receiver: 'default'
  routes:
    - match:
        severity: critical
      receiver: 'critical'
      continue: true

receivers:
  - name: 'default'
    webhook_configs:
      - url: 'http://localhost:5001/alerts'
        send_resolved: true

  - name: 'critical'
    webhook_configs:
      - url: 'http://localhost:5001/alerts/critical'
        send_resolved: true

inhibit_rules:
  - source_match:
      severity: 'critical'
    target_match:
      severity: 'warning'
    equal: ['alertname', 'cluster', 'service']
EOF

    log_info "AlertManager configured at $config_file"
}

# Create systemd services (Linux only)
create_systemd_services() {
    if [[ "$OS" == "macos" ]]; then
        log_info "Skipping systemd service creation on macOS (use brew services instead)"
        return
    fi

    log_info "Creating systemd services..."

    # Prometheus service
    sudo tee /etc/systemd/system/prometheus.service > /dev/null << EOF
[Unit]
Description=Prometheus
Wants=network-online.target
After=network-online.target

[Service]
User=prometheus
Group=prometheus
Type=simple
ExecStart=$INSTALL_DIR/prometheus/prometheus \\
  --config.file=$CONFIG_DIR/prometheus/prometheus.yml \\
  --storage.tsdb.path=$DATA_DIR/prometheus \\
  --web.console.templates=$INSTALL_DIR/prometheus/consoles \\
  --web.console.libraries=$INSTALL_DIR/prometheus/console_libraries

[Install]
WantedBy=multi-user.target
EOF

    # Grafana service (already created by package)

    # AlertManager service
    sudo tee /etc/systemd/system/alertmanager.service > /dev/null << EOF
[Unit]
Description=AlertManager
Wants=network-online.target
After=network-online.target

[Service]
User=prometheus
Group=prometheus
Type=simple
ExecStart=$INSTALL_DIR/alertmanager/alertmanager \\
  --config.file=$CONFIG_DIR/alertmanager/alertmanager.yml \\
  --storage.path=$DATA_DIR/alertmanager

[Install]
WantedBy=multi-user.target
EOF

    # Create prometheus user and set permissions
    sudo useradd --no-create-home --shell /bin/false prometheus || true
    sudo chown -R prometheus:prometheus "$INSTALL_DIR/prometheus"
    sudo chown -R prometheus:prometheus "$DATA_DIR/prometheus"
    sudo chown -R prometheus:prometheus "$CONFIG_DIR/prometheus"
    sudo chown -R prometheus:prometheus "$INSTALL_DIR/alertmanager"
    sudo chown -R prometheus:prometheus "$DATA_DIR/alertmanager"
    sudo chown -R prometheus:prometheus "$CONFIG_DIR/alertmanager"

    sudo systemctl daemon-reload

    log_info "Systemd services created"
}

# Start services
start_services() {
    log_info "Starting services..."

    if [[ "$OS" == "macos" ]]; then
        brew services start prometheus
        brew services start grafana
        brew services start alertmanager
    else
        sudo systemctl enable prometheus grafana-server alertmanager
        sudo systemctl start prometheus grafana-server alertmanager
    fi

    log_info "Services started"
}

# Verify installation
verify_installation() {
    log_info "Verifying installation..."

    sleep 5  # Wait for services to start

    local all_ok=true

    # Check Prometheus
    if curl -s http://localhost:9090/-/healthy > /dev/null; then
        log_info "✓ Prometheus is running (http://localhost:9090)"
    else
        log_error "✗ Prometheus is not responding"
        all_ok=false
    fi

    # Check Grafana
    if curl -s http://localhost:3000/api/health > /dev/null; then
        log_info "✓ Grafana is running (http://localhost:3000)"
    else
        log_error "✗ Grafana is not responding"
        all_ok=false
    fi

    # Check AlertManager
    if curl -s http://localhost:9093/-/healthy > /dev/null; then
        log_info "✓ AlertManager is running (http://localhost:9093)"
    else
        log_error "✗ AlertManager is not responding"
        all_ok=false
    fi

    if [ "$all_ok" = true ]; then
        echo ""
        echo "========================================================================"
        echo "Monitoring Stack Installation Complete!"
        echo "========================================================================"
        echo ""
        echo "Access the services:"
        echo "  Prometheus:    http://localhost:9090"
        echo "  Grafana:       http://localhost:3000 (admin/admin)"
        echo "  AlertManager:  http://localhost:9093"
        echo ""
        echo "The EMQX-Go dashboard has been automatically provisioned in Grafana."
        echo ""
        return 0
    else
        log_error "Some services failed to start. Check logs for details."
        return 1
    fi
}

# Print usage information
usage() {
    echo "Usage: $0 [install|uninstall|status]"
    echo ""
    echo "Commands:"
    echo "  install    - Install and configure monitoring stack"
    echo "  uninstall  - Remove monitoring stack"
    echo "  status     - Check status of monitoring services"
    exit 1
}

# Uninstall monitoring stack
uninstall() {
    log_warn "Uninstalling monitoring stack..."

    if [[ "$OS" == "macos" ]]; then
        brew services stop prometheus || true
        brew services stop grafana || true
        brew services stop alertmanager || true
        brew uninstall prometheus grafana alertmanager || true
    else
        sudo systemctl stop prometheus grafana-server alertmanager || true
        sudo systemctl disable prometheus grafana-server alertmanager || true
        sudo apt-get remove -y prometheus grafana alertmanager || true
        sudo yum remove -y prometheus grafana alertmanager || true
        sudo rm -rf "$INSTALL_DIR" "$DATA_DIR" "$CONFIG_DIR"
        sudo rm -f /etc/systemd/system/prometheus.service
        sudo rm -f /etc/systemd/system/alertmanager.service
        sudo systemctl daemon-reload
    fi

    log_info "Monitoring stack uninstalled"
}

# Check status
check_status() {
    echo "Monitoring Stack Status:"
    echo "========================"
    echo ""

    if [[ "$OS" == "macos" ]]; then
        brew services list | grep -E "(prometheus|grafana|alertmanager)"
    else
        systemctl status prometheus --no-pager || true
        echo ""
        systemctl status grafana-server --no-pager || true
        echo ""
        systemctl status alertmanager --no-pager || true
    fi
}

# Main execution
main() {
    case "${1:-install}" in
        install)
            echo "========================================================================"
            echo "EMQX-Go Monitoring Stack Setup"
            echo "========================================================================"
            echo ""

            detect_os
            install_prometheus
            configure_prometheus
            install_grafana
            configure_grafana
            install_alertmanager
            configure_alertmanager
            create_systemd_services
            start_services
            verify_installation
            ;;
        uninstall)
            detect_os
            uninstall
            ;;
        status)
            detect_os
            check_status
            ;;
        *)
            usage
            ;;
    esac
}

main "$@"
