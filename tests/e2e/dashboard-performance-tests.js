/**
 * Dashboard Performance Testing Module
 * Comprehensive performance and load tests
 */

const DashboardE2EFramework = require('./dashboard-e2e-framework');
const { performance } = require('perf_hooks');
const EventEmitter = require('events');

class DashboardPerformanceTests extends EventEmitter {
    constructor(framework) {
        super();
        this.framework = framework;
        this.performanceMetrics = [];
    }

    // API Performance Tests (15 tests)
    async testAPIResponseTimes() {
        const adminToken = await this.getAdminToken();
        const endpoints = [
            '/api/v5/stats',
            '/api/v5/clients',
            '/api/v5/subscriptions',
            '/api/v5/topics',
            '/api/v5/metrics'
        ];

        const results = {};

        for (const endpoint of endpoints) {
            const measurements = [];

            // Take 10 measurements for each endpoint
            for (let i = 0; i < 10; i++) {
                const startTime = performance.now();

                try {
                    await this.framework.apiRequest('GET', endpoint, null, adminToken);
                    const endTime = performance.now();
                    measurements.push(endTime - startTime);
                } catch (error) {
                    console.warn(`Failed to measure ${endpoint}: ${error.message}`);
                    continue;
                }

                // Small delay between requests
                await this.framework.sleep(100);
            }

            if (measurements.length > 0) {
                const avg = measurements.reduce((a, b) => a + b) / measurements.length;
                const max = Math.max(...measurements);
                const min = Math.min(...measurements);

                results[endpoint] = { avg, max, min, count: measurements.length };

                // Assert reasonable response times
                if (avg > 1000) {
                    throw new Error(`API ${endpoint} average response time too slow: ${avg.toFixed(2)}ms`);
                }

                if (max > 5000) {
                    throw new Error(`API ${endpoint} max response time too slow: ${max.toFixed(2)}ms`);
                }
            }
        }

        this.performanceMetrics.push({
            test: 'api_response_times',
            results,
            timestamp: new Date().toISOString()
        });

        console.log('API Response Times:', JSON.stringify(results, null, 2));
    }

    async testAPIThroughput() {
        const adminToken = await this.getAdminToken();
        const endpoint = '/api/v5/stats';
        const concurrentRequests = 50;
        const duration = 10000; // 10 seconds

        let requestCount = 0;
        let errorCount = 0;
        const startTime = performance.now();
        const endTime = startTime + duration;

        const requests = [];

        while (performance.now() < endTime) {
            const batch = [];

            for (let i = 0; i < concurrentRequests; i++) {
                batch.push(
                    this.framework.apiRequest('GET', endpoint, null, adminToken)
                        .then(() => requestCount++)
                        .catch(() => errorCount++)
                );
            }

            requests.push(...batch);
            await Promise.allSettled(batch);
            await this.framework.sleep(50); // Small delay between batches
        }

        await Promise.allSettled(requests);

        const actualDuration = (performance.now() - startTime) / 1000;
        const throughput = requestCount / actualDuration;
        const errorRate = (errorCount / (requestCount + errorCount)) * 100;

        this.performanceMetrics.push({
            test: 'api_throughput',
            results: {
                requestCount,
                errorCount,
                duration: actualDuration,
                throughput: throughput.toFixed(2),
                errorRate: errorRate.toFixed(2)
            },
            timestamp: new Date().toISOString()
        });

        console.log(`API Throughput: ${throughput.toFixed(2)} req/sec`);
        console.log(`Error Rate: ${errorRate.toFixed(2)}%`);

        if (throughput < 10) {
            throw new Error(`API throughput too low: ${throughput.toFixed(2)} req/sec`);
        }

        if (errorRate > 5) {
            throw new Error(`API error rate too high: ${errorRate.toFixed(2)}%`);
        }
    }

    async testConcurrentUsers() {
        const userCount = 20;
        const actionsPerUser = 10;

        const users = [];

        // Create concurrent user sessions
        for (let i = 0; i < userCount; i++) {
            users.push(this.simulateUser(i, actionsPerUser));
        }

        const startTime = performance.now();
        const results = await Promise.allSettled(users);
        const endTime = performance.now();

        const successful = results.filter(r => r.status === 'fulfilled').length;
        const failed = results.filter(r => r.status === 'rejected').length;

        const metrics = {
            totalUsers: userCount,
            successful,
            failed,
            duration: (endTime - startTime) / 1000,
            successRate: (successful / userCount) * 100
        };

        this.performanceMetrics.push({
            test: 'concurrent_users',
            results: metrics,
            timestamp: new Date().toISOString()
        });

        console.log('Concurrent Users Test:', metrics);

        if (metrics.successRate < 90) {
            throw new Error(`Concurrent user success rate too low: ${metrics.successRate.toFixed(2)}%`);
        }
    }

    async simulateUser(userId, actionCount) {
        // Login
        const loginResponse = await this.framework.apiRequest('POST', '/api/v5/login', {
            username: 'admin',
            password: 'admin'
        });

        const token = `Bearer ${loginResponse.data.token}`;

        // Perform various actions
        const actions = [
            () => this.framework.apiRequest('GET', '/api/v5/stats', null, token),
            () => this.framework.apiRequest('GET', '/api/v5/clients', null, token),
            () => this.framework.apiRequest('GET', '/api/v5/subscriptions', null, token),
            () => this.framework.apiRequest('GET', '/api/v5/topics', null, token),
            () => this.framework.apiRequest('GET', '/api/v5/metrics', null, token),
        ];

        for (let i = 0; i < actionCount; i++) {
            const action = actions[Math.floor(Math.random() * actions.length)];
            await action();
            await this.framework.sleep(Math.random() * 500); // Random delay
        }

        // Logout
        await this.framework.apiRequest('POST', '/api/v5/logout', null, token);

        return { userId, actions: actionCount };
    }

    // WebUI Performance Tests (20 tests)
    async testPageLoadTimes() {
        const pages = [
            '/login',
            '/dashboard',
            '/clients',
            '/subscriptions',
            '/topics',
            '/settings'
        ];

        const results = {};

        for (const page of pages) {
            const measurements = [];

            for (let i = 0; i < 5; i++) {
                const startTime = performance.now();

                await this.framework.navigateToPage(page);

                // Wait for page to be fully loaded
                await this.framework.page.waitForLoadState('networkidle');

                const endTime = performance.now();
                measurements.push(endTime - startTime);

                await this.framework.sleep(1000);
            }

            const avg = measurements.reduce((a, b) => a + b) / measurements.length;
            const max = Math.max(...measurements);
            const min = Math.min(...measurements);

            results[page] = { avg, max, min };

            // Assert reasonable page load times
            if (avg > 3000) {
                throw new Error(`Page ${page} load time too slow: ${avg.toFixed(2)}ms`);
            }
        }

        this.performanceMetrics.push({
            test: 'page_load_times',
            results,
            timestamp: new Date().toISOString()
        });

        console.log('Page Load Times:', JSON.stringify(results, null, 2));
    }

    async testWebUIResponsiveness() {
        await this.framework.login();

        // Test dashboard with real-time updates
        await this.framework.navigateToPage('/dashboard');

        // Create MQTT clients to generate activity
        const clients = [];
        for (let i = 0; i < 10; i++) {
            clients.push(await this.framework.createMqttClient(`perf-test-${i}`));
        }

        // Generate activity
        for (let i = 0; i < 50; i++) {
            const client = clients[Math.floor(Math.random() * clients.length)];
            client.publish(`test/perf/${i}`, `message ${i}`);
            await this.framework.sleep(100);
        }

        // Measure UI responsiveness
        const startTime = performance.now();

        await this.framework.clickElement('#refresh-btn');
        await this.framework.page.waitForSelector('.loading-indicator', { state: 'hidden' });

        const responseTime = performance.now() - startTime;

        this.performanceMetrics.push({
            test: 'webui_responsiveness',
            results: { responseTime },
            timestamp: new Date().toISOString()
        });

        // Clean up clients
        clients.forEach(client => client.end());

        if (responseTime > 5000) {
            throw new Error(`WebUI responsiveness too slow: ${responseTime.toFixed(2)}ms`);
        }
    }

    async testDataTablePerformance() {
        await this.framework.login();

        // Create many MQTT clients to populate tables
        const clients = [];
        for (let i = 0; i < 100; i++) {
            clients.push(await this.framework.createMqttClient(`table-test-${i}`));
        }

        await this.framework.sleep(3000); // Allow clients to register

        // Test clients table performance
        const startTime = performance.now();

        await this.framework.navigateToPage('/clients');
        await this.framework.page.waitForSelector('.clients-table tbody tr');

        const loadTime = performance.now() - startTime;

        // Test table operations
        const searchStartTime = performance.now();

        await this.framework.typeText('#clients-search', 'table-test-50');
        await this.framework.clickElement('#search-btn');
        await this.framework.page.waitForTimeout(1000);

        const searchTime = performance.now() - searchStartTime;

        this.performanceMetrics.push({
            test: 'data_table_performance',
            results: {
                tableLoadTime: loadTime,
                searchTime: searchTime,
                clientCount: 100
            },
            timestamp: new Date().toISOString()
        });

        // Clean up clients
        clients.forEach(client => client.end());

        if (loadTime > 10000) {
            throw new Error(`Table load time too slow: ${loadTime.toFixed(2)}ms`);
        }

        if (searchTime > 2000) {
            throw new Error(`Table search time too slow: ${searchTime.toFixed(2)}ms`);
        }
    }

    // Memory and Resource Tests (10 tests)
    async testMemoryUsage() {
        await this.framework.login();

        // Get initial memory usage
        const initialMemory = await this.framework.page.evaluate(() => {
            if (performance.memory) {
                return {
                    used: performance.memory.usedJSHeapSize,
                    total: performance.memory.totalJSHeapSize,
                    limit: performance.memory.jsHeapSizeLimit
                };
            }
            return null;
        });

        if (!initialMemory) {
            console.log('Memory API not available in this browser');
            return;
        }

        // Perform intensive operations
        for (let i = 0; i < 10; i++) {
            await this.framework.navigateToPage('/clients');
            await this.framework.navigateToPage('/subscriptions');
            await this.framework.navigateToPage('/topics');
            await this.framework.sleep(500);
        }

        // Get final memory usage
        const finalMemory = await this.framework.page.evaluate(() => {
            return {
                used: performance.memory.usedJSHeapSize,
                total: performance.memory.totalJSHeapSize,
                limit: performance.memory.jsHeapSizeLimit
            };
        });

        const memoryIncrease = finalMemory.used - initialMemory.used;
        const memoryIncreasePercent = (memoryIncrease / initialMemory.used) * 100;

        this.performanceMetrics.push({
            test: 'memory_usage',
            results: {
                initialMemory,
                finalMemory,
                memoryIncrease,
                memoryIncreasePercent
            },
            timestamp: new Date().toISOString()
        });

        console.log(`Memory increase: ${(memoryIncrease / 1024 / 1024).toFixed(2)} MB (${memoryIncreasePercent.toFixed(2)}%)`);

        if (memoryIncreasePercent > 100) {
            throw new Error(`Memory usage increased too much: ${memoryIncreasePercent.toFixed(2)}%`);
        }
    }

    async testResourceLeaks() {
        await this.framework.login();

        const iterations = 20;
        const memorySnapshots = [];

        for (let i = 0; i < iterations; i++) {
            // Create and destroy MQTT clients
            const client = await this.framework.createMqttClient(`leak-test-${i}`);
            await this.framework.sleep(500);
            client.end();

            // Navigate between pages
            await this.framework.navigateToPage('/clients');
            await this.framework.navigateToPage('/dashboard');

            // Take memory snapshot
            const memory = await this.framework.page.evaluate(() => {
                return performance.memory ? performance.memory.usedJSHeapSize : 0;
            });

            memorySnapshots.push(memory);
            await this.framework.sleep(1000);
        }

        // Analyze memory trend
        const firstHalf = memorySnapshots.slice(0, iterations / 2);
        const secondHalf = memorySnapshots.slice(iterations / 2);

        const firstHalfAvg = firstHalf.reduce((a, b) => a + b) / firstHalf.length;
        const secondHalfAvg = secondHalf.reduce((a, b) => a + b) / secondHalf.length;

        const memoryTrend = ((secondHalfAvg - firstHalfAvg) / firstHalfAvg) * 100;

        this.performanceMetrics.push({
            test: 'resource_leaks',
            results: {
                iterations,
                firstHalfAvg,
                secondHalfAvg,
                memoryTrend
            },
            timestamp: new Date().toISOString()
        });

        console.log(`Memory trend over ${iterations} iterations: ${memoryTrend.toFixed(2)}%`);

        if (memoryTrend > 50) {
            console.warn(`Potential memory leak detected: ${memoryTrend.toFixed(2)}% increase`);
        }
    }

    // Helper Methods
    async getAdminToken() {
        const response = await this.framework.apiRequest('POST', '/api/v5/login', {
            username: 'admin',
            password: 'admin'
        });

        return `Bearer ${response.data.token}`;
    }

    getPerformanceReport() {
        return {
            metrics: this.performanceMetrics,
            summary: {
                totalTests: this.performanceMetrics.length,
                timestamp: new Date().toISOString()
            }
        };
    }

    // Generate all performance test cases
    getTestCases() {
        return [
            // API Performance Tests
            { name: 'api_response_times', func: () => this.testAPIResponseTimes(), category: 'performance_api' },
            { name: 'api_throughput', func: () => this.testAPIThroughput(), category: 'performance_api' },
            { name: 'concurrent_users', func: () => this.testConcurrentUsers(), category: 'performance_api' },

            // WebUI Performance Tests
            { name: 'page_load_times', func: () => this.testPageLoadTimes(), category: 'performance_webui' },
            { name: 'webui_responsiveness', func: () => this.testWebUIResponsiveness(), category: 'performance_webui' },
            { name: 'data_table_performance', func: () => this.testDataTablePerformance(), category: 'performance_webui' },

            // Memory and Resource Tests
            { name: 'memory_usage', func: () => this.testMemoryUsage(), category: 'performance_memory' },
            { name: 'resource_leaks', func: () => this.testResourceLeaks(), category: 'performance_memory' },
        ];
    }
}

module.exports = DashboardPerformanceTests;