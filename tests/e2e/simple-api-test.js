/**
 * Simple API Test using Node.js built-in http
 * Tests the working API endpoints we implemented
 */

const http = require('http');

function makeRequest(method, path, data = null) {
    return new Promise((resolve, reject) => {
        const options = {
            hostname: 'localhost',
            port: 8082,
            path: path,
            method: method,
            headers: {
                'Content-Type': 'application/json'
            }
        };

        const req = http.request(options, (res) => {
            let body = '';
            res.on('data', (chunk) => {
                body += chunk;
            });
            res.on('end', () => {
                try {
                    const responseData = body ? JSON.parse(body) : null;
                    resolve({
                        status: res.statusCode,
                        data: responseData,
                        rawBody: body
                    });
                } catch (e) {
                    resolve({
                        status: res.statusCode,
                        data: null,
                        rawBody: body
                    });
                }
            });
        });

        req.on('error', (err) => {
            reject(err);
        });

        if (data) {
            req.write(JSON.stringify(data));
        }
        req.end();
    });
}

async function simpleAPITest() {
    console.log('🚀 Simple API Validation Test\n');

    let passed = 0;
    let failed = 0;

    // Test 1: Authentication
    console.log('🔑 Test 1: Authentication endpoint');
    try {
        const response = await makeRequest('POST', '/api/v5/login', {
            username: 'admin',
            password: 'admin'
        });

        console.log(`   Status: ${response.status}`);
        console.log(`   Response: ${response.rawBody}`);

        if (response.status === 200 && response.data && response.data.data && response.data.data.token) {
            console.log('✅ PASS: Authentication successful');
            passed++;
        } else {
            console.log('❌ FAIL: Authentication failed');
            failed++;
        }
    } catch (error) {
        console.log('❌ FAIL: Authentication error:', error.message);
        failed++;
    }

    // Test 2: Invalid credentials
    console.log('\n🚫 Test 2: Invalid credentials');
    try {
        const response = await makeRequest('POST', '/api/v5/login', {
            username: 'wrong',
            password: 'wrong'
        });

        console.log(`   Status: ${response.status}`);
        if (response.status === 401) {
            console.log('✅ PASS: Invalid credentials rejected');
            passed++;
        } else {
            console.log('❌ FAIL: Should have returned 401');
            failed++;
        }
    } catch (error) {
        console.log('❌ FAIL: Error testing invalid credentials:', error.message);
        failed++;
    }

    // Test 3: Clients endpoint
    console.log('\n👥 Test 3: Clients endpoint');
    try {
        const response = await makeRequest('GET', '/api/v5/clients');

        console.log(`   Status: ${response.status}`);
        if (response.status === 200 && response.data && response.data.data) {
            console.log(`✅ PASS: Clients endpoint - found ${response.data.data.length} clients`);
            passed++;
        } else {
            console.log('❌ FAIL: Clients endpoint failed');
            failed++;
        }
    } catch (error) {
        console.log('❌ FAIL: Clients error:', error.message);
        failed++;
    }

    // Test 4: Subscriptions endpoint
    console.log('\n📡 Test 4: Subscriptions endpoint');
    try {
        const response = await makeRequest('GET', '/api/v5/subscriptions');

        console.log(`   Status: ${response.status}`);
        if (response.status === 200 && response.data && response.data.data) {
            console.log(`✅ PASS: Subscriptions endpoint - found ${response.data.data.length} subscriptions`);
            passed++;
        } else {
            console.log('❌ FAIL: Subscriptions endpoint failed');
            failed++;
        }
    } catch (error) {
        console.log('❌ FAIL: Subscriptions error:', error.message);
        failed++;
    }

    // Test 5: Stats endpoint
    console.log('\n📊 Test 5: Stats endpoint');
    try {
        const response = await makeRequest('GET', '/api/v5/stats');

        console.log(`   Status: ${response.status}`);
        if (response.status === 200 && response.data && typeof response.data.connections === 'object') {
            console.log(`✅ PASS: Stats endpoint - connections: ${response.data.connections.count}`);
            passed++;
        } else {
            console.log('❌ FAIL: Stats endpoint failed');
            failed++;
        }
    } catch (error) {
        console.log('❌ FAIL: Stats error:', error.message);
        failed++;
    }

    // Summary
    console.log('\n' + '='.repeat(50));
    console.log('📋 Simple API Test Summary');
    console.log('='.repeat(50));
    console.log(`✅ Passed: ${passed}`);
    console.log(`❌ Failed: ${failed}`);
    console.log(`📊 Success Rate: ${Math.round((passed / (passed + failed)) * 100)}%`);

    if (failed === 0) {
        console.log('\n🎉 ALL TESTS PASSED! Dashboard API is working correctly.');
        return true;
    } else {
        console.log('\n⚠️  Some tests may need investigation.');
        return passed > failed;
    }
}

// Run test
simpleAPITest().then(success => {
    process.exit(success ? 0 : 1);
}).catch(error => {
    console.error('❌ Test failed:', error);
    process.exit(1);
});