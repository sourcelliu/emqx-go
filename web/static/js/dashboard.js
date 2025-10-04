// EMQX-GO Dashboard JavaScript
class EMQXDashboard {
    constructor() {
        this.refreshInterval = 5000; // 5 seconds
        this.intervalId = null;
        this.authToken = null;
        this.init();
    }

    init() {
        this.checkAuthentication();
        this.bindEvents();
        if (this.authToken) {
            this.startAutoRefresh();
        }
    }

    checkAuthentication() {
        this.authToken = localStorage.getItem('emqx_token');
        if (!this.authToken && !window.location.pathname.includes('/login')) {
            window.location.href = '/login';
        }
    }

    bindEvents() {
        // Login form
        const loginForm = document.getElementById('login-form');
        if (loginForm) {
            loginForm.addEventListener('submit', (e) => this.handleLogin(e));
        }

        // Logout button
        const logoutBtn = document.getElementById('logout-btn');
        if (logoutBtn) {
            logoutBtn.addEventListener('click', () => this.handleLogout());
        }

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

        // Client action buttons
        document.addEventListener('click', (e) => {
            if (e.target.classList.contains('disconnect-btn')) {
                const clientId = e.target.dataset.clientId;
                this.disconnectClient(clientId);
            }
        });
    }

    async handleLogin(e) {
        e.preventDefault();
        const form = e.target;
        const formData = new FormData(form);
        const username = formData.get('username');
        const password = formData.get('password');

        try {
            const response = await fetch('/api/v5/login', {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                },
                body: JSON.stringify({ username, password }),
            });

            const data = await response.json();

            if (response.ok && data.data && data.data.token) {
                localStorage.setItem('emqx_token', data.data.token);
                localStorage.setItem('emqx_username', data.data.username);
                window.location.href = '/dashboard';
            } else {
                this.showError('Invalid credentials');
            }
        } catch (error) {
            this.showError('Login failed: ' + error.message);
        }
    }

    handleLogout() {
        localStorage.removeItem('emqx_token');
        localStorage.removeItem('emqx_username');
        window.location.href = '/login';
    }

    async apiRequest(url, options = {}) {
        const headers = {
            'Content-Type': 'application/json',
            ...options.headers,
        };

        if (this.authToken) {
            headers['Authorization'] = `Bearer ${this.authToken}`;
        }

        const response = await fetch(url, {
            ...options,
            headers,
        });

        if (response.status === 401) {
            this.handleLogout();
            return null;
        }

        return response;
    }

    async refreshData() {
        try {
            // Update stats
            const statsResponse = await this.apiRequest('/api/v5/stats');
            if (statsResponse && statsResponse.ok) {
                const stats = await statsResponse.json();
                this.updateDashboardMetrics(stats);
            }

            // Update clients
            const clientsResponse = await this.apiRequest('/api/v5/clients');
            if (clientsResponse && clientsResponse.ok) {
                const clients = await clientsResponse.json();
                this.updateClientsTable(clients.data);
            }

            // Update subscriptions
            const subsResponse = await this.apiRequest('/api/v5/subscriptions');
            if (subsResponse && subsResponse.ok) {
                const subscriptions = await subsResponse.json();
                this.updateSubscriptionsTable(subscriptions.data);
            }

            // Update last refresh time
            this.updateLastRefreshTime();

        } catch (error) {
            console.error('Failed to refresh data:', error);
        }
    }

    updateDashboardMetrics(stats) {
        // Update connection metrics
        this.updateElement('connections-count', stats.connections.count);
        this.updateElement('connections-max', stats.connections.max);

        // Update session metrics
        this.updateElement('sessions-count', stats.sessions.count);
        this.updateElement('sessions-persistent', stats.sessions.persistent);
        this.updateElement('sessions-transient', stats.sessions.transient);

        // Update subscription metrics
        this.updateElement('subscriptions-count', stats.subscriptions.count);
        this.updateElement('subscriptions-max', stats.subscriptions.max);

        // Update message metrics
        this.updateElement('messages-received', stats.messages.received);
        this.updateElement('messages-sent', stats.messages.sent);
        this.updateElement('messages-qos0', stats.messages.qos0);
        this.updateElement('messages-qos1', stats.messages.qos1);
        this.updateElement('messages-qos2', stats.messages.qos2);

        // Update uptime
        this.updateElement('node-uptime', this.formatUptime(stats.node.uptime));
    }

    updateClientsTable(clients) {
        const tableBody = document.getElementById('clients-table-body');
        if (!tableBody) return;

        tableBody.innerHTML = clients.map(client => `
            <tr>
                <td>${client.clientid}</td>
                <td>${client.username || 'N/A'}</td>
                <td>
                    <span class="status-dot ${client.connected ? 'online' : 'offline'}"></span>
                    ${client.connected ? 'Connected' : 'Disconnected'}
                </td>
                <td>${client.ip_address}:${client.port}</td>
                <td>${this.formatDateTime(client.connected_at)}</td>
                <td>
                    ${client.connected ?
                        `<button class="btn btn-small btn-danger disconnect-btn" data-client-id="${client.clientid}">Disconnect</button>` :
                        '<span class="text-muted">N/A</span>'
                    }
                </td>
            </tr>
        `).join('');
    }

    updateSubscriptionsTable(subscriptions) {
        const tableBody = document.getElementById('subscriptions-table-body');
        if (!tableBody) return;

        tableBody.innerHTML = subscriptions.map(sub => `
            <tr>
                <td>${sub.clientid}</td>
                <td><code>${sub.topic}</code></td>
                <td><span class="badge badge-qos-${sub.qos}">QoS ${sub.qos}</span></td>
                <td>${sub.nl}</td>
                <td>${sub.rap}</td>
                <td>${sub.rh}</td>
            </tr>
        `).join('');
    }

    updateElement(id, value) {
        const element = document.getElementById(id);
        if (element) {
            element.textContent = value;
        }
    }

    updateLastRefreshTime() {
        const timeElement = document.getElementById('last-refresh-time');
        if (timeElement) {
            timeElement.textContent = new Date().toLocaleTimeString();
        }
    }

    formatUptime(seconds) {
        const hours = Math.floor(seconds / 3600);
        const minutes = Math.floor((seconds % 3600) / 60);
        const secs = seconds % 60;
        return `${hours}h ${minutes}m ${secs}s`;
    }

    formatDateTime(dateString) {
        if (!dateString) return 'N/A';
        return new Date(dateString).toLocaleString();
    }

    showError(message) {
        const errorDiv = document.getElementById('error-message');
        if (errorDiv) {
            errorDiv.textContent = message;
            errorDiv.style.display = 'block';
            setTimeout(() => {
                errorDiv.style.display = 'none';
            }, 5000);
        } else {
            alert(message);
        }
    }

    async disconnectClient(clientId) {
        if (!confirm(`Are you sure you want to disconnect client "${clientId}"?`)) {
            return;
        }

        try {
            const response = await this.apiRequest(`/api/v5/clients/${clientId}/disconnect`, {
                method: 'POST',
            });

            if (response && response.ok) {
                this.showSuccess(`Client "${clientId}" disconnected successfully`);
                this.refreshData();
            } else {
                this.showError('Failed to disconnect client');
            }
        } catch (error) {
            this.showError('Failed to disconnect client: ' + error.message);
        }
    }

    showSuccess(message) {
        // Simple success notification
        const notification = document.createElement('div');
        notification.className = 'notification success';
        notification.textContent = message;
        notification.style.cssText = `
            position: fixed;
            top: 20px;
            right: 20px;
            background: #52c41a;
            color: white;
            padding: 12px 24px;
            border-radius: 6px;
            z-index: 9999;
        `;
        document.body.appendChild(notification);

        setTimeout(() => {
            document.body.removeChild(notification);
        }, 3000);
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
}

// Initialize dashboard when DOM is loaded
document.addEventListener('DOMContentLoaded', () => {
    new EMQXDashboard();
});

// Global error handler
window.addEventListener('error', (e) => {
    console.error('Dashboard error:', e.error);
});

// Handle visibility change to pause/resume auto-refresh
document.addEventListener('visibilitychange', () => {
    const dashboard = window.dashboard;
    if (dashboard) {
        if (document.hidden) {
            dashboard.stopAutoRefresh();
        } else {
            const autoRefresh = document.getElementById('auto-refresh');
            if (autoRefresh && autoRefresh.checked) {
                dashboard.startAutoRefresh();
            }
        }
    }
});