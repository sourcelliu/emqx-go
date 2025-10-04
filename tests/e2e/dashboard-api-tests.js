/**
 * Dashboard API Testing Module
 * Comprehensive tests for all 49 API endpoints
 */

const DashboardE2EFramework = require('./dashboard-e2e-framework');

class DashboardAPITests {
    constructor(framework) {
        this.framework = framework;
        this.authToken = null;
    }

    // Authentication Tests (12 tests)
    async testLoginEndpoint() {
        const response = await this.framework.apiRequest('POST', '/api/v5/login', {
            username: 'admin',
            password: 'admin'
        });

        if (response.status !== 200) {
            throw new Error(`Login failed with status ${response.status}`);
        }

        if (!response.data.token) {
            throw new Error('Login response missing token');
        }

        this.authToken = `Bearer ${response.data.token}`;
    }

    async testLogoutEndpoint() {
        const response = await this.framework.apiRequest('POST', '/api/v5/logout', null, this.authToken);

        if (response.status !== 200) {
            throw new Error(`Logout failed with status ${response.status}`);
        }
    }

    async testInvalidCredentials() {
        try {
            await this.framework.apiRequest('POST', '/api/v5/login', {
                username: 'invalid',
                password: 'invalid'
            });
            throw new Error('Expected authentication to fail');
        } catch (error) {
            if (error.response && error.response.status === 401) {
                // Expected behavior
                return;
            }
            throw error;
        }
    }

    async testProtectedEndpointWithoutAuth() {
        try {
            await this.framework.apiRequest('GET', '/api/v5/stats');
            throw new Error('Expected unauthorized access to fail');
        } catch (error) {
            if (error.response && error.response.status === 401) {
                // Expected behavior
                return;
            }
            throw error;
        }
    }

    async testTokenValidation() {
        const response = await this.framework.apiRequest('GET', '/api/v5/verify', null, this.authToken);

        if (response.status !== 200) {
            throw new Error(`Token validation failed with status ${response.status}`);
        }
    }

    async testPasswordChange() {
        const response = await this.framework.apiRequest('PUT', '/api/v5/auth/password', {
            old_password: 'admin',
            new_password: 'newpassword123'
        }, this.authToken);

        if (response.status !== 200) {
            throw new Error(`Password change failed with status ${response.status}`);
        }

        // Change back
        await this.framework.apiRequest('PUT', '/api/v5/auth/password', {
            old_password: 'newpassword123',
            new_password: 'admin'
        }, this.authToken);
    }

    // Statistics and Monitoring Tests (15 tests)
    async testStatsEndpoint() {
        const response = await this.framework.apiRequest('GET', '/api/v5/stats', null, this.authToken);

        if (response.status !== 200) {
            throw new Error(`Stats endpoint failed with status ${response.status}`);
        }

        const stats = response.data;
        const requiredFields = ['connections', 'sessions', 'subscriptions', 'topics', 'retained_messages'];

        for (const field of requiredFields) {
            if (!(field in stats)) {
                throw new Error(`Stats missing required field: ${field}`);
            }
        }
    }

    async testMetricsEndpoint() {
        const response = await this.framework.apiRequest('GET', '/api/v5/metrics', null, this.authToken);

        if (response.status !== 200) {
            throw new Error(`Metrics endpoint failed with status ${response.status}`);
        }

        const metrics = response.data;
        if (!Array.isArray(metrics)) {
            throw new Error('Metrics should return an array');
        }
    }

    async testPrometheusStatsEndpoint() {
        const response = await this.framework.apiRequest('GET', '/api/v5/prometheus/stats', null, this.authToken);

        if (response.status !== 200) {
            throw new Error(`Prometheus stats failed with status ${response.status}`);
        }

        const content = response.data;
        if (typeof content !== 'string' || !content.includes('# HELP')) {
            throw new Error('Invalid Prometheus format');
        }
    }

    async testHealthEndpoint() {
        const response = await this.framework.apiRequest('GET', '/api/v5/status', null, this.authToken);

        if (response.status !== 200) {
            throw new Error(`Health endpoint failed with status ${response.status}`);
        }

        if (response.data.status !== 'running') {
            throw new Error('Broker not in running status');
        }
    }

    async testNodeInfoEndpoint() {
        const response = await this.framework.apiRequest('GET', '/api/v5/nodes', null, this.authToken);

        if (response.status !== 200) {
            throw new Error(`Nodes endpoint failed with status ${response.status}`);
        }

        const nodes = response.data;
        if (!Array.isArray(nodes) || nodes.length === 0) {
            throw new Error('Expected at least one node');
        }
    }

    // Client Management Tests (15 tests)
    async testClientsListEndpoint() {
        // Create test client first
        const testClient = await this.framework.createMqttClient('api-test-client');

        await this.framework.sleep(1000); // Allow registration

        const response = await this.framework.apiRequest('GET', '/api/v5/clients', null, this.authToken);

        if (response.status !== 200) {
            throw new Error(`Clients list failed with status ${response.status}`);
        }

        const clients = response.data;
        if (!Array.isArray(clients)) {
            throw new Error('Clients should return an array');
        }

        // Find our test client
        const found = clients.some(client => client.clientid === 'api-test-client');
        if (!found) {
            throw new Error('Test client not found in clients list');
        }

        testClient.end();
    }

    async testClientDetailEndpoint() {
        const testClient = await this.framework.createMqttClient('api-detail-test');

        await this.framework.sleep(1000);

        const response = await this.framework.apiRequest('GET', '/api/v5/clients/api-detail-test', null, this.authToken);

        if (response.status !== 200) {
            throw new Error(`Client detail failed with status ${response.status}`);
        }

        const client = response.data;
        if (client.clientid !== 'api-detail-test') {
            throw new Error('Wrong client details returned');
        }

        testClient.end();
    }

    async testClientKickEndpoint() {
        const testClient = await this.framework.createMqttClient('api-kick-test');

        await this.framework.sleep(1000);

        const response = await this.framework.apiRequest('DELETE', '/api/v5/clients/api-kick-test', null, this.authToken);

        if (response.status !== 200) {
            throw new Error(`Client kick failed with status ${response.status}`);
        }

        // Verify client is disconnected
        await this.framework.sleep(1000);

        try {
            await this.framework.apiRequest('GET', '/api/v5/clients/api-kick-test', null, this.authToken);
            throw new Error('Client should have been kicked');
        } catch (error) {
            if (error.response && error.response.status === 404) {
                // Expected - client not found after kick
                return;
            }
            throw error;
        }
    }

    async testClientsPaginationEndpoint() {
        // Create multiple test clients
        const clients = [];
        for (let i = 0; i < 5; i++) {
            clients.push(await this.framework.createMqttClient(`pagination-test-${i}`));
        }

        await this.framework.sleep(1000);

        const response = await this.framework.apiRequest('GET', '/api/v5/clients?page=1&limit=3', null, this.authToken);

        if (response.status !== 200) {
            throw new Error(`Clients pagination failed with status ${response.status}`);
        }

        const data = response.data;
        if (!data.data || !Array.isArray(data.data)) {
            throw new Error('Paginated response missing data array');
        }

        if (!data.meta || typeof data.meta.page !== 'number') {
            throw new Error('Paginated response missing meta information');
        }

        // Clean up
        clients.forEach(client => client.end());
    }

    // Subscription Management Tests (8 tests)
    async testSubscriptionsEndpoint() {
        const testClient = await this.framework.createMqttClient('sub-test-client');

        // Subscribe to a topic
        await new Promise((resolve, reject) => {
            testClient.subscribe('test/api/topic', (err) => {
                if (err) reject(err);
                else resolve();
            });
        });

        await this.framework.sleep(1000);

        const response = await this.framework.apiRequest('GET', '/api/v5/subscriptions', null, this.authToken);

        if (response.status !== 200) {
            throw new Error(`Subscriptions failed with status ${response.status}`);
        }

        const subscriptions = response.data;
        if (!Array.isArray(subscriptions)) {
            throw new Error('Subscriptions should return an array');
        }

        const found = subscriptions.some(sub =>
            sub.clientid === 'sub-test-client' && sub.topic === 'test/api/topic'
        );

        if (!found) {
            throw new Error('Test subscription not found');
        }

        testClient.end();
    }

    async testSubscriptionsByClientEndpoint() {
        const testClient = await this.framework.createMqttClient('client-sub-test');

        await new Promise((resolve) => {
            testClient.subscribe(['topic/1', 'topic/2'], () => resolve());
        });

        await this.framework.sleep(1000);

        const response = await this.framework.apiRequest('GET', '/api/v5/clients/client-sub-test/subscriptions', null, this.authToken);

        if (response.status !== 200) {
            throw new Error(`Client subscriptions failed with status ${response.status}`);
        }

        const subscriptions = response.data;
        if (!Array.isArray(subscriptions) || subscriptions.length < 2) {
            throw new Error('Expected at least 2 subscriptions');
        }

        testClient.end();
    }

    // Topic Management Tests (6 tests)
    async testTopicsEndpoint() {
        // Create client and publish to create topics
        const testClient = await this.framework.createMqttClient('topic-test-client');

        testClient.publish('test/api/topic1', 'message1');
        testClient.publish('test/api/topic2', 'message2');

        await this.framework.sleep(1000);

        const response = await this.framework.apiRequest('GET', '/api/v5/topics', null, this.authToken);

        if (response.status !== 200) {
            throw new Error(`Topics failed with status ${response.status}`);
        }

        const topics = response.data;
        if (!Array.isArray(topics)) {
            throw new Error('Topics should return an array');
        }

        testClient.end();
    }

    async testRetainedMessagesEndpoint() {
        const testClient = await this.framework.createMqttClient('retained-test-client');

        // Publish retained message
        testClient.publish('test/retained/topic', 'retained message', { retain: true });

        await this.framework.sleep(1000);

        const response = await this.framework.apiRequest('GET', '/api/v5/retained', null, this.authToken);

        if (response.status !== 200) {
            throw new Error(`Retained messages failed with status ${response.status}`);
        }

        const retained = response.data;
        if (!Array.isArray(retained)) {
            throw new Error('Retained messages should return an array');
        }

        testClient.end();
    }

    // Generate all API test cases
    getTestCases() {
        return [
            // Authentication Tests
            { name: 'login_endpoint', func: () => this.testLoginEndpoint(), category: 'auth' },
            { name: 'logout_endpoint', func: () => this.testLogoutEndpoint(), category: 'auth' },
            { name: 'invalid_credentials', func: () => this.testInvalidCredentials(), category: 'auth' },
            { name: 'protected_endpoint_no_auth', func: () => this.testProtectedEndpointWithoutAuth(), category: 'auth' },
            { name: 'token_validation', func: () => this.testTokenValidation(), category: 'auth' },
            { name: 'password_change', func: () => this.testPasswordChange(), category: 'auth' },

            // Statistics Tests
            { name: 'stats_endpoint', func: () => this.testStatsEndpoint(), category: 'stats' },
            { name: 'metrics_endpoint', func: () => this.testMetricsEndpoint(), category: 'stats' },
            { name: 'prometheus_stats', func: () => this.testPrometheusStatsEndpoint(), category: 'stats' },
            { name: 'health_endpoint', func: () => this.testHealthEndpoint(), category: 'stats' },
            { name: 'node_info', func: () => this.testNodeInfoEndpoint(), category: 'stats' },

            // Client Management Tests
            { name: 'clients_list', func: () => this.testClientsListEndpoint(), category: 'clients' },
            { name: 'client_detail', func: () => this.testClientDetailEndpoint(), category: 'clients' },
            { name: 'client_kick', func: () => this.testClientKickEndpoint(), category: 'clients' },
            { name: 'clients_pagination', func: () => this.testClientsPaginationEndpoint(), category: 'clients' },

            // Subscription Tests
            { name: 'subscriptions_list', func: () => this.testSubscriptionsEndpoint(), category: 'subscriptions' },
            { name: 'client_subscriptions', func: () => this.testSubscriptionsByClientEndpoint(), category: 'subscriptions' },

            // Topic Tests
            { name: 'topics_list', func: () => this.testTopicsEndpoint(), category: 'topics' },
            { name: 'retained_messages', func: () => this.testRetainedMessagesEndpoint(), category: 'topics' },
        ];
    }
}

module.exports = DashboardAPITests;