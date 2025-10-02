// Quick test to debug payload issue
const mqtt = require('mqtt');

const client = mqtt.connect('mqtt://localhost:1883', {
  username: 'test',
  password: 'test',
  clientId: 'debug-client'
});

client.on('connect', () => {
  console.log('Connected to broker');

  client.subscribe('debug/test', { qos: 1 }, (err) => {
    if (err) {
      console.error('Subscribe error:', err);
      return;
    }
    console.log('Subscribed to debug/test');

    client.publish('debug/test', 'Hello World', { qos: 1 }, (err) => {
      if (err) {
        console.error('Publish error:', err);
      } else {
        console.log('Published message');
      }
    });
  });
});

client.on('message', (topic, message) => {
  console.log(`Received message on ${topic}:`);
  console.log('Raw message:', message);
  console.log('Message as string:', message.toString());
  console.log('Message length:', message.length);
  console.log('First bytes:', Array.from(message.slice(0, 5)));

  client.end();
});