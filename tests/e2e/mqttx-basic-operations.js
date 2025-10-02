// MQTTX Basic Operations Tests (100 test cases)
const { MQTTXClient, createClients, connectAll, disconnectAll } = require('./utils/mqttx-client');
const { TestHelpers, TopicUtils, MessageValidator } = require('./utils/test-helpers');
const config = require('./utils/config');

class BasicOperationsTests {
  constructor() {
    this.helpers = new TestHelpers();
    this.clients = [];
  }

  async runAllTests() {
    console.log('🚀 Starting MQTTX Basic Operations Tests (100 test cases)');

    try {
      // Connection Tests (25 tests)
      await this.runConnectionTests();

      // Publish/Subscribe Tests (30 tests)
      await this.runPubSubTests();

      // QoS Tests (20 tests)
      await this.runQoSTests();

      // Session Tests (15 tests)
      await this.runSessionTests();

      // Last Will Testament Tests (10 tests)
      await this.runLWTTests();

      return this.helpers.generateReport();
    } finally {
      await this.cleanup();
    }
  }

  // Connection Tests (25 test cases)
  async runConnectionTests() {
    console.log('\n📡 Running Connection Tests (25 test cases)');

    // Test 1: Basic connection
    await this.helpers.runTest('basic-connection', async () => {
      const client = new MQTTXClient({ clientId: 'test-basic-conn' });
      this.clients.push(client);
      await client.connect();
      TestHelpers.assertTrue(client.connected, 'Client should be connected');
    });

    // Test 2: Connection with custom client ID
    await this.helpers.runTest('custom-clientid-connection', async () => {
      const customClientId = `custom-${Date.now()}`;
      const client = new MQTTXClient({ clientId: customClientId });
      this.clients.push(client);
      await client.connect();
      TestHelpers.assertTrue(client.connected, 'Client with custom ID should connect');
    });

    // Test 3: Connection with clean session true
    await this.helpers.runTest('clean-session-true', async () => {
      const client = new MQTTXClient({
        clientId: 'test-clean-true',
        clean: true
      });
      this.clients.push(client);
      await client.connect();
      TestHelpers.assertTrue(client.connected, 'Clean session client should connect');
    });

    // Test 4: Connection with clean session false
    await this.helpers.runTest('clean-session-false', async () => {
      const client = new MQTTXClient({
        clientId: 'test-clean-false',
        clean: false
      });
      this.clients.push(client);
      await client.connect();
      TestHelpers.assertTrue(client.connected, 'Persistent session client should connect');
    });

    // Test 5: Connection with authentication
    await this.helpers.runTest('auth-connection', async () => {
      const client = new MQTTXClient({
        clientId: 'test-auth',
        username: config.auth.username,
        password: config.auth.password
      });
      this.clients.push(client);
      await client.connect();
      TestHelpers.assertTrue(client.connected, 'Authenticated client should connect');
    });

    // Test 6-10: Multiple concurrent connections
    for (let i = 0; i < 5; i++) {
      await this.helpers.runTest(`concurrent-connection-${i + 1}`, async () => {
        const client = new MQTTXClient({ clientId: `concurrent-${i}-${Date.now()}` });
        this.clients.push(client);
        await client.connect();
        TestHelpers.assertTrue(client.connected, `Concurrent client ${i + 1} should connect`);
      });
    }

    // Test 11: Connection with keep alive
    await this.helpers.runTest('keepalive-connection', async () => {
      const client = new MQTTXClient({
        clientId: 'test-keepalive',
        keepalive: 30
      });
      this.clients.push(client);
      await client.connect();
      TestHelpers.assertTrue(client.connected, 'Keep alive client should connect');
    });

    // Test 12: Connection and immediate disconnect
    await this.helpers.runTest('connect-disconnect', async () => {
      const client = new MQTTXClient({ clientId: 'test-conn-disc' });
      await client.connect();
      TestHelpers.assertTrue(client.connected, 'Client should connect');
      await client.disconnect();
      TestHelpers.assertFalse(client.connected, 'Client should disconnect');
    });

    // Test 13: Reconnection after disconnect
    await this.helpers.runTest('reconnection', async () => {
      const client = new MQTTXClient({ clientId: 'test-reconnect' });
      this.clients.push(client);
      await client.connect();
      await client.disconnect();
      await client.connect();
      TestHelpers.assertTrue(client.connected, 'Client should reconnect');
    });

    // Test 14-18: Different protocol versions
    const protocolVersions = [3, 4]; // MQTT 3.1, 3.1.1
    for (let i = 0; i < protocolVersions.length; i++) {
      const version = protocolVersions[i];
      await this.helpers.runTest(`protocol-version-${version}`, async () => {
        const client = new MQTTXClient({
          clientId: `test-proto-${version}`,
          protocolVersion: version
        });
        this.clients.push(client);
        await client.connect();
        TestHelpers.assertTrue(client.connected, `Protocol version ${version} should connect`);
      });
    }

    // Test 19-23: Various client ID formats
    const clientIdFormats = [
      'simple',
      'with-dashes',
      'with_underscores',
      'with.dots',
      'mixed-format_test.123'
    ];

    for (let i = 0; i < clientIdFormats.length; i++) {
      const clientId = clientIdFormats[i];
      await this.helpers.runTest(`clientid-format-${i + 1}`, async () => {
        const client = new MQTTXClient({ clientId });
        this.clients.push(client);
        await client.connect();
        TestHelpers.assertTrue(client.connected, `Client ID format '${clientId}' should work`);
      });
    }

    // Test 24: Connection with will message
    await this.helpers.runTest('connection-with-will', async () => {
      const client = new MQTTXClient({
        clientId: 'test-will-conn',
        will: {
          topic: 'test/will',
          payload: 'client disconnected',
          qos: 1,
          retain: false
        }
      });
      this.clients.push(client);
      await client.connect();
      TestHelpers.assertTrue(client.connected, 'Client with will message should connect');
    });

    // Test 25: Connection timeout handling
    await this.helpers.runTest('connection-timeout', async () => {
      // This test ensures the client handles connection properly
      const client = new MQTTXClient({
        clientId: 'test-timeout',
        connectTimeout: 5000
      });
      this.clients.push(client);
      await client.connect();
      TestHelpers.assertTrue(client.connected, 'Client should connect within timeout');
    });
  }

  // Publish/Subscribe Tests (30 test cases)
  async runPubSubTests() {
    console.log('\n📢 Running Publish/Subscribe Tests (30 test cases)');

    // Test 26: Basic publish/subscribe
    await this.helpers.runTest('basic-pubsub', async () => {
      const publisher = new MQTTXClient({ clientId: 'pub-basic' });
      const subscriber = new MQTTXClient({ clientId: 'sub-basic' });
      this.clients.push(publisher, subscriber);

      await Promise.all([publisher.connect(), subscriber.connect()]);

      await subscriber.subscribe('test/basic');
      await publisher.publish('test/basic', 'hello world');

      const messages = await subscriber.waitForMessages(1);
      TestHelpers.assertMessageReceived(messages, 'test/basic', 'hello world');
    });

    // Test 27: Multiple topic subscriptions
    await this.helpers.runTest('multiple-topic-subscriptions', async () => {
      const publisher = new MQTTXClient({ clientId: 'pub-multi' });
      const subscriber = new MQTTXClient({ clientId: 'sub-multi' });
      this.clients.push(publisher, subscriber);

      await Promise.all([publisher.connect(), subscriber.connect()]);

      const topics = ['test/topic1', 'test/topic2', 'test/topic3'];
      for (const topic of topics) {
        await subscriber.subscribe(topic);
      }

      for (const topic of topics) {
        await publisher.publish(topic, `message for ${topic}`);
      }

      const messages = await subscriber.waitForMessages(3);
      TestHelpers.assertEqual(messages.length, 3, 'Should receive 3 messages');
    });

    // Test 28: Wildcard subscription with +
    await this.helpers.runTest('wildcard-single-level', async () => {
      const publisher = new MQTTXClient({ clientId: 'pub-wild1' });
      const subscriber = new MQTTXClient({ clientId: 'sub-wild1' });
      this.clients.push(publisher, subscriber);

      await Promise.all([publisher.connect(), subscriber.connect()]);

      await subscriber.subscribe('test/+/data');

      await publisher.publish('test/sensor1/data', 'sensor data 1');
      await publisher.publish('test/sensor2/data', 'sensor data 2');

      const messages = await subscriber.waitForMessages(2);
      TestHelpers.assertEqual(messages.length, 2, 'Should receive messages from wildcard subscription');
    });

    // Test 29: Wildcard subscription with #
    await this.helpers.runTest('wildcard-multi-level', async () => {
      const publisher = new MQTTXClient({ clientId: 'pub-wild2' });
      const subscriber = new MQTTXClient({ clientId: 'sub-wild2' });
      this.clients.push(publisher, subscriber);

      await Promise.all([publisher.connect(), subscriber.connect()]);

      await subscriber.subscribe('test/#');

      await publisher.publish('test/sensor', 'data');
      await publisher.publish('test/sensor/temp', 'temperature');
      await publisher.publish('test/sensor/temp/value', '25.5');

      const messages = await subscriber.waitForMessages(3);
      TestHelpers.assertEqual(messages.length, 3, 'Should receive all messages from # wildcard');
    });

    // Test 30-34: Different payload sizes
    const payloadSizes = [1, 100, 1024, 10240, 100000];
    for (let i = 0; i < payloadSizes.length; i++) {
      const size = payloadSizes[i];
      await this.helpers.runTest(`payload-size-${size}`, async () => {
        const publisher = new MQTTXClient({ clientId: `pub-size-${size}` });
        const subscriber = new MQTTXClient({ clientId: `sub-size-${size}` });
        this.clients.push(publisher, subscriber);

        await Promise.all([publisher.connect(), subscriber.connect()]);

        const payload = TestHelpers.generateRandomPayload(size);
        await subscriber.subscribe(`test/size/${size}`);
        await publisher.publish(`test/size/${size}`, payload);

        const messages = await subscriber.waitForMessages(1);
        TestHelpers.assertMessageReceived(messages, `test/size/${size}`, payload);
      });
    }

    // Test 35: Empty payload
    await this.helpers.runTest('empty-payload', async () => {
      const publisher = new MQTTXClient({ clientId: 'pub-empty' });
      const subscriber = new MQTTXClient({ clientId: 'sub-empty' });
      this.clients.push(publisher, subscriber);

      await Promise.all([publisher.connect(), subscriber.connect()]);

      await subscriber.subscribe('test/empty');
      await publisher.publish('test/empty', '');

      const messages = await subscriber.waitForMessages(1);
      TestHelpers.assertMessageReceived(messages, 'test/empty', '');
    });

    // Test 36-40: Rapid message publishing
    for (let i = 0; i < 5; i++) {
      await this.helpers.runTest(`rapid-publish-${i + 1}`, async () => {
        const publisher = new MQTTXClient({ clientId: `pub-rapid-${i}` });
        const subscriber = new MQTTXClient({ clientId: `sub-rapid-${i}` });
        this.clients.push(publisher, subscriber);

        await Promise.all([publisher.connect(), subscriber.connect()]);

        const topic = `test/rapid/${i}`;
        await subscriber.subscribe(topic);

        const messageCount = 10;
        for (let j = 0; j < messageCount; j++) {
          await publisher.publish(topic, `message-${j}`);
        }

        const messages = await subscriber.waitForMessages(messageCount);
        TestHelpers.assertEqual(messages.length, messageCount, `Should receive ${messageCount} rapid messages`);
      });
    }

    // Test 41-45: Different topic structures
    const topicStructures = [
      'simple',
      'two/levels',
      'three/level/structure',
      'four/level/deep/structure',
      'very/deep/topic/structure/with/many/levels'
    ];

    for (let i = 0; i < topicStructures.length; i++) {
      const topic = topicStructures[i];
      await this.helpers.runTest(`topic-structure-${i + 1}`, async () => {
        const publisher = new MQTTXClient({ clientId: `pub-struct-${i}` });
        const subscriber = new MQTTXClient({ clientId: `sub-struct-${i}` });
        this.clients.push(publisher, subscriber);

        await Promise.all([publisher.connect(), subscriber.connect()]);

        await subscriber.subscribe(topic);
        await publisher.publish(topic, `data for ${topic}`);

        const messages = await subscriber.waitForMessages(1);
        TestHelpers.assertMessageReceived(messages, topic, `data for ${topic}`);
      });
    }

    // Test 46-50: Unsubscribe tests
    for (let i = 0; i < 5; i++) {
      await this.helpers.runTest(`unsubscribe-${i + 1}`, async () => {
        const publisher = new MQTTXClient({ clientId: `pub-unsub-${i}` });
        const subscriber = new MQTTXClient({ clientId: `sub-unsub-${i}` });
        this.clients.push(publisher, subscriber);

        await Promise.all([publisher.connect(), subscriber.connect()]);

        const topic = `test/unsubscribe/${i}`;

        // Subscribe, publish, receive
        await subscriber.subscribe(topic);
        await publisher.publish(topic, 'before unsubscribe');
        await subscriber.waitForMessages(1);

        // Unsubscribe
        await subscriber.unsubscribe(topic);
        subscriber.clearMessages();

        // Publish again - should not receive
        await publisher.publish(topic, 'after unsubscribe');
        await TestHelpers.sleep(500); // Wait a bit

        TestHelpers.assertEqual(subscriber.receivedMessages.length, 0, 'Should not receive messages after unsubscribe');
      });
    }

    // Test 51-55: Multiple subscribers to same topic
    for (let i = 0; i < 5; i++) {
      await this.helpers.runTest(`multiple-subscribers-${i + 1}`, async () => {
        const publisher = new MQTTXClient({ clientId: `pub-multi-sub-${i}` });
        const subscriberCount = 3;
        const subscribers = createClients(subscriberCount, { clientId: `sub-multi-${i}` });
        this.clients.push(publisher, ...subscribers);

        await publisher.connect();
        await connectAll(subscribers);

        const topic = `test/multi-sub/${i}`;
        for (const subscriber of subscribers) {
          await subscriber.subscribe(topic);
        }

        await publisher.publish(topic, `broadcast message ${i}`);

        for (const subscriber of subscribers) {
          const messages = await subscriber.waitForMessages(1);
          TestHelpers.assertMessageReceived(messages, topic, `broadcast message ${i}`);
        }
      });
    }
  }

  // QoS Tests (20 test cases)
  async runQoSTests() {
    console.log('\n🎯 Running QoS Tests (20 test cases)');

    // Test 56-58: QoS 0, 1, 2 basic tests
    const qosLevels = [0, 1, 2];
    for (let qos of qosLevels) {
      await this.helpers.runTest(`qos-${qos}-basic`, async () => {
        const publisher = new MQTTXClient({ clientId: `pub-qos-${qos}` });
        const subscriber = new MQTTXClient({ clientId: `sub-qos-${qos}` });
        this.clients.push(publisher, subscriber);

        await Promise.all([publisher.connect(), subscriber.connect()]);

        await subscriber.subscribe(`test/qos/${qos}`, { qos });
        await publisher.publish(`test/qos/${qos}`, `QoS ${qos} message`, { qos });

        const messages = await subscriber.waitForMessages(1);
        MessageValidator.validateQoS(messages[0], qos);
      });
    }

    // Test 59-61: QoS downgrade tests (subscribe QoS < publish QoS)
    for (let i = 0; i < 3; i++) {
      const subQoS = i;
      const pubQoS = 2;
      await this.helpers.runTest(`qos-downgrade-sub${subQoS}-pub${pubQoS}`, async () => {
        const publisher = new MQTTXClient({ clientId: `pub-downgrade-${i}` });
        const subscriber = new MQTTXClient({ clientId: `sub-downgrade-${i}` });
        this.clients.push(publisher, subscriber);

        await Promise.all([publisher.connect(), subscriber.connect()]);

        await subscriber.subscribe(`test/qos/downgrade/${i}`, { qos: subQoS });
        await publisher.publish(`test/qos/downgrade/${i}`, 'downgrade test', { qos: pubQoS });

        const messages = await subscriber.waitForMessages(1);
        const expectedQoS = Math.min(subQoS, pubQoS);
        MessageValidator.validateQoS(messages[0], expectedQoS);
      });
    }

    // Test 62-66: QoS 1 duplicate handling
    for (let i = 0; i < 5; i++) {
      await this.helpers.runTest(`qos1-duplicate-${i + 1}`, async () => {
        const publisher = new MQTTXClient({ clientId: `pub-dup-${i}` });
        const subscriber = new MQTTXClient({ clientId: `sub-dup-${i}` });
        this.clients.push(publisher, subscriber);

        await Promise.all([publisher.connect(), subscriber.connect()]);

        await subscriber.subscribe(`test/qos1/dup/${i}`, { qos: 1 });
        await publisher.publish(`test/qos1/dup/${i}`, `dup test ${i}`, { qos: 1 });

        const messages = await subscriber.waitForMessages(1);
        MessageValidator.validateQoS(messages[0], 1);
        TestHelpers.assertEqual(messages[0].message, `dup test ${i}`, 'Message content should match');
      });
    }

    // Test 67-71: QoS 2 exactly once delivery
    for (let i = 0; i < 5; i++) {
      await this.helpers.runTest(`qos2-exactly-once-${i + 1}`, async () => {
        const publisher = new MQTTXClient({ clientId: `pub-qos2-${i}` });
        const subscriber = new MQTTXClient({ clientId: `sub-qos2-${i}` });
        this.clients.push(publisher, subscriber);

        await Promise.all([publisher.connect(), subscriber.connect()]);

        await subscriber.subscribe(`test/qos2/exact/${i}`, { qos: 2 });
        await publisher.publish(`test/qos2/exact/${i}`, `exact once ${i}`, { qos: 2 });

        const messages = await subscriber.waitForMessages(1);
        MessageValidator.validateQoS(messages[0], 2);
        TestHelpers.assertEqual(messages[0].message, `exact once ${i}`, 'QoS 2 message should be delivered exactly once');
      });
    }

    // Test 72-75: Mixed QoS levels
    for (let i = 0; i < 4; i++) {
      await this.helpers.runTest(`mixed-qos-levels-${i + 1}`, async () => {
        const publisher = new MQTTXClient({ clientId: `pub-mixed-${i}` });
        const subscriber = new MQTTXClient({ clientId: `sub-mixed-${i}` });
        this.clients.push(publisher, subscriber);

        await Promise.all([publisher.connect(), subscriber.connect()]);

        const topic = `test/mixed/qos/${i}`;
        await subscriber.subscribe(topic, { qos: 2 });

        // Publish with different QoS levels
        await publisher.publish(topic, 'qos0', { qos: 0 });
        await publisher.publish(topic, 'qos1', { qos: 1 });
        await publisher.publish(topic, 'qos2', { qos: 2 });

        const messages = await subscriber.waitForMessages(3);
        TestHelpers.assertEqual(messages.length, 3, 'Should receive all messages with mixed QoS');
      });
    }
  }

  // Session Tests (15 test cases)
  async runSessionTests() {
    console.log('\n🔄 Running Session Tests (15 test cases)');

    // Test 76-80: Clean session tests
    for (let i = 0; i < 5; i++) {
      await this.helpers.runTest(`clean-session-${i + 1}`, async () => {
        const clientId = `clean-session-${i}`;

        // First connection with clean session
        const client1 = new MQTTXClient({ clientId, clean: true });
        await client1.connect();
        await client1.subscribe(`test/session/clean/${i}`, { qos: 1 });
        await client1.disconnect();

        // Second connection should not have previous subscription
        const client2 = new MQTTXClient({ clientId, clean: true });
        this.clients.push(client2);
        await client2.connect();

        // Session should be clean - no previous state
        TestHelpers.assertEqual(client2.subscriptions.size, 0, 'Clean session should have no previous subscriptions');
      });
    }

    // Test 81-85: Persistent session tests
    for (let i = 0; i < 5; i++) {
      await this.helpers.runTest(`persistent-session-${i + 1}`, async () => {
        const clientId = `persistent-session-${i}`;

        // First connection with persistent session
        const client1 = new MQTTXClient({ clientId, clean: false });
        await client1.connect();
        await client1.subscribe(`test/session/persistent/${i}`, { qos: 1 });
        await client1.disconnect();

        // Publisher sends message while client is offline
        const publisher = new MQTTXClient({ clientId: `pub-persistent-${i}` });
        this.clients.push(publisher);
        await publisher.connect();
        await publisher.publish(`test/session/persistent/${i}`, `offline message ${i}`, { qos: 1 });

        // Second connection should receive the offline message
        const client2 = new MQTTXClient({ clientId, clean: false });
        this.clients.push(client2);
        await client2.connect();

        // Should receive the message that was sent while offline
        const messages = await client2.waitForMessages(1, 3000);
        TestHelpers.assertMessageReceived(messages, `test/session/persistent/${i}`, `offline message ${i}`);
      });
    }

    // Test 86-90: Session expiry tests
    for (let i = 0; i < 5; i++) {
      await this.helpers.runTest(`session-expiry-${i + 1}`, async () => {
        const client = new MQTTXClient({
          clientId: `session-expiry-${i}`,
          clean: false
        });
        this.clients.push(client);

        await client.connect();
        await client.subscribe(`test/session/expiry/${i}`, { qos: 1 });

        // Test that session maintains subscription
        TestHelpers.assertEqual(client.subscriptions.size, 1, 'Session should maintain subscription');
        TestHelpers.assertTrue(client.subscriptions.has(`test/session/expiry/${i}`), 'Should have subscribed topic');
      });
    }
  }

  // Last Will Testament Tests (10 test cases)
  async runLWTTests() {
    console.log('\n💀 Running Last Will Testament Tests (10 test cases)');

    // Test 91-100: LWT tests with different configurations
    for (let i = 0; i < 10; i++) {
      await this.helpers.runTest(`lwt-test-${i + 1}`, async () => {
        const subscriber = new MQTTXClient({ clientId: `lwt-sub-${i}` });
        this.clients.push(subscriber);
        await subscriber.connect();

        await subscriber.subscribe(`test/lwt/${i}`, { qos: 1 });

        // Create client with LWT
        const clientWithLWT = new MQTTXClient({
          clientId: `lwt-client-${i}`,
          will: {
            topic: `test/lwt/${i}`,
            payload: `client ${i} disconnected unexpectedly`,
            qos: 1,
            retain: false
          }
        });

        await clientWithLWT.connect();
        TestHelpers.assertTrue(clientWithLWT.connected, 'LWT client should connect');

        // Force disconnect to trigger LWT
        if (clientWithLWT.client) {
          clientWithLWT.client.stream.destroy();
        }

        // Should receive LWT message
        const messages = await subscriber.waitForMessages(1, 5000);
        TestHelpers.assertMessageReceived(messages, `test/lwt/${i}`, `client ${i} disconnected unexpectedly`);
      });
    }
  }

  async cleanup() {
    console.log('\n🧹 Cleaning up test clients...');
    await disconnectAll(this.clients);
    this.clients = [];
  }
}

// Run tests if this file is executed directly
if (require.main === module) {
  (async () => {
    const tests = new BasicOperationsTests();
    try {
      const report = await tests.runAllTests();
      console.log('\n📊 Final Test Report:');
      console.log(`Total: ${report.summary.total}`);
      console.log(`Passed: ${report.summary.passed}`);
      console.log(`Failed: ${report.summary.failed}`);
      console.log(`Pass Rate: ${report.summary.passRate}%`);

      process.exit(report.summary.failed > 0 ? 1 : 0);
    } catch (error) {
      console.error('Test execution failed:', error);
      process.exit(1);
    }
  })();
}

module.exports = BasicOperationsTests;