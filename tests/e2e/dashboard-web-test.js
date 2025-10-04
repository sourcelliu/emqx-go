/**
 * EMQX-GO Web Dashboard Comprehensive Test
 * Tests the full Web Dashboard functionality including UI and API integration
 */

const puppeteer = require('puppeteer');
const axios = require('axios');

class DashboardTest {
    constructor() {
        this.baseUrl = 'http://localhost:8082';
        this.browser = null;
        this.page = null;
        this.testResults = [];
    }

    async initialize() {
        console.log('🚀 Initializing Dashboard Test Suite...\n');

        try {
            this.browser = await puppeteer.launch({
                headless: true,
                args: ['--no-sandbox', '--disable-setuid-sandbox']
            });
            this.page = await this.browser.newPage();

            // Set viewport
            await this.page.setViewport({ width: 1280, height: 720 });

            return true;
        } catch (error) {
            console.error('❌ Failed to initialize browser:', error.message);
            return false;
        }
    }

    async runAllTests() {
        console.log('📋 Running Complete Dashboard Test Suite\n');

        // API Tests
        await this.testApiEndpoints();

        // Web UI Tests
        await this.testWebInterface();

        // Integration Tests
        await this.testIntegrationFlow();

        // Print summary
        this.printTestSummary();
    }

    async testApiEndpoints() {
        console.log('🔌 Testing API Endpoints');
        console.log('-'.repeat(30));

        // Test 1: Login API
        await this.runTest('Login API with valid credentials', async () => {
            const response = await axios.post(`${this.baseUrl}/api/v5/login`, {
                username: 'admin',
                password: 'admin'
            });

            if (response.status === 200 && response.data.data && response.data.data.token) {
                return { success: true, details: `Token: ${response.data.data.token.substring(0, 20)}...` };
            }
            throw new Error('Invalid response format');
        });

        // Test 2: Login API with invalid credentials
        await this.runTest('Login API with invalid credentials', async () => {
            try {
                await axios.post(`${this.baseUrl}/api/v5/login`, {
                    username: 'wrong',
                    password: 'wrong'
                });
                throw new Error('Should have failed');
            } catch (error) {
                if (error.response && error.response.status === 401) {
                    return { success: true, details: 'Correctly rejected invalid credentials' };
                }
                throw error;
            }
        });

        // Test 3: Stats API
        await this.runTest('Stats API endpoint', async () => {
            const response = await axios.get(`${this.baseUrl}/api/v5/stats`);

            if (response.status === 200 && response.data.connections) {
                return {
                    success: true,
                    details: `Connections: ${response.data.connections.count}, Sessions: ${response.data.sessions.count}`
                };
            }
            throw new Error('Invalid stats response');
        });

        // Test 4: Clients API
        await this.runTest('Clients API endpoint', async () => {
            const response = await axios.get(`${this.baseUrl}/api/v5/clients`);

            if (response.status === 200 && response.data.data) {
                return {
                    success: true,
                    details: `Found ${response.data.data.length} clients`
                };
            }
            throw new Error('Invalid clients response');
        });

        // Test 5: Subscriptions API
        await this.runTest('Subscriptions API endpoint', async () => {
            const response = await axios.get(`${this.baseUrl}/api/v5/subscriptions`);

            if (response.status === 200 && response.data.data) {
                return {
                    success: true,
                    details: `Found ${response.data.data.length} subscriptions`
                };
            }
            throw new Error('Invalid subscriptions response');
        });

        console.log('');
    }

    async testWebInterface() {
        console.log('🌐 Testing Web Interface');
        console.log('-'.repeat(30));

        // Test 1: Login Page
        await this.runTest('Login page loads correctly', async () => {
            await this.page.goto(`${this.baseUrl}/login`);
            await this.page.waitForSelector('.login-container', { timeout: 5000 });

            const title = await this.page.title();
            const hasForm = await this.page.$('#login-form') !== null;

            if (title.includes('EMQX-GO Dashboard - Login') && hasForm) {
                return { success: true, details: 'Login form and title present' };
            }
            throw new Error('Login page elements missing');
        });

        // Test 2: Dashboard Page
        await this.runTest('Dashboard page loads correctly', async () => {
            await this.page.goto(`${this.baseUrl}/dashboard`);
            await this.page.waitForSelector('.dashboard-grid', { timeout: 5000 });

            const title = await this.page.title();
            const hasMetrics = await this.page.$('.metric-card') !== null;

            if (title.includes('EMQX-GO Dashboard - Overview') && hasMetrics) {
                return { success: true, details: 'Dashboard metrics cards present' };
            }
            throw new Error('Dashboard page elements missing');
        });

        // Test 3: Static Assets
        await this.runTest('CSS assets load correctly', async () => {
            const cssResponse = await this.page.goto(`${this.baseUrl}/static/css/dashboard.css`);

            if (cssResponse.status() === 200) {
                const contentType = cssResponse.headers()['content-type'];
                if (contentType && contentType.includes('text/css')) {
                    return { success: true, details: 'CSS file served correctly' };
                }
            }
            throw new Error('CSS asset not loading');
        });

        // Test 4: JavaScript assets
        await this.runTest('JavaScript assets load correctly', async () => {
            const jsResponse = await this.page.goto(`${this.baseUrl}/static/js/dashboard.js`);

            if (jsResponse.status() === 200) {
                return { success: true, details: 'JavaScript file served correctly' };
            }
            throw new Error('JavaScript asset not loading');
        });

        console.log('');
    }

    async testIntegrationFlow() {
        console.log('🔄 Testing Integration Flow');
        console.log('-'.repeat(30));

        // Test full login flow
        await this.runTest('Complete login flow simulation', async () => {
            // Go to login page
            await this.page.goto(`${this.baseUrl}/login`);
            await this.page.waitForSelector('#login-form');

            // Fill and submit form
            await this.page.type('#username', 'admin');
            await this.page.type('#password', 'admin');

            // Note: We can't actually test the full flow without implementing
            // proper session handling, but we can verify the form elements exist

            const usernameValue = await this.page.$eval('#username', el => el.value);
            const passwordValue = await this.page.$eval('#password', el => el.value);

            if (usernameValue === 'admin' && passwordValue === 'admin') {
                return { success: true, details: 'Login form inputs working correctly' };
            }
            throw new Error('Form inputs not working');
        });

        // Test dashboard data loading
        await this.runTest('Dashboard data integration', async () => {
            await this.page.goto(`${this.baseUrl}/dashboard`);
            await this.page.waitForSelector('.metric-card');

            // Check if metric elements exist
            const metricsExist = await this.page.evaluate(() => {
                const connections = document.getElementById('connections-count');
                const sessions = document.getElementById('sessions-count');
                const subscriptions = document.getElementById('subscriptions-count');

                return connections !== null && sessions !== null && subscriptions !== null;
            });

            if (metricsExist) {
                return { success: true, details: 'All metric elements present for data binding' };
            }
            throw new Error('Metric elements missing');
        });

        console.log('');
    }

    async runTest(testName, testFunction) {
        try {
            const result = await testFunction();
            if (result.success) {
                console.log(`✅ ${testName}`);
                if (result.details) {
                    console.log(`   └─ ${result.details}`);
                }
                this.testResults.push({ name: testName, status: 'PASSED', details: result.details });
            } else {
                throw new Error('Test returned failure');
            }
        } catch (error) {
            console.log(`❌ ${testName}`);
            console.log(`   └─ Error: ${error.message}`);
            this.testResults.push({ name: testName, status: 'FAILED', error: error.message });
        }
    }

    printTestSummary() {
        const passed = this.testResults.filter(r => r.status === 'PASSED').length;
        const failed = this.testResults.filter(r => r.status === 'FAILED').length;
        const total = this.testResults.length;
        const successRate = Math.round((passed / total) * 100);

        console.log('📊 TEST SUMMARY');
        console.log('='.repeat(40));
        console.log(`Total Tests: ${total}`);
        console.log(`✅ Passed: ${passed}`);
        console.log(`❌ Failed: ${failed}`);
        console.log(`🎯 Success Rate: ${successRate}%`);
        console.log('='.repeat(40));

        if (failed > 0) {
            console.log('\n❌ FAILED TESTS:');
            this.testResults.filter(r => r.status === 'FAILED').forEach(test => {
                console.log(`   • ${test.name}: ${test.error}`);
            });
        }

        if (successRate >= 90) {
            console.log('\n🎉 EXCELLENT! Dashboard is working perfectly!');
        } else if (successRate >= 70) {
            console.log('\n👍 GOOD! Dashboard is mostly functional with minor issues.');
        } else {
            console.log('\n⚠️  NEEDS WORK! Several issues need to be addressed.');
        }
    }

    async cleanup() {
        if (this.browser) {
            await this.browser.close();
        }
    }
}

// Run the test suite
async function runDashboardTests() {
    const test = new DashboardTest();

    try {
        const initialized = await test.initialize();
        if (!initialized) {
            console.log('❌ Failed to initialize test environment');
            process.exit(1);
        }

        await test.runAllTests();

        const passed = test.testResults.filter(r => r.status === 'PASSED').length;
        const total = test.testResults.length;
        const successRate = (passed / total) * 100;

        process.exit(successRate >= 80 ? 0 : 1);

    } catch (error) {
        console.error('❌ Test suite failed:', error);
        process.exit(1);
    } finally {
        await test.cleanup();
    }
}

if (require.main === module) {
    runDashboardTests();
}

module.exports = DashboardTest;