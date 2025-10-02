// Test Configuration for MQTTX E2E Tests
const config = {
  broker: {
    url: 'mqtt://localhost:1883',
    host: 'localhost',
    port: 1883,
    protocolVersion: 4, // MQTT 3.1.1
    keepalive: 60,
    connectTimeout: 30 * 1000,
    reconnectPeriod: 1000,
    clean: true,
    username: 'test',
    password: 'test'
  },

  mqtt5: {
    url: 'mqtt://localhost:1883',
    protocolVersion: 5,
    properties: {
      sessionExpiryInterval: 300,
      receiveMaximum: 65535,
      maximumPacketSize: 268435455,
      topicAliasMaximum: 65535
    }
  },

  ssl: {
    url: 'mqtts://localhost:8883',
    rejectUnauthorized: false
  },

  auth: {
    username: 'test',
    password: 'test'
  },

  test: {
    timeout: 10000,
    maxClients: 1000,
    messageCount: 10000,
    payloadSizes: [10, 100, 1024, 10240, 102400], // bytes
    topics: {
      base: 'test/mqttx',
      levels: ['level1', 'level2', 'level3', 'level4'],
      wildcards: ['test/+/data', 'test/#', '+/+/data', '#']
    }
  },

  performance: {
    targetLatency: 100, // ms
    targetThroughput: 10000, // messages/second
    maxMemoryUsage: 500 * 1024 * 1024, // 500MB
    maxCpuUsage: 80 // percent
  }
};

module.exports = config;