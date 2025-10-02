// MQTTX Comprehensive Test Runner - Executes All 1000+ Test Cases
const fs = require('fs');
const path = require('path');

// Import all test suites
const BasicOperationsTests = require('./mqttx-basic-operations');
const ProtocolComplianceTests = require('./mqttx-protocol-compliance');
const TopicManagementTests = require('./mqttx-topic-management');
const ConcurrencyPerformanceTests = require('./mqttx-concurrency-performance');
const SecurityAuthTests = require('./mqttx-security-auth');
const ErrorHandlingTests = require('./mqttx-error-handling');
const RealWorldScenariosTests = require('./mqttx-realworld-scenarios');

// Additional 500 test suites
const AdvancedMQTT5Tests = require('./mqttx-advanced-mqtt5');
const WebSocketTransportTests = require('./mqttx-websocket-transport');
const ProtocolEdgeCasesTests = require('./mqttx-protocol-edge-cases');
const LargeScaleIntegrationTests = require('./mqttx-large-scale-integration');
const IoTScenariosTests = require('./mqttx-iot-scenarios');
const SecurityComplianceTests = require('./mqttx-security-compliance');

class MQTTXTestRunner {
  constructor() {
    this.testSuites = [];
    this.totalTestResults = {
      totalSuites: 0,
      totalTests: 0,
      totalPassed: 0,
      totalFailed: 0,
      totalSkipped: 0,
      suiteResults: [],
      startTime: null,
      endTime: null,
      duration: 0
    };
    this.testConfig = {
      stopOnFirstFailure: false,
      parallel: false,
      timeout: 300000, // 5 minutes per test suite
      generateReport: true,
      reportPath: './test-reports'
    };
  }

  // Initialize all test suites
  initializeTestSuites() {
    console.log('🚀 Initializing MQTTX Comprehensive Test Suite (1000+ test cases)');

    // Original 500 test suites
    this.testSuites.push(
      { name: 'Basic Operations', class: BasicOperationsTests, tests: 100, category: 'core' },
      { name: 'Protocol Compliance', class: ProtocolComplianceTests, tests: 80, category: 'core' },
      { name: 'Topic Management', class: TopicManagementTests, tests: 75, category: 'core' },
      { name: 'Concurrency Performance', class: ConcurrencyPerformanceTests, tests: 60, category: 'performance' },
      { name: 'Security Auth', class: SecurityAuthTests, tests: 50, category: 'security' },
      { name: 'Error Handling', class: ErrorHandlingTests, tests: 60, category: 'reliability' },
      { name: 'Real World Scenarios', class: RealWorldScenariosTests, tests: 75, category: 'integration' }
    );

    // Additional 500 test suites
    this.testSuites.push(
      { name: 'Advanced MQTT 5.0', class: AdvancedMQTT5Tests, tests: 60, category: 'advanced' },
      { name: 'WebSocket Transport', class: WebSocketTransportTests, tests: 50, category: 'transport' },
      { name: 'Protocol Edge Cases', class: ProtocolEdgeCasesTests, tests: 60, category: 'edge_cases' },
      { name: 'Large Scale Integration', class: LargeScaleIntegrationTests, tests: 80, category: 'scale' },
      { name: 'IoT Scenarios', class: IoTScenariosTests, tests: 120, category: 'iot' },
      { name: 'Security Compliance', class: SecurityComplianceTests, tests: 130, category: 'compliance' }
    );

    this.totalTestResults.totalSuites = this.testSuites.length;
    this.totalTestResults.totalTests = this.testSuites.reduce((sum, suite) => sum + suite.tests, 0);

    console.log(`📊 Test Suite Summary:`);
    console.log(`- Total Suites: ${this.totalTestResults.totalSuites}`);
    console.log(`- Total Tests: ${this.totalTestResults.totalTests}`);
    console.log(`- Categories: ${[...new Set(this.testSuites.map(s => s.category))].join(', ')}`);
  }

  // Run a single test suite
  async runTestSuite(suite) {
    console.log(`\\n🧪 Running ${suite.name} Tests (${suite.tests} test cases)`);
    console.log(`Category: ${suite.category}`);
    console.log(`${'='.repeat(60)}`);

    const startTime = Date.now();
    let result = {
      suiteName: suite.name,
      category: suite.category,
      expectedTests: suite.tests,
      actualTests: 0,
      passed: 0,
      failed: 0,
      skipped: 0,
      duration: 0,
      startTime: startTime,
      endTime: null,
      error: null,
      details: null
    };

    try {
      const testInstance = new suite.class();
      const testReport = await testInstance.runAllTests();

      result.actualTests = testReport.summary.total;
      result.passed = testReport.summary.passed;
      result.failed = testReport.summary.failed;
      result.skipped = testReport.summary.skipped || 0;
      result.details = testReport;

      console.log(`✅ ${suite.name} completed:`);
      console.log(`   Tests: ${result.actualTests}/${result.expectedTests}`);
      console.log(`   Passed: ${result.passed}`);
      console.log(`   Failed: ${result.failed}`);
      console.log(`   Pass Rate: ${testReport.summary.passRate}%`);

    } catch (error) {
      result.error = error.message;
      result.failed = result.expectedTests; // Mark all as failed if suite crashes

      console.error(`❌ ${suite.name} failed with error: ${error.message}`);

      if (this.testConfig.stopOnFirstFailure) {
        throw error;
      }
    }

    result.endTime = Date.now();
    result.duration = result.endTime - result.startTime;

    this.totalTestResults.suiteResults.push(result);
    this.totalTestResults.totalPassed += result.passed;
    this.totalTestResults.totalFailed += result.failed;
    this.totalTestResults.totalSkipped += result.skipped;

    return result;
  }

  // Run all test suites
  async runAllTests() {
    console.log('\\n🎯 Starting MQTTX Comprehensive Test Execution');
    console.log(`Mode: ${this.testConfig.parallel ? 'Parallel' : 'Sequential'}`);
    console.log(`Stop on failure: ${this.testConfig.stopOnFirstFailure}`);
    console.log(`${'='.repeat(80)}`);

    this.totalTestResults.startTime = Date.now();

    try {
      if (this.testConfig.parallel) {
        // Run test suites in parallel (for independent suites)
        const promises = this.testSuites.map(suite => this.runTestSuite(suite));
        await Promise.all(promises);
      } else {
        // Run test suites sequentially
        for (const suite of this.testSuites) {
          await this.runTestSuite(suite);

          // Add delay between suites to prevent resource exhaustion
          await new Promise(resolve => setTimeout(resolve, 2000));
        }
      }
    } catch (error) {
      console.error(`❌ Test execution stopped due to error: ${error.message}`);
    }

    this.totalTestResults.endTime = Date.now();
    this.totalTestResults.duration = this.totalTestResults.endTime - this.totalTestResults.startTime;

    return this.generateFinalReport();
  }

  // Generate comprehensive test report
  generateFinalReport() {
    const passRate = this.totalTestResults.totalTests > 0
      ? ((this.totalTestResults.totalPassed / this.totalTestResults.totalTests) * 100).toFixed(2)
      : 0;

    const report = {
      summary: {
        totalSuites: this.totalTestResults.totalSuites,
        totalTests: this.totalTestResults.totalTests,
        totalPassed: this.totalTestResults.totalPassed,
        totalFailed: this.totalTestResults.totalFailed,
        totalSkipped: this.totalTestResults.totalSkipped,
        passRate: parseFloat(passRate),
        duration: this.totalTestResults.duration,
        startTime: new Date(this.totalTestResults.startTime).toISOString(),
        endTime: new Date(this.totalTestResults.endTime).toISOString()
      },
      suiteResults: this.totalTestResults.suiteResults,
      categoryBreakdown: this.generateCategoryBreakdown(),
      recommendations: this.generateRecommendations()
    };

    // Console output
    console.log('\\n' + '='.repeat(80));
    console.log('📊 MQTTX COMPREHENSIVE TEST RESULTS');
    console.log('='.repeat(80));
    console.log(`🕒 Execution Time: ${(report.summary.duration / 1000 / 60).toFixed(2)} minutes`);
    console.log(`📈 Overall Pass Rate: ${report.summary.passRate}%`);
    console.log(`\\n📋 Summary:`);
    console.log(`   Total Test Suites: ${report.summary.totalSuites}`);
    console.log(`   Total Test Cases: ${report.summary.totalTests}`);
    console.log(`   ✅ Passed: ${report.summary.totalPassed}`);
    console.log(`   ❌ Failed: ${report.summary.totalFailed}`);
    console.log(`   ⏭️  Skipped: ${report.summary.totalSkipped}`);

    // Category breakdown
    console.log(`\\n📊 Results by Category:`);
    Object.entries(report.categoryBreakdown).forEach(([category, stats]) => {
      console.log(`   ${category}: ${stats.passed}/${stats.total} (${stats.passRate}%)`);
    });

    // Suite results
    console.log(`\\n📝 Suite Results:`);
    report.suiteResults.forEach(result => {
      const status = result.failed === 0 ? '✅' : '❌';
      const suitePassRate = result.actualTests > 0
        ? ((result.passed / result.actualTests) * 100).toFixed(1)
        : 0;
      console.log(`   ${status} ${result.suiteName}: ${result.passed}/${result.actualTests} (${suitePassRate}%)`);
      if (result.error) {
        console.log(`      Error: ${result.error}`);
      }
    });

    // Recommendations
    if (report.recommendations.length > 0) {
      console.log(`\\n💡 Recommendations:`);
      report.recommendations.forEach((rec, index) => {
        console.log(`   ${index + 1}. ${rec}`);
      });
    }

    // Save detailed report to file
    if (this.testConfig.generateReport) {
      this.saveReportToFile(report);
    }

    return report;
  }

  generateCategoryBreakdown() {
    const breakdown = {};

    this.totalTestResults.suiteResults.forEach(result => {
      if (!breakdown[result.category]) {
        breakdown[result.category] = { total: 0, passed: 0, failed: 0, passRate: 0 };
      }

      breakdown[result.category].total += result.actualTests;
      breakdown[result.category].passed += result.passed;
      breakdown[result.category].failed += result.failed;
    });

    // Calculate pass rates
    Object.keys(breakdown).forEach(category => {
      const stats = breakdown[category];
      stats.passRate = stats.total > 0
        ? ((stats.passed / stats.total) * 100).toFixed(1)
        : 0;
    });

    return breakdown;
  }

  generateRecommendations() {
    const recommendations = [];
    const failedSuites = this.totalTestResults.suiteResults.filter(r => r.failed > 0);

    if (failedSuites.length > 0) {
      recommendations.push('Review failed test cases and fix underlying issues in emqx-go source code');
    }

    if (this.totalTestResults.totalFailed > this.totalTestResults.totalPassed * 0.1) {
      recommendations.push('High failure rate detected - prioritize basic functionality fixes');
    }

    const slowSuites = this.totalTestResults.suiteResults.filter(r => r.duration > 60000);
    if (slowSuites.length > 0) {
      recommendations.push('Optimize performance for slow test suites: ' + slowSuites.map(s => s.suiteName).join(', '));
    }

    if (this.totalTestResults.suiteResults.some(r => r.error)) {
      recommendations.push('Fix test suite execution errors before proceeding with source code fixes');
    }

    return recommendations;
  }

  saveReportToFile(report) {
    try {
      // Ensure report directory exists
      if (!fs.existsSync(this.testConfig.reportPath)) {
        fs.mkdirSync(this.testConfig.reportPath, { recursive: true });
      }

      // Generate timestamp for filename
      const timestamp = new Date().toISOString().replace(/[:.]/g, '-');

      // Save JSON report
      const jsonPath = path.join(this.testConfig.reportPath, `mqttx-test-report-${timestamp}.json`);
      fs.writeFileSync(jsonPath, JSON.stringify(report, null, 2));

      // Save HTML report
      const htmlPath = path.join(this.testConfig.reportPath, `mqttx-test-report-${timestamp}.html`);
      fs.writeFileSync(htmlPath, this.generateHTMLReport(report));

      console.log(`\\n📄 Reports saved:`);
      console.log(`   JSON: ${jsonPath}`);
      console.log(`   HTML: ${htmlPath}`);

    } catch (error) {
      console.error(`❌ Failed to save report: ${error.message}`);
    }
  }

  generateHTMLReport(report) {
    return `
<!DOCTYPE html>
<html>
<head>
    <title>MQTTX Comprehensive Test Report</title>
    <style>
        body { font-family: Arial, sans-serif; margin: 20px; }
        .header { background: #f4f4f4; padding: 20px; border-radius: 5px; }
        .summary { display: flex; gap: 20px; margin: 20px 0; }
        .metric { background: #e9ecef; padding: 15px; border-radius: 5px; text-align: center; }
        .passed { color: #28a745; }
        .failed { color: #dc3545; }
        .suite { margin: 10px 0; padding: 10px; border: 1px solid #ddd; border-radius: 5px; }
        .category { background: #f8f9fa; padding: 10px; margin: 5px 0; border-radius: 3px; }
    </style>
</head>
<body>
    <div class="header">
        <h1>MQTTX Comprehensive Test Report</h1>
        <p>Generated: ${report.summary.endTime}</p>
        <p>Duration: ${(report.summary.duration / 1000 / 60).toFixed(2)} minutes</p>
    </div>

    <div class="summary">
        <div class="metric">
            <h3>Total Tests</h3>
            <div>${report.summary.totalTests}</div>
        </div>
        <div class="metric">
            <h3>Pass Rate</h3>
            <div class="${report.summary.passRate >= 80 ? 'passed' : 'failed'}">${report.summary.passRate}%</div>
        </div>
        <div class="metric passed">
            <h3>Passed</h3>
            <div>${report.summary.totalPassed}</div>
        </div>
        <div class="metric failed">
            <h3>Failed</h3>
            <div>${report.summary.totalFailed}</div>
        </div>
    </div>

    <h2>Results by Category</h2>
    ${Object.entries(report.categoryBreakdown).map(([category, stats]) => `
        <div class="category">
            <strong>${category}:</strong> ${stats.passed}/${stats.total} (${stats.passRate}%)
        </div>
    `).join('')}

    <h2>Suite Results</h2>
    ${report.suiteResults.map(result => `
        <div class="suite">
            <h3>${result.suiteName} <span class="${result.failed === 0 ? 'passed' : 'failed'}">(${result.passed}/${result.actualTests})</span></h3>
            <p>Category: ${result.category} | Duration: ${(result.duration / 1000).toFixed(1)}s</p>
            ${result.error ? `<p class="failed">Error: ${result.error}</p>` : ''}
        </div>
    `).join('')}

    ${report.recommendations.length > 0 ? `
        <h2>Recommendations</h2>
        <ul>
            ${report.recommendations.map(rec => `<li>${rec}</li>`).join('')}
        </ul>
    ` : ''}
</body>
</html>`;
  }

  // Configure test execution
  configure(options = {}) {
    this.testConfig = { ...this.testConfig, ...options };
  }
}

// CLI execution
if (require.main === module) {
  (async () => {
    const runner = new MQTTXTestRunner();

    // Parse command line arguments
    const args = process.argv.slice(2);
    const config = {};

    if (args.includes('--parallel')) {
      config.parallel = true;
    }
    if (args.includes('--stop-on-failure')) {
      config.stopOnFirstFailure = true;
    }
    if (args.includes('--no-report')) {
      config.generateReport = false;
    }

    runner.configure(config);
    runner.initializeTestSuites();

    try {
      const report = await runner.runAllTests();
      const exitCode = report.summary.totalFailed > 0 ? 1 : 0;

      console.log(`\\n🏁 Test execution completed with exit code: ${exitCode}`);
      process.exit(exitCode);

    } catch (error) {
      console.error(`❌ Test runner failed: ${error.message}`);
      process.exit(1);
    }
  })();
}

module.exports = MQTTXTestRunner;