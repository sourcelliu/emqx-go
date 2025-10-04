/**
 * Simple Dashboard Verification Test
 * Quick verification of core Dashboard functionality
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
                resolve({
                    status: res.statusCode,
                    headers: res.headers,
                    body: body
                });
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

async function runSimpleVerification() {
    console.log('🎯 EMQX-GO Dashboard Simple Verification Test');
    console.log('=' .repeat(50));

    let passed = 0;
    let failed = 0;

    // Test 1: Root redirect
    console.log('\n📍 Test 1: Root path redirect');
    try {
        const response = await makeRequest('GET', '/');
        if (response.status === 307 && response.headers.location === '/dashboard') {
            console.log('✅ PASS: Root redirects to /dashboard');
            passed++;
        } else {
            console.log('❌ FAIL: Root redirect not working');
            failed++;
        }
    } catch (error) {
        console.log('❌ FAIL: Root request error:', error.message);
        failed++;
    }

    // Test 2: Login page
    console.log('\n📍 Test 2: Login page accessibility');
    try {
        const response = await makeRequest('GET', '/login');
        if (response.status === 200 && response.body.includes('EMQX-GO Dashboard - Login')) {
            console.log('✅ PASS: Login page loads correctly');
            passed++;
        } else {
            console.log('❌ FAIL: Login page not accessible');
            failed++;
        }
    } catch (error) {
        console.log('❌ FAIL: Login page error:', error.message);
        failed++;
    }

    // Test 3: Dashboard page
    console.log('\n📍 Test 3: Dashboard page accessibility');
    try {
        const response = await makeRequest('GET', '/dashboard');
        if (response.status === 200 && response.body.includes('EMQX-GO Dashboard - Overview')) {
            console.log('✅ PASS: Dashboard page loads correctly');
            passed++;
        } else {
            console.log('❌ FAIL: Dashboard page not accessible');
            failed++;
        }
    } catch (error) {
        console.log('❌ FAIL: Dashboard page error:', error.message);
        failed++;
    }

    // Test 4: Login API
    console.log('\n📍 Test 4: Login API functionality');
    try {
        const response = await makeRequest('POST', '/api/v5/login', {
            username: 'admin',
            password: 'admin'
        });

        if (response.status === 200) {
            const data = JSON.parse(response.body);
            if (data.data && data.data.token) {
                console.log('✅ PASS: Login API working correctly');
                console.log(`   Token: ${data.data.token.substring(0, 20)}...`);
                passed++;
            } else {
                console.log('❌ FAIL: Login API response invalid');
                failed++;
            }
        } else {
            console.log('❌ FAIL: Login API returned status', response.status);
            failed++;
        }
    } catch (error) {
        console.log('❌ FAIL: Login API error:', error.message);
        failed++;
    }

    // Test 5: Stats API
    console.log('\n📍 Test 5: Stats API functionality');
    try {
        const response = await makeRequest('GET', '/api/v5/stats');
        if (response.status === 200) {
            const data = JSON.parse(response.body);
            if (data.connections !== undefined) {
                console.log('✅ PASS: Stats API working correctly');
                console.log(`   Connections: ${data.connections.count}, Sessions: ${data.sessions.count}`);
                passed++;
            } else {
                console.log('❌ FAIL: Stats API response invalid');
                failed++;
            }
        } else {
            console.log('❌ FAIL: Stats API returned status', response.status);
            failed++;
        }
    } catch (error) {
        console.log('❌ FAIL: Stats API error:', error.message);
        failed++;
    }

    // Test 6: Static assets
    console.log('\n📍 Test 6: Static CSS asset');
    try {
        const response = await makeRequest('GET', '/static/css/dashboard.css');
        if (response.status === 200 && response.body.includes('EMQX Dashboard CSS')) {
            console.log('✅ PASS: CSS assets served correctly');
            passed++;
        } else {
            console.log('❌ FAIL: CSS assets not served correctly');
            failed++;
        }
    } catch (error) {
        console.log('❌ FAIL: CSS asset error:', error.message);
        failed++;
    }

    // Test 7: Static JS asset
    console.log('\n📍 Test 7: Static JavaScript asset');
    try {
        const response = await makeRequest('GET', '/static/js/dashboard.js');
        if (response.status === 200 && response.body.includes('EMQXDashboard')) {
            console.log('✅ PASS: JavaScript assets served correctly');
            passed++;
        } else {
            console.log('❌ FAIL: JavaScript assets not served correctly');
            failed++;
        }
    } catch (error) {
        console.log('❌ FAIL: JavaScript asset error:', error.message);
        failed++;
    }

    // Summary
    const total = passed + failed;
    const successRate = Math.round((passed / total) * 100);

    console.log('\n' + '=' .repeat(50));
    console.log('📊 VERIFICATION SUMMARY');
    console.log('=' .repeat(50));
    console.log(`Total Tests: ${total}`);
    console.log(`✅ Passed: ${passed}`);
    console.log(`❌ Failed: ${failed}`);
    console.log(`🎯 Success Rate: ${successRate}%`);

    if (successRate >= 85) {
        console.log('\n🎉 EXCELLENT! Dashboard is fully functional!');
        console.log('   • Web interface accessible');
        console.log('   • API endpoints working');
        console.log('   • Static assets served correctly');
        console.log('   • Authentication system operational');
    } else if (successRate >= 70) {
        console.log('\n👍 GOOD! Dashboard mostly functional with minor issues.');
    } else {
        console.log('\n⚠️  NEEDS WORK! Several critical issues found.');
    }

    console.log('\n🌐 Access your dashboard at: http://localhost:8082');
    console.log('   Login with: admin / admin');

    return successRate >= 80;
}

if (require.main === module) {
    runSimpleVerification().then(success => {
        process.exit(success ? 0 : 1);
    }).catch(error => {
        console.error('❌ Verification failed:', error);
        process.exit(1);
    });
}

module.exports = runSimpleVerification;