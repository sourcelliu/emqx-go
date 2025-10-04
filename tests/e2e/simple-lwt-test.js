/**
 * Simple LWT Verification Test
 * 基于成功案例的简单LWT验证
 */

const mqtt = require('mqtt');

async function simpleLWTTest() {
    console.log('🔍 Simple LWT Verification Test');

    try {
        // Step 1: Create subscriber
        const subscriber = mqtt.connect('mqtt://localhost:1883', {
            clientId: 'simple-lwt-sub',
            username: 'test',
            password: 'test',
            clean: true
        });

        // Wait for subscriber connection
        await new Promise((resolve, reject) => {
            const timeout = setTimeout(() => reject(new Error('Subscriber connection timeout')), 5000);
            subscriber.on('connect', () => {
                clearTimeout(timeout);
                console.log('✅ Subscriber connected');
                resolve();
            });
            subscriber.on('error', reject);
        });

        // Step 2: Subscribe to LWT topic
        await new Promise((resolve, reject) => {
            subscriber.subscribe('test/lwt/simple', { qos: 1 }, (err) => {
                if (err) reject(err);
                else {
                    console.log('✅ Subscribed to test/lwt/simple');
                    resolve();
                }
            });
        });

        // Step 3: Create LWT client (similar to working mqttx test)
        const lwtClient = mqtt.connect('mqtt://localhost:1883', {
            clientId: 'simple-lwt-client',
            username: 'test',
            password: 'test',
            clean: true,
            will: {
                topic: 'test/lwt/simple',
                payload: 'simple client disconnected unexpectedly',
                qos: 1,
                retain: false
            }
        });

        // Wait for LWT client connection
        await new Promise((resolve, reject) => {
            const timeout = setTimeout(() => reject(new Error('LWT client connection timeout')), 5000);
            lwtClient.on('connect', () => {
                clearTimeout(timeout);
                console.log('✅ LWT client connected');
                resolve();
            });
            lwtClient.on('error', reject);
        });

        // Step 4: Set up message receiver
        let messageReceived = false;
        const messagePromise = new Promise((resolve) => {
            subscriber.on('message', (topic, message) => {
                if (topic === 'test/lwt/simple' && !messageReceived) {
                    messageReceived = true;
                    resolve({ topic, message: message.toString() });
                }
            });
        });

        // Step 5: Force unexpected disconnect
        console.log('🔌 Forcing unexpected disconnect...');
        lwtClient.stream.destroy();

        // Step 6: Wait for message with timeout
        const result = await Promise.race([
            messagePromise,
            new Promise((_, reject) =>
                setTimeout(() => reject(new Error('Message timeout')), 3000)
            )
        ]);

        console.log('✅ LWT message received!');
        console.log(`   Topic: ${result.topic}`);
        console.log(`   Message: ${result.message}`);

        // Cleanup
        subscriber.end();
        return true;

    } catch (error) {
        console.error('❌ Test failed:', error.message);
        return false;
    }
}

// Run test
if (require.main === module) {
    (async () => {
        const success = await simpleLWTTest();
        console.log(success ? '\n🎉 LWT Test PASSED!' : '\n❌ LWT Test FAILED!');
        process.exit(success ? 0 : 1);
    })();
}

module.exports = simpleLWTTest;