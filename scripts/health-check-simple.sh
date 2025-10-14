#!/usr/bin/env bash
# Simple cluster health check for EMQX-Go

GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m'

echo ""
echo "EMQX-Go Cluster Health Check"
echo "============================="
echo ""

# Check processes
echo "1. Process Check"
RUNNING=$(pgrep -f "emqx-go" | wc -l | tr -d ' ')
if [ "$RUNNING" -eq 3 ]; then
    echo -e "${GREEN}✓${NC} All 3 nodes running"
else
    echo -e "${YELLOW}⚠${NC} Only $RUNNING/3 nodes running"
fi
echo ""

# Check ports  
echo "2. Port Check"
for port in 1883 1884 1885 8081 8083 8085; do
    if nc -z localhost $port 2>/dev/null; then
        echo -e "${GREEN}✓${NC} Port $port OK"
    else
        echo -e "${RED}✗${NC} Port $port FAILED"
    fi
done
echo ""

# Check metrics
echo "3. Metrics Check"
for port in 8082 8084 8086; do
    CONN=$(curl -s http://localhost:$port/metrics | grep "^emqx_connections_count" | awk '{print $2}')
    echo "  Node port $port: ${CONN:-0} connections"
done
echo ""

# Test connectivity
echo "4. Connectivity Test"
if [ -f "./bin/cluster-test" ]; then
    if timeout 5s ./bin/cluster-test >/dev/null 2>&1; then
        echo -e "${GREEN}✓${NC} Cross-node messaging OK"
    else
        echo -e "${RED}✗${NC} Cross-node messaging FAILED"
    fi
else
    echo -e "${YELLOW}⚠${NC} cluster-test not found"
fi
echo ""

echo "Health check completed at $(date)"
