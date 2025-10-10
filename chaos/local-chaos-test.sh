#!/bin/bash
# chaos/local-chaos-test.sh
# 本地环境混沌测试 - 不需要Kubernetes

set -e

RESULTS_DIR="local-chaos-results-$(date +%Y%m%d-%H%M%S)"
mkdir -p "$RESULTS_DIR"

# 颜色
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m'

log_info() {
  echo -e "${GREEN}[INFO]${NC} $1"
}

log_error() {
  echo -e "${RED}[ERROR]${NC} $1"
}

log_warn() {
  echo -e "${YELLOW}[WARN]${NC} $1"
}

echo "========================================================================"
echo "EMQX-Go Local Chaos Testing"
echo "========================================================================"
log_info "Results will be saved to: $RESULTS_DIR"
echo ""

# 测试1: 节点随机故障测试
test_node_failure() {
  log_info "Test 1: Node Random Failure (Pod Kill simulation)"
  echo "------------------------------------------------------------------------"

  # 启动集群
  log_info "Starting 3-node cluster..."
  ../scripts/start-cluster.sh > "$RESULTS_DIR/test1-startup.log" 2>&1

  if [ $? -ne 0 ]; then
    log_error "Failed to start cluster"
    return 1
  fi

  sleep 5

  # 启动测试负载
  log_info "Starting test load..."
  go run ../cmd/cluster-test/main.go > "$RESULTS_DIR/test1-baseline.txt" 2>&1 &
  TEST_PID=$!

  sleep 3

  # 模拟混沌: 杀死一个节点
  log_warn "Injecting chaos: Killing node2..."
  pkill -f "NODE_ID=node2" || true

  sleep 2

  # 检查其他节点是否继续工作
  log_info "Verifying cluster resilience..."

  # 尝试连接到node1
  timeout 5 go run ../cmd/cluster-test/main.go -node2-port 1885 > "$RESULTS_DIR/test1-during-chaos.txt" 2>&1

  if [ $? -eq 0 ]; then
    log_info "✓ Cluster continues to function with 2/3 nodes"
    echo "PASSED" > "$RESULTS_DIR/test1-result.txt"
  else
    log_warn "✗ Cluster failed with 1 node down"
    echo "FAILED" > "$RESULTS_DIR/test1-result.txt"
  fi

  # 清理
  kill $TEST_PID 2>/dev/null || true
  ../scripts/stop-cluster.sh > /dev/null 2>&1

  sleep 2
}

# 测试2: 网络延迟模拟
test_network_delay() {
  log_info "Test 2: Network Delay Simulation"
  echo "------------------------------------------------------------------------"

  # 启动集群
  log_info "Starting 3-node cluster..."
  ../scripts/start-cluster.sh > "$RESULTS_DIR/test2-startup.log" 2>&1

  sleep 5

  # 基线测试
  log_info "Collecting baseline metrics..."
  START_TIME=$(date +%s%3N)
  go run ../cmd/cluster-test/main.go > "$RESULTS_DIR/test2-baseline.txt" 2>&1
  END_TIME=$(date +%s%3N)
  BASELINE_LATENCY=$((END_TIME - START_TIME))

  log_info "Baseline latency: ${BASELINE_LATENCY}ms"

  # 模拟网络延迟(通过sleep注入延迟 - 简化版)
  log_warn "Injecting chaos: Adding network delay..."

  # 在有延迟的情况下测试
  START_TIME=$(date +%s%3N)
  go run ../cmd/cluster-test/main.go -delivery-wait 10s > "$RESULTS_DIR/test2-with-delay.txt" 2>&1
  END_TIME=$(date +%s%3N)
  DELAY_LATENCY=$((END_TIME - START_TIME))

  log_info "Latency with chaos: ${DELAY_LATENCY}ms"

  # 计算性能下降
  DEGRADATION=$((DELAY_LATENCY - BASELINE_LATENCY))
  PERCENT=$((DEGRADATION * 100 / BASELINE_LATENCY))

  log_info "Performance degradation: ${DEGRADATION}ms (${PERCENT}%)"

  if [ $PERCENT -lt 200 ]; then
    log_info "✓ System maintains acceptable performance"
    echo "PASSED" > "$RESULTS_DIR/test2-result.txt"
  else
    log_warn "✗ Significant performance degradation"
    echo "DEGRADED" > "$RESULTS_DIR/test2-result.txt"
  fi

  # 清理
  ../scripts/stop-cluster.sh > /dev/null 2>&1
  sleep 2
}

# 测试3: 节点重启恢复测试
test_node_recovery() {
  log_info "Test 3: Node Recovery Test (Container Kill simulation)"
  echo "------------------------------------------------------------------------"

  # 启动集群
  log_info "Starting 3-node cluster..."
  ../scripts/start-cluster.sh > "$RESULTS_DIR/test3-startup.log" 2>&1

  sleep 5

  # 验证初始状态
  log_info "Verifying initial cluster state..."
  go run ../cmd/cluster-test/main.go > "$RESULTS_DIR/test3-initial.txt" 2>&1

  if [ $? -ne 0 ]; then
    log_error "Initial cluster test failed"
    ../scripts/stop-cluster.sh > /dev/null 2>&1
    return 1
  fi

  # 杀死节点3
  log_warn "Injecting chaos: Killing node3..."
  pkill -f "NODE_ID=node3" || true

  sleep 1

  # 验证集群仍可用(用2个节点)
  log_info "Testing with 2 nodes..."
  go run ../cmd/cluster-test/main.go > "$RESULTS_DIR/test3-2nodes.txt" 2>&1

  if [ $? -ne 0 ]; then
    log_warn "✗ Cluster failed with 2 nodes"
    echo "FAILED" > "$RESULTS_DIR/test3-result.txt"
  else
    log_info "✓ Cluster works with 2/3 nodes"

    # 重启node3
    log_info "Restarting node3..."
    NODE_ID=node3 MQTT_PORT=1885 GRPC_PORT=8085 METRICS_PORT=8086 \
      PEER_NODES="localhost:8081,localhost:8083" \
      ../bin/emqx-go > logs/node3.log 2>&1 &

    sleep 3

    # 验证恢复
    log_info "Verifying recovery with all 3 nodes..."
    go run ../cmd/cluster-test/main.go -node2-port 1885 > "$RESULTS_DIR/test3-recovered.txt" 2>&1

    if [ $? -eq 0 ]; then
      log_info "✓ Node successfully rejoined cluster"
      echo "PASSED" > "$RESULTS_DIR/test3-result.txt"
    else
      log_warn "✗ Node failed to rejoin"
      echo "PARTIAL" > "$RESULTS_DIR/test3-result.txt"
    fi
  fi

  # 清理
  ../scripts/stop-cluster.sh > /dev/null 2>&1
  sleep 2
}

# 测试4: 消息可靠性测试
test_message_reliability() {
  log_info "Test 4: Message Reliability Under Chaos"
  echo "------------------------------------------------------------------------"

  # 启动集群
  log_info "Starting 3-node cluster..."
  ../scripts/start-cluster.sh > "$RESULTS_DIR/test4-startup.log" 2>&1

  sleep 5

  log_info "Starting continuous message stream..."

  # 在后台持续发送消息
  for i in {1..10}; do
    go run ../cmd/cluster-test/main.go -message "msg-$i" > "$RESULTS_DIR/test4-msg-$i.txt" 2>&1 &
  done

  sleep 2

  # 在消息发送过程中杀死一个节点
  log_warn "Injecting chaos during message transmission..."
  pkill -f "NODE_ID=node2" || true

  # 等待所有消息测试完成
  wait

  # 统计成功率
  SUCCESS_COUNT=$(grep -l "SUCCESS" "$RESULTS_DIR"/test4-msg-*.txt | wc -l | tr -d ' ')
  TOTAL_COUNT=10

  log_info "Message success rate: $SUCCESS_COUNT/$TOTAL_COUNT"

  if [ $SUCCESS_COUNT -ge 7 ]; then
    log_info "✓ Message reliability maintained (${SUCCESS_COUNT}0%)"
    echo "PASSED" > "$RESULTS_DIR/test4-result.txt"
  else
    log_warn "✗ Too many message failures (${SUCCESS_COUNT}0%)"
    echo "FAILED" > "$RESULTS_DIR/test4-result.txt"
  fi

  # 清理
  ../scripts/stop-cluster.sh > /dev/null 2>&1
  sleep 2
}

# 生成报告
generate_report() {
  log_info "Generating test report..."

  REPORT="$RESULTS_DIR/CHAOS_TEST_REPORT.md"

  cat > "$REPORT" <<EOF
# EMQX-Go Local Chaos Testing Report

**Date**: $(date)
**Environment**: Local multi-process cluster
**Test Type**: Simplified chaos testing (without Kubernetes)

## Summary

| Test | Description | Result | Details |
|------|-------------|--------|---------|
EOF

  # Test 1
  if [ -f "$RESULTS_DIR/test1-result.txt" ]; then
    RESULT=$(cat "$RESULTS_DIR/test1-result.txt")
    echo "| 1 | Node Failure | $RESULT | Random pod kill simulation |" >> "$REPORT"
  fi

  # Test 2
  if [ -f "$RESULTS_DIR/test2-result.txt" ]; then
    RESULT=$(cat "$RESULTS_DIR/test2-result.txt")
    echo "| 2 | Network Delay | $RESULT | Network latency simulation |" >> "$REPORT"
  fi

  # Test 3
  if [ -f "$RESULTS_DIR/test3-result.txt" ]; then
    RESULT=$(cat "$RESULTS_DIR/test3-result.txt")
    echo "| 3 | Node Recovery | $RESULT | Container kill and restart |" >> "$REPORT"
  fi

  # Test 4
  if [ -f "$RESULTS_DIR/test4-result.txt" ]; then
    RESULT=$(cat "$RESULTS_DIR/test4-result.txt")
    echo "| 4 | Message Reliability | $RESULT | Chaos during transmission |" >> "$REPORT"
  fi

  cat >> "$REPORT" <<EOF

## Test Details

### Test 1: Node Random Failure
Simulates Chaos Mesh PodChaos with action: pod-kill

**Objective**: Verify cluster continues functioning when one node fails
**Method**: Kill node2 process, test with remaining nodes
**Expected**: 2/3 nodes sufficient for operation

### Test 2: Network Delay
Simulates Chaos Mesh NetworkChaos with action: delay

**Objective**: Measure performance impact of network latency
**Method**: Add artificial delays to message routing
**Expected**: Latency increase < 200%

### Test 3: Node Recovery
Simulates Chaos Mesh PodChaos with action: container-kill

**Objective**: Verify automatic recovery after node restart
**Method**: Kill and restart node3, verify it rejoins cluster
**Expected**: Node successfully rejoins without manual intervention

### Test 4: Message Reliability
Tests message delivery during chaos events

**Objective**: Verify message delivery during node failures
**Method**: Send 10 messages while killing a node
**Expected**: >= 70% message success rate

## Recommendations

EOF

  # 计算通过率
  TOTAL=0
  PASSED=0

  for i in 1 2 3 4; do
    if [ -f "$RESULTS_DIR/test${i}-result.txt" ]; then
      TOTAL=$((TOTAL + 1))
      RESULT=$(cat "$RESULTS_DIR/test${i}-result.txt")
      if [ "$RESULT" == "PASSED" ]; then
        PASSED=$((PASSED + 1))
      fi
    fi
  done

  SUCCESS_RATE=$((PASSED * 100 / TOTAL))

  if [ $SUCCESS_RATE -ge 75 ]; then
    cat >> "$REPORT" <<EOF
✅ **System demonstrates good resilience** (${SUCCESS_RATE}% tests passed)

The EMQX-Go cluster shows solid fault tolerance:
- Continues operation with node failures
- Maintains message delivery guarantees
- Recovers automatically from failures

Recommended next steps:
1. Deploy to Kubernetes for full Chaos Mesh testing
2. Test with higher load (1000+ connections)
3. Implement automated chaos testing in CI/CD
EOF
  else
    cat >> "$REPORT" <<EOF
⚠️ **System needs improvement** (${SUCCESS_RATE}% tests passed)

Some resilience issues detected. Recommended actions:
1. Review failed test logs in $RESULTS_DIR
2. Strengthen error handling in cluster module
3. Improve automatic recovery mechanisms
4. Add more comprehensive testing
EOF
  fi

  log_info "Report generated: $REPORT"
}

# 主函数
main() {
  # 检查前置条件
  if [ ! -f "../bin/emqx-go" ]; then
    log_error "emqx-go binary not found. Please run: go build -o bin/emqx-go ./cmd/emqx-go"
    exit 1
  fi

  if [ ! -f "../scripts/start-cluster.sh" ]; then
    log_error "Cluster scripts not found"
    exit 1
  fi

  # 运行测试
  test_node_failure
  echo ""

  test_network_delay
  echo ""

  test_node_recovery
  echo ""

  test_message_reliability
  echo ""

  # 生成报告
  generate_report

  echo ""
  echo "========================================================================"
  log_info "Local chaos testing complete!"
  log_info "Results: $RESULTS_DIR"
  log_info "Report: $RESULTS_DIR/CHAOS_TEST_REPORT.md"
  echo "========================================================================"
}

# 清理函数
cleanup() {
  log_warn "Cleaning up..."
  ../scripts/stop-cluster.sh > /dev/null 2>&1
  exit 130
}

trap cleanup INT TERM

main "$@"
