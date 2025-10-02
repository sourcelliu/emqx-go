// Debug LWT with detailed logging
const mqtt = require('mqtt');

async function testLWTDetailed() {
  console.log('🧪 Starting detailed LWT test...');

  // First, create a subscriber
  const subscriber = mqtt.connect('mqtt://localhost:1883', {
    clientId: 'lwt-debug-subscriber',
    username: 'test',
    password: 'test',
    clean: true
  });

  await new Promise((resolve, reject) => {
    subscriber.on('connect', () => {
      console.log('✅ Subscriber connected');
      resolve();
    });
    subscriber.on('error', reject);
    setTimeout(() => reject(new Error('Subscriber connection timeout')), 5000);
  });

  // Subscribe to LWT topic
  await new Promise((resolve, reject) => {
    subscriber.subscribe('test/lwt/detailed', { qos: 1 }, (err) => {
      if (err) {
        reject(err);
      } else {
        console.log('✅ Subscribed to LWT topic');
        resolve();
      }
    });
    setTimeout(() => reject(new Error('Subscribe timeout')), 5000);
  });

  // Set up message listener early
  let messageReceived = false;
  subscriber.on('message', (topic, message) => {
    console.log(`📨 Received LWT message on ${topic}: ${message.toString()}`);
    messageReceived = true;
  });

  // Create client with LWT message (using clean=false to maintain session)
  console.log('🔧 Creating client with LWT...');
  const clientWithLWT = mqtt.connect('mqtt://localhost:1883', {
    clientId: 'lwt-debug-client',
    username: 'test',
    password: 'test',
    clean: false, // Try with persistent session
    will: {
      topic: 'test/lwt/detailed',
      payload: 'DETAILED LWT MESSAGE TRIGGERED',
      qos: 1,
      retain: false
    }
  });

  await new Promise((resolve, reject) => {
    clientWithLWT.on('connect', () => {
      console.log('✅ LWT client connected');
      resolve();
    });
    clientWithLWT.on('error', reject);
    setTimeout(() => reject(new Error('LWT client connection timeout')), 5000);
  });

  // Wait a moment to ensure everything is set up
  await new Promise(resolve => setTimeout(resolve, 1000));

  console.log('💥 Simulating ungraceful disconnect...');

  // Method: Force close the underlying socket
  if (clientWithLWT.stream && clientWithLWT.stream.destroy) {
    clientWithLWT.stream.destroy();
    console.log('🔧 Destroyed client socket');
  } else {
    console.log('❌ Could not destroy socket');
  }

  // Wait for LWT message
  console.log('⏳ Waiting 10 seconds for LWT message...');
  await new Promise(resolve => setTimeout(resolve, 10000));

  if (messageReceived) {
    console.log('✅ DETAILED LWT TEST PASSED - Message received');
  } else {
    console.log('❌ DETAILED LWT TEST FAILED - No message received');
  }

  // Cleanup
  subscriber.end();
  if (!clientWithLWT.disconnected && !clientWithLWT.destroyed) {
    clientWithLWT.end();
  }

  process.exit(messageReceived ? 0 : 1);
}

testLWTDetailed().catch(err => {
  console.error('❌ Detailed LWT test error:', err);
  process.exit(1);
});