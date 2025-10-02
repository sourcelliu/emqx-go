// MQTTX Concurrency & Performance Tests (60 test cases)
const { MQTTXClient, createClients, connectAll, disconnectAll } = require('./utils/mqttx-client');
const { TestHelpers, TopicUtils, MessageValidator } = require('./utils/test-helpers');
const config = require('./utils/config');

class ConcurrencyPerformanceTests {
  constructor() {
    this.helpers = new TestHelpers();
    this.clients = [];
  }

  async runAllTests() {
    console.log('🚀 Starting MQTTX Concurrency & Performance Tests (60 test cases)');

    try {
      // Concurrent Connection Tests (15 tests)
      await this.runConcurrentConnectionTests();

      // High Throughput Tests (15 tests)
      await this.runHighThroughputTests();

      // Load Testing (15 tests)
      await this.runLoadTests();

      // Resource Usage Tests (15 tests)
      await this.runResourceUsageTests();

      return this.helpers.generateReport();
    } finally {
      await this.cleanup();
    }
  }

  // Concurrent Connection Tests (15 test cases)
  async runConcurrentConnectionTests() {
    console.log('\n🔗 Running Concurrent Connection Tests (15 test cases)');

    // Test 256-260: Simultaneous connections
    for (let i = 0; i < 5; i++) {
      await this.helpers.runTest(`simultaneous-connections-${i + 1}`, async () => {
        const connectionCount = 10 + i * 5; // 10, 15, 20, 25, 30 connections
        const clients = createClients(connectionCount, { clientId: `simul-conn-${i}` });
        this.clients.push(...clients);

        const startTime = Date.now();
        await connectAll(clients);
        const connectionTime = Date.now() - startTime;

        // Verify all clients are connected
        for (const client of clients) {
          TestHelpers.assertTrue(client.connected, 'All clients should be connected');
        }

        console.log(`Connected ${connectionCount} clients in ${connectionTime}ms`);
        TestHelpers.assertLessThan(connectionTime, 10000,
          `${connectionCount} connections should complete within 10 seconds`);
      });
    }

    // Test 261-265: Connection burst tests
    for (let i = 0; i < 5; i++) {
      await this.helpers.runTest(`connection-burst-${i + 1}`, async () => {
        const burstSize = 20 + i * 10; // 20, 30, 40, 50, 60 connections
        const clients = [];

        // Create connections in rapid succession
        const connectionPromises = [];
        for (let j = 0; j < burstSize; j++) {
          const client = new MQTTXClient({ clientId: `burst-${i}-${j}` });
          clients.push(client);
          connectionPromises.push(client.connect());
        }

        this.clients.push(...clients);

        const { latencyMs } = await TestHelpers.measureLatency(async () => {
          await Promise.all(connectionPromises);
        });

        console.log(`Connection burst of ${burstSize} clients completed in ${latencyMs.toFixed(2)}ms`);
        TestHelpers.assertLessThan(latencyMs, 15000,
          `Connection burst should complete within 15 seconds`);
      });
    }

    // Test 266-270: Connection stability under load
    for (let i = 0; i < 5; i++) {
      await this.helpers.runTest(`connection-stability-${i + 1}`, async () => {
        const clientCount = 15 + i * 5;
        const clients = createClients(clientCount, {
          clientId: `stability-${i}`,
          keepalive: 30
        });
        this.clients.push(...clients);

        await connectAll(clients);

        // Keep connections alive for a period
        await TestHelpers.sleep(2000 + i * 1000);

        // Verify all connections are still stable
        let connectedCount = 0;
        for (const client of clients) {
          if (client.connected) {
            connectedCount++;
          }
        }

        const stabilityRate = (connectedCount / clientCount) * 100;
        TestHelpers.assertGreaterThan(stabilityRate, 95,
          'At least 95% of connections should remain stable');
      });
    }
  }

  // High Throughput Tests (15 test cases)
  async runHighThroughputTests() {
    console.log('\n⚡ Running High Throughput Tests (15 test cases)');

    // Test 271-275: Message publishing throughput
    for (let i = 0; i < 5; i++) {
      await this.helpers.runTest(`publish-throughput-${i + 1}`, async () => {
        const publisher = new MQTTXClient({ clientId: `throughput-pub-${i}` });
        const subscriber = new MQTTXClient({ clientId: `throughput-sub-${i}` });
        this.clients.push(publisher, subscriber);

        await Promise.all([publisher.connect(), subscriber.connect()]);

        const topic = `test/throughput/${i}`;
        await subscriber.subscribe(topic);

        const messageCount = 1000 + i * 500; // 1000, 1500, 2000, 2500, 3000 messages
        const payload = TestHelpers.generateRandomPayload(100);

        const { throughput } = await TestHelpers.measureThroughput(async (index) => {
          await publisher.publish(topic, `${payload}-${index}`, { qos: 0 });
        }, messageCount);

        console.log(`Publish throughput: ${throughput.toFixed(2)} messages/second`);

        // Wait for messages to be received
        const messages = await subscriber.waitForMessages(messageCount, 30000);
        TestHelpers.assertEqual(messages.length, messageCount,
          `Should receive all ${messageCount} published messages`);

        TestHelpers.assertGreaterThan(throughput, 100,
          'Publish throughput should exceed 100 messages/second');
      });
    }

    // Test 276-280: Subscription throughput
    for (let i = 0; i < 5; i++) {
      await this.helpers.runTest(`subscription-throughput-${i + 1}`, async () => {
        const publisher = new MQTTXClient({ clientId: `sub-throughput-pub-${i}` });
        const subscriberCount = 5 + i * 2; // 5, 7, 9, 11, 13 subscribers
        const subscribers = createClients(subscriberCount, { clientId: `sub-throughput-sub-${i}` });

        this.clients.push(publisher, ...subscribers);

        await publisher.connect();
        await connectAll(subscribers);

        const topic = `test/sub-throughput/${i}`;

        // All subscribers subscribe to the same topic
        for (const subscriber of subscribers) {
          await subscriber.subscribe(topic);
        }

        const messageCount = 500;
        const startTime = Date.now();

        // Publish messages rapidly
        for (let j = 0; j < messageCount; j++) {
          await publisher.publish(topic, `broadcast-${j}`, { qos: 0 });
        }

        // Each subscriber should receive all messages
        for (const subscriber of subscribers) {
          const messages = await subscriber.waitForMessages(messageCount, 20000);
          TestHelpers.assertEqual(messages.length, messageCount,
            `Each subscriber should receive all ${messageCount} messages`);
        }

        const totalTime = Date.now() - startTime;
        const totalMessages = messageCount * subscriberCount;
        const throughput = (totalMessages / totalTime) * 1000;

        console.log(`Subscription throughput: ${throughput.toFixed(2)} messages/second (${subscriberCount} subscribers)`);
      });
    }

    // Test 281-285: Mixed QoS throughput
    for (let i = 0; i < 5; i++) {
      await this.helpers.runTest(`mixed-qos-throughput-${i + 1}`, async () => {
        const publisher = new MQTTXClient({ clientId: `mixed-qos-pub-${i}` });
        const subscriber = new MQTTXClient({ clientId: `mixed-qos-sub-${i}` });
        this.clients.push(publisher, subscriber);

        await Promise.all([publisher.connect(), subscriber.connect()]);

        const topic = `test/mixed-qos-throughput/${i}`;
        await subscriber.subscribe(topic, { qos: 2 });

        const messageCount = 300;
        const qosLevels = [0, 1, 2];

        const { throughput } = await TestHelpers.measureThroughput(async (index) => {
          const qos = qosLevels[index % 3];
          await publisher.publish(topic, `mixed-qos-${index}`, { qos });
        }, messageCount);

        const messages = await subscriber.waitForMessages(messageCount, 25000);
        TestHelpers.assertEqual(messages.length, messageCount,
          'Should receive all messages with mixed QoS levels');

        console.log(`Mixed QoS throughput: ${throughput.toFixed(2)} messages/second`);
        TestHelpers.assertGreaterThan(throughput, 50,
          'Mixed QoS throughput should exceed 50 messages/second');
      });
    }
  }

  // Load Testing (15 test cases)
  async runLoadTests() {
    console.log('\n📊 Running Load Tests (15 test cases)');

    // Test 286-290: Concurrent publishers
    for (let i = 0; i < 5; i++) {
      await this.helpers.runTest(`concurrent-publishers-${i + 1}`, async () => {
        const publisherCount = 5 + i * 2; // 5, 7, 9, 11, 13 publishers
        const subscriber = new MQTTXClient({ clientId: `load-sub-${i}` });
        const publishers = createClients(publisherCount, { clientId: `load-pub-${i}` });

        this.clients.push(subscriber, ...publishers);

        await subscriber.connect();
        await connectAll(publishers);

        const topic = `test/load/concurrent-pub/${i}`;
        await subscriber.subscribe(topic);

        const messagesPerPublisher = 100;
        const totalMessages = publisherCount * messagesPerPublisher;

        // All publishers publish simultaneously
        const publishPromises = publishers.map((publisher, pubIndex) => {
          return (async () => {
            for (let msgIndex = 0; msgIndex < messagesPerPublisher; msgIndex++) {
              await publisher.publish(topic, `pub-${pubIndex}-msg-${msgIndex}`, { qos: 0 });
            }
          })();
        });

        const startTime = Date.now();
        await Promise.all(publishPromises);
        const publishTime = Date.now() - startTime;

        const messages = await subscriber.waitForMessages(totalMessages, 30000);
        TestHelpers.assertEqual(messages.length, totalMessages,
          `Should receive all ${totalMessages} messages from concurrent publishers`);

        const throughput = (totalMessages / publishTime) * 1000;
        console.log(`Concurrent publishers throughput: ${throughput.toFixed(2)} messages/second (${publisherCount} publishers)`);
      });
    }

    // Test 291-295: Concurrent subscribers
    for (let i = 0; i < 5; i++) {
      await this.helpers.runTest(`concurrent-subscribers-${i + 1}`, async () => {
        const subscriberCount = 8 + i * 3; // 8, 11, 14, 17, 20 subscribers
        const publisher = new MQTTXClient({ clientId: `load-pub-${i}` });
        const subscribers = createClients(subscriberCount, { clientId: `load-sub-${i}` });

        this.clients.push(publisher, ...subscribers);

        await publisher.connect();
        await connectAll(subscribers);

        const topic = `test/load/concurrent-sub/${i}`;

        // All subscribers subscribe to the same topic
        for (const subscriber of subscribers) {
          await subscriber.subscribe(topic);
        }

        const messageCount = 200;

        // Publish messages
        for (let j = 0; j < messageCount; j++) {
          await publisher.publish(topic, `load-test-${j}`, { qos: 0 });
        }

        // Verify all subscribers receive all messages
        let totalReceived = 0;
        for (const subscriber of subscribers) {
          const messages = await subscriber.waitForMessages(messageCount, 20000);
          totalReceived += messages.length;
        }

        const expectedTotal = messageCount * subscriberCount;
        TestHelpers.assertEqual(totalReceived, expectedTotal,
          `All ${subscriberCount} subscribers should receive all ${messageCount} messages`);
      });
    }

    // Test 296-300: Mixed load scenarios
    for (let i = 0; i < 5; i++) {
      await this.helpers.runTest(`mixed-load-scenario-${i + 1}`, async () => {
        const clientCount = 10 + i * 5; // 10, 15, 20, 25, 30 clients
        const publisherRatio = 0.3; // 30% publishers, 70% subscribers

        const publisherCount = Math.ceil(clientCount * publisherRatio);
        const subscriberCount = clientCount - publisherCount;

        const publishers = createClients(publisherCount, { clientId: `mixed-pub-${i}` });
        const subscribers = createClients(subscriberCount, { clientId: `mixed-sub-${i}` });

        this.clients.push(...publishers, ...subscribers);

        await connectAll([...publishers, ...subscribers]);

        const topics = [
          `test/mixed/topic1/${i}`,
          `test/mixed/topic2/${i}`,
          `test/mixed/topic3/${i}`
        ];

        // Subscribers subscribe to different topics
        for (let j = 0; j < subscribers.length; j++) {
          const topic = topics[j % topics.length];
          await subscribers[j].subscribe(topic);
        }

        const messagesPerTopic = 100;

        // Publishers publish to different topics
        const publishPromises = publishers.map((publisher, pubIndex) => {
          return (async () => {
            for (const topic of topics) {
              for (let msgIndex = 0; msgIndex < messagesPerTopic; msgIndex++) {
                await publisher.publish(topic, `mixed-${pubIndex}-${msgIndex}`, { qos: 0 });
              }
            }
          })();
        });

        await Promise.all(publishPromises);

        // Verify message delivery
        let totalReceived = 0;
        for (const subscriber of subscribers) {
          try {
            const messages = await subscriber.waitForMessages(1, 5000);
            totalReceived += messages.length;
          } catch (error) {
            // Some subscribers might not receive messages depending on topic distribution
          }
        }

        TestHelpers.assertGreaterThan(totalReceived, 0,
          'Mixed load scenario should result in message delivery');

        console.log(`Mixed load: ${publisherCount} publishers, ${subscriberCount} subscribers, ${totalReceived} messages delivered`);
      });
    }
  }

  // Resource Usage Tests (15 test cases)
  async runResourceUsageTests() {
    console.log('\n💾 Running Resource Usage Tests (15 test cases)');

    // Test 301-305: Memory usage monitoring
    for (let i = 0; i < 5; i++) {
      await this.helpers.runTest(`memory-usage-${i + 1}`, async () => {
        const initialMemory = process.memoryUsage();

        const clientCount = 20 + i * 10; // 20, 30, 40, 50, 60 clients
        const clients = createClients(clientCount, { clientId: `memory-test-${i}` });
        this.clients.push(...clients);

        await connectAll(clients);

        // Perform some operations
        const topic = `test/memory/${i}`;
        for (const client of clients.slice(0, Math.ceil(clientCount / 2))) {
          await client.subscribe(topic);
        }

        for (const client of clients.slice(Math.ceil(clientCount / 2))) {
          await client.publish(topic, `memory test ${i}`, { qos: 0 });
        }

        const finalMemory = process.memoryUsage();
        const memoryIncrease = finalMemory.heapUsed - initialMemory.heapUsed;
        const memoryPerClient = memoryIncrease / clientCount;

        console.log(`Memory usage: ${memoryIncrease} bytes total, ${memoryPerClient.toFixed(2)} bytes per client`);

        // Memory usage should be reasonable (less than 10MB per client)
        TestHelpers.assertLessThan(memoryPerClient, 10 * 1024 * 1024,
          'Memory usage per client should be less than 10MB');
      });
    }

    // Test 306-310: Connection pool efficiency
    for (let i = 0; i < 5; i++) {
      await this.helpers.runTest(`connection-pool-efficiency-${i + 1}`, async () => {
        const poolSize = 15 + i * 5; // 15, 20, 25, 30, 35 connections
        const clients = createClients(poolSize, {
          clientId: `pool-${i}`,
          keepalive: 60
        });
        this.clients.push(...clients);

        // Measure connection time
        const { latencyMs } = await TestHelpers.measureLatency(async () => {
          await connectAll(clients);
        });

        // Test connection reuse efficiency
        const operationsPerClient = 10;
        const topic = `test/pool/${i}`;

        const operationTime = await TestHelpers.measureLatency(async () => {
          for (const client of clients) {
            await client.subscribe(topic);
            for (let j = 0; j < operationsPerClient; j++) {
              await client.publish(topic, `pool-test-${j}`, { qos: 0 });
            }
          }
        });

        console.log(`Pool efficiency: ${poolSize} connections in ${latencyMs.toFixed(2)}ms, operations in ${operationTime.latencyMs.toFixed(2)}ms`);

        const avgConnectionTime = latencyMs / poolSize;
        TestHelpers.assertLessThan(avgConnectionTime, 1000,
          'Average connection time should be less than 1 second');
      });
    }

    // Test 311-315: Scalability limits
    for (let i = 0; i < 5; i++) {
      await this.helpers.runTest(`scalability-limits-${i + 1}`, async () => {
        const targetConnections = 50 + i * 25; // 50, 75, 100, 125, 150 connections
        const clients = [];
        let successfulConnections = 0;

        try {
          // Gradually increase connections to find limits
          for (let j = 0; j < targetConnections; j++) {
            const client = new MQTTXClient({
              clientId: `scale-${i}-${j}`,
              connectTimeout: 5000
            });
            clients.push(client);

            try {
              await client.connect();
              successfulConnections++;
            } catch (error) {
              console.log(`Connection ${j + 1} failed: ${error.message}`);
              break;
            }
          }

          this.clients.push(...clients);

          const connectionRate = (successfulConnections / targetConnections) * 100;
          console.log(`Scalability: ${successfulConnections}/${targetConnections} connections (${connectionRate.toFixed(1)}%)`);

          TestHelpers.assertGreaterThan(successfulConnections, targetConnections * 0.8,
            'Should achieve at least 80% of target connections');

        } catch (error) {
          TestHelpers.assertTrue(false, `Scalability test failed: ${error.message}`);
        }
      });
    }
  }

  async cleanup() {
    console.log('\n🧹 Cleaning up concurrency & performance test clients...');
    await disconnectAll(this.clients);
    this.clients = [];
  }
}

// Run tests if this file is executed directly
if (require.main === module) {
  (async () => {
    const tests = new ConcurrencyPerformanceTests();
    try {
      const report = await tests.runAllTests();
      console.log('\n📊 Concurrency & Performance Test Report:');
      console.log(`Total: ${report.summary.total}`);
      console.log(`Passed: ${report.summary.passed}`);
      console.log(`Failed: ${report.summary.failed}`);
      console.log(`Pass Rate: ${report.summary.passRate}%`);

      process.exit(report.summary.failed > 0 ? 1 : 0);
    } catch (error) {
      console.error('Concurrency & performance test execution failed:', error);
      process.exit(1);
    }
  })();
}

module.exports = ConcurrencyPerformanceTests;