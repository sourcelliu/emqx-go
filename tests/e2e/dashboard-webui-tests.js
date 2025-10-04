/**
 * Dashboard WebUI Testing Module
 * Comprehensive E2E tests for all frontend functionality
 */

const DashboardE2EFramework = require('./dashboard-e2e-framework');

class DashboardWebUITests {
    constructor(framework) {
        this.framework = framework;
    }

    // Login/Authentication UI Tests (10 tests)
    async testLoginPageLoad() {
        await this.framework.navigateToPage('/login');

        await this.framework.waitForElement('#username');
        await this.framework.waitForElement('#password');
        await this.framework.waitForElement('#login-btn');

        const title = await this.framework.page.title();
        if (!title.includes('EMQX')) {
            throw new Error(`Unexpected page title: ${title}`);
        }
    }

    async testLoginFormValidation() {
        await this.framework.navigateToPage('/login');

        // Try empty form
        await this.framework.clickElement('#login-btn');

        // Check for validation messages
        const usernameError = await this.framework.page.$eval('#username', el => el.validationMessage);
        if (!usernameError) {
            throw new Error('Username validation message not shown');
        }
    }

    async testSuccessfulLogin() {
        await this.framework.navigateToPage('/login');

        await this.framework.typeText('#username', 'admin');
        await this.framework.typeText('#password', 'admin');
        await this.framework.clickElement('#login-btn');

        // Wait for navigation to dashboard
        await this.framework.page.waitForNavigation({ waitUntil: 'networkidle0' });

        const currentUrl = this.framework.page.url();
        if (!currentUrl.includes('/dashboard')) {
            throw new Error('Failed to navigate to dashboard after login');
        }
    }

    async testFailedLogin() {
        await this.framework.navigateToPage('/login');

        await this.framework.typeText('#username', 'invalid');
        await this.framework.typeText('#password', 'invalid');
        await this.framework.clickElement('#login-btn');

        // Wait for error message
        await this.framework.waitForElement('.error-message', 5000);

        const errorText = await this.framework.getElementText('.error-message');
        if (!errorText.toLowerCase().includes('invalid')) {
            throw new Error('Error message not displayed correctly');
        }
    }

    async testLogoutFunctionality() {
        await this.framework.login();

        // Find and click logout button
        await this.framework.waitForElement('#logout-btn');
        await this.framework.clickElement('#logout-btn');

        // Should redirect to login page
        await this.framework.page.waitForNavigation({ waitUntil: 'networkidle0' });

        const currentUrl = this.framework.page.url();
        if (!currentUrl.includes('/login')) {
            throw new Error('Failed to redirect to login after logout');
        }
    }

    // Dashboard Overview UI Tests (15 tests)
    async testDashboardOverviewLoad() {
        await this.framework.login();

        // Wait for overview elements
        await this.framework.waitForElement('.stats-overview');
        await this.framework.waitForElement('.connections-chart');
        await this.framework.waitForElement('.topics-chart');

        // Check stats cards
        const statsCards = await this.framework.page.$$('.stat-card');
        if (statsCards.length < 4) {
            throw new Error('Expected at least 4 statistics cards');
        }
    }

    async testRealTimeStatsUpdate() {
        await this.framework.login();

        // Get initial connection count
        await this.framework.waitForElement('#connections-count');
        const initialCount = await this.framework.getElementText('#connections-count');

        // Create a new MQTT connection
        const testClient = await this.framework.createMqttClient('webui-stats-test');

        // Wait for stats to update
        await this.framework.sleep(3000);

        const updatedCount = await this.framework.getElementText('#connections-count');

        if (parseInt(updatedCount) <= parseInt(initialCount)) {
            throw new Error('Connection count did not update in real-time');
        }

        testClient.end();
    }

    async testChartsRendering() {
        await this.framework.login();

        // Wait for charts to load
        await this.framework.waitForElement('.chart-container canvas');

        // Check if charts are rendered
        const charts = await this.framework.page.$$('.chart-container canvas');
        if (charts.length < 2) {
            throw new Error('Expected at least 2 charts to be rendered');
        }

        // Verify chart data is loaded
        const chartData = await this.framework.page.evaluate(() => {
            return window.chartInstances && Object.keys(window.chartInstances).length > 0;
        });

        if (!chartData) {
            throw new Error('Chart data not properly loaded');
        }
    }

    async testDashboardRefresh() {
        await this.framework.login();

        await this.framework.waitForElement('#refresh-btn');
        await this.framework.clickElement('#refresh-btn');

        // Wait for refresh animation/loading
        await this.framework.waitForElement('.loading-indicator');

        // Wait for loading to complete
        await this.framework.page.waitForSelector('.loading-indicator', { hidden: true });

        // Verify data is refreshed
        const lastUpdate = await this.framework.getElementText('#last-update');
        if (!lastUpdate) {
            throw new Error('Last update timestamp not shown');
        }
    }

    // Client Management UI Tests (20 tests)
    async testClientsPageLoad() {
        await this.framework.login();

        await this.framework.navigateToPage('/clients');

        await this.framework.waitForElement('.clients-table');
        await this.framework.waitForElement('#clients-search');
        await this.framework.waitForElement('.pagination-controls');

        const pageTitle = await this.framework.getElementText('h1');
        if (!pageTitle.toLowerCase().includes('client')) {
            throw new Error('Clients page title not correct');
        }
    }

    async testClientsTableDisplay() {
        await this.framework.login();

        // Create test clients
        const testClients = [];
        for (let i = 0; i < 3; i++) {
            testClients.push(await this.framework.createMqttClient(`webui-client-${i}`));
        }

        await this.framework.sleep(2000);

        await this.framework.navigateToPage('/clients');

        // Wait for table to load
        await this.framework.waitForElement('.clients-table tbody tr');

        const rows = await this.framework.page.$$('.clients-table tbody tr');
        if (rows.length < 3) {
            throw new Error('Not all test clients displayed in table');
        }

        // Check table headers
        const headers = await this.framework.page.$$eval('.clients-table thead th',
            elements => elements.map(el => el.textContent.trim())
        );

        const expectedHeaders = ['Client ID', 'Username', 'IP Address', 'Connected At', 'Actions'];
        for (const header of expectedHeaders) {
            if (!headers.some(h => h.includes(header))) {
                throw new Error(`Missing table header: ${header}`);
            }
        }

        // Clean up
        testClients.forEach(client => client.end());
    }

    async testClientSearch() {
        await this.framework.login();

        const testClient = await this.framework.createMqttClient('searchable-client-test');

        await this.framework.sleep(2000);

        await this.framework.navigateToPage('/clients');

        // Search for specific client
        await this.framework.typeText('#clients-search', 'searchable-client-test');
        await this.framework.clickElement('#search-btn');

        await this.framework.sleep(1000);

        // Verify search results
        const rows = await this.framework.page.$$('.clients-table tbody tr');
        if (rows.length !== 1) {
            throw new Error('Search should return exactly one result');
        }

        const clientId = await this.framework.page.$eval('.clients-table tbody tr td:first-child',
            el => el.textContent.trim()
        );

        if (clientId !== 'searchable-client-test') {
            throw new Error('Search returned wrong client');
        }

        testClient.end();
    }

    async testClientKickAction() {
        await this.framework.login();

        const testClient = await this.framework.createMqttClient('kick-test-client');

        await this.framework.sleep(2000);

        await this.framework.navigateToPage('/clients');

        // Find client row and click kick button
        await this.framework.page.evaluate(() => {
            const rows = Array.from(document.querySelectorAll('.clients-table tbody tr'));
            const targetRow = rows.find(row =>
                row.querySelector('td:first-child').textContent.trim() === 'kick-test-client'
            );
            if (targetRow) {
                targetRow.querySelector('.kick-btn').click();
            }
        });

        // Confirm kick action
        await this.framework.waitForElement('.confirm-dialog');
        await this.framework.clickElement('#confirm-kick');

        await this.framework.sleep(2000);

        // Verify client is removed from table
        const rows = await this.framework.page.$$eval('.clients-table tbody tr',
            rows => rows.map(row => row.querySelector('td:first-child').textContent.trim())
        );

        if (rows.includes('kick-test-client')) {
            throw new Error('Client was not removed after kick');
        }
    }

    async testClientPagination() {
        await this.framework.login();

        await this.framework.navigateToPage('/clients');

        // Check if pagination controls exist
        await this.framework.waitForElement('.pagination-controls');

        // Test page navigation if multiple pages exist
        const nextBtn = await this.framework.page.$('.pagination-next');
        if (nextBtn) {
            await this.framework.clickElement('.pagination-next');
            await this.framework.sleep(1000);

            // Verify page changed
            const pageNumber = await this.framework.getElementText('.current-page');
            if (pageNumber !== '2') {
                throw new Error('Pagination navigation failed');
            }
        }
    }

    // Subscription Management UI Tests (15 tests)
    async testSubscriptionsPageLoad() {
        await this.framework.login();

        await this.framework.navigateToPage('/subscriptions');

        await this.framework.waitForElement('.subscriptions-table');
        await this.framework.waitForElement('#subscriptions-search');

        const pageTitle = await this.framework.getElementText('h1');
        if (!pageTitle.toLowerCase().includes('subscription')) {
            throw new Error('Subscriptions page title not correct');
        }
    }

    async testSubscriptionsDisplay() {
        await this.framework.login();

        // Create client with subscriptions
        const testClient = await this.framework.createMqttClient('sub-display-test');

        await new Promise((resolve) => {
            testClient.subscribe(['test/topic/1', 'test/topic/2'], () => resolve());
        });

        await this.framework.sleep(2000);

        await this.framework.navigateToPage('/subscriptions');

        // Wait for subscriptions to load
        await this.framework.waitForElement('.subscriptions-table tbody tr');

        const rows = await this.framework.page.$$('.subscriptions-table tbody tr');
        if (rows.length < 2) {
            throw new Error('Not all subscriptions displayed');
        }

        testClient.end();
    }

    async testSubscriptionSearch() {
        await this.framework.login();

        const testClient = await this.framework.createMqttClient('sub-search-test');

        await new Promise((resolve) => {
            testClient.subscribe('unique/search/topic', () => resolve());
        });

        await this.framework.sleep(2000);

        await this.framework.navigateToPage('/subscriptions');

        // Search for specific topic
        await this.framework.typeText('#subscriptions-search', 'unique/search/topic');
        await this.framework.clickElement('#search-btn');

        await this.framework.sleep(1000);

        const rows = await this.framework.page.$$('.subscriptions-table tbody tr');
        if (rows.length !== 1) {
            throw new Error('Subscription search failed');
        }

        testClient.end();
    }

    // Topics Management UI Tests (12 tests)
    async testTopicsPageLoad() {
        await this.framework.login();

        await this.framework.navigateToPage('/topics');

        await this.framework.waitForElement('.topics-table');
        await this.framework.waitForElement('#topics-search');

        const pageTitle = await this.framework.getElementText('h1');
        if (!pageTitle.toLowerCase().includes('topic')) {
            throw new Error('Topics page title not correct');
        }
    }

    async testTopicsDisplay() {
        await this.framework.login();

        // Create topics by publishing
        const testClient = await this.framework.createMqttClient('topic-display-test');

        testClient.publish('test/display/topic1', 'message1');
        testClient.publish('test/display/topic2', 'message2');

        await this.framework.sleep(2000);

        await this.framework.navigateToPage('/topics');

        await this.framework.waitForElement('.topics-table tbody tr');

        const rows = await this.framework.page.$$('.topics-table tbody tr');
        if (rows.length === 0) {
            throw new Error('No topics displayed');
        }

        testClient.end();
    }

    // Settings and Configuration UI Tests (10 tests)
    async testSettingsPageLoad() {
        await this.framework.login();

        await this.framework.navigateToPage('/settings');

        await this.framework.waitForElement('.settings-form');
        await this.framework.waitForElement('#save-settings-btn');

        const pageTitle = await this.framework.getElementText('h1');
        if (!pageTitle.toLowerCase().includes('setting')) {
            throw new Error('Settings page title not correct');
        }
    }

    async testSettingsFormValidation() {
        await this.framework.login();

        await this.framework.navigateToPage('/settings');

        // Clear required field and try to save
        await this.framework.page.evaluate(() => {
            document.querySelector('#broker-name').value = '';
        });

        await this.framework.clickElement('#save-settings-btn');

        // Check for validation error
        const validationMessage = await this.framework.page.$eval('#broker-name',
            el => el.validationMessage
        );

        if (!validationMessage) {
            throw new Error('Form validation not working');
        }
    }

    // System Information UI Tests (8 tests)
    async testSystemInfoPageLoad() {
        await this.framework.login();

        await this.framework.navigateToPage('/system');

        await this.framework.waitForElement('.system-info');
        await this.framework.waitForElement('.memory-usage');
        await this.framework.waitForElement('.cpu-usage');

        const pageTitle = await this.framework.getElementText('h1');
        if (!pageTitle.toLowerCase().includes('system')) {
            throw new Error('System info page title not correct');
        }
    }

    async testSystemMetricsDisplay() {
        await this.framework.login();

        await this.framework.navigateToPage('/system');

        // Check system metrics are displayed
        await this.framework.waitForElement('#uptime');
        await this.framework.waitForElement('#memory-total');
        await this.framework.waitForElement('#cpu-cores');

        const uptime = await this.framework.getElementText('#uptime');
        if (!uptime || uptime === '0') {
            throw new Error('System uptime not displayed correctly');
        }
    }

    // Generate all WebUI test cases
    getTestCases() {
        return [
            // Login/Authentication UI Tests
            { name: 'login_page_load', func: () => this.testLoginPageLoad(), category: 'webui_auth' },
            { name: 'login_form_validation', func: () => this.testLoginFormValidation(), category: 'webui_auth' },
            { name: 'successful_login', func: () => this.testSuccessfulLogin(), category: 'webui_auth' },
            { name: 'failed_login', func: () => this.testFailedLogin(), category: 'webui_auth' },
            { name: 'logout_functionality', func: () => this.testLogoutFunctionality(), category: 'webui_auth' },

            // Dashboard Overview UI Tests
            { name: 'dashboard_overview_load', func: () => this.testDashboardOverviewLoad(), category: 'webui_overview' },
            { name: 'realtime_stats_update', func: () => this.testRealTimeStatsUpdate(), category: 'webui_overview' },
            { name: 'charts_rendering', func: () => this.testChartsRendering(), category: 'webui_overview' },
            { name: 'dashboard_refresh', func: () => this.testDashboardRefresh(), category: 'webui_overview' },

            // Client Management UI Tests
            { name: 'clients_page_load', func: () => this.testClientsPageLoad(), category: 'webui_clients' },
            { name: 'clients_table_display', func: () => this.testClientsTableDisplay(), category: 'webui_clients' },
            { name: 'client_search', func: () => this.testClientSearch(), category: 'webui_clients' },
            { name: 'client_kick_action', func: () => this.testClientKickAction(), category: 'webui_clients' },
            { name: 'client_pagination', func: () => this.testClientPagination(), category: 'webui_clients' },

            // Subscription Management UI Tests
            { name: 'subscriptions_page_load', func: () => this.testSubscriptionsPageLoad(), category: 'webui_subscriptions' },
            { name: 'subscriptions_display', func: () => this.testSubscriptionsDisplay(), category: 'webui_subscriptions' },
            { name: 'subscription_search', func: () => this.testSubscriptionSearch(), category: 'webui_subscriptions' },

            // Topics Management UI Tests
            { name: 'topics_page_load', func: () => this.testTopicsPageLoad(), category: 'webui_topics' },
            { name: 'topics_display', func: () => this.testTopicsDisplay(), category: 'webui_topics' },

            // Settings UI Tests
            { name: 'settings_page_load', func: () => this.testSettingsPageLoad(), category: 'webui_settings' },
            { name: 'settings_form_validation', func: () => this.testSettingsFormValidation(), category: 'webui_settings' },

            // System Information UI Tests
            { name: 'system_info_page_load', func: () => this.testSystemInfoPageLoad(), category: 'webui_system' },
            { name: 'system_metrics_display', func: () => this.testSystemMetricsDisplay(), category: 'webui_system' },
        ];
    }
}

module.exports = DashboardWebUITests;