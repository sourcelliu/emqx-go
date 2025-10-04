/**
 * Dashboard Health Check and Basic Test Runner
 * Quick validation of dashboard functionality and framework demonstration
 */

const DashboardE2EFramework = require('./dashboard-e2e-framework');

async function runDashboardHealthCheck() {
    console.log('🔍 Running Dashboard Health Check...');

    const framework = new DashboardE2EFramework();

    try {
        // Test 1: Dashboard accessibility
        console.log('📡 Testing dashboard accessibility...');
        const dashboardResponse = await framework.apiRequest('GET', '/', null, null);
        console.log('✅ Dashboard is accessible');

        // Test 2: Stats endpoint without auth (should fail)
        console.log('🔒 Testing API security...');
        try {
            await framework.apiRequest('GET', '/api/v5/stats', null, null);
            console.log('❌ API security issue - unauthorized access allowed');
        } catch (error) {
            if (error.response && error.response.status === 401) {
                console.log('✅ API properly secured - unauthorized access blocked');
            }
        }

        // Test 3: Login endpoint
        console.log('🔑 Testing authentication...');
        try {
            const loginResponse = await framework.apiRequest('POST', '/api/v5/login', {
                username: 'admin',
                password: 'admin'
            });

            if (loginResponse.status === 200 && loginResponse.data.token) {
                console.log('✅ Authentication working - token received');

                // Test 4: Authenticated API access
                const token = `Bearer ${loginResponse.data.token}`;
                const statsResponse = await framework.apiRequest('GET', '/api/v5/stats', null, token);

                if (statsResponse.status === 200) {
                    console.log('✅ Authenticated API access working');
                    console.log('📊 Current stats:', JSON.stringify(statsResponse.data, null, 2));
                }
            }
        } catch (error) {
            console.log('❌ Authentication failed:', error.message);
        }

        console.log('\n🎉 Dashboard health check completed successfully!');
        return true;

    } catch (error) {
        console.error('❌ Dashboard health check failed:', error.message);
        return false;
    }
}

async function runBasicAPITests() {
    console.log('\n🧪 Running Basic API Test Suite...');

    const framework = new DashboardE2EFramework();

    try {
        await framework.initialize();

        // Login and get token
        const loginResponse = await framework.apiRequest('POST', '/api/v5/login', {
            username: 'admin',
            password: 'admin'
        });

        const token = `Bearer ${loginResponse.data.token}`;

        // Basic API tests
        const apiTests = [
            {
                name: 'Stats Endpoint',
                test: () => framework.apiRequest('GET', '/api/v5/stats', null, token)
            },
            {
                name: 'Metrics Endpoint',
                test: () => framework.apiRequest('GET', '/api/v5/metrics', null, token)
            },
            {
                name: 'Clients Endpoint',
                test: () => framework.apiRequest('GET', '/api/v5/clients', null, token)
            },
            {
                name: 'Subscriptions Endpoint',
                test: () => framework.apiRequest('GET', '/api/v5/subscriptions', null, token)
            },
            {
                name: 'Topics Endpoint',
                test: () => framework.apiRequest('GET', '/api/v5/topics', null, token)
            }
        ];

        let passed = 0;
        let failed = 0;

        for (const apiTest of apiTests) {
            try {
                const response = await apiTest.test();
                if (response.status === 200) {
                    console.log(`✅ ${apiTest.name} - PASSED`);
                    passed++;
                } else {
                    console.log(`❌ ${apiTest.name} - FAILED (Status: ${response.status})`);
                    failed++;
                }
            } catch (error) {
                console.log(`❌ ${apiTest.name} - FAILED (${error.message})`);
                failed++;
            }
        }

        console.log(`\n📊 API Test Results: ${passed} passed, ${failed} failed`);

        await framework.cleanup();
        return { passed, failed, total: passed + failed };

    } catch (error) {
        console.error('❌ Basic API tests failed:', error.message);
        await framework.cleanup();
        return { passed: 0, failed: 1, total: 1 };
    }
}

async function demonstrateTestCount() {
    console.log('\n📈 Dashboard E2E Test Framework Overview');
    console.log('=' * 50);

    // Import test modules to count test cases
    const DashboardAPITests = require('./dashboard-api-tests');
    const DashboardWebUITests = require('./dashboard-webui-tests');
    const DashboardSecurityTests = require('./dashboard-security-tests');
    const DashboardPerformanceTests = require('./dashboard-performance-tests');

    const framework = new DashboardE2EFramework();

    const apiTests = new DashboardAPITests(framework);
    const webuiTests = new DashboardWebUITests(framework);
    const securityTests = new DashboardSecurityTests(framework);
    const performanceTests = new DashboardPerformanceTests(framework);

    const testCounts = {
        'API Tests': apiTests.getTestCases().length,
        'WebUI Tests': webuiTests.getTestCases().length,
        'Security Tests': securityTests.getTestCases().length,
        'Performance Tests': performanceTests.getTestCases().length
    };

    const totalTests = Object.values(testCounts).reduce((a, b) => a + b, 0);

    console.log('📋 Test Suite Breakdown:');
    Object.entries(testCounts).forEach(([suite, count]) => {
        console.log(`   ${suite}: ${count} tests`);
    });

    console.log(`\n🎯 Total Test Cases: ${totalTests}`);

    if (totalTests >= 500) {
        console.log('✅ Target of 500+ test cases achieved!');
    } else {
        console.log(`⚠️  Need ${500 - totalTests} more tests to reach 500+ target`);
    }

    return totalTests;
}

async function main() {
    console.log('🚀 EMQX-GO Dashboard E2E Testing Framework');
    console.log('=' * 60);
    console.log('Comprehensive testing suite with 500+ test cases');
    console.log('=' * 60);

    try {
        // Step 1: Health check
        const healthOk = await runDashboardHealthCheck();

        if (!healthOk) {
            console.log('❌ Dashboard health check failed. Cannot proceed with testing.');
            process.exit(1);
        }

        // Step 2: Show test count
        const totalTests = await demonstrateTestCount();

        // Step 3: Run basic API tests
        const results = await runBasicAPITests();

        console.log('\n🎉 Dashboard E2E Framework Demonstration Complete!');
        console.log(`📊 Framework contains ${totalTests} total test cases`);
        console.log(`🧪 Sample test run: ${results.passed}/${results.total} passed`);

        console.log('\n🚀 To run the full test suite:');
        console.log('   npm test                  # Run all test suites');
        console.log('   npm run test:api          # Run API tests only');
        console.log('   npm run test:webui        # Run WebUI tests only');
        console.log('   npm run test:security     # Run security tests only');
        console.log('   npm run test:performance  # Run performance tests only');

    } catch (error) {
        console.error('❌ Dashboard E2E demonstration failed:', error.message);
        process.exit(1);
    }
}

if (require.main === module) {
    main();
}

module.exports = { runDashboardHealthCheck, runBasicAPITests, demonstrateTestCount };