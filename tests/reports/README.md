# Test Reports and Strategy

This document provides an overview of the testing strategy for the EMQX-Go project and serves as an index for the various testing artifacts created throughout the development phases.

## 1. Testing Philosophy

Our testing strategy is built on a multi-layered approach to ensure the reliability, performance, and correctness of the application. The key layers are:

*   **Unit Tests**: To verify the correctness of individual functions and components in isolation.
*   **Contract Tests**: To ensure that the public-facing API of the broker (i.e., the MQTT protocol implementation) behaves as expected.
*   **Load Tests**: To measure the performance and scalability of the system under heavy load.
*   **Chaos Tests**: To ensure the resilience and self-healing capabilities of the system in a clustered environment.

## 2. Test Artifacts Index

### Unit Tests

*   **Location**: Found in `_test.go` files alongside the source code (e.g., `pkg/actor/actor_test.go`, `pkg/supervisor/supervisor_test.go`).
*   **Purpose**: To test the logic of individual packages and components. For example, the supervisor tests verify that a panicked actor is correctly restarted.
*   **How to Run**:
    ```bash
    go test ./...
    ```

### Contract Tests

*   **Location**: `tests/contract/mqtt_basic_test.go`
*   **Purpose**: To provide an end-to-end validation of the core MQTT functionalities (`CONNECT`, `SUBSCRIBE`, `PUBLISH`). This test starts a broker instance and uses a real MQTT client to interact with it, ensuring that the broker adheres to the MQTT protocol contract.
*   **How to Run**:
    ```bash
    go test ./tests/contract/...
    ```

### Load and Performance Tests

*   **Location**: `tests/load/k6_stress.js`
*   **Purpose**: To simulate a high volume of concurrent users and messages to stress test the broker and measure its performance against our SLOs (e.g., connection rate, message latency).
*   **Tool**: [k6](https://k6.io/) with the `xk6-mqtt` extension.
*   **How to Run**:
    ```bash
    # This requires a k6 build with the MQTT extension.
    k6 run tests/load/k6_stress.js
    ```

### Chaos and Resilience Tests

*   **Location**: `tests/chaos/kill_pod.sh`
*   **Purpose**: To test the self-healing capabilities of the clustered deployment in Kubernetes. This script randomly terminates a pod to ensure that the `StatefulSet` correctly recreates it and that the cluster remains stable.
*   **Tool**: `kubectl`
*   **How to Run**:
    ```bash
    # Ensure you are connected to the correct Kubernetes cluster.
    ./tests/chaos/kill_pod.sh
    ```

## 3. Reporting

This directory is intended to be used for storing the output reports from these tests. For example, the results of a `k6` run can be saved here for historical analysis:

```bash
k6 run tests/load/k6_stress.js --out json=tests/reports/k6_report_$(date +%s).json
```

By maintaining this structured approach to testing, we can ensure a high level of quality and confidence in the EMQX-Go broker as it moves towards production.