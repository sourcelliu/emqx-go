# MQTT Test Scripts

This directory contains JavaScript-based MQTT test scripts for testing the EMQX-Go broker.

## Test Files

| File | Description |
|------|-------------|
| `mqttx-test.js` | Basic MQTT client test using MQTTX |
| `mqttx-multi-client-test.js` | Multi-client concurrent connection test |
| `mqttx-wildcard-test.js` | Wildcard subscription test |
| `mqtt-diagnostic.js` | MQTT connection diagnostic tool |
| `debug-hash-wildcard.js` | Debug script for hash-based wildcard routing |
| `test-testtopic-hash.js` | Test script for topic hash functionality |

## Prerequisites

Install dependencies from the project root:

```bash
npm install --prefix configs/
```

Or if you have package.json in root:

```bash
cd ../../configs
npm install
```

## Running Tests

From the project root directory:

```bash
# Run basic MQTT test
node tests/mqtt/mqttx-test.js

# Run multi-client test
node tests/mqtt/mqttx-multi-client-test.js

# Run wildcard subscription test
node tests/mqtt/mqttx-wildcard-test.js

# Run diagnostic tool
node tests/mqtt/mqtt-diagnostic.js
```

## Configuration

Most tests connect to `mqtt://localhost:1883` by default.

Ensure the EMQX-Go broker is running before executing tests:

```bash
go run ./cmd/emqx-go
```

## Related Documentation

- [E2E Testing Guide](../e2e/README.md)
- [Main Project README](../../README.md)
