#!/bin/bash
# chaos/run-all-tests.sh
# 自动化运行所有混沌测试

set -e

CHAOS_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
RESULTS_DIR="chaos-results-$(date +%Y%m%d-%H%M%S)"

# 测试用例列表
CHAOS_FILES=(
  "01-pod-kill.yaml"
  "02-network-partition.yaml"
  "03-network-delay.yaml"
  "04-cpu-stress.yaml"
  "05-memory-stress.yaml"
  "06-network-loss.yaml"
  "07-io-stress.yaml"
  "08-time-skew.yaml"
  "09-dns-failure.yaml"
  "10-container-kill.yaml"
)

# 每个测试的等待时间(秒)
declare -A TEST_DURATION
TEST_DURATION["01-pod-kill.yaml"]=360
TEST_DURATION["02-network-partition.yaml"]=180
TEST_DURATION["03-network-delay.yaml"]=360
TEST_DURATION["04-cpu-stress.yaml"]=240
TEST_DURATION["05-memory-stress.yaml"]=240
TEST_DURATION["06-network-loss.yaml"]=360
TEST_DURATION["07-io-stress.yaml"]=240
TEST_DURATION["08-time-skew.yaml"]=180
TEST_DURATION["09-dns-failure.yaml"]=180
TEST_DURATION["10-container-kill.yaml"]=90

# 颜色输出
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# 日志函数
log_info() {
  echo -e "${GREEN}[INFO]${NC} $1"
}

log_warn() {
  echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
  echo -e "${RED}[ERROR]${NC} $1"
}

# 检查前置条件
check_prerequisites() {
  log_info "Checking prerequisites..."

  # 检查 kubectl
  if ! command -v kubectl &> /dev/null; then
    log_error "kubectl not found. Please install kubectl first."
    exit 1
  fi

  # 检查 Kubernetes 连接
  if ! kubectl cluster-info &> /dev/null; then
    log_error "Cannot connect to Kubernetes cluster"
    exit 1
  fi

  # 检查 Chaos Mesh 是否安装
  if ! kubectl get ns chaos-mesh &> /dev/null; then
    log_warn "Chaos Mesh namespace not found. Is Chaos Mesh installed?"
  fi

  # 检查 EMQX-Go 是否部署
  POD_COUNT=$(kubectl get pods -l app=emqx-go --no-headers 2>/dev/null | wc -l | tr -d ' ')
  if [ "$POD_COUNT" -eq 0 ]; then
    log_error "No EMQX-Go pods found. Please deploy EMQX-Go first."
    exit 1
  fi

  log_info "Prerequisites check passed"
}

# 收集基线指标
collect_baseline() {
  log_info "Collecting baseline metrics..."

  mkdir -p "$RESULTS_DIR/baseline"

  kubectl get pods -l app=emqx-go -o wide > "$RESULTS_DIR/baseline/pods.txt"
  kubectl get services -l app=emqx-go > "$RESULTS_DIR/baseline/services.txt"
  kubectl top pods -l app=emqx-go > "$RESULTS_DIR/baseline/resources.txt" 2>/dev/null || true
  kubectl logs -l app=emqx-go --tail=100 > "$RESULTS_DIR/baseline/logs.txt" 2>/dev/null || true

  log_info "Baseline metrics saved to $RESULTS_DIR/baseline/"
}

# 运行单个测试
run_test() {
  local chaos_file=$1
  local test_name=$(basename "$chaos_file" .yaml)
  local duration=${TEST_DURATION[$chaos_file]:-300}

  echo ""
  echo "========================================================================"
  log_info "Running test: $test_name"
  echo "========================================================================"

  local test_dir="$RESULTS_DIR/$test_name"
  mkdir -p "$test_dir"

  # 记录测试开始时间
  date > "$test_dir/start_time.txt"

  # 应用混沌实验
  log_info "Applying chaos experiment..."
  if kubectl apply -f "$CHAOS_DIR/$chaos_file" > "$test_dir/chaos_apply.txt" 2>&1; then
    log_info "Chaos experiment applied successfully"
  else
    log_error "Failed to apply chaos experiment"
    cat "$test_dir/chaos_apply.txt"
    return 1
  fi

  # 等待实验运行
  log_info "Waiting for test to complete (${duration}s)..."
  local elapsed=0
  local interval=30

  while [ $elapsed -lt $duration ]; do
    sleep $interval
    elapsed=$((elapsed + interval))

    # 定期收集状态
    if [ $((elapsed % 60)) -eq 0 ]; then
      log_info "Progress: ${elapsed}/${duration}s"
      kubectl get pods -l app=emqx-go --no-headers > "$test_dir/pods_${elapsed}s.txt" 2>&1
    fi
  done

  # 收集测试结果
  log_info "Collecting test results..."
  kubectl get pods -l app=emqx-go -o wide > "$test_dir/pods_final.txt"
  kubectl logs -l app=emqx-go --tail=500 --since=10m > "$test_dir/logs.txt" 2>&1
  kubectl describe pods -l app=emqx-go > "$test_dir/pods_describe.txt" 2>&1
  kubectl get events --sort-by='.lastTimestamp' > "$test_dir/events.txt" 2>&1
  kubectl top pods -l app=emqx-go > "$test_dir/resources.txt" 2>/dev/null || true

  # 获取混沌实验状态
  CHAOS_KIND=$(kubectl get -f "$CHAOS_DIR/$chaos_file" -o jsonpath='{.kind}' 2>/dev/null || echo "Unknown")
  kubectl get $CHAOS_KIND -o yaml > "$test_dir/chaos_status.yaml" 2>&1

  # 清理混沌实验
  log_info "Cleaning up chaos experiment..."
  kubectl delete -f "$CHAOS_DIR/$chaos_file" > "$test_dir/chaos_delete.txt" 2>&1

  # 等待系统稳定
  log_info "Waiting for system to stabilize (60s)..."
  sleep 60

  # 验证恢复
  log_info "Verifying recovery..."
  if "$CHAOS_DIR/verify-recovery.sh" > "$test_dir/recovery.txt" 2>&1; then
    log_info "✓ Recovery verification passed"
    echo "PASSED" > "$test_dir/result.txt"
    return 0
  else
    log_warn "✗ Recovery verification failed"
    echo "FAILED" > "$test_dir/result.txt"
    cat "$test_dir/recovery.txt"
    return 1
  fi

  # 记录测试结束时间
  date > "$test_dir/end_time.txt"
}

# 生成测试报告
generate_report() {
  log_info "Generating test report..."

  local report_file="$RESULTS_DIR/REPORT.md"

  cat > "$report_file" <<EOF
# EMQX-Go Chaos Testing Report

**Generated**: $(date)
**Test Suite**: All Chaos Tests

## Summary

| Test Case | Result | Duration |
|-----------|--------|----------|
EOF

  local passed=0
  local failed=0

  for chaos_file in "${CHAOS_FILES[@]}"; do
    local test_name=$(basename "$chaos_file" .yaml)
    local test_dir="$RESULTS_DIR/$test_name"

    if [ -f "$test_dir/result.txt" ]; then
      local result=$(cat "$test_dir/result.txt")
      local start_time=$(cat "$test_dir/start_time.txt" 2>/dev/null || echo "N/A")
      local end_time=$(cat "$test_dir/end_time.txt" 2>/dev/null || echo "N/A")

      echo "| $test_name | $result | $start_time - $end_time |" >> "$report_file"

      if [ "$result" == "PASSED" ]; then
        passed=$((passed + 1))
      else
        failed=$((failed + 1))
      fi
    else
      echo "| $test_name | SKIPPED | N/A |" >> "$report_file"
    fi
  done

  cat >> "$report_file" <<EOF

## Statistics

- **Total Tests**: ${#CHAOS_FILES[@]}
- **Passed**: $passed
- **Failed**: $failed
- **Success Rate**: $(awk "BEGIN {printf \"%.1f\", ($passed / ${#CHAOS_FILES[@]}) * 100}")%

## Recommendations

EOF

  if [ $failed -gt 0 ]; then
    cat >> "$report_file" <<EOF
### Failed Tests

Please review the following test cases:

EOF
    for chaos_file in "${CHAOS_FILES[@]}"; do
      local test_name=$(basename "$chaos_file" .yaml")
      local test_dir="$RESULTS_DIR/$test_name"
      if [ -f "$test_dir/result.txt" ] && [ "$(cat "$test_dir/result.txt")" == "FAILED" ]; then
        echo "- **$test_name**: Check \`$test_dir/recovery.txt\` for details" >> "$report_file"
      fi
    done
  else
    echo "✅ All tests passed! The system demonstrates excellent resilience." >> "$report_file"
  fi

  log_info "Report saved to: $report_file"
}

# 主函数
main() {
  echo "========================================================================"
  echo "EMQX-Go Chaos Testing Suite"
  echo "========================================================================"

  # 检查前置条件
  check_prerequisites

  # 创建结果目录
  mkdir -p "$RESULTS_DIR"
  log_info "Results will be saved to: $RESULTS_DIR"

  # 收集基线
  collect_baseline

  # 运行测试
  local test_count=0
  local passed_count=0

  for chaos_file in "${CHAOS_FILES[@]}"; do
    test_count=$((test_count + 1))
    log_info "Test $test_count/${#CHAOS_FILES[@]}: $chaos_file"

    if run_test "$chaos_file"; then
      passed_count=$((passed_count + 1))
    fi

    echo ""
  done

  # 生成报告
  generate_report

  # 打印总结
  echo ""
  echo "========================================================================"
  echo "Testing Complete"
  echo "========================================================================"
  log_info "Total Tests: $test_count"
  log_info "Passed: $passed_count"
  log_info "Failed: $((test_count - passed_count))"
  log_info "Results saved to: $RESULTS_DIR"

  if [ $passed_count -eq $test_count ]; then
    log_info "✓✓✓ All tests passed!"
    exit 0
  else
    log_warn "Some tests failed. Please review the report."
    exit 1
  fi
}

# 处理中断
trap 'log_error "Test interrupted"; exit 130' INT TERM

# 运行主函数
main "$@"
