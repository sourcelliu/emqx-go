<p align="center">
  <img src="logo.png" alt="emqx-go Logo" width="200" height="200">
</p>

<h1 align="center">emqx-go</h1>

<p align="center">
  <strong>Proof-of-Concept Implementation of the EMQX MQTT broker, rewritten in Go</strong>
</p>

<p align="center">
  <a href="https://github.com/turtacn/emqx-go/actions"><img src="https://img.shields.io/github/actions/workflow/status/turtacn/emqx-go/ci.yml?branch=main" alt="Build Status"></a>
  <a href="https://github.com/turtacn/emqx-go/blob/main/LICENSE"><img src="https://img.shields.io/badge/License-Apache%202.0-blue.svg" alt="License"></a>
  <a href="https://golang.org/"><img src="https://img.shields.io/badge/Go-1.21+-blue.svg" alt="Go Version"></a>
  <a href="https://github.com/turtacn/emqx-go/releases"><img src="https://img.shields.io/github/v/release/turtacn/emqx-go" alt="Latest Release"></a>
  <a href="https://goreportcard.com/report/github.com/turtacn/emqx-go"><img src="https://goreportcard.com/badge/github.com/turtacn/emqx-go" alt="Go Report Card"></a>
</p>

<p align="center">
  <a href="README-zh.md">简体中文</a> |
  <a href="#installation">Installation</a> |
  <a href="docs/architecture.md">Architecture</a> |
  <a href="docs/apis.md">API Reference</a> |
  <a href="#contributing">Contributing</a>
</p>

---

# EMQX-Go: A Golang Implementation of EMQX

This repository is a proof-of-concept implementation of the EMQX MQTT broker, rewritten in Go. The project aims to replicate the core functionalities of the original Erlang-based EMQX, including MQTT connection handling, message publishing and subscribing, session management, and clustering.

## 🌟 Features

*   **Core MQTT v3.1.1 Broker**: Supports essential MQTT functionalities, including client connections (`CONNECT`), topic subscriptions (`SUBSCRIBE`), and message publishing (`PUBLISH`).
*   **Actor-Based Concurrency**: Leverages a lightweight, OTP-inspired actor model for robust and concurrent management of client sessions. Includes supervisors that follow the "let it crash" philosophy for fault tolerance.
*   **Dynamic Clustering**: Nodes can form a distributed cluster for high availability and load distribution. Message routing between nodes is handled automatically.
*   **Kubernetes-Native Discovery**: Automatically discovers peer nodes within a Kubernetes environment using a headless service, enabling seamless cluster formation.
*   **High-Performance Communication**: Utilizes gRPC for efficient and strongly-typed inter-node communication for routing, state synchronization, and cluster management.
*   **Prometheus Metrics**: Exposes key operational metrics (e.g., connection counts, actor restarts) in a Prometheus-compatible format for easy monitoring.

## 🚀 Getting Started


### Prerequisites

*   Go (version 1.20 or later)
*   An MQTT client (e.g., [MQTTX](https://mqttx.app/) or `mosquitto_clients`)

### Building and Running

1.  **Clone the repository**:
    ```sh
    git clone https://github.com/turtacn/emqx-go.git
    cd emqx-go
    ```

2.  **Build the application**:
    ```sh
    go build ./cmd/emqx-go
    ```

3.  **Run the broker**:
    ```sh
    ./emqx-go
    ```
    The broker will start and listen for:
    *   MQTT connections on port `1883`.
    *   gRPC connections for clustering on port `8081`.
    *   Prometheus metrics on port `8082` at the `/metrics` endpoint.

### Connecting an MQTT Client

You can connect to the broker using any standard MQTT client.

*   **Host**: `localhost`
*   **Port**: `1883`

Once connected, you can subscribe to topics and publish messages to test the broker's functionality.

### Testing Cluster Functionality

EMQX-Go supports distributed clustering for high availability and load distribution. You can test the cluster functionality locally:

**Quick Start - 3-Node Cluster**:

```sh
# Start a 3-node cluster
./scripts/start-cluster.sh

# Run the cluster test
go run ./cmd/cluster-test/main.go

# Stop the cluster
./scripts/stop-cluster.sh
```

The test verifies cross-node message routing by:
1. Connecting a subscriber to Node2 (port 1884)
2. Connecting a publisher to Node1 (port 1883)
3. Publishing a message from Node1 and verifying it reaches Node2's subscriber

**Using Docker Compose**:

```sh
# Start cluster with Docker
docker-compose -f docker-compose-cluster.yaml up -d

# Stop cluster
docker-compose -f docker-compose-cluster.yaml down
```

For detailed information about cluster testing, see:
- [Cluster Testing Guide](./CLUSTER_TESTING_GUIDE.md) - How to deploy and test
- [E2E Test Report](./CLUSTER_E2E_TEST_REPORT.md) - Detailed test results and bug fixes
- [Quick Summary](./CLUSTER_TEST_SUMMARY.md) - Quick reference

## 🏗️ Project Structure

The repository is organized into the following main directories:

*   `cmd/emqx-go`: The main application entrypoint, responsible for initializing and orchestrating all services.
*   `cmd/cluster-test`: Cluster testing tool for verifying cross-node message routing.
*   `pkg/`: Contains all the core packages of the broker.
    *   `actor`: A lightweight, OTP-inspired actor model for concurrency.
    *   `broker`: The central MQTT broker logic, responsible for handling connections and orchestrating message flow.
    *   `cluster`: Components for clustering, including the gRPC server/client and the cluster state manager.
    *   `discovery`: Service discovery, with a Kubernetes implementation for automatic peer finding.
    *   `metrics`: Defines and exposes Prometheus metrics for monitoring.
    *   `proto`: Contains the Protobuf definitions (`.proto` files) and generated Go code for gRPC-based cluster communication.
    *   `protocol/mqtt`: Low-level parsing and encoding of MQTT packets.
    *   `session`: An actor-based implementation for managing a single client session.
    *   `storage`: A generic key-value store interface with an in-memory implementation for session management.
    *   `supervisor`: An OTP-style supervisor for managing actor lifecycles and implementing fault tolerance.
    *   `topic`: A thread-safe store for managing topic subscriptions and routing.
*   `scripts/`: Utility scripts for cluster deployment and testing.
*   `docs/`: Contains additional documentation on architecture and APIs.
*   `k8s/`: Kubernetes manifests for deploying the application.

## 📚 Documentation

The source code is thoroughly documented using GoDoc conventions, providing detailed explanations for all public packages, types, and functions.

You can view the documentation online at [pkg.go.dev](https://pkg.go.dev/github.com/turtacn/emqx-go).

Alternatively, you can generate and view the documentation locally by running:

```sh
godoc -http=:6060
```

Then, open your browser to `http://localhost:6060/pkg/github.com/turtacn/emqx-go/`.