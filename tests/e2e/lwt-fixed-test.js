/**
 * Fixed LWT Diagnostic Test
 * 修复了LWT设置问题的诊断测试
 */

const mqtt = require('mqtt');

class FixedLWTTest {
    constructor() {
        this.clients = [];
    }

    async runFixedLWTTest() {
        console.log('🔧 Starting Fixed LWT Test...');

        try {
            // Test 1: Corrected LWT test with proper will configuration
            await this.testProperLWT();

        } catch (error) {
            console.error('❌ Fixed LWT test error:', error.message);
        } finally {
            await this.cleanup();
        }
    }

    async testProperLWT() {
        console.log('\n🧪 Test: Proper LWT with correct MQTT will property');

        // Create subscriber first
        const subscriber = mqtt.connect('mqtt://localhost:1883', {
            clientId: 'lwt-fixed-sub',
            username: 'test',
            password: 'test',
            clean: true
        });

        this.clients.push(subscriber);
        await this.waitForConnect(subscriber, 'subscriber');

        // Subscribe to LWT topic
        await new Promise((resolve, reject) => {
            subscriber.subscribe('test/lwt/fixed', { qos: 1 }, (err) => {
                if (err) reject(err);
                else {
                    console.log('✅ Subscribed to LWT topic');
                    resolve();
                }
            });
        });

        // Create client with proper LWT configuration
        const lwtClient = mqtt.connect('mqtt://localhost:1883', {
            clientId: 'lwt-fixed-client',
            username: 'test',
            password: 'test',
            clean: true,
            // MQTT standard will property (not legacy)
            will: {
                topic: 'test/lwt/fixed',
                payload: 'Fixed LWT message - unexpected disconnect',
                qos: 1,
                retain: false
            }
        });

        this.clients.push(lwtClient);
        await this.waitForConnect(lwtClient, 'LWT client');

        console.log('✅ LWT client connected with will message configured');

        // Set up message listener with timeout
        const messagePromise = new Promise((resolve, reject) => {
            const timeout = setTimeout(() => {
                reject(new Error('Message timeout after 5 seconds'));
            }, 5000);

            subscriber.on('message', (topic, message) => {
                if (topic === 'test/lwt/fixed') {
                    clearTimeout(timeout);
                    resolve({ topic, message: message.toString() });
                }
            });
        });

        // Wait a moment to ensure connection is stable
        await new Promise(resolve => setTimeout(resolve, 100));

        // Force unexpected disconnect by destroying the socket
        console.log('🔌 Forcing unexpected disconnect...');
        if (lwtClient.stream) {
            lwtClient.stream.destroy();
        }

        try {
            const result = await messagePromise;
            console.log('✅ LWT message received successfully!');
            console.log(`   Topic: ${result.topic}`);
            console.log(`   Message: ${result.message}`);
            return true;
        } catch (error) {
            console.log('❌ LWT message not received:', error.message);
            return false;
        }
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

// Run fixed test
if (require.main === module) {
    (async () => {
        const test = new FixedLWTTest();
        const success = await test.runFixedLWTTest();

        if (success) {
            console.log('\n🎉 LWT Test PASSED! The broker LWT functionality is working correctly.');
        } else {
            console.log('\n❌ LWT Test FAILED! There might be an issue with the broker or test setup.');
        }

        process.exit(success ? 0 : 1);
    })();
}

module.exports = FixedLWTTest;