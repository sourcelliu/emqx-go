// EMQX Dashboard JavaScript

class Dashboard {
    constructor() {
        this.refreshInterval = 5000; // 5 seconds
        this.intervalId = null;
        this.init();
    }

    init() {
        this.bindEvents();
        this.startAutoRefresh();
    }

    bindEvents() {
        // Refresh button
        const refreshBtn = document.getElementById('refresh-btn');
        if (refreshBtn) {
            refreshBtn.addEventListener('click', () => this.refreshData());
        }

        // Auto-refresh toggle
        const autoRefreshToggle = document.getElementById('auto-refresh');
        if (autoRefreshToggle) {
            autoRefreshToggle.addEventListener('change', (e) => {
                if (e.target.checked) {
                    this.startAutoRefresh();
                } else {
                    this.stopAutoRefresh();
                }
            });
        }

        // Connection actions
        document.addEventListener('click', (e) => {
            if (e.target.classList.contains('disconnect-btn')) {
                const clientId = e.target.dataset.clientId;
                this.disconnectClient(clientId);
            }
        });
    }

    startAutoRefresh() {
        this.stopAutoRefresh();
        this.intervalId = setInterval(() => this.refreshData(), this.refreshInterval);
    }

    stopAutoRefresh() {
        if (this.intervalId) {
            clearInterval(this.intervalId);
            this.intervalId = null;
        }
    }

    async refreshData() {
        const currentPage = this.getCurrentPage();

        try {
            switch (currentPage) {
                case 'dashboard':
                    await this.refreshDashboardData();
                    break;
                case 'connections':
                    await this.refreshConnectionsData();
                    break;
                case 'sessions':
                    await this.refreshSessionsData();
                    break;
                case 'subscriptions':
                    await this.refreshSubscriptionsData();
                    break;
                case 'monitoring':
                    await this.refreshMonitoringData();
                    break;
            }
        } catch (error) {
            console.error('Failed to refresh data:', error);
            this.showError('Failed to refresh data');
        }
    }

    getCurrentPage() {
        const path = window.location.pathname;
        if (path.includes('connections')) return 'connections';
        if (path.includes('sessions')) return 'sessions';
        if (path.includes('subscriptions')) return 'subscriptions';
        if (path.includes('monitoring')) return 'monitoring';
        return 'dashboard';
    }

    async refreshDashboardData() {
        const response = await this.apiCall('/api/dashboard/stats');
        if (response) {
            this.updateMetricCards(response);
        }
    }

    async refreshConnectionsData() {
        const response = await this.apiCall('/api/dashboard/connections');
        if (response) {
            this.updateConnectionsTable(response.connections || []);
        }
    }

    async refreshSessionsData() {
        const response = await this.apiCall('/api/dashboard/sessions');
        if (response) {
            this.updateSessionsTable(response.sessions || []);
        }
    }

    async refreshSubscriptionsData() {
        const response = await this.apiCall('/api/dashboard/subscriptions');
        if (response) {
            this.updateSubscriptionsTable(response.subscriptions || []);
        }
    }

    async refreshMonitoringData() {
        const [statsResponse, healthResponse] = await Promise.all([
            this.apiCall('/api/dashboard/stats'),
            this.apiCall('/api/dashboard/health')
        ]);

        if (statsResponse) {
            this.updateMetricCards(statsResponse);
        }

        if (healthResponse) {
            this.updateHealthStatus(healthResponse);
        }
    }

    updateMetricCards(stats) {
        // Update connections
        this.updateMetricCard('connections-count', stats.connections?.count || 0);
        this.updateMetricCard('connections-max', stats.connections?.max || 0);

        // Update sessions
        this.updateMetricCard('sessions-count', stats.sessions?.count || 0);
        this.updateMetricCard('sessions-persistent', stats.sessions?.persistent || 0);
        this.updateMetricCard('sessions-transient', stats.sessions?.transient || 0);

        // Update subscriptions
        this.updateMetricCard('subscriptions-count', stats.subscriptions?.count || 0);

        // Update messages
        this.updateMetricCard('messages-received', stats.messages?.received || 0);
        this.updateMetricCard('messages-sent', stats.messages?.sent || 0);
        this.updateMetricCard('messages-dropped', stats.messages?.dropped || 0);

        // Update packets
        this.updateMetricCard('packets-received', stats.packets?.received || 0);
        this.updateMetricCard('packets-sent', stats.packets?.sent || 0);

        // Update uptime
        if (stats.node?.uptime) {
            const uptime = this.formatUptime(stats.node.uptime);
            this.updateMetricCard('node-uptime', uptime);
        }
    }

    updateMetricCard(id, value) {
        const element = document.getElementById(id);
        if (element) {
            if (typeof value === 'number') {
                element.textContent = this.formatNumber(value);
            } else {
                element.textContent = value;
            }

            // Add pulse effect for visual feedback
            element.classList.add('pulse');
            setTimeout(() => element.classList.remove('pulse'), 1000);
        }
    }

    updateConnectionsTable(connections) {
        const tbody = document.querySelector('#connections-table tbody');
        if (!tbody) return;

        tbody.innerHTML = '';

        connections.forEach(conn => {
            const row = this.createConnectionRow(conn);
            tbody.appendChild(row);
        });
    }

    createConnectionRow(connection) {
        const row = document.createElement('tr');
        row.innerHTML = `
            <td>${this.escapeHtml(connection.clientid || '')}</td>
            <td>${this.escapeHtml(connection.username || '')}</td>
            <td>${this.escapeHtml(connection.peerhost || '')}</td>
            <td>${connection.sockport || ''}</td>
            <td><span class="status ${connection.connected ? 'online' : 'offline'}">${connection.connected ? 'Connected' : 'Disconnected'}</span></td>
            <td>${this.formatDate(connection.connected_at)}</td>
            <td>
                <button class="btn btn-danger btn-small disconnect-btn" data-client-id="${this.escapeHtml(connection.clientid || '')}">
                    Disconnect
                </button>
            </td>
        `;
        return row;
    }

    updateSessionsTable(sessions) {
        const tbody = document.querySelector('#sessions-table tbody');
        if (!tbody) return;

        tbody.innerHTML = '';

        sessions.forEach(session => {
            const row = this.createSessionRow(session);
            tbody.appendChild(row);
        });
    }

    createSessionRow(session) {
        const row = document.createElement('tr');
        row.innerHTML = `
            <td>${this.escapeHtml(session.clientid || '')}</td>
            <td>${this.escapeHtml(session.username || '')}</td>
            <td><span class="status ${session.clean_start ? 'offline' : 'online'}">${session.clean_start ? 'Clean' : 'Persistent'}</span></td>
            <td>${this.formatDate(session.created_at)}</td>
            <td>${session.subscriptions_cnt || 0}</td>
            <td>
                <button class="btn btn-danger btn-small kickout-btn" data-client-id="${this.escapeHtml(session.clientid || '')}">
                    Kickout
                </button>
            </td>
        `;
        return row;
    }

    updateSubscriptionsTable(subscriptions) {
        const tbody = document.querySelector('#subscriptions-table tbody');
        if (!tbody) return;

        tbody.innerHTML = '';

        subscriptions.forEach(sub => {
            const row = this.createSubscriptionRow(sub);
            tbody.appendChild(row);
        });
    }

    createSubscriptionRow(subscription) {
        const row = document.createElement('tr');
        row.innerHTML = `
            <td>${this.escapeHtml(subscription.clientid || '')}</td>
            <td>${this.escapeHtml(subscription.topic || '')}</td>
            <td>QoS ${subscription.qos || 0}</td>
            <td>${this.escapeHtml(subscription.node || '')}</td>
            <td>
                <button class="btn btn-danger btn-small unsubscribe-btn" data-client-id="${this.escapeHtml(subscription.clientid || '')}" data-topic="${this.escapeHtml(subscription.topic || '')}">
                    Unsubscribe
                </button>
            </td>
        `;
        return row;
    }

    updateHealthStatus(health) {
        const statusElement = document.getElementById('health-status');
        if (statusElement) {
            const isHealthy = health.status === 'healthy';
            statusElement.className = `status ${isHealthy ? 'online' : 'offline'}`;
            statusElement.textContent = isHealthy ? 'Healthy' : 'Unhealthy';
        }

        // Update system info
        const systemInfo = health.system_info;
        if (systemInfo) {
            this.updateMetricCard('memory-alloc', this.formatBytes(systemInfo.memory?.alloc || 0));
            this.updateMetricCard('memory-sys', this.formatBytes(systemInfo.memory?.sys || 0));
            this.updateMetricCard('goroutines-count', systemInfo.goroutines || 0);
        }
    }

    async disconnectClient(clientId) {
        if (!confirm(`Are you sure you want to disconnect client "${clientId}"?`)) {
            return;
        }

        try {
            const response = await this.apiCall(`/api/v5/connections/${encodeURIComponent(clientId)}`, {
                method: 'DELETE'
            });

            if (response) {
                this.showSuccess(`Client "${clientId}" disconnected successfully`);
                this.refreshConnectionsData();
            }
        } catch (error) {
            this.showError(`Failed to disconnect client: ${error.message}`);
        }
    }

    async apiCall(url, options = {}) {
        const defaultOptions = {
            method: 'GET',
            headers: {
                'Content-Type': 'application/json'
            }
        };

        const config = { ...defaultOptions, ...options };

        const response = await fetch(url, config);

        if (!response.ok) {
            throw new Error(`HTTP ${response.status}: ${response.statusText}`);
        }

        return await response.json();
    }

    formatNumber(num) {
        if (num >= 1000000) {
            return (num / 1000000).toFixed(1) + 'M';
        } else if (num >= 1000) {
            return (num / 1000).toFixed(1) + 'K';
        }
        return num.toString();
    }

    formatBytes(bytes) {
        if (bytes === 0) return '0 B';
        const k = 1024;
        const sizes = ['B', 'KB', 'MB', 'GB'];
        const i = Math.floor(Math.log(bytes) / Math.log(k));
        return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
    }

    formatUptime(seconds) {
        const days = Math.floor(seconds / 86400);
        const hours = Math.floor((seconds % 86400) / 3600);
        const minutes = Math.floor((seconds % 3600) / 60);

        if (days > 0) {
            return `${days}d ${hours}h ${minutes}m`;
        } else if (hours > 0) {
            return `${hours}h ${minutes}m`;
        } else {
            return `${minutes}m`;
        }
    }

    formatDate(dateString) {
        if (!dateString) return '';
        const date = new Date(dateString);
        return date.toLocaleString();
    }

    escapeHtml(text) {
        const div = document.createElement('div');
        div.textContent = text;
        return div.innerHTML;
    }

    showSuccess(message) {
        this.showNotification(message, 'success');
    }

    showError(message) {
        this.showNotification(message, 'error');
    }

    showNotification(message, type) {
        // Create notification element
        const notification = document.createElement('div');
        notification.className = `notification notification-${type}`;
        notification.textContent = message;

        // Style the notification
        Object.assign(notification.style, {
            position: 'fixed',
            top: '20px',
            right: '20px',
            padding: '12px 24px',
            backgroundColor: type === 'success' ? '#52c41a' : '#ff4d4f',
            color: 'white',
            borderRadius: '4px',
            boxShadow: '0 4px 12px rgba(0,0,0,0.15)',
            zIndex: '1000',
            maxWidth: '400px'
        });

        document.body.appendChild(notification);

        // Remove after 3 seconds
        setTimeout(() => {
            notification.remove();
        }, 3000);
    }
}

// Initialize dashboard when DOM is loaded
document.addEventListener('DOMContentLoaded', () => {
    new Dashboard();
});