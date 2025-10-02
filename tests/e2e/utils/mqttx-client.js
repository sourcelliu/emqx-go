// MQTTX Client Utilities for E2E Testing
const mqtt = require('mqtt');
const config = require('./config');

class MQTTXClient {
  constructor(options = {}) {
    this.options = { ...config.broker, ...options };
    this.client = null;
    this.connected = false;
    this.subscriptions = new Map();
    this.publishedMessages = [];
    this.receivedMessages = [];
    this.errors = [];
  }

  async connect() {
    return new Promise((resolve, reject) => {
      try {
        this.client = mqtt.connect(this.options.url, this.options);

        this.client.on('connect', () => {
          this.connected = true;
          console.log(`[${this.options.clientId || 'client'}] Connected to broker`);
          resolve();
        });

        this.client.on('error', (error) => {
          this.errors.push({ timestamp: Date.now(), error: error.message });
          console.error(`[${this.options.clientId || 'client'}] Connection error:`, error.message);
          if (!this.connected) {
            reject(error);
          }
        });

        this.client.on('message', (topic, message, packet) => {
          this.receivedMessages.push({
            timestamp: Date.now(),
            topic,
            message: message.toString(),
            qos: packet.qos,
            retain: packet.retain,
            dup: packet.dup,
            packet
          });
        });

        this.client.on('disconnect', () => {
          this.connected = false;
          console.log(`[${this.options.clientId || 'client'}] Disconnected`);
        });

        // Connection timeout
        setTimeout(() => {
          if (!this.connected) {
            reject(new Error('Connection timeout'));
          }
        }, this.options.connectTimeout || 30000);

      } catch (error) {
        reject(error);
      }
    });
  }

  async disconnect() {
    return new Promise((resolve) => {
      if (this.client && this.connected) {
        this.client.end(false, () => {
          this.connected = false;
          resolve();
        });
      } else {
        resolve();
      }
    });
  }

  async publish(topic, message, options = {}) {
    return new Promise((resolve, reject) => {
      if (!this.connected) {
        reject(new Error('Client not connected'));
        return;
      }

      const publishOptions = {
        qos: 0,
        retain: false,
        dup: false,
        ...options
      };

      this.client.publish(topic, message, publishOptions, (error) => {
        if (error) {
          this.errors.push({ timestamp: Date.now(), error: error.message });
          reject(error);
        } else {
          this.publishedMessages.push({
            timestamp: Date.now(),
            topic,
            message,
            options: publishOptions
          });
          resolve();
        }
      });
    });
  }

  async subscribe(topic, options = {}) {
    return new Promise((resolve, reject) => {
      if (!this.connected) {
        reject(new Error('Client not connected'));
        return;
      }

      const subscribeOptions = {
        qos: 0,
        ...options
      };

      this.client.subscribe(topic, subscribeOptions, (error, granted) => {
        if (error) {
          this.errors.push({ timestamp: Date.now(), error: error.message });
          reject(error);
        } else {
          this.subscriptions.set(topic, {
            options: subscribeOptions,
            granted,
            timestamp: Date.now()
          });
          resolve(granted);
        }
      });
    });
  }

  async unsubscribe(topic) {
    return new Promise((resolve, reject) => {
      if (!this.connected) {
        reject(new Error('Client not connected'));
        return;
      }

      this.client.unsubscribe(topic, (error) => {
        if (error) {
          this.errors.push({ timestamp: Date.now(), error: error.message });
          reject(error);
        } else {
          this.subscriptions.delete(topic);
          resolve();
        }
      });
    });
  }

  // Wait for messages with timeout
  async waitForMessages(count = 1, timeout = 5000) {
    return new Promise((resolve, reject) => {
      const startTime = Date.now();
      const check = () => {
        if (this.receivedMessages.length >= count) {
          resolve(this.receivedMessages.slice(-count));
        } else if (Date.now() - startTime > timeout) {
          reject(new Error(`Timeout waiting for ${count} messages. Received: ${this.receivedMessages.length}`));
        } else {
          setTimeout(check, 50);
        }
      };
      check();
    });
  }

  // Clear message buffers
  clearMessages() {
    this.receivedMessages = [];
    this.publishedMessages = [];
  }

  // Get statistics
  getStats() {
    return {
      connected: this.connected,
      subscriptions: this.subscriptions.size,
      publishedCount: this.publishedMessages.length,
      receivedCount: this.receivedMessages.length,
      errorCount: this.errors.length,
      lastError: this.errors.length > 0 ? this.errors[this.errors.length - 1] : null
    };
  }
}

// Factory function for creating multiple clients
function createClients(count, baseOptions = {}) {
  const clients = [];
  for (let i = 0; i < count; i++) {
    const options = {
      ...baseOptions,
      clientId: `${baseOptions.clientId || 'client'}-${i}`
    };
    clients.push(new MQTTXClient(options));
  }
  return clients;
}

// Utility function to connect all clients
async function connectAll(clients) {
  return Promise.all(clients.map(client => client.connect()));
}

// Utility function to disconnect all clients
async function disconnectAll(clients) {
  return Promise.all(clients.map(client => client.disconnect()));
}

module.exports = {
  MQTTXClient,
  createClients,
  connectAll,
  disconnectAll
};