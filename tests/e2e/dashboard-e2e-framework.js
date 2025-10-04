/**
 * Comprehensive Dashboard E2E Testing Framework
 * Supports 500+ test cases covering all dashboard functionality
 */

const axios = require('axios');
const mqtt = require('mqtt');
const puppeteer = require('puppeteer');
const WebSocket = require('ws');
const fs = require('fs').promises;
const path = require('path');

class DashboardE2EFramework {
    constructor(config = {}) {
        this.config = {
            brokerUrl: config.brokerUrl || 'mqtt://localhost:1883',
            dashboardUrl: config.dashboardUrl || 'http://localhost:8082',
            wsUrl: config.wsUrl || 'ws://localhost:18083',
            grpcUrl: config.grpcUrl || 'localhost:8081',
            metricsUrl: config.metricsUrl || 'http://localhost:8082',
            testTimeout: config.testTimeout || 30000,
            retryAttempts: config.retryAttempts || 3,
            ...config
        };

        this.testResults = [];
        this.browser = null;
        this.page = null;
        this.mqttClients = new Map();
        this.wsConnections = new Map();
        this.testStats = {
            total: 0,
            passed: 0,
            failed: 0,
            skipped: 0,
            startTime: null,
            endTime: null
        };
    }

    // Test Infrastructure Methods
    async initialize() {
        console.log('🚀 Initializing Dashboard E2E Testing Framework...');

        // Start browser for WebUI testing
        this.browser = await puppeteer.launch({
            headless: true,
            args: ['--no-sandbox', '--disable-setuid-sandbox']
        });
        this.page = await this.browser.newPage();

        // Set viewport and timeouts
        await this.page.setViewport({ width: 1920, height: 1080 });
        await this.page.setDefaultTimeout(this.config.testTimeout);

        // Wait for dashboard to be ready
        await this.waitForDashboard();

        console.log('✅ Framework initialized successfully');
    }

    async cleanup() {
        console.log('🧹 Cleaning up test environment...');

        // Close all MQTT clients
        for (const [clientId, client] of this.mqttClients) {
            try {
                client.end(true);
            } catch (err) {
                console.warn(`Failed to close MQTT client ${clientId}: ${err.message}`);
            }
        }
        this.mqttClients.clear();

        // Close all WebSocket connections
        for (const [connId, ws] of this.wsConnections) {
            try {
                ws.close();
            } catch (err) {
                console.warn(`Failed to close WebSocket ${connId}: ${err.message}`);
            }
        }
        this.wsConnections.clear();

        // Close browser
        if (this.browser) {
            await this.browser.close();
        }

        console.log('✅ Cleanup completed');
    }

    async waitForDashboard() {
        console.log('⏳ Waiting for dashboard to be ready...');
        let attempts = 0;
        const maxAttempts = 30;

        while (attempts < maxAttempts) {
            try {
                const response = await axios.get(`${this.config.dashboardUrl}/api/v5/stats`, {
                    timeout: 5000
                });
                if (response.status === 200) {
                    console.log('✅ Dashboard is ready');
                    return;
                }
            } catch (err) {
                // Dashboard not ready yet
            }

            attempts++;
            await this.sleep(2000);
        }

        throw new Error('Dashboard failed to become ready within timeout');
    }

    // Test Execution Methods
    async runTest(testName, testFunction, category = 'general') {
        const testId = `${category}_${testName}_${Date.now()}`;
        const startTime = Date.now();

        console.log(`\n🧪 Running test: ${testName} (${category})`);

        try {
            this.testStats.total++;
            await testFunction();

            const duration = Date.now() - startTime;
            this.testResults.push({
                id: testId,
                name: testName,
                category,
                status: 'PASSED',
                duration,
                timestamp: new Date().toISOString()
            });

            this.testStats.passed++;
            console.log(`✅ Test passed: ${testName} (${duration}ms)`);

        } catch (error) {
            const duration = Date.now() - startTime;
            this.testResults.push({
                id: testId,
                name: testName,
                category,
                status: 'FAILED',
                error: error.message,
                stack: error.stack,
                duration,
                timestamp: new Date().toISOString()
            });

            this.testStats.failed++;
            console.log(`❌ Test failed: ${testName} (${duration}ms)`);
            console.log(`   Error: ${error.message}`);
        }
    }

    async runTestSuite(suiteName, tests) {
        console.log(`\n📋 Running test suite: ${suiteName}`);
        console.log(`   Tests count: ${tests.length}`);

        for (const test of tests) {
            await this.runTest(test.name, test.func, test.category || suiteName);
        }
    }

    // Helper Methods
    async createMqttClient(clientId, options = {}) {
        const client = mqtt.connect(this.config.brokerUrl, {
            clientId,
            username: 'test',
            password: 'test',
            clean: true,
            keepalive: 60,
            ...options
        });

        return new Promise((resolve, reject) => {
            const timeout = setTimeout(() => {
                reject(new Error(`MQTT client ${clientId} connection timeout`));
            }, 10000);

            client.on('connect', () => {
                clearTimeout(timeout);
                this.mqttClients.set(clientId, client);
                resolve(client);
            });

            client.on('error', (err) => {
                clearTimeout(timeout);
                reject(err);
            });
        });
    }

    async createWebSocketConnection(connId, path = '/ws') {
        const ws = new WebSocket(`${this.config.wsUrl}${path}`);

        return new Promise((resolve, reject) => {
            const timeout = setTimeout(() => {
                reject(new Error(`WebSocket ${connId} connection timeout`));
            }, 10000);

            ws.on('open', () => {
                clearTimeout(timeout);
                this.wsConnections.set(connId, ws);
                resolve(ws);
            });

            ws.on('error', (err) => {
                clearTimeout(timeout);
                reject(err);
            });
        });
    }

    async apiRequest(method, endpoint, data = null, auth = null) {
        const config = {
            method,
            url: `${this.config.dashboardUrl}${endpoint}`,
            timeout: 10000,
            headers: {
                'Content-Type': 'application/json'
            }
        };

        if (auth) {
            config.headers.Authorization = auth;
        }

        if (data) {
            config.data = data;
        }

        return axios(config);
    }

    async navigateToPage(path) {
        const url = `${this.config.dashboardUrl}${path}`;
        await this.page.goto(url, { waitUntil: 'networkidle0' });
    }

    async login(username = 'admin', password = 'admin') {
        await this.navigateToPage('/login');

        await this.page.waitForSelector('#username');
        await this.page.type('#username', username);
        await this.page.type('#password', password);
        await this.page.click('#login-btn');

        await this.page.waitForNavigation({ waitUntil: 'networkidle0' });
    }

    async waitForElement(selector, timeout = 5000) {
        return this.page.waitForSelector(selector, { timeout });
    }

    async clickElement(selector) {
        await this.page.click(selector);
    }

    async typeText(selector, text) {
        await this.page.type(selector, text);
    }

    async getElementText(selector) {
        return this.page.$eval(selector, el => el.textContent);
    }

    async sleep(ms) {
        return new Promise(resolve => setTimeout(resolve, ms));
    }

    // Result Methods
    generateReport() {
        this.testStats.endTime = Date.now();
        const duration = this.testStats.endTime - this.testStats.startTime;

        const report = {
            summary: {
                ...this.testStats,
                duration,
                successRate: ((this.testStats.passed / this.testStats.total) * 100).toFixed(2)
            },
            results: this.testResults,
            timestamp: new Date().toISOString()
        };

        return report;
    }

    async saveReport(filename = null) {
        if (!filename) {
            filename = `dashboard-e2e-report-${Date.now()}.json`;
        }

        const report = this.generateReport();
        const reportPath = path.join(__dirname, 'reports', filename);

        // Ensure reports directory exists
        await fs.mkdir(path.dirname(reportPath), { recursive: true });

        await fs.writeFile(reportPath, JSON.stringify(report, null, 2));
        console.log(`📊 Test report saved: ${reportPath}`);

        return reportPath;
    }

    printSummary() {
        const report = this.generateReport();
        const summary = report.summary;

        console.log('\n' + '='.repeat(60));
        console.log('📊 DASHBOARD E2E TEST SUMMARY');
        console.log('='.repeat(60));
        console.log(`Total Tests:    ${summary.total}`);
        console.log(`Passed:         ${summary.passed} ✅`);
        console.log(`Failed:         ${summary.failed} ❌`);
        console.log(`Success Rate:   ${summary.successRate}%`);
        console.log(`Duration:       ${(summary.duration / 1000).toFixed(2)}s`);
        console.log('='.repeat(60));

        if (summary.failed > 0) {
            console.log('\n❌ FAILED TESTS:');
            this.testResults
                .filter(t => t.status === 'FAILED')
                .forEach(test => {
                    console.log(`   - ${test.name} (${test.category}): ${test.error}`);
                });
        }
    }
}

module.exports = DashboardE2EFramework;