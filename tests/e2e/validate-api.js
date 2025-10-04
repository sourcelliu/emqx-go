/**
 * Simple API Validation Test
 * Tests the working API endpoints we implemented
 */

const axios = require('axios');

const baseUrl = 'http://localhost:8082';

async function validateAPI() {
    console.log('🚀 Starting API Validation Tests...\n');

    let passed = 0;
    let failed = 0;

    // Test 1: Authentication endpoint
    console.log('🔑 Test 1: Authentication endpoint');
    try {
        const loginResponse = await axios.post(`${baseUrl}/api/v5/login`, {
            username: 'admin',
            password: 'admin'
        });

        if (loginResponse.status === 200 && loginResponse.data.token) {
            console.log('✅ PASS: Authentication successful');
            console.log(`   Token: ${loginResponse.data.token.substring(0, 20)}...`);
            passed++;
        } else {
            console.log('❌ FAIL: Authentication response invalid');
            failed++;
        }
    } catch (error) {
        console.log('❌ FAIL: Authentication error:', error.message);
        failed++;
    }

    // Test 2: Invalid credentials
    console.log('\n🚫 Test 2: Invalid credentials test');
    try {
        await axios.post(`${baseUrl}/api/v5/login`, {
            username: 'invalid',
            password: 'invalid'
        });
        console.log('❌ FAIL: Invalid credentials accepted');
        failed++;
    } catch (error) {
        if (error.response && error.response.status === 401) {
            console.log('✅ PASS: Invalid credentials rejected');
            passed++;
        } else {
            console.log('❌ FAIL: Unexpected error for invalid credentials');
            failed++;
        }
    }

    // Test 3: Clients endpoint
    console.log('\n👥 Test 3: Clients endpoint');
    try {
        const clientsResponse = await axios.get(`${baseUrl}/api/v5/clients`);

        if (clientsResponse.status === 200 && clientsResponse.data.data) {
            console.log('✅ PASS: Clients endpoint working');
            console.log(`   Found ${clientsResponse.data.data.length} clients`);
            console.log(`   Sample client: ${JSON.stringify(clientsResponse.data.data[0], null, 2)}`);
            passed++;
        } else {
            console.log('❌ FAIL: Clients response invalid');
            failed++;
        }
    } catch (error) {
        console.log('❌ FAIL: Clients endpoint error:', error.message);
        failed++;
    }

    // Test 4: Subscriptions endpoint
    console.log('\n📡 Test 4: Subscriptions endpoint');
    try {
        const subsResponse = await axios.get(`${baseUrl}/api/v5/subscriptions`);

        if (subsResponse.status === 200 && subsResponse.data.data) {
            console.log('✅ PASS: Subscriptions endpoint working');
            console.log(`   Found ${subsResponse.data.data.length} subscriptions`);
            console.log(`   Sample subscription: ${JSON.stringify(subsResponse.data.data[0], null, 2)}`);
            passed++;
        } else {
            console.log('❌ FAIL: Subscriptions response invalid');
            failed++;
        }
    } catch (error) {
        console.log('❌ FAIL: Subscriptions endpoint error:', error.message);
        failed++;
    }

    // Test 5: Stats endpoint
    console.log('\n📊 Test 5: Stats endpoint');
    try {
        const statsResponse = await axios.get(`${baseUrl}/api/v5/stats`);

        if (statsResponse.status === 200 && statsResponse.data.connections) {
            console.log('✅ PASS: Stats endpoint working');
            console.log(`   Connections: ${statsResponse.data.connections.count}`);
            console.log(`   Sessions: ${statsResponse.data.sessions.count}`);
            passed++;
        } else {
            console.log('❌ FAIL: Stats response invalid');
            failed++;
        }
    } catch (error) {
        console.log('❌ FAIL: Stats endpoint error:', error.message);
        failed++;
    }

    // Test 6: Metrics endpoint
    console.log('\n📈 Test 6: Prometheus metrics endpoint');
    try {
        const metricsResponse = await axios.get(`${baseUrl}/metrics`);

        if (metricsResponse.status === 200 && metricsResponse.data.includes('emqx_')) {
            console.log('✅ PASS: Prometheus metrics working');
            console.log('   Metrics include EMQX-compatible metrics');
            passed++;
        } else {
            console.log('❌ FAIL: Metrics response invalid');
            failed++;
        }
    } catch (error) {
        console.log('❌ FAIL: Metrics endpoint error:', error.message);
        failed++;
    }

    // Summary
    console.log('\n' + '='.repeat(50));
    console.log('📋 API Validation Test Summary');
    console.log('='.repeat(50));
    console.log(`✅ Passed: ${passed}`);
    console.log(`❌ Failed: ${failed}`);
    console.log(`📊 Success Rate: ${Math.round((passed / (passed + failed)) * 100)}%`);

    if (failed === 0) {
        console.log('\n🎉 ALL TESTS PASSED! Dashboard API is working correctly.');
        return true;
    } else {
        console.log('\n❌ Some tests failed. Please check the implementation.');
        return false;
    }
}

// Run validation
if (require.main === module) {
    validateAPI().then(success => {
        process.exit(success ? 0 : 1);
    }).catch(error => {
        console.error('❌ Validation failed with error:', error);
        process.exit(1);
    });
}

module.exports = validateAPI;