// Debug disconnect timeout issue
const mqtt = require('mqtt');

async function testDisconnect() {
    console.log('🧪 Starting disconnect test...');

    return new Promise((resolve, reject) => {
        const client = mqtt.connect('mqtt://localhost:1883', {
            clientId: 'debug-disconnect-test',
            username: 'test',
            password: 'test',
            clean: true,
            keepalive: 60
        });

        client.on('connect', () => {
            console.log('✅ Connected to broker');

            console.log('🔧 Starting disconnect process...');
            const startTime = Date.now();

            client.end(false, () => {
                const endTime = Date.now();
                console.log(`✅ Disconnect completed in ${endTime - startTime}ms`);
                resolve();
            });

            // Add timeout to detect hanging disconnect
            setTimeout(() => {
                console.log('❌ Disconnect timed out after 5 seconds');
                reject(new Error('Disconnect timeout'));
            }, 5000);
        });

        client.on('error', (error) => {
            console.error('❌ Connection error:', error.message);
            reject(error);
        });

        client.on('disconnect', () => {
            console.log('📡 Client disconnected event fired');
        });

        client.on('close', () => {
            console.log('🔌 Connection closed event fired');
        });
    });
}

testDisconnect()
    .then(() => {
        console.log('🎉 Test completed successfully');
        process.exit(0);
    })
    .catch((error) => {
        console.error('💥 Test failed:', error.message);
        process.exit(1);
    });