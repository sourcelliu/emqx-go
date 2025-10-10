#!/bin/bash
# Stop the 3-node EMQX-Go cluster

echo "Stopping cluster nodes..."
pkill -f "emqx-go" || true
echo "Cluster stopped."
