#!/bin/bash
# Start a 3-node EMQX-Go cluster locally

set -e

# Build the application
echo "Building emqx-go..."
go build -o ./bin/emqx-go ./cmd/emqx-go

# Create log directory
mkdir -p ./logs

# Kill existing processes
echo "Stopping existing cluster nodes..."
pkill -f "emqx-go" || true
sleep 1

# Start node 1
echo "Starting node1..."
NODE_ID=node1 \
MQTT_PORT=:1883 \
GRPC_PORT=:8081 \
METRICS_PORT=:8082 \
./bin/emqx-go > ./logs/node1.log 2>&1 &
NODE1_PID=$!
echo "Node1 started with PID: $NODE1_PID"

# Wait for node1 to start
sleep 2

# Start node 2
echo "Starting node2..."
NODE_ID=node2 \
MQTT_PORT=:1884 \
GRPC_PORT=:8083 \
METRICS_PORT=:8084 \
PEER_NODES=localhost:8081 \
./bin/emqx-go > ./logs/node2.log 2>&1 &
NODE2_PID=$!
echo "Node2 started with PID: $NODE2_PID"

# Wait for node2 to start
sleep 2

# Start node 3
echo "Starting node3..."
NODE_ID=node3 \
MQTT_PORT=:1885 \
GRPC_PORT=:8085 \
METRICS_PORT=:8086 \
PEER_NODES=localhost:8081,localhost:8083 \
./bin/emqx-go > ./logs/node3.log 2>&1 &
NODE3_PID=$!
echo "Node3 started with PID: $NODE3_PID"

echo ""
echo "Cluster started successfully!"
echo "Node1 PID: $NODE1_PID (MQTT: 1883, gRPC: 8081, Metrics: 8082)"
echo "Node2 PID: $NODE2_PID (MQTT: 1884, gRPC: 8083, Metrics: 8084)"
echo "Node3 PID: $NODE3_PID (MQTT: 1885, gRPC: 8085, Metrics: 8086)"
echo ""
echo "Log files:"
echo "  - ./logs/node1.log"
echo "  - ./logs/node2.log"
echo "  - ./logs/node3.log"
echo ""
echo "To stop the cluster, run: ./scripts/stop-cluster.sh"
