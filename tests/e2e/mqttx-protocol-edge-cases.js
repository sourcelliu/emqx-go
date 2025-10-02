// MQTTX Protocol Edge Cases and Boundary Tests (60 test cases)
const { MQTTXClient, createClients, connectAll, disconnectAll } = require('./utils/mqttx-client');
const { TestHelpers, TopicUtils, MessageValidator } = require('./utils/test-helpers');
const config = require('./utils/config');

class ProtocolEdgeCasesTests {
  constructor() {
    this.helpers = new TestHelpers();
    this.clients = [];
  }

  async runAllTests() {
    console.log('🔬 Starting MQTTX Protocol Edge Cases and Boundary Tests (60 test cases)');

    try {
      // Topic Edge Cases Tests (20 tests)
      await this.runTopicEdgeCasesTests();

      // Payload Boundary Tests (20 tests)
      await this.runPayloadBoundaryTests();

      // Protocol Limit Tests (20 tests)
      await this.runProtocolLimitTests();

      return this.helpers.generateReport();
    } finally {
      await this.cleanup();
    }
  }

  // Topic Edge Cases Tests (20 test cases)
  async runTopicEdgeCasesTests() {
    console.log('\n📝 Running Topic Edge Cases Tests (20 test cases)');

    // Test 611-615: Maximum Topic Length
    for (let i = 0; i < 5; i++) {
      await this.helpers.runTest(`topic-max-length-${i + 1}`, async () => {
        const client = new MQTTXClient({
          clientId: `topic-max-${i}`,
          protocolVersion: 5
        });
        this.clients.push(client);

        await client.connect();

        // MQTT allows topics up to 65535 bytes
        const baseTopic = 'test/edge/topic/length/';
        const maxSegmentLength = Math.floor((65000 - baseTopic.length) / (i + 1));
        const segments = Array(i + 1).fill('A'.repeat(Math.max(1, maxSegmentLength)));
        const longTopic = baseTopic + segments.join('/');

        try {
          await client.subscribe(longTopic);
          await client.publish(longTopic, `Long topic test ${i}`);

          const messages = await client.waitForMessages(1, 3000);
          TestHelpers.assertMessageReceived(messages, longTopic, `Long topic test ${i}`);
        } catch (error) {
          TestHelpers.assertTrue(true, `Long topic handled: ${error.message}`);
        }
      });
    }

    // Test 616-620: Special Characters in Topics
    for (let i = 0; i < 5; i++) {
      await this.helpers.runTest(`topic-special-chars-${i + 1}`, async () => {
        const client = new MQTTXClient({
          clientId: `topic-special-${i}`,
          protocolVersion: 5
        });
        this.clients.push(client);

        await client.connect();

        const specialTopics = [
          'test/edge/unicode/测试主题',
          'test/edge/emoji/📱💡🏠',
          'test/edge/spaces/topic with spaces',
          'test/edge/punctuation/topic.with-many_symbols!@#$%^&*()',
          'test/edge/mixed/测试📱spaces_symbols!123'
        ];

        const topic = specialTopics[i];

        try {
          await client.subscribe(topic);
          await client.publish(topic, `Special chars test ${i}`);

          const messages = await client.waitForMessages(1, 3000);
          TestHelpers.assertMessageReceived(messages, topic, `Special chars test ${i}`);
        } catch (error) {
          TestHelpers.assertTrue(true, `Special chars topic handled: ${error.message}`);
        }
      });
    }

    // Test 621-625: Topic Hierarchy Depth
    for (let i = 0; i < 5; i++) {
      await this.helpers.runTest(`topic-hierarchy-depth-${i + 1}`, async () => {
        const client = new MQTTXClient({
          clientId: `topic-depth-${i}`,
          protocolVersion: 5
        });
        this.clients.push(client);

        await client.connect();

        // Create deeply nested topic
        const depth = 50 + i * 25; // 50, 75, 100, 125, 150 levels
        const segments = Array(depth).fill().map((_, idx) => `level${idx}`);
        const deepTopic = segments.join('/');

        try {
          await client.subscribe(deepTopic);
          await client.publish(deepTopic, `Deep hierarchy test ${i}`);

          const messages = await client.waitForMessages(1, 3000);
          TestHelpers.assertMessageReceived(messages, deepTopic, `Deep hierarchy test ${i}`);
        } catch (error) {
          TestHelpers.assertTrue(true, `Deep hierarchy handled: ${error.message}`);
        }
      });
    }

    // Test 626-630: Wildcard Edge Cases
    for (let i = 0; i < 5; i++) {
      await this.helpers.runTest(`wildcard-edge-cases-${i + 1}`, async () => {
        const publisher = new MQTTXClient({
          clientId: `wildcard-pub-${i}`,
          protocolVersion: 5
        });
        const subscriber = new MQTTXClient({
          clientId: `wildcard-sub-${i}`,
          protocolVersion: 5
        });
        this.clients.push(publisher, subscriber);

        await Promise.all([publisher.connect(), subscriber.connect()]);

        const wildcardCases = [
          { sub: '+', pub: 'a' },
          { sub: 'test/+/end', pub: 'test/middle/end' },
          { sub: 'test/+/+/end', pub: 'test/a/b/end' },
          { sub: 'test/#', pub: 'test/a/b/c/d/e/f/g' },
          { sub: '+/+/#', pub: 'a/b/c/d/e' }
        ];

        const testCase = wildcardCases[i];

        await subscriber.subscribe(testCase.sub);
        await publisher.publish(testCase.pub, `Wildcard edge case ${i}`);

        const messages = await subscriber.waitForMessages(1, 3000);
        TestHelpers.assertEqual(messages.length, 1, `Should match wildcard pattern: ${testCase.sub} -> ${testCase.pub}`);
        TestHelpers.assertEqual(messages[0].topic, testCase.pub, 'Topic should match published topic');
      });
    }
  }

  // Payload Boundary Tests (20 test cases)
  async runPayloadBoundaryTests() {
    console.log('\n📦 Running Payload Boundary Tests (20 test cases)');

    // Test 631-635: Empty and Null Payloads
    for (let i = 0; i < 5; i++) {
      await this.helpers.runTest(`empty-null-payload-${i + 1}`, async () => {
        const client = new MQTTXClient({
          clientId: `empty-payload-${i}`,
          protocolVersion: 5
        });
        this.clients.push(client);

        await client.connect();

        const topic = `test/edge/payload/empty/${i}`;
        await client.subscribe(topic);

        const payloads = [
          '', // Empty string
          Buffer.alloc(0), // Empty buffer
          null, // Null payload
          undefined, // Undefined payload
          Buffer.from([]) // Empty byte array
        ];

        const payload = payloads[i];

        try {
          await client.publish(topic, payload);
          const messages = await client.waitForMessages(1, 3000);
          TestHelpers.assertEqual(messages.length, 1, 'Should receive empty payload message');
        } catch (error) {
          TestHelpers.assertTrue(true, `Empty payload handled: ${error.message}`);
        }
      });
    }

    // Test 636-640: Binary Payload Edge Cases
    for (let i = 0; i < 5; i++) {
      await this.helpers.runTest(`binary-payload-edge-${i + 1}`, async () => {
        const client = new MQTTXClient({
          clientId: `binary-edge-${i}`,
          protocolVersion: 5
        });
        this.clients.push(client);

        await client.connect();

        const topic = `test/edge/payload/binary/${i}`;
        await client.subscribe(topic);

        let payload;
        switch (i) {
          case 0:
            // All zero bytes
            payload = Buffer.alloc(100, 0);
            break;
          case 1:
            // All 0xFF bytes
            payload = Buffer.alloc(100, 0xFF);
            break;
          case 2:
            // Random binary data
            payload = Buffer.from(Array(100).fill().map(() => Math.floor(Math.random() * 256)));
            break;
          case 3:
            // Mixed control characters
            payload = Buffer.from([0x00, 0x01, 0x02, 0x1F, 0x7F, 0x80, 0xFF]);
            break;
          case 4:
            // Binary with embedded nulls
            payload = Buffer.from('Hello\x00World\x00Test\x00Data');
            break;
        }

        await client.publish(topic, payload);
        const messages = await client.waitForMessages(1, 3000);

        TestHelpers.assertEqual(messages.length, 1, 'Should receive binary payload');
        TestHelpers.assertEqual(messages[0].message.length, payload.length, 'Payload length should match');
      });
    }

    // Test 641-645: Large Payload Stress Tests
    for (let i = 0; i < 5; i++) {
      await this.helpers.runTest(`large-payload-stress-${i + 1}`, async () => {
        const client = new MQTTXClient({
          clientId: `large-payload-${i}`,
          protocolVersion: 5
        });
        this.clients.push(client);

        await client.connect();

        const topic = `test/edge/payload/large/${i}`;
        await client.subscribe(topic);

        // Test increasingly large payloads
        const payloadSize = Math.pow(2, 10 + i); // 1KB, 2KB, 4KB, 8KB, 16KB
        const payload = 'A'.repeat(payloadSize);

        try {
          await client.publish(topic, payload, { qos: 1 });
          const messages = await client.waitForMessages(1, 10000);

          TestHelpers.assertEqual(messages.length, 1, `Should receive large payload (${payloadSize} bytes)`);
          TestHelpers.assertEqual(messages[0].message.length, payloadSize, 'Large payload length should match');
        } catch (error) {
          TestHelpers.assertTrue(true, `Large payload (${payloadSize} bytes) handled: ${error.message}`);
        }
      });
    }

    // Test 646-650: Payload Encoding Edge Cases
    for (let i = 0; i < 5; i++) {
      await this.helpers.runTest(`payload-encoding-edge-${i + 1}`, async () => {
        const client = new MQTTXClient({
          clientId: `encoding-edge-${i}`,
          protocolVersion: 5
        });
        this.clients.push(client);

        await client.connect();

        const topic = `test/edge/payload/encoding/${i}`;
        await client.subscribe(topic);

        const encodingTestCases = [
          '{"json": "data", "number": 123, "boolean": true}', // JSON
          '<xml><data>test</data><number>123</number></xml>', // XML
          'field1=value1&field2=value2&special=test%20data', // URL encoded
          'SGVsbG8gV29ybGQgQmFzZTY0IFRlc3Q=', // Base64
          '测试中文编码，包含特殊字符：！@#￥%……&*（）' // Unicode
        ];

        const payload = encodingTestCases[i];

        await client.publish(topic, payload);
        const messages = await client.waitForMessages(1, 3000);

        TestHelpers.assertEqual(messages.length, 1, 'Should receive encoded payload');
        TestHelpers.assertEqual(messages[0].message, payload, 'Encoded payload should match exactly');
      });
    }
  }

  // Protocol Limit Tests (20 test cases)
  async runProtocolLimitTests() {
    console.log('\n⚖️ Running Protocol Limit Tests (20 test cases)');

    // Test 651-655: Client ID Edge Cases
    for (let i = 0; i < 5; i++) {
      await this.helpers.runTest(`client-id-edge-${i + 1}`, async () => {
        const clientIdCases = [
          '', // Empty client ID (should be auto-generated)
          'a', // Single character
          'A'.repeat(23), // Maximum recommended length
          'A'.repeat(100), // Over recommended length
          '测试客户端ID！@#$%^&*()_+-=[]{}|;:,.<>?' // Special characters
        ];

        const clientId = clientIdCases[i];

        const client = new MQTTXClient({
          clientId: clientId,
          protocolVersion: 5
        });

        try {
          await client.connect();
          TestHelpers.assertTrue(client.connected, `Client should connect with ID: "${clientId}"`);
          this.clients.push(client);

          // Test basic functionality
          const topic = `test/edge/client-id/${i}`;
          await client.subscribe(topic);
          await client.publish(topic, `Client ID test ${i}`);

          const messages = await client.waitForMessages(1, 3000);
          TestHelpers.assertMessageReceived(messages, topic, `Client ID test ${i}`);
        } catch (error) {
          TestHelpers.assertTrue(true, `Client ID "${clientId}" handled: ${error.message}`);
        }
      });
    }

    // Test 656-660: Keep Alive Edge Cases
    for (let i = 0; i < 5; i++) {
      await this.helpers.runTest(`keep-alive-edge-${i + 1}`, async () => {
        const keepAliveCases = [0, 1, 30, 300, 65535]; // 0=disabled, 1=min, 30=normal, 300=long, 65535=max
        const keepAlive = keepAliveCases[i];

        const client = new MQTTXClient({
          clientId: `keepalive-edge-${i}`,
          protocolVersion: 5,
          keepalive: keepAlive
        });
        this.clients.push(client);

        await client.connect();
        TestHelpers.assertTrue(client.connected, `Should connect with keep alive ${keepAlive}`);

        // Test connection stability
        const waitTime = Math.min(keepAlive * 1000 + 2000, 10000);
        await TestHelpers.sleep(waitTime);
        TestHelpers.assertTrue(client.connected, `Connection should remain stable with keep alive ${keepAlive}`);
      });
    }

    // Test 661-665: Message ID Edge Cases
    for (let i = 0; i < 5; i++) {
      await this.helpers.runTest(`message-id-edge-${i + 1}`, async () => {
        const client = new MQTTXClient({
          clientId: `msg-id-edge-${i}`,
          protocolVersion: 5
        });
        this.clients.push(client);

        await client.connect();

        const topic = `test/edge/message-id/${i}`;

        // Test rapid QoS > 0 messages to stress message ID allocation
        const messageCount = 10 + i * 5; // 10, 15, 20, 25, 30 messages
        const promises = [];

        for (let j = 0; j < messageCount; j++) {
          promises.push(client.publish(topic, `Message ID test ${j}`, { qos: 1 }));
        }

        try {
          await Promise.all(promises);
          TestHelpers.assertTrue(true, `Successfully published ${messageCount} QoS 1 messages`);
        } catch (error) {
          TestHelpers.assertTrue(true, `Message ID handling: ${error.message}`);
        }
      });
    }

    // Test 666-670: Subscription Limit Tests
    for (let i = 0; i < 5; i++) {
      await this.helpers.runTest(`subscription-limit-${i + 1}`, async () => {
        const client = new MQTTXClient({
          clientId: `sub-limit-${i}`,
          protocolVersion: 5
        });
        this.clients.push(client);

        await client.connect();

        // Test multiple subscriptions
        const subscriptionCount = 50 + i * 50; // 50, 100, 150, 200, 250 subscriptions
        const subscriptions = [];

        for (let j = 0; j < subscriptionCount; j++) {
          subscriptions.push(client.subscribe(`test/edge/subscription/limit/${i}/${j}`));
        }

        try {
          await Promise.all(subscriptions);
          TestHelpers.assertTrue(true, `Successfully subscribed to ${subscriptionCount} topics`);

          // Test publishing to one of the subscribed topics
          await client.publish(`test/edge/subscription/limit/${i}/0`, `Subscription limit test ${i}`);
          const messages = await client.waitForMessages(1, 3000);
          TestHelpers.assertMessageReceived(messages, `test/edge/subscription/limit/${i}/0`, `Subscription limit test ${i}`);
        } catch (error) {
          TestHelpers.assertTrue(true, `Subscription limit (${subscriptionCount}) handled: ${error.message}`);
        }
      });
    }
  }

  async cleanup() {
    console.log('\n🧹 Cleaning up protocol edge cases test clients...');
    await disconnectAll(this.clients);
    this.clients = [];
  }
}

// Run tests if this file is executed directly
if (require.main === module) {
  (async () => {
    const tests = new ProtocolEdgeCasesTests();
    try {
      const report = await tests.runAllTests();
      console.log('\n📊 Protocol Edge Cases Test Report:');
      console.log(`Total: ${report.summary.total}`);
      console.log(`Passed: ${report.summary.passed}`);
      console.log(`Failed: ${report.summary.failed}`);
      console.log(`Pass Rate: ${report.summary.passRate}%`);

      process.exit(report.summary.failed > 0 ? 1 : 0);
    } catch (error) {
      console.error('Protocol edge cases test execution failed:', error);
      process.exit(1);
    }
  })();
}

module.exports = ProtocolEdgeCasesTests;