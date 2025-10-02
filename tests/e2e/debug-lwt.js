// Debug LWT functionality
const mqtt = require('mqtt');

async function testLWT() {
  console.log('🧪 Testing LWT functionality...');

  // First, create a subscriber to listen for LWT messages
  const subscriber = mqtt.connect('mqtt://localhost:1883', {
    clientId: 'lwt-subscriber',
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
  });

  // Subscribe to LWT topic
  await new Promise((resolve, reject) => {
    subscriber.subscribe('test/lwt/debug', { qos: 1 }, (err) => {
      if (err) {
        reject(err);
      } else {
        console.log('✅ Subscribed to LWT topic');
        resolve();
      }
    });
  });

  // Create client with LWT message
  console.log('🔧 Creating client with LWT...');
  const clientWithLWT = mqtt.connect('mqtt://localhost:1883', {
    clientId: 'lwt-client-debug',
    username: 'test',
    password: 'test',
    clean: true,
    will: {
      topic: 'test/lwt/debug',
      payload: 'Client disconnected unexpectedly!',
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
  });

  // Set up message listener
  let messageReceived = false;
  subscriber.on('message', (topic, message) => {
    console.log(`📨 Received LWT message on ${topic}: ${message.toString()}`);
    messageReceived = true;
  });

  // Wait a moment to ensure everything is set up
  await new Promise(resolve => setTimeout(resolve, 1000));

  console.log('💥 Force disconnecting LWT client...');

  // Method 1: Try destroying the stream
  if (clientWithLWT.stream) {
    clientWithLWT.stream.destroy();
    console.log('🔧 Destroyed client stream');
  }

  // Method 2: Try ending abruptly
  // clientWithLWT.end(true);

  // Wait for LWT message
  console.log('⏳ Waiting 5 seconds for LWT message...');
  await new Promise(resolve => setTimeout(resolve, 5000));

  if (messageReceived) {
    console.log('✅ LWT test PASSED - Message received');
  } else {
    console.log('❌ LWT test FAILED - No message received');
  }

  // Cleanup
  subscriber.end();
  if (!clientWithLWT.disconnected) {
    clientWithLWT.end();
  }

  process.exit(messageReceived ? 0 : 1);
}

testLWT().catch(err => {
  console.error('❌ LWT test error:', err);
  process.exit(1);
});