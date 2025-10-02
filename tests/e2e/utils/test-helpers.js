// Test Helper Functions for MQTTX E2E Tests
const assert = require('assert');
const crypto = require('crypto');

class TestHelpers {
  constructor() {
    this.testResults = [];
    this.startTime = Date.now();
  }

  // Generate random test data
  static generateRandomString(length = 10) {
    return crypto.randomBytes(length).toString('hex').slice(0, length);
  }

  static generateRandomPayload(size = 100) {
    return crypto.randomBytes(size).toString('base64').slice(0, size);
  }

  static generateRandomTopic(levels = 3) {
    const parts = [];
    for (let i = 0; i < levels; i++) {
      parts.push(this.generateRandomString(8));
    }
    return parts.join('/');
  }

  // Test execution framework
  async runTest(testName, testFunction, timeout = 10000) {
    console.log(`🧪 Running test: ${testName}`);
    const startTime = Date.now();

    try {
      // Run test with timeout
      const result = await Promise.race([
        testFunction(),
        new Promise((_, reject) =>
          setTimeout(() => reject(new Error('Test timeout')), timeout)
        )
      ]);

      const duration = Date.now() - startTime;
      this.testResults.push({
        name: testName,
        status: 'PASS',
        duration,
        result
      });

      console.log(`✅ ${testName} - PASSED (${duration}ms)`);
      return result;
    } catch (error) {
      const duration = Date.now() - startTime;
      this.testResults.push({
        name: testName,
        status: 'FAIL',
        duration,
        error: error.message
      });

      console.log(`❌ ${testName} - FAILED (${duration}ms): ${error.message}`);
      throw error;
    }
  }

  // Assertion helpers
  static assertEqual(actual, expected, message = 'Values should be equal') {
    assert.strictEqual(actual, expected, `${message}. Expected: ${expected}, Actual: ${actual}`);
  }

  static assertNotEqual(actual, expected, message = 'Values should not be equal') {
    assert.notStrictEqual(actual, expected, `${message}. Both values: ${actual}`);
  }

  static assertTrue(condition, message = 'Condition should be true') {
    assert.strictEqual(condition, true, message);
  }

  static assertFalse(condition, message = 'Condition should be false') {
    assert.strictEqual(condition, false, message);
  }

  static assertGreaterThan(actual, expected, message = 'Value should be greater') {
    assert(actual > expected, `${message}. Expected > ${expected}, Actual: ${actual}`);
  }

  static assertLessThan(actual, expected, message = 'Value should be less') {
    assert(actual < expected, `${message}. Expected < ${expected}, Actual: ${actual}`);
  }

  static assertContains(array, item, message = 'Array should contain item') {
    assert(array.includes(item), `${message}. Array: ${JSON.stringify(array)}, Item: ${item}`);
  }

  static assertMessageReceived(messages, expectedTopic, expectedPayload = null) {
    const message = messages.find(msg => msg.topic === expectedTopic);
    assert(message, `Message not received for topic: ${expectedTopic}`);

    if (expectedPayload !== null) {
      assert.strictEqual(message.message, expectedPayload,
        `Message payload mismatch. Expected: ${expectedPayload}, Actual: ${message.message}`);
    }

    return message;
  }

  // Performance measurement helpers
  static async measureLatency(asyncFunction) {
    const start = process.hrtime.bigint();
    const result = await asyncFunction();
    const end = process.hrtime.bigint();
    const latencyMs = Number(end - start) / 1000000; // Convert to milliseconds
    return { result, latencyMs };
  }

  static async measureThroughput(asyncFunction, count) {
    const start = Date.now();
    const results = [];

    for (let i = 0; i < count; i++) {
      results.push(await asyncFunction(i));
    }

    const duration = Date.now() - start;
    const throughput = (count / duration) * 1000; // operations per second

    return { results, duration, throughput };
  }

  // Wait utilities
  static async sleep(ms) {
    return new Promise(resolve => setTimeout(resolve, ms));
  }

  static async waitForCondition(condition, timeout = 5000, interval = 100) {
    const start = Date.now();
    while (Date.now() - start < timeout) {
      if (await condition()) {
        return true;
      }
      await this.sleep(interval);
    }
    throw new Error(`Condition not met within ${timeout}ms`);
  }

  // Test report generation
  generateReport() {
    const totalTests = this.testResults.length;
    const passedTests = this.testResults.filter(r => r.status === 'PASS').length;
    const failedTests = this.testResults.filter(r => r.status === 'FAIL').length;
    const totalDuration = Date.now() - this.startTime;

    const report = {
      summary: {
        total: totalTests,
        passed: passedTests,
        failed: failedTests,
        passRate: totalTests > 0 ? (passedTests / totalTests * 100).toFixed(1) : 0,
        duration: totalDuration
      },
      results: this.testResults,
      failedTests: this.testResults.filter(r => r.status === 'FAIL')
    };

    return report;
  }

  printReport() {
    const report = this.generateReport();

    console.log('\n📊 Test Results Summary');
    console.log('======================');
    console.log(`Total Tests: ${report.summary.total}`);
    console.log(`Passed: ${report.summary.passed}`);
    console.log(`Failed: ${report.summary.failed}`);
    console.log(`Pass Rate: ${report.summary.passRate}%`);
    console.log(`Duration: ${report.summary.duration}ms`);

    if (report.failedTests.length > 0) {
      console.log('\n❌ Failed Tests:');
      report.failedTests.forEach(test => {
        console.log(`  - ${test.name}: ${test.error}`);
      });
    }

    return report;
  }

  // Reset test state
  reset() {
    this.testResults = [];
    this.startTime = Date.now();
  }
}

// Topic utilities
class TopicUtils {
  static isWildcard(topic) {
    return topic.includes('+') || topic.includes('#');
  }

  static matchesWildcard(topic, pattern) {
    // Convert MQTT wildcard pattern to regex
    let regex = pattern
      .replace(/\+/g, '[^/]+')  // + matches one level
      .replace(/#/g, '.*');     // # matches multiple levels

    regex = '^' + regex + '$';
    return new RegExp(regex).test(topic);
  }

  static generateTopicLevels(base, levels) {
    const topics = [];
    let current = base;

    for (let i = 0; i < levels; i++) {
      current += `/level${i}`;
      topics.push(current);
    }

    return topics;
  }
}

// Message validation utilities
class MessageValidator {
  static validateQoS(message, expectedQoS) {
    TestHelpers.assertEqual(message.qos, expectedQoS,
      `QoS mismatch for topic ${message.topic}`);
  }

  static validateRetain(message, expectedRetain) {
    TestHelpers.assertEqual(message.retain, expectedRetain,
      `Retain flag mismatch for topic ${message.topic}`);
  }

  static validatePayload(message, expectedPayload) {
    TestHelpers.assertEqual(message.message, expectedPayload,
      `Payload mismatch for topic ${message.topic}`);
  }

  static validateMessageOrder(messages, expectedOrder) {
    const actualOrder = messages.map(m => m.topic);
    TestHelpers.assertEqual(JSON.stringify(actualOrder), JSON.stringify(expectedOrder),
      'Message order mismatch');
  }
}

module.exports = {
  TestHelpers,
  TopicUtils,
  MessageValidator
};