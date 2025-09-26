# Baseline Performance Metrics

This document outlines the steps required to establish a performance baseline for the original Erlang-based `emqx` application. These metrics will serve as the target for the new Go implementation.

As the agent, I cannot execute these steps myself. Please perform the following benchmarks and record the results in the table below.

## Benchmarking Tool

We will use `emqtt-bench`, a powerful MQTT benchmark tool.

**Installation (if not already installed):**
```bash
git clone https://github.com/emqx/emqtt-bench.git
cd emqtt-bench
make
```

## Benchmarking Scenarios

Please run the following tests against a single, running `emqx` node. Ensure the node is in a stable state with no other significant load.

### 1. Connection Benchmark

This test measures the rate at which the broker can accept new connections.

**Command:**
```bash
./emqtt-bench conn -c 500 -i 10ms -h localhost -p 1883
```
*   `-c 500`: 500 concurrent connections
*   `-i 10ms`: 10ms interval between new connections

### 2. Publish Benchmark

This test measures the message publishing throughput.

**Command:**
```bash
./emqtt-bench pub -t perf/test -h localhost -p 1883 -c 100 -s 256 -i 1ms
```
*   `-t perf/test`: Topic to publish to
*   `-c 100`: 100 concurrent publishers
*   `-s 256`: Payload size of 256 bytes
*   `-i 1ms`: 1ms interval between publishes per client

### 3. Subscription Benchmark

This test measures the message receiving throughput for subscribers.

**Step 3.1: Start Subscribers**
```bash
./emqtt-bench sub -t perf/test -h localhost -p 1883 -c 100
```
*   `-t perf/test`: Topic to subscribe to
*   `-c 100`: 100 concurrent subscribers

**Step 3.2: Start Publisher (in a separate terminal)**
```bash
./emqtt-bench pub -t perf/test -h localhost -p 1883 -c 1 -s 256 -r 5000
```
*   `-r 5000`: Rate of 5000 messages per second

## Results Table

Please fill in the results from the `emqtt-bench` output in the table below.

| Benchmark Scenario      | Metric                     | Value                                  |
| ----------------------- | -------------------------- | -------------------------------------- |
| **Connection**          | Connections per second     | `[Please fill in]`                     |
|                         | Average Connection Time    | `[Please fill in]`                     |
| **Publish Throughput**  | Messages per second (out)  | `[Please fill in]`                     |
|                         | Average Latency (pub)      | `[Please fill in]`                     |
| **Subscribe Throughput**| Messages per second (in)   | `[Please fill in]`                     |
|                         | Average Latency (sub)      | `[Please fill in]`                     |

These baseline metrics are crucial for validating the performance of the new Go implementation in later phases.