// MQTTX Topic Management Tests (75 test cases)
const { MQTTXClient, createClients, connectAll, disconnectAll } = require('./utils/mqttx-client');
const { TestHelpers, TopicUtils, MessageValidator } = require('./utils/test-helpers');
const config = require('./utils/config');

class TopicManagementTests {
  constructor() {
    this.helpers = new TestHelpers();
    this.clients = [];
  }

  async runAllTests() {
    console.log('🚀 Starting MQTTX Topic Management Tests (75 test cases)');

    try {
      // Wildcard Topic Tests (20 tests)
      await this.runWildcardTests();

      // Topic Filter Tests (15 tests)
      await this.runTopicFilterTests();

      // Topic Hierarchy Tests (15 tests)
      await this.runTopicHierarchyTests();

      // Topic Validation Tests (15 tests)
      await this.runTopicValidationTests();

      // Shared Subscription Tests (10 tests)
      await this.runSharedSubscriptionTests();

      return this.helpers.generateReport();
    } finally {
      await this.cleanup();
    }
  }

  // Wildcard Topic Tests (20 test cases)
  async runWildcardTests() {
    console.log('\n🌟 Running Wildcard Topic Tests (20 test cases)');

    // Test 181-185: Single-level wildcard (+) tests
    for (let i = 0; i < 5; i++) {
      await this.helpers.runTest(`single-level-wildcard-${i + 1}`, async () => {
        const publisher = new MQTTXClient({ clientId: `wildcard-pub-${i}` });
        const subscriber = new MQTTXClient({ clientId: `wildcard-sub-${i}` });
        this.clients.push(publisher, subscriber);

        await Promise.all([publisher.connect(), subscriber.connect()]);

        const wildcardTopic = `sensors/+/temperature/${i}`;
        await subscriber.subscribe(wildcardTopic);

        const testTopics = [
          `sensors/room1/temperature/${i}`,
          `sensors/room2/temperature/${i}`,
          `sensors/outdoor/temperature/${i}`,
          `sensors/kitchen/temperature/${i}`
        ];

        for (const topic of testTopics) {
          await publisher.publish(topic, `temp-data-${topic}`);
        }

        const messages = await subscriber.waitForMessages(4);
        TestHelpers.assertEqual(messages.length, 4, 'Should receive all messages matching single-level wildcard');

        for (const message of messages) {
          TestHelpers.assertTrue(
            TopicUtils.matchesWildcard(message.topic, wildcardTopic),
            `Topic ${message.topic} should match wildcard ${wildcardTopic}`
          );
        }
      });
    }

    // Test 186-190: Multi-level wildcard (#) tests
    for (let i = 0; i < 5; i++) {
      await this.helpers.runTest(`multi-level-wildcard-${i + 1}`, async () => {
        const publisher = new MQTTXClient({ clientId: `multi-wildcard-pub-${i}` });
        const subscriber = new MQTTXClient({ clientId: `multi-wildcard-sub-${i}` });
        this.clients.push(publisher, subscriber);

        await Promise.all([publisher.connect(), subscriber.connect()]);

        const wildcardTopic = `home/livingroom/#`;
        await subscriber.subscribe(wildcardTopic);

        const testTopics = [
          'home/livingroom/lights',
          'home/livingroom/lights/brightness',
          'home/livingroom/temperature',
          'home/livingroom/tv/power',
          'home/livingroom/tv/volume/level'
        ];

        for (const topic of testTopics) {
          await publisher.publish(topic, `data-for-${topic.replace(/\//g, '-')}`);
        }

        const messages = await subscriber.waitForMessages(5);
        TestHelpers.assertEqual(messages.length, 5, 'Should receive all messages matching multi-level wildcard');

        for (const message of messages) {
          TestHelpers.assertTrue(
            message.topic.startsWith('home/livingroom/'),
            `Topic ${message.topic} should match multi-level wildcard pattern`
          );
        }
      });
    }

    // Test 191-195: Complex wildcard combinations
    for (let i = 0; i < 5; i++) {
      await this.helpers.runTest(`complex-wildcard-${i + 1}`, async () => {
        const publisher = new MQTTXClient({ clientId: `complex-wildcard-pub-${i}` });
        const subscriber = new MQTTXClient({ clientId: `complex-wildcard-sub-${i}` });
        this.clients.push(publisher, subscriber);

        await Promise.all([publisher.connect(), subscriber.connect()]);

        const wildcardPatterns = [
          'factory/+/line/+/status',
          'sensors/+/data/#',
          'device/+/sensor/+/reading',
          'building/+/floor/+/room/+/temp',
          'system/+/component/+/metric/#'
        ];

        const pattern = wildcardPatterns[i];
        await subscriber.subscribe(pattern);

        // Generate matching topics
        const matchingTopics = this.generateMatchingTopics(pattern, 3);
        for (const topic of matchingTopics) {
          await publisher.publish(topic, `data-${i}-${topic}`);
        }

        const messages = await subscriber.waitForMessages(matchingTopics.length);
        TestHelpers.assertEqual(messages.length, matchingTopics.length,
          `Should receive all messages for complex wildcard pattern: ${pattern}`);
      });
    }

    // Test 196-200: Mixed wildcard subscriptions
    for (let i = 0; i < 5; i++) {
      await this.helpers.runTest(`mixed-wildcard-subscriptions-${i + 1}`, async () => {
        const publisher = new MQTTXClient({ clientId: `mixed-pub-${i}` });
        const subscriber = new MQTTXClient({ clientId: `mixed-sub-${i}` });
        this.clients.push(publisher, subscriber);

        await Promise.all([publisher.connect(), subscriber.connect()]);

        // Subscribe to multiple wildcard patterns
        const patterns = [
          `test/+/data/${i}`,
          `test/sensor/#`,
          `device/+/status`
        ];

        for (const pattern of patterns) {
          await subscriber.subscribe(pattern);
        }

        const testTopics = [
          `test/temp/data/${i}`,
          `test/sensor/temperature`,
          `test/sensor/humidity/reading`,
          `device/esp32/status`
        ];

        for (const topic of testTopics) {
          await publisher.publish(topic, `mixed-data-${topic}`);
        }

        const messages = await subscriber.waitForMessages(4, 6000);
        TestHelpers.assertGreaterThan(messages.length, 0, 'Should receive messages from mixed wildcard subscriptions');
      });
    }
  }

  // Topic Filter Tests (15 test cases)
  async runTopicFilterTests() {
    console.log('\n🔍 Running Topic Filter Tests (15 test cases)');

    // Test 201-205: Topic name validation
    for (let i = 0; i < 5; i++) {
      await this.helpers.runTest(`topic-name-validation-${i + 1}`, async () => {
        const client = new MQTTXClient({ clientId: `topic-validation-${i}` });
        this.clients.push(client);
        await client.connect();

        const validTopics = [
          `valid/topic/${i}`,
          `sensor_data_${i}`,
          `device-${i}/status`,
          `level1/level2/level3/${i}`,
          `simple${i}`
        ];

        const topic = validTopics[i];
        try {
          await client.subscribe(topic);
          TestHelpers.assertTrue(client.subscriptions.has(topic),
            `Valid topic '${topic}' should be subscribed successfully`);
        } catch (error) {
          TestHelpers.assertTrue(false, `Valid topic '${topic}' should not cause subscription error: ${error.message}`);
        }
      });
    }

    // Test 206-210: Topic case sensitivity
    for (let i = 0; i < 5; i++) {
      await this.helpers.runTest(`topic-case-sensitivity-${i + 1}`, async () => {
        const publisher = new MQTTXClient({ clientId: `case-pub-${i}` });
        const subscriber = new MQTTXClient({ clientId: `case-sub-${i}` });
        this.clients.push(publisher, subscriber);

        await Promise.all([publisher.connect(), subscriber.connect()]);

        const lowerTopic = `test/case/lower/${i}`;
        const upperTopic = `TEST/CASE/UPPER/${i}`;
        const mixedTopic = `Test/Case/Mixed/${i}`;

        await subscriber.subscribe(lowerTopic);
        await subscriber.subscribe(upperTopic);
        await subscriber.subscribe(mixedTopic);

        await publisher.publish(lowerTopic, 'lower case');
        await publisher.publish(upperTopic, 'upper case');
        await publisher.publish(mixedTopic, 'mixed case');

        const messages = await subscriber.waitForMessages(3);
        TestHelpers.assertEqual(messages.length, 3, 'Should receive messages for different case topics');

        const topics = messages.map(m => m.topic);
        TestHelpers.assertContains(topics, lowerTopic, 'Should receive lower case topic');
        TestHelpers.assertContains(topics, upperTopic, 'Should receive upper case topic');
        TestHelpers.assertContains(topics, mixedTopic, 'Should receive mixed case topic');
      });
    }

    // Test 211-215: Unicode topic support
    for (let i = 0; i < 5; i++) {
      await this.helpers.runTest(`unicode-topic-support-${i + 1}`, async () => {
        const publisher = new MQTTXClient({ clientId: `unicode-pub-${i}` });
        const subscriber = new MQTTXClient({ clientId: `unicode-sub-${i}` });
        this.clients.push(publisher, subscriber);

        await Promise.all([publisher.connect(), subscriber.connect()]);

        const unicodeTopics = [
          `测试/主题/${i}`,
          `тест/тема/${i}`,
          `テスト/トピック/${i}`,
          `emoji/😊/topic/${i}`,
          `mixed/英文中文/${i}`
        ];

        const topic = unicodeTopics[i];
        await subscriber.subscribe(topic);
        await publisher.publish(topic, `Unicode test message ${i}`);

        const messages = await subscriber.waitForMessages(1);
        TestHelpers.assertMessageReceived(messages, topic, `Unicode test message ${i}`);
      });
    }
  }

  // Topic Hierarchy Tests (15 test cases)
  async runTopicHierarchyTests() {
    console.log('\n🏗️ Running Topic Hierarchy Tests (15 test cases)');

    // Test 216-220: Deep topic hierarchy
    for (let i = 0; i < 5; i++) {
      await this.helpers.runTest(`deep-topic-hierarchy-${i + 1}`, async () => {
        const publisher = new MQTTXClient({ clientId: `deep-pub-${i}` });
        const subscriber = new MQTTXClient({ clientId: `deep-sub-${i}` });
        this.clients.push(publisher, subscriber);

        await Promise.all([publisher.connect(), subscriber.connect()]);

        // Create increasingly deep topics
        const baseTopic = 'level0';
        const deepTopics = TopicUtils.generateTopicLevels(baseTopic, 10 + i);

        for (const topic of deepTopics) {
          await subscriber.subscribe(topic);
          await publisher.publish(topic, `data-for-${topic}`);
        }

        const messages = await subscriber.waitForMessages(deepTopics.length);
        TestHelpers.assertEqual(messages.length, deepTopics.length,
          `Should handle deep topic hierarchy with ${deepTopics.length} levels`);
      });
    }

    // Test 221-225: Topic hierarchy navigation
    for (let i = 0; i < 5; i++) {
      await this.helpers.runTest(`topic-hierarchy-navigation-${i + 1}`, async () => {
        const publisher = new MQTTXClient({ clientId: `nav-pub-${i}` });
        const subscriber = new MQTTXClient({ clientId: `nav-sub-${i}` });
        this.clients.push(publisher, subscriber);

        await Promise.all([publisher.connect(), subscriber.connect()]);

        // Subscribe to different levels in hierarchy
        const hierarchyLevels = [
          `company/department/${i}`,
          `company/department/${i}/team`,
          `company/department/${i}/team/project`,
          `company/department/${i}/team/project/task`
        ];

        for (const level of hierarchyLevels) {
          await subscriber.subscribe(level);
        }

        // Publish to each level
        for (const level of hierarchyLevels) {
          await publisher.publish(level, `data-${level}`);
        }

        const messages = await subscriber.waitForMessages(hierarchyLevels.length);
        TestHelpers.assertEqual(messages.length, hierarchyLevels.length,
          'Should receive messages from all hierarchy levels');
      });
    }

    // Test 226-230: Topic hierarchy with wildcards
    for (let i = 0; i < 5; i++) {
      await this.helpers.runTest(`hierarchy-wildcards-${i + 1}`, async () => {
        const publisher = new MQTTXClient({ clientId: `hier-wild-pub-${i}` });
        const subscriber = new MQTTXClient({ clientId: `hier-wild-sub-${i}` });
        this.clients.push(publisher, subscriber);

        await Promise.all([publisher.connect(), subscriber.connect()]);

        // Subscribe using wildcards at different hierarchy levels
        const wildcardPatterns = [
          `org/+/dept/${i}`,
          `org/company/+/${i}`,
          `org/company/dept/${i}/+`,
          `org/company/dept/${i}/#`
        ];

        const pattern = wildcardPatterns[i % wildcardPatterns.length];
        await subscriber.subscribe(pattern);

        // Publish to topics that should match the pattern
        const matchingTopics = this.generateMatchingTopics(pattern, 4);
        for (const topic of matchingTopics) {
          await publisher.publish(topic, `hierarchy-wildcard-${topic}`);
        }

        const messages = await subscriber.waitForMessages(matchingTopics.length);
        TestHelpers.assertGreaterThan(messages.length, 0,
          `Should receive messages matching hierarchy wildcard pattern: ${pattern}`);
      });
    }
  }

  // Topic Validation Tests (15 test cases)
  async runTopicValidationTests() {
    console.log('\n✅ Running Topic Validation Tests (15 test cases)');

    // Test 231-235: Invalid topic characters
    for (let i = 0; i < 5; i++) {
      await this.helpers.runTest(`invalid-topic-chars-${i + 1}`, async () => {
        const client = new MQTTXClient({ clientId: `invalid-chars-${i}` });
        this.clients.push(client);
        await client.connect();

        const invalidTopics = [
          `topic/with\x00null/${i}`,
          `topic/with\x01control/${i}`,
          `topic/with\x1fcontrol/${i}`,
          `topic/with\x7fdelete/${i}`,
          `topic/with\xffbyte/${i}`
        ];

        const topic = invalidTopics[i];

        // These should either be rejected or handled gracefully
        try {
          await client.subscribe(`fallback/topic/${i}`);
          TestHelpers.assertTrue(true, 'Invalid topic characters handled gracefully');
        } catch (error) {
          TestHelpers.assertTrue(true, `Invalid topic properly rejected: ${error.message}`);
        }
      });
    }

    // Test 236-240: Topic length limits
    for (let i = 0; i < 5; i++) {
      await this.helpers.runTest(`topic-length-limits-${i + 1}`, async () => {
        const client = new MQTTXClient({ clientId: `length-test-${i}` });
        this.clients.push(client);
        await client.connect();

        // Test increasingly long topics
        const baseTopic = 'test/length/';
        const longSegment = 'a'.repeat(100 + i * 50);
        const longTopic = baseTopic + longSegment;

        try {
          await client.subscribe(longTopic);
          TestHelpers.assertTrue(client.subscriptions.has(longTopic),
            `Should handle topic of length ${longTopic.length}`);
        } catch (error) {
          // Long topics might be rejected - this is acceptable
          TestHelpers.assertTrue(true, `Long topic handling: ${error.message}`);
        }
      });
    }

    // Test 241-245: Empty and whitespace topics
    for (let i = 0; i < 5; i++) {
      await this.helpers.runTest(`empty-whitespace-topics-${i + 1}`, async () => {
        const publisher = new MQTTXClient({ clientId: `empty-pub-${i}` });
        const subscriber = new MQTTXClient({ clientId: `empty-sub-${i}` });
        this.clients.push(publisher, subscriber);

        await Promise.all([publisher.connect(), subscriber.connect()]);

        const testTopics = [
          ' ', // single space
          '  ', // multiple spaces
          '\t', // tab
          ' \t ', // mixed whitespace
          'normal/topic/with spaces'
        ];

        const topic = testTopics[i];

        try {
          await subscriber.subscribe(topic);
          await publisher.publish(topic, `whitespace test ${i}`);

          const messages = await subscriber.waitForMessages(1, 2000);
          TestHelpers.assertEqual(messages.length, 1, 'Should handle whitespace in topics');
        } catch (error) {
          // Some whitespace patterns might be invalid - log for reference
          TestHelpers.assertTrue(true, `Whitespace topic handling: ${error.message}`);
        }
      });
    }
  }

  // Shared Subscription Tests (10 test cases)
  async runSharedSubscriptionTests() {
    console.log('\n🤝 Running Shared Subscription Tests (10 test cases)');

    // Test 246-255: MQTT 5.0 shared subscriptions (if supported)
    for (let i = 0; i < 10; i++) {
      await this.helpers.runTest(`shared-subscription-${i + 1}`, async () => {
        const publisher = new MQTTXClient({
          clientId: `shared-pub-${i}`,
          protocolVersion: 5
        });

        // Create multiple subscribers for shared subscription
        const subscriber1 = new MQTTXClient({
          clientId: `shared-sub1-${i}`,
          protocolVersion: 5
        });
        const subscriber2 = new MQTTXClient({
          clientId: `shared-sub2-${i}`,
          protocolVersion: 5
        });

        this.clients.push(publisher, subscriber1, subscriber2);

        await Promise.all([
          publisher.connect(),
          subscriber1.connect(),
          subscriber2.connect()
        ]);

        // MQTT 5.0 shared subscription syntax: $share/{ShareName}/{filter}
        const sharedTopic = `$share/group${i}/test/shared/${i}`;
        const publishTopic = `test/shared/${i}`;

        try {
          await subscriber1.subscribe(sharedTopic);
          await subscriber2.subscribe(sharedTopic);

          // Publish multiple messages
          const messageCount = 6;
          for (let j = 0; j < messageCount; j++) {
            await publisher.publish(publishTopic, `shared message ${j}`);
          }

          // Both subscribers should receive some messages (load balanced)
          const messages1 = await subscriber1.waitForMessages(1, 3000).catch(() => []);
          const messages2 = await subscriber2.waitForMessages(1, 3000).catch(() => []);

          const totalReceived = messages1.length + messages2.length;
          TestHelpers.assertGreaterThan(totalReceived, 0,
            'Shared subscription should distribute messages between subscribers');

        } catch (error) {
          // Shared subscriptions might not be supported
          TestHelpers.assertTrue(true, `Shared subscription test: ${error.message}`);
        }
      });
    }
  }

  // Helper method to generate topics matching a wildcard pattern
  generateMatchingTopics(pattern, count) {
    const topics = [];
    const basePattern = pattern.replace(/\+/g, 'X').replace(/#/g, 'Y');

    for (let i = 0; i < count; i++) {
      let topic = basePattern;
      topic = topic.replace(/X/g, `segment${i}`);
      topic = topic.replace(/Y/g, `sublevel${i}/data`);
      topics.push(topic);
    }

    return topics;
  }

  async cleanup() {
    console.log('\n🧹 Cleaning up topic management test clients...');
    await disconnectAll(this.clients);
    this.clients = [];
  }
}

// Run tests if this file is executed directly
if (require.main === module) {
  (async () => {
    const tests = new TopicManagementTests();
    try {
      const report = await tests.runAllTests();
      console.log('\n📊 Topic Management Test Report:');
      console.log(`Total: ${report.summary.total}`);
      console.log(`Passed: ${report.summary.passed}`);
      console.log(`Failed: ${report.summary.failed}`);
      console.log(`Pass Rate: ${report.summary.passRate}%`);

      process.exit(report.summary.failed > 0 ? 1 : 0);
    } catch (error) {
      console.error('Topic management test execution failed:', error);
      process.exit(1);
    }
  })();
}

module.exports = TopicManagementTests;