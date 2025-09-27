/*
 * k6_stress.js
 *
 * This script is designed to stress test the EMQX-Go MQTT broker using k6.
 * It simulates a number of concurrent clients (Virtual Users) that connect,
 * publish messages for a period, and then disconnect.
 *
 * Prerequisites:
 *  - k6 installed (https://k6.io/docs/getting-started/installation/)
 *  - The k6 MQTT extension (xk6-mqtt):
 *    https://github.com/grafana/xk6-mqtt
 *
 * How to run:
 *  k6 run tests/load/k6_stress.js
 */

import { check, sleep } from 'k6';
import mqtt from 'k6/x/mqtt';

// --- Test Configuration ---
export const options = {
  scenarios: {
    // A single scenario that ramps up Virtual Users (VUs) over time.
    emqx_stress_test: {
      executor: 'ramping-vus',
      startVUs: 0,
      stages: [
        { duration: '1m', target: 100 }, // Ramp up to 100 users over 1 minute
        { duration: '3m', target: 100 }, // Stay at 100 users for 3 minutes
        { duration: '1m', target: 0 },   // Ramp down to 0 users
      ],
      gracefulRampDown: '30s',
    },
  },
  // Define thresholds for success criteria. The test will fail if these are not met.
  thresholds: {
    'mqtt_connection_duration': ['p(95)<500'], // 95% of connections should be faster than 500ms
    'checks': ['rate>0.99'],                   // More than 99% of checks should pass
  },
};

// --- MQTT Broker Connection Details ---
const BROKER_URL = 'localhost:1883';
const CLIENT_ID_PREFIX = 'k6-client-';

export default function () {
  // --- Create MQTT Client ---
  // Each VU gets a unique client ID to avoid conflicts.
  const client = mqtt.connect({
    url: BROKER_URL,
    clientId: `${CLIENT_ID_PREFIX}${__VU}-${__ITER}`,
  });

  // Check if the connection was successful.
  check(client, {
    'is connected': (c) => c !== null,
  });

  // If the connection failed, we can't proceed with this iteration.
  if (!client) {
    return;
  }

  // --- Publish Loop ---
  // Each VU will publish messages for a short period.
  for (let i = 0; i < 10; i++) {
    const topic = `k6/stress/vu-${__VU}`;
    const payload = JSON.stringify({
      vu: __VU,
      iter: __ITER,
      timestamp: Date.now(),
    });

    // Publish the message and check if the operation was successful.
    const success = client.publish(topic, payload, { qos: 1, retain: false });
    check(success, {
      'is published': (s) => s,
    });

    sleep(1); // Wait for 1 second between publishes.
  }

  // --- Disconnect ---
  client.disconnect();
}