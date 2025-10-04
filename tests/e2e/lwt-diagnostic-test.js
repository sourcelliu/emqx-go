/**
 * LWT Issue Diagnosis and Fix
 * Detailed debugging of LWT timeout issue
 */

const mqtt = require('mqtt');

class LWTDiagnosticTest {
    constructor() {
        this.clients = [];
    }

    async runDiagnosticTest() {
        console.log('🔍 Starting LWT Diagnostic Test...');

        try {
            // Test 1: Basic LWT with proper disconnect
            await this.testBasicLWT();

            // Test 2: LWT with stream destroy (the failing case)
            await this.testStreamDestroyLWT();

            // Test 3: LWT with network error simulation
            await this.testNetworkErrorLWT();

        } catch (error) {
            console.error('❌ Diagnostic test error:', error.message);
        } finally {
            await this.cleanup();
        }
    }

    async testBasicLWT() {
        console.log('\n🧪 Test 1: Basic LWT with proper connection close');

        // Create subscriber
        const subscriber = mqtt.connect('mqtt://localhost:1883', {
            clientId: 'lwt-diag-sub',
            username: 'test',
            password: 'test',
            clean: true
        });

        await this.waitForConnect(subscriber, 'subscriber');

        // Subscribe to LWT topic
        subscriber.subscribe('test/lwt/diagnostic', { qos: 1 });
        console.log('✅ Subscriber connected and subscribed');

        // Create client with LWT
        const lwtClient = mqtt.connect('mqtt://localhost:1883', {
            clientId: 'lwt-diag-client',
            username: 'test',
            password: 'test',
            clean: true,
            will: {
                topic: 'test/lwt/diagnostic',
                payload: 'LWT triggered - basic test',
                qos: 1,
                retain: false
            }
        });

        await this.waitForConnect(lwtClient, 'LWT client');

        // Set up message listener
        const messagePromise = new Promise((resolve, reject) => {
            const timeout = setTimeout(() => {
                reject(new Error('Message timeout'));
            }, 3000);

            subscriber.on('message', (topic, message) => {
                if (topic === 'test/lwt/diagnostic') {
                    clearTimeout(timeout);
                    resolve({ topic, message: message.toString() });
                }
            });
        });

        // Force disconnect by ending connection
        console.log('🔌 Forcing disconnect...');
        lwtClient.end(true); // Force close

        try {
            const result = await messagePromise;
            console.log('✅ LWT message received:', result.message);
        } catch (error) {
            console.log('❌ LWT message not received:', error.message);
        }

        subscriber.end();
    }

    async testStreamDestroyLWT() {
        console.log('\n🧪 Test 2: LWT with stream destroy (reproducing failure)');

        // Create subscriber
        const subscriber = mqtt.connect('mqtt://localhost:1883', {
            clientId: 'lwt-diag-sub2',
            username: 'test',
            password: 'test',
            clean: true
        });

        await this.waitForConnect(subscriber, 'subscriber');

        // Subscribe to LWT topic
        subscriber.subscribe('test/lwt/diagnostic2', { qos: 1 });
        console.log('✅ Subscriber connected and subscribed');

        // Create client with LWT
        const lwtClient = mqtt.connect('mqtt://localhost:1883', {
            clientId: 'lwt-diag-client2',
            username: 'test',
            password: 'test',
            clean: true,
            will: {
                topic: 'test/lwt/diagnostic2',
                payload: 'LWT triggered - stream destroy test',
                qos: 1,
                retain: false
            }
        });

        await this.waitForConnect(lwtClient, 'LWT client');

        // Set up message listener
        const messagePromise = new Promise((resolve, reject) => {
            const timeout = setTimeout(() => {
                reject(new Error('Message timeout'));
            }, 3000);

            subscriber.on('message', (topic, message) => {
                if (topic === 'test/lwt/diagnostic2') {
                    clearTimeout(timeout);
                    resolve({ topic, message: message.toString() });
                }
            });
        });

        // Force disconnect by destroying stream (the failing method)
        console.log('🔌 Destroying stream...');
        if (lwtClient.stream) {
            lwtClient.stream.destroy();
        }

        try {
            const result = await messagePromise;
            console.log('✅ LWT message received:', result.message);
        } catch (error) {
            console.log('❌ LWT message not received:', error.message);
        }

        subscriber.end();
    }

    async testNetworkErrorLWT() {
        console.log('\n🧪 Test 3: LWT with network error simulation');

        // Create subscriber
        const subscriber = mqtt.connect('mqtt://localhost:1883', {
            clientId: 'lwt-diag-sub3',
            username: 'test',
            password: 'test',
            clean: true
        });

        await this.waitForConnect(subscriber, 'subscriber');

        // Subscribe to LWT topic
        subscriber.subscribe('test/lwt/diagnostic3', { qos: 1 });
        console.log('✅ Subscriber connected and subscribed');

        // Create client with LWT
        const lwtClient = mqtt.connect('mqtt://localhost:1883', {
            clientId: 'lwt-diag-client3',
            username: 'test',
            password: 'test',
            clean: true,
            will: {
                topic: 'test/lwt/diagnostic3',
                payload: 'LWT triggered - network error test',
                qos: 1,
                retain: false
            }
        });

        await this.waitForConnect(lwtClient, 'LWT client');

        // Set up message listener
        const messagePromise = new Promise((resolve, reject) => {
            const timeout = setTimeout(() => {
                reject(new Error('Message timeout'));
            }, 3000);

            subscriber.on('message', (topic, message) => {
                if (topic === 'test/lwt/diagnostic3') {
                    clearTimeout(timeout);
                    resolve({ topic, message: message.toString() });
                }
            });
        });

        // Simulate network error
        console.log('🔌 Simulating network error...');
        if (lwtClient.stream) {
            lwtClient.stream.emit('error', new Error('Simulated network error'));
        }

        try {
            const result = await messagePromise;
            console.log('✅ LWT message received:', result.message);
        } catch (error) {
            console.log('❌ LWT message not received:', error.message);
        }

        subscriber.end();
    }

    async waitForConnect(client, name) {
        return new Promise((resolve, reject) => {
            const timeout = setTimeout(() => {
                reject(new Error(`${name} connection timeout`));
            }, 5000);

            client.on('connect', () => {
                clearTimeout(timeout);
                console.log(`✅ ${name} connected`);
                resolve();
            });

            client.on('error', (error) => {
                clearTimeout(timeout);
                reject(error);
            });
        });
    }

    async cleanup() {
        console.log('\n🧹 Cleaning up...');
        for (const client of this.clients) {
            if (client && !client.disconnected) {
                client.end();
            }
        }
    }
}

// Run diagnostic test
if (require.main === module) {
    (async () => {
        const test = new LWTDiagnosticTest();
        await test.runDiagnosticTest();
    })();
}

module.exports = LWTDiagnosticTest;