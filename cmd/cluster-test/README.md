# Cluster Test Tool

A comprehensive testing tool for verifying EMQX-Go cluster cross-node message routing functionality.

## Features

- ✅ Automated cross-node messaging test
- ✅ Configurable test parameters via command-line flags
- ✅ Detailed error handling and reporting
- ✅ Connection timeout and error detection
- ✅ Verbose logging mode for debugging
- ✅ Helpful troubleshooting tips on failure
- ✅ Message delivery latency measurement

## Quick Start

### 1. Start the cluster

```bash
./scripts/start-cluster.sh
```

### 2. Run the test

```bash
# Using default settings
go run ./cmd/cluster-test/main.go

# Or use the compiled binary
./bin/cluster-test
```

### 3. Stop the cluster

```bash
./scripts/stop-cluster.sh
```

## Command-Line Options

```bash
./bin/cluster-test [options]

Options:
  -node1-host string
        Node1 hostname (default "localhost")
  -node1-port int
        Node1 MQTT port (default 1883)
  -node2-host string
        Node2 hostname (default "localhost")
  -node2-port int
        Node2 MQTT port (default 1884)
  -username string
        MQTT username (default "test")
  -password string
        MQTT password (default "test")
  -topic string
        Test topic (default "cluster/test")
  -message string
        Test message (default "Hello from Node1 to Node2!")
  -route-wait duration
        Time to wait for route propagation (default 2s)
  -delivery-wait duration
        Time to wait for message delivery (default 3s)
  -verbose
        Enable verbose logging
  -h
        Show help message
```

## Usage Examples

### Basic Test

```bash
./bin/cluster-test
```

### Test with Custom Ports

```bash
./bin/cluster-test -node1-port 1883 -node2-port 1884
```

### Test with Custom Message

```bash
./bin/cluster-test -message "Custom test message"
```

### Test with Verbose Logging

```bash
./bin/cluster-test -verbose
```

### Test with Longer Wait Times

```bash
./bin/cluster-test -route-wait 5s -delivery-wait 10s
```

### Test Remote Cluster

```bash
./bin/cluster-test \
  -node1-host node1.example.com \
  -node1-port 1883 \
  -node2-host node2.example.com \
  -node2-port 1883 \
  -username admin \
  -password secret
```

## Expected Output

### Successful Test

```
======================================================================
EMQX-Go Cluster Cross-Node Messaging Test
======================================================================
Test started at: 2025-10-10 21:30:00

1. Creating subscriber connecting to localhost:1884...
✓ Subscriber connected to node2 (localhost:1884)

2. Subscribing to topic 'cluster/test'...
✓ Subscribed successfully

3. Waiting for route propagation (2s)...

4. Creating publisher connecting to localhost:1883...
✓ Publisher connected to node1 (localhost:1883)

5. Publishing message from node1 to topic 'cluster/test'...
   Message: "Hello from Node1 to Node2!"
✓ Message published successfully

6. Waiting for cross-node message delivery (3s)...
✓ [21:30:05.123] RECEIVED: 'Hello from Node1 to Node2!' on topic 'cluster/test' (QoS: 1, Retained: false)
✓ Message received in 45ms

======================================================================
Test Results:
======================================================================
✓✓✓ SUCCESS! Cross-node messaging works!
✓ Message 'Hello from Node1 to Node2!' successfully routed from node1 to node2
  → Publisher: localhost:1883
  → Subscriber: localhost:1884
  → Topic: cluster/test
======================================================================
Test completed at: 2025-10-10 21:30:05
```

### Failed Test

```
======================================================================
EMQX-Go Cluster Cross-Node Messaging Test
======================================================================
Test started at: 2025-10-10 21:30:00

1. Creating subscriber connecting to localhost:1884...
✗ Failed to connect subscriber: network Error : dial tcp 127.0.0.1:1884: connect: connection refused
  → Make sure node2 is running on localhost:1884
  → Check if the cluster is started (run: ./scripts/start-cluster.sh)
```

## Troubleshooting

### Test fails with "connection refused"

**Problem**: Cannot connect to MQTT broker

**Solutions**:
1. Ensure the cluster is started: `./scripts/start-cluster.sh`
2. Check if processes are running: `ps aux | grep emqx-go`
3. Verify ports are not already in use: `lsof -i :1883`

### Test times out with no message received

**Problem**: Nodes connected but message not routed

**Solutions**:
1. Check cluster logs for errors:
   ```bash
   tail -f ./logs/node1.log
   tail -f ./logs/node2.log
   ```
2. Look for "Successfully joined cluster" in logs
3. Look for "BatchUpdateRoutes" messages indicating route propagation
4. Check for "Cannot forward publish" errors

### Test fails intermittently

**Problem**: Race condition or timing issue

**Solutions**:
1. Increase wait times:
   ```bash
   ./bin/cluster-test -route-wait 5s -delivery-wait 10s
   ```
2. Enable verbose logging to see detailed connection info:
   ```bash
   ./bin/cluster-test -verbose
   ```

## Exit Codes

- `0`: Test passed successfully
- `1`: Test failed (connection error, timeout, or wrong message received)

## Integration with CI/CD

You can use this tool in automated testing pipelines:

```bash
#!/bin/bash
# Start cluster
./scripts/start-cluster.sh

# Wait for cluster to be ready
sleep 5

# Run test
if ./bin/cluster-test; then
  echo "Cluster test passed"
  EXIT_CODE=0
else
  echo "Cluster test failed"
  EXIT_CODE=1
fi

# Cleanup
./scripts/stop-cluster.sh

exit $EXIT_CODE
```

## Architecture

The test performs the following steps:

1. **Subscriber Setup**: Connects to Node2 and subscribes to test topic
2. **Route Propagation**: Waits for subscription route to propagate to Node1
3. **Publisher Setup**: Connects to Node1
4. **Message Publish**: Publishes test message to topic on Node1
5. **Message Delivery**: Waits for message to arrive at Node2 subscriber
6. **Verification**: Compares received message with expected message

```
┌─────────┐                    ┌─────────┐
│  Node1  │                    │  Node2  │
│ (1883)  │                    │ (1884)  │
└────┬────┘                    └────┬────┘
     │                              │
     │                              │ 1. Subscribe
     │                              │◄───────┐
     │                              │        │
     │ 2. Route propagation (gRPC) │        │
     │◄─────────────────────────────┤        │
     │                              │        │
     │ 3. Publish message           │        │
     │◄─────┐                       │        │
     │      │                       │        │
     │ 4. Forward to Node2 (gRPC)  │        │
     ├──────────────────────────────►        │
     │                              │        │
     │                              │ 5. Deliver
     │                              ├───────►│
     │                              │        │
```

## See Also

- [Cluster Testing Guide](../CLUSTER_TESTING_GUIDE.md) - Complete cluster testing documentation
- [E2E Test Report](../CLUSTER_E2E_TEST_REPORT.md) - Detailed bug fixes and test results
- [Test Summary](../CLUSTER_TEST_SUMMARY.md) - Quick reference in Chinese
