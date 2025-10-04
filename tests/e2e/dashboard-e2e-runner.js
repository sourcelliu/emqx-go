/**
 * Main Dashboard E2E Test Runner
 * Orchestrates all test suites and generates comprehensive reports
 */

const DashboardE2EFramework = require('./dashboard-e2e-framework');
const DashboardAPITests = require('./dashboard-api-tests');
const DashboardWebUITests = require('./dashboard-webui-tests');
const DashboardSecurityTests = require('./dashboard-security-tests');
const DashboardPerformanceTests = require('./dashboard-performance-tests');

class DashboardE2ERunner {
    constructor() {
        this.framework = new DashboardE2EFramework();
        this.apiTests = new DashboardAPITests(this.framework);
        this.webuiTests = new DashboardWebUITests(this.framework);
        this.securityTests = new DashboardSecurityTests(this.framework);
        this.performanceTests = new DashboardPerformanceTests(this.framework);

        this.testSuites = {
            api: this.apiTests,
            webui: this.webuiTests,
            security: this.securityTests,
            performance: this.performanceTests
        };
    }

    async runAllTests() {
        console.log('🚀 Starting Dashboard E2E Test Suite');
        console.log('=' * 60);

        this.framework.testStats.startTime = Date.now();

        try {
            // Initialize framework
            await this.framework.initialize();

            // Run all test suites
            await this.runAPITests();
            await this.runWebUITests();
            await this.runSecurityTests();
            await this.runPerformanceTests();

            // Generate comprehensive report
            await this.generateFinalReport();

        } catch (error) {
            console.error('❌ Test execution failed:', error.message);
            throw error;
        } finally {
            // Always cleanup
            await this.framework.cleanup();
        }
    }

    async runAPITests() {
        console.log('\n📡 Running API Test Suite...');
        const testCases = this.apiTests.getTestCases();

        await this.framework.runTestSuite('API Tests', testCases);

        console.log(`✅ API Tests completed: ${testCases.length} tests`);
    }

    async runWebUITests() {
        console.log('\n🖥️ Running WebUI Test Suite...');
        const testCases = this.webuiTests.getTestCases();

        await this.framework.runTestSuite('WebUI Tests', testCases);

        console.log(`✅ WebUI Tests completed: ${testCases.length} tests`);
    }

    async runSecurityTests() {
        console.log('\n🔒 Running Security Test Suite...');
        const testCases = this.securityTests.getTestCases();

        await this.framework.runTestSuite('Security Tests', testCases);

        console.log(`✅ Security Tests completed: ${testCases.length} tests`);
    }

    async runPerformanceTests() {
        console.log('\n⚡ Running Performance Test Suite...');
        const testCases = this.performanceTests.getTestCases();

        await this.framework.runTestSuite('Performance Tests', testCases);

        console.log(`✅ Performance Tests completed: ${testCases.length} tests`);
    }

    async runSpecificSuite(suiteName) {
        console.log(`🎯 Running specific test suite: ${suiteName}`);

        this.framework.testStats.startTime = Date.now();

        try {
            await this.framework.initialize();

            switch (suiteName.toLowerCase()) {
                case 'api':
                    await this.runAPITests();
                    break;
                case 'webui':
                    await this.runWebUITests();
                    break;
                case 'security':
                    await this.runSecurityTests();
                    break;
                case 'performance':
                    await this.runPerformanceTests();
                    break;
                default:
                    throw new Error(`Unknown test suite: ${suiteName}`);
            }

            await this.generateFinalReport();

        } finally {
            await this.framework.cleanup();
        }
    }

    async runSpecificTests(testNames) {
        console.log(`🎯 Running specific tests: ${testNames.join(', ')}`);

        this.framework.testStats.startTime = Date.now();

        try {
            await this.framework.initialize();

            // Collect all test cases from all suites
            const allTestCases = [];

            for (const suite of Object.values(this.testSuites)) {
                allTestCases.push(...suite.getTestCases());
            }

            // Filter for requested tests
            const requestedTests = allTestCases.filter(test =>
                testNames.includes(test.name)
            );

            if (requestedTests.length === 0) {
                throw new Error('No matching tests found');
            }

            console.log(`Found ${requestedTests.length} matching tests`);

            // Run the filtered tests
            for (const test of requestedTests) {
                await this.framework.runTest(test.name, test.func, test.category);
            }

            await this.generateFinalReport();

        } finally {
            await this.framework.cleanup();
        }
    }

    async generateFinalReport() {
        console.log('\n📊 Generating comprehensive test report...');

        const report = this.framework.generateReport();

        // Add performance metrics if available
        if (this.performanceTests.performanceMetrics) {
            report.performance = this.performanceTests.getPerformanceReport();
        }

        // Add detailed analysis
        report.analysis = this.analyzeResults(report);

        // Save report
        const reportPath = await this.framework.saveReport();

        // Generate HTML report
        await this.generateHTMLReport(report);

        // Print summary
        this.framework.printSummary();

        console.log(`\n📄 Detailed report saved: ${reportPath}`);

        return report;
    }

    analyzeResults(report) {
        const analysis = {
            coverage: {},
            recommendations: [],
            criticalIssues: [],
            summary: {}
        };

        // Analyze test coverage by category
        const categories = {};
        report.results.forEach(test => {
            if (!categories[test.category]) {
                categories[test.category] = { total: 0, passed: 0, failed: 0 };
            }
            categories[test.category].total++;
            if (test.status === 'PASSED') {
                categories[test.category].passed++;
            } else {
                categories[test.category].failed++;
            }
        });

        analysis.coverage = categories;

        // Identify critical issues
        const failedTests = report.results.filter(t => t.status === 'FAILED');
        const securityFailures = failedTests.filter(t => t.category.includes('security'));
        const performanceFailures = failedTests.filter(t => t.category.includes('performance'));

        if (securityFailures.length > 0) {
            analysis.criticalIssues.push({
                type: 'SECURITY',
                count: securityFailures.length,
                tests: securityFailures.map(t => t.name)
            });
        }

        if (performanceFailures.length > 0) {
            analysis.criticalIssues.push({
                type: 'PERFORMANCE',
                count: performanceFailures.length,
                tests: performanceFailures.map(t => t.name)
            });
        }

        // Generate recommendations
        if (report.summary.successRate < 95) {
            analysis.recommendations.push('Overall success rate is below 95%. Review failed tests and improve implementation.');
        }

        if (securityFailures.length > 0) {
            analysis.recommendations.push('Security tests failed. This requires immediate attention before production deployment.');
        }

        if (performanceFailures.length > 0) {
            analysis.recommendations.push('Performance tests failed. Consider optimization before handling increased load.');
        }

        // Summary
        analysis.summary = {
            totalCategories: Object.keys(categories).length,
            categoryWithMostFailures: Object.entries(categories)
                .sort((a, b) => b[1].failed - a[1].failed)[0],
            overallHealth: this.calculateOverallHealth(report.summary.successRate, analysis.criticalIssues.length)
        };

        return analysis;
    }

    calculateOverallHealth(successRate, criticalIssues) {
        if (criticalIssues > 0) return 'CRITICAL';
        if (successRate >= 95) return 'EXCELLENT';
        if (successRate >= 90) return 'GOOD';
        if (successRate >= 80) return 'FAIR';
        return 'POOR';
    }

    async generateHTMLReport(report) {
        const html = `
<!DOCTYPE html>
<html>
<head>
    <title>Dashboard E2E Test Report</title>
    <style>
        body { font-family: Arial, sans-serif; margin: 20px; }
        .header { background: #f5f5f5; padding: 20px; border-radius: 5px; }
        .summary { display: flex; gap: 20px; margin: 20px 0; }
        .metric { background: #e7f3ff; padding: 15px; border-radius: 5px; text-align: center; }
        .passed { color: green; }
        .failed { color: red; }
        .category { margin: 20px 0; }
        .test-result { padding: 10px; margin: 5px 0; border-radius: 3px; }
        .test-passed { background: #d4edda; }
        .test-failed { background: #f8d7da; }
        .critical { background: #f5c6cb; border: 2px solid #dc3545; padding: 15px; margin: 10px 0; }
        .recommendations { background: #d1ecf1; border: 2px solid #17a2b8; padding: 15px; margin: 10px 0; }
    </style>
</head>
<body>
    <div class="header">
        <h1>📊 Dashboard E2E Test Report</h1>
        <p>Generated: ${report.timestamp}</p>
        <p>Overall Health: <strong>${report.analysis.summary.overallHealth}</strong></p>
    </div>

    <div class="summary">
        <div class="metric">
            <h3>Total Tests</h3>
            <div>${report.summary.total}</div>
        </div>
        <div class="metric">
            <h3 class="passed">Passed</h3>
            <div>${report.summary.passed}</div>
        </div>
        <div class="metric">
            <h3 class="failed">Failed</h3>
            <div>${report.summary.failed}</div>
        </div>
        <div class="metric">
            <h3>Success Rate</h3>
            <div>${report.summary.successRate}%</div>
        </div>
    </div>

    ${report.analysis.criticalIssues.length > 0 ? `
    <div class="critical">
        <h3>🚨 Critical Issues</h3>
        ${report.analysis.criticalIssues.map(issue => `
            <p><strong>${issue.type}:</strong> ${issue.count} failed tests</p>
            <ul>${issue.tests.map(test => `<li>${test}</li>`).join('')}</ul>
        `).join('')}
    </div>
    ` : ''}

    ${report.analysis.recommendations.length > 0 ? `
    <div class="recommendations">
        <h3>💡 Recommendations</h3>
        <ul>
            ${report.analysis.recommendations.map(rec => `<li>${rec}</li>`).join('')}
        </ul>
    </div>
    ` : ''}

    <h2>📋 Test Results by Category</h2>
    ${Object.entries(report.analysis.coverage).map(([category, stats]) => `
        <div class="category">
            <h3>${category.replace('_', ' ').toUpperCase()}</h3>
            <p>Total: ${stats.total}, Passed: ${stats.passed}, Failed: ${stats.failed}</p>
            ${report.results
                .filter(test => test.category === category)
                .map(test => `
                    <div class="test-result test-${test.status.toLowerCase()}">
                        <strong>${test.name}</strong> - ${test.status}
                        ${test.error ? `<br><small>Error: ${test.error}</small>` : ''}
                    </div>
                `).join('')}
        </div>
    `).join('')}

    ${report.performance ? `
    <h2>⚡ Performance Metrics</h2>
    <pre>${JSON.stringify(report.performance.metrics, null, 2)}</pre>
    ` : ''}

</body>
</html>`;

        const fs = require('fs').promises;
        const path = require('path');

        const htmlPath = path.join(__dirname, 'reports', `dashboard-e2e-report-${Date.now()}.html`);
        await fs.mkdir(path.dirname(htmlPath), { recursive: true });
        await fs.writeFile(htmlPath, html);

        console.log(`📄 HTML report saved: ${htmlPath}`);
    }

    // Command line interface
    static async main(args = process.argv.slice(2)) {
        const runner = new DashboardE2ERunner();

        try {
            if (args.length === 0) {
                // Run all tests
                await runner.runAllTests();
            } else if (args[0] === '--suite') {
                // Run specific suite
                if (!args[1]) {
                    throw new Error('Suite name required. Options: api, webui, security, performance');
                }
                await runner.runSpecificSuite(args[1]);
            } else if (args[0] === '--tests') {
                // Run specific tests
                const testNames = args.slice(1);
                if (testNames.length === 0) {
                    throw new Error('Test names required');
                }
                await runner.runSpecificTests(testNames);
            } else {
                console.log(`
Usage: node dashboard-e2e-runner.js [OPTIONS]

Options:
  (no args)                  Run all test suites
  --suite <name>            Run specific suite (api, webui, security, performance)
  --tests <name> [name...]  Run specific tests by name

Examples:
  node dashboard-e2e-runner.js
  node dashboard-e2e-runner.js --suite api
  node dashboard-e2e-runner.js --tests login_endpoint client_search
                `);
                process.exit(1);
            }

            console.log('\n🎉 Dashboard E2E testing completed successfully!');
            process.exit(0);

        } catch (error) {
            console.error('\n💥 Dashboard E2E testing failed:', error.message);
            process.exit(1);
        }
    }
}

// Allow both programmatic use and command line execution
if (require.main === module) {
    DashboardE2ERunner.main();
}

module.exports = DashboardE2ERunner;