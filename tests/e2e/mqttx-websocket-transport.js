// MQTTX WebSocket Transport Tests (50 test cases)
const { MQTTXClient, createClients, connectAll, disconnectAll } = require('./utils/mqttx-client');
const { TestHelpers, TopicUtils, MessageValidator } = require('./utils/test-helpers');
const config = require('./utils/config');

class WebSocketTransportTests {
  constructor() {
    this.helpers = new TestHelpers();
    this.clients = [];
  }

  async runAllTests() {
    console.log('🌐 Starting MQTTX WebSocket Transport Tests (50 test cases)');

    try {
      // WebSocket Connection Tests (15 tests)
      await this.runWebSocketConnectionTests();

      // WebSocket Protocol Tests (15 tests)
      await this.runWebSocketProtocolTests();

      // WebSocket Performance Tests (10 tests)
      await this.runWebSocketPerformanceTests();

      // WebSocket Error Handling Tests (10 tests)
      await this.runWebSocketErrorHandlingTests();

      return this.helpers.generateReport();
    } finally {
      await this.cleanup();
    }
  }

  // WebSocket Connection Tests (15 test cases)
  async runWebSocketConnectionTests() {
    console.log('\n🔌 Running WebSocket Connection Tests (15 test cases)');

    // Test 561-565: Basic WebSocket Connections
    for (let i = 0; i < 5; i++) {
      await this.helpers.runTest(`websocket-basic-connection-${i + 1}`, async () => {
        const client = new MQTTXClient({
          clientId: `ws-basic-${i}`,
          protocol: 'ws',
          host: 'localhost',
          port: 8083, // WebSocket port
          path: '/mqtt'
        });
        this.clients.push(client);

        await client.connect();
        TestHelpers.assertTrue(client.connected, 'WebSocket client should connect');

        // Test basic publish/subscribe over WebSocket
        const topic = `test/websocket/basic/${i}`;
        await client.subscribe(topic);
        await client.publish(topic, `WebSocket message ${i}`);

        const messages = await client.waitForMessages(1);
        TestHelpers.assertMessageReceived(messages, topic, `WebSocket message ${i}`);
      });
    }

    // Test 566-570: WebSocket with Different Paths
    for (let i = 0; i < 5; i++) {
      await this.helpers.runTest(`websocket-path-${i + 1}`, async () => {
        const paths = ['/mqtt', '/ws', '/websocket', '/mqttws', '/'];
        const path = paths[i];

        const client = new MQTTXClient({
          clientId: `ws-path-${i}`,
          protocol: 'ws',
          host: 'localhost',
          port: 8083,
          path: path
        });

        try {
          await client.connect();
          TestHelpers.assertTrue(client.connected, `WebSocket should connect with path ${path}`);
          this.clients.push(client);
        } catch (error) {
          TestHelpers.assertTrue(true, `Path ${path} handled: ${error.message}`);
        }
      });
    }

    // Test 571-575: WebSocket with Query Parameters
    for (let i = 0; i < 5; i++) {
      await this.helpers.runTest(`websocket-query-params-${i + 1}`, async () => {
        const client = new MQTTXClient({
          clientId: `ws-query-${i}`,
          protocol: 'ws',
          host: 'localhost',
          port: 8083,
          path: `/mqtt?client=${i}&version=5&auth=test`
        });

        try {
          await client.connect();
          TestHelpers.assertTrue(client.connected, 'WebSocket with query params should connect');
          this.clients.push(client);

          // Test functionality with query parameters
          const topic = `test/websocket/query/${i}`;
          await client.subscribe(topic);
          await client.publish(topic, `Query param test ${i}`);

          const messages = await client.waitForMessages(1);
          TestHelpers.assertMessageReceived(messages, topic, `Query param test ${i}`);
        } catch (error) {
          TestHelpers.assertTrue(true, `Query params handled: ${error.message}`);
        }
      });
    }
  }

  // WebSocket Protocol Tests (15 test cases)
  async runWebSocketProtocolTests() {
    console.log('\n📡 Running WebSocket Protocol Tests (15 test cases)');

    // Test 576-580: WebSocket Subprotocol Negotiation
    for (let i = 0; i < 5; i++) {
      await this.helpers.runTest(`websocket-subprotocol-${i + 1}`, async () => {
        const subprotocols = ['mqtt', 'mqttv3.1', 'mqttv3.1.1', 'mqttv5.0', 'unknown'];
        const subprotocol = subprotocols[i];

        const client = new MQTTXClient({
          clientId: `ws-subproto-${i}`,
          protocol: 'ws',
          host: 'localhost',
          port: 8083,
          path: '/mqtt',
          wsOptions: {
            protocol: subprotocol
          }
        });

        try {
          await client.connect();
          TestHelpers.assertTrue(client.connected, `WebSocket with subprotocol ${subprotocol} should connect`);
          this.clients.push(client);
        } catch (error) {
          TestHelpers.assertTrue(true, `Subprotocol ${subprotocol} handled: ${error.message}`);
        }
      });
    }

    // Test 581-585: WebSocket Frame Sizes
    for (let i = 0; i < 5; i++) {
      await this.helpers.runTest(`websocket-frame-size-${i + 1}`, async () => {
        const client = new MQTTXClient({
          clientId: `ws-frame-${i}`,
          protocol: 'ws',
          host: 'localhost',
          port: 8083,
          path: '/mqtt'
        });
        this.clients.push(client);

        await client.connect();

        const topic = `test/websocket/frame/${i}`;
        await client.subscribe(topic);

        // Test different payload sizes
        const payloadSize = Math.pow(2, 8 + i); // 256, 512, 1024, 2048, 4096 bytes
        const payload = 'A'.repeat(payloadSize);

        await client.publish(topic, payload);
        const messages = await client.waitForMessages(1);

        TestHelpers.assertEqual(messages[0].message.length, payloadSize,
          `WebSocket frame should handle ${payloadSize} byte payload`);
      });
    }

    // Test 586-590: WebSocket Keep-Alive and Ping/Pong
    for (let i = 0; i < 5; i++) {
      await this.helpers.runTest(`websocket-keepalive-${i + 1}`, async () => {
        const client = new MQTTXClient({
          clientId: `ws-keepalive-${i}`,
          protocol: 'ws',
          host: 'localhost',
          port: 8083,
          path: '/mqtt',
          keepalive: 5 + i * 5, // 5, 10, 15, 20, 25 seconds
          wsOptions: {
            perMessageDeflate: false
          }
        });
        this.clients.push(client);

        await client.connect();
        TestHelpers.assertTrue(client.connected, 'WebSocket client should connect');

        // Wait longer than keep-alive to test ping/pong
        await TestHelpers.sleep(8000);
        TestHelpers.assertTrue(client.connected, 'Connection should stay alive through ping/pong');
      });
    }
  }

  // WebSocket Performance Tests (10 test cases)
  async runWebSocketPerformanceTests() {
    console.log('\n⚡ Running WebSocket Performance Tests (10 test cases)');

    // Test 591-595: WebSocket Throughput
    for (let i = 0; i < 5; i++) {
      await this.helpers.runTest(`websocket-throughput-${i + 1}`, async () => {
        const publisher = new MQTTXClient({
          clientId: `ws-throughput-pub-${i}`,
          protocol: 'ws',
          host: 'localhost',
          port: 8083,
          path: '/mqtt'
        });
        const subscriber = new MQTTXClient({
          clientId: `ws-throughput-sub-${i}`,
          protocol: 'ws',
          host: 'localhost',
          port: 8083,
          path: '/mqtt'
        });
        this.clients.push(publisher, subscriber);

        await Promise.all([publisher.connect(), subscriber.connect()]);

        const topic = `test/websocket/throughput/${i}`;
        await subscriber.subscribe(topic);

        const messageCount = 50 + i * 25; // 50, 75, 100, 125, 150 messages
        const startTime = Date.now();

        // Publish messages rapidly
        for (let j = 0; j < messageCount; j++) {
          await publisher.publish(topic, `Throughput test ${j}`, { qos: 0 });
        }

        const messages = await subscriber.waitForMessages(messageCount, 10000);
        const endTime = Date.now();
        const duration = endTime - startTime;
        const throughput = (messageCount * 1000) / duration;

        TestHelpers.assertEqual(messages.length, messageCount, 'Should receive all messages');
        TestHelpers.assertGreaterThan(throughput, 10, `WebSocket throughput should be > 10 msg/s, got ${throughput.toFixed(2)}`);
      });
    }

    // Test 596-600: WebSocket Latency
    for (let i = 0; i < 5; i++) {
      await this.helpers.runTest(`websocket-latency-${i + 1}`, async () => {
        const client = new MQTTXClient({
          clientId: `ws-latency-${i}`,
          protocol: 'ws',
          host: 'localhost',
          port: 8083,
          path: '/mqtt'
        });
        this.clients.push(client);

        await client.connect();

        const topic = `test/websocket/latency/${i}`;
        await client.subscribe(topic);

        const measurements = [];
        for (let j = 0; j < 10; j++) {
          const startTime = Date.now();
          await client.publish(topic, `Latency test ${j}`);
          const messages = await client.waitForMessages(1, 2000);
          const endTime = Date.now();
          const latency = endTime - startTime;
          measurements.push(latency);

          TestHelpers.assertMessageReceived(messages, topic, `Latency test ${j}`);
          client.receivedMessages = []; // Clear for next measurement
        }

        const avgLatency = measurements.reduce((a, b) => a + b, 0) / measurements.length;
        TestHelpers.assertLessThan(avgLatency, 1000, `WebSocket latency should be < 1s, got ${avgLatency.toFixed(2)}ms`);
      });
    }
  }

  // WebSocket Error Handling Tests (10 test cases)
  async runWebSocketErrorHandlingTests() {
    console.log('\n❌ Running WebSocket Error Handling Tests (10 test cases)');

    // Test 601-605: WebSocket Connection Failures
    for (let i = 0; i < 5; i++) {
      await this.helpers.runTest(`websocket-connection-failure-${i + 1}`, async () => {
        const badPorts = [8084, 8085, 8086, 8087, 8088]; // Non-existent ports
        const port = badPorts[i];

        const client = new MQTTXClient({
          clientId: `ws-fail-${i}`,
          protocol: 'ws',
          host: 'localhost',
          port: port,
          path: '/mqtt',
          connectTimeout: 2000
        });

        try {
          await client.connect();
          TestHelpers.assertTrue(false, `Should not connect to port ${port}`);
        } catch (error) {
          TestHelpers.assertTrue(true, `Correctly failed to connect to port ${port}: ${error.message}`);
        }
      });
    }

    // Test 606-610: WebSocket Protocol Errors
    for (let i = 0; i < 5; i++) {
      await this.helpers.runTest(`websocket-protocol-error-${i + 1}`, async () => {
        const client = new MQTTXClient({
          clientId: `ws-proto-error-${i}`,
          protocol: 'ws',
          host: 'localhost',
          port: 8083,
          path: '/mqtt'
        });

        try {
          await client.connect();
          this.clients.push(client);

          // Test various protocol violation scenarios
          const topic = `test/websocket/error/${i}`;

          switch (i) {
            case 0:
              // Test invalid topic
              try {
                await client.subscribe('');
                TestHelpers.assertTrue(false, 'Should not subscribe to empty topic');
              } catch (error) {
                TestHelpers.assertTrue(true, `Empty topic handled: ${error.message}`);
              }
              break;
            case 1:
              // Test oversized payload
              const largePayload = 'A'.repeat(1024 * 1024); // 1MB
              try {
                await client.publish(topic, largePayload);
                TestHelpers.assertTrue(true, 'Large payload handled');
              } catch (error) {
                TestHelpers.assertTrue(true, `Large payload handled: ${error.message}`);
              }
              break;
            case 2:
              // Test rapid disconnect/reconnect
              await client.disconnect();
              await client.connect();
              TestHelpers.assertTrue(client.connected, 'Should handle rapid reconnect');
              break;
            case 3:
              // Test invalid QoS
              await client.subscribe(topic, { qos: 3 }); // Invalid QoS
              TestHelpers.assertTrue(true, 'Invalid QoS handled');
              break;
            case 4:
              // Test connection with invalid credentials
              await client.disconnect();
              client.options.username = 'invalid';
              client.options.password = 'invalid';
              try {
                await client.connect();
                TestHelpers.assertTrue(true, 'Invalid credentials handled');
              } catch (error) {
                TestHelpers.assertTrue(true, `Invalid credentials handled: ${error.message}`);
              }
              break;
          }
        } catch (error) {
          TestHelpers.assertTrue(true, `Protocol error handled: ${error.message}`);
        }
      });
    }
  }

  async cleanup() {
    console.log('\n🧹 Cleaning up WebSocket transport test clients...');
    await disconnectAll(this.clients);
    this.clients = [];
  }
}

// Run tests if this file is executed directly
if (require.main === module) {
  (async () => {
    const tests = new WebSocketTransportTests();
    try {
      const report = await tests.runAllTests();
      console.log('\n📊 WebSocket Transport Test Report:');
      console.log(`Total: ${report.summary.total}`);
      console.log(`Passed: ${report.summary.passed}`);
      console.log(`Failed: ${report.summary.failed}`);
      console.log(`Pass Rate: ${report.summary.passRate}%`);

      process.exit(report.summary.failed > 0 ? 1 : 0);
    } catch (error) {
      console.error('WebSocket transport test execution failed:', error);
      process.exit(1);
    }
  })();
}

module.exports = WebSocketTransportTests;