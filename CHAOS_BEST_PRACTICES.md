# EMQX-Go 混沌工程最佳实践

本文档总结了EMQX-Go项目混沌工程实践的最佳经验和建议。

---

## 📋 目录

- [混沌工程原则](#混沌工程原则)
- [测试设计指南](#测试设计指南)
- [故障注入模式](#故障注入模式)
- [性能基准建立](#性能基准建立)
- [渐进式测试](#渐进式测试)
- [生产环境准备](#生产环境准备)
- [自动化实践](#自动化实践)

---

## 🎯 混沌工程原则

### 1. 建立稳态假设 (Steady State Hypothesis)

在进行混沌实验前，明确定义系统的"正常状态"：

```yaml
稳态指标:
  - 消息延迟 P99 < 100ms
  - 消息成功率 > 99.9%
  - 集群可用性 = 100%
  - CPU使用率 < 80%
  - 内存使用率 < 75%
  - 连接成功率 > 99%
```

**实施方法**:
```bash
# 1. 运行baseline测试建立基准
./bin/chaos-test-runner -scenario baseline -duration 300

# 2. 记录关键指标
curl http://localhost:8082/metrics > baseline-metrics.txt

# 3. 定义可接受的偏差范围
# 例如：在混沌条件下，延迟可接受增加2x，成功率可接受降至95%
```

### 2. 假设稳态持续 (Hypothesis of Steady State)

混沌实验的核心假设：

> "即使在故障条件下，系统仍能保持可接受的服务水平"

**验证方法**:
```go
func ValidateSteadyState(metrics Metrics) error {
    if metrics.Latency.P99 > baseline.Latency.P99 * 2 {
        return fmt.Errorf("latency degraded beyond acceptable level")
    }
    if metrics.SuccessRate < 0.95 {
        return fmt.Errorf("success rate too low")
    }
    return nil
}
```

### 3. 变量化真实世界事件 (Vary Real-world Events)

注入的故障应该反映真实可能发生的问题：

**网络问题**:
- ✅ 100-500ms延迟 (跨区域网络)
- ✅ 5-30%丢包率 (网络拥塞)
- ✅ 网络分区 (交换机故障)

**资源压力**:
- ✅ 70-95% CPU (高负载)
- ✅ 80-95%内存 (内存泄漏)
- ✅ 磁盘I/O饱和

**时间问题**:
- ✅ 时钟偏移 1-10秒 (NTP同步失败)

**错误配置**:
- ✅ DNS解析失败
- ✅ 证书过期

### 4. 在生产环境运行 (Run Experiments in Production)

**阶段化方法**:

```
阶段1: 开发环境 (当前)
  - 代码级故障注入
  - 单个开发者的本地环境
  - 快速迭代和调试

阶段2: 测试环境
  - Kubernetes + Chaos Mesh
  - 持续运行的混沌实验
  - 自动化验证

阶段3: 预生产环境
  - 真实流量镜像
  - 完整的故障场景
  - Game Day演练

阶段4: 生产环境
  - 受控的混沌实验
  - 金丝雀部署
  - 实时监控和回滚
```

### 5. 自动化实验 (Automate Experiments)

```yaml
# .github/workflows/chaos-tests.yml
name: Chaos Tests
on:
  schedule:
    - cron: '0 2 * * *'  # 每天凌晨2点
  workflow_dispatch:

jobs:
  chaos:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v2
      - name: Run Chaos Tests
        run: |
          ./scripts/advanced-chaos-test.sh 60
          python3 scripts/analyze-chaos-results.py chaos-results-*
      - name: Upload Results
        uses: actions/upload-artifact@v2
        with:
          name: chaos-results
          path: chaos-results-*
```

### 6. 最小化爆炸半径 (Minimize Blast Radius)

**故障隔离策略**:
```
1. 单节点故障 → 验证集群容错
2. 单AZ故障 → 验证跨AZ恢复
3. 区域故障 → 验证多区域部署
4. 全系统故障 → 验证灾难恢复
```

**安全措施**:
```go
type ChaosGuard struct {
    MaxFailedNodes  int       // 最多故障节点数
    MaxDuration     time.Duration  // 最长持续时间
    AutoRollback    bool      // 自动回滚
    AlertThreshold  float64   // 告警阈值
}
```

---

## 🧪 测试设计指南

### 测试金字塔

```
           /\
          /极\
         /限测\
        /____试\         # 级联故障、长时间中断
       /       \
      / 压力测试 \        # 高负载、资源耗尽
     /__________\
    /            \
   /  故障注入测试 \      # 网络故障、服务故障
  /_______________\
 /                 \
/   基础可靠性测试    \    # 单点故障、重启恢复
/_____________________\

执行频率:
  基础测试: 每次提交
  故障注入: 每天
  压力测试: 每周
  极限测试: 每月
```

### 场景设计模板

```go
type ChaosScenario struct {
    // 基本信息
    Name        string
    Description string
    Category    string  // "network", "resource", "time", "combined"
    Severity    string  // "low", "medium", "high", "critical"

    // 执行配置
    Duration    time.Duration
    WarmupTime  time.Duration
    CooldownTime time.Duration

    // 故障配置
    FaultInjection func(*chaos.Injector) error

    // 验证配置
    SuccessCriteria []Criteria
    Observations    []string

    // 恢复配置
    RecoveryVerification func() error
    RollbackProcedure    func() error
}

// 示例：网络分区测试
var NetworkPartitionScenario = ChaosScenario{
    Name:     "network-partition",
    Category: "network",
    Severity: "high",
    Duration: 5 * time.Minute,
    WarmupTime: 30 * time.Second,
    CooldownTime: 30 * time.Second,

    FaultInjection: func(i *chaos.Injector) error {
        i.Enable()
        i.InjectNetworkPartition("node2")
        return nil
    },

    SuccessCriteria: []Criteria{
        {"cluster_available", ">", 0.66},  // 2/3节点仍可用
        {"message_loss_rate", "<", 0.05},  // 消息丢失<5%
        {"recovery_time", "<", 10},        // 10秒内恢复
    },

    RecoveryVerification: func() error {
        // 验证分区恢复后数据一致性
        return verifyDataConsistency()
    },
}
```

### 测试矩阵

| 维度 | 变量 | 测试值 |
|------|------|--------|
| **网络延迟** | latency | 0ms, 50ms, 100ms, 500ms, 1s |
| **丢包率** | loss_rate | 0%, 5%, 10%, 20%, 30% |
| **CPU负载** | cpu_usage | 50%, 70%, 80%, 90%, 95% |
| **内存压力** | memory_usage | 60%, 75%, 85%, 90%, 95% |
| **时钟偏移** | clock_skew | 0s, 1s, 5s, 10s, 30s |
| **并发连接** | connections | 10, 100, 1K, 10K, 100K |

---

## 🔧 故障注入模式

### 1. 单点故障模式

**目的**: 验证系统对单个组件失败的容错能力

```go
// 示例：单节点失败
func TestSingleNodeFailure(t *testing.T) {
    cluster := StartCluster(3)
    defer cluster.Stop()

    // 注入故障：杀死node2
    cluster.StopNode("node2")

    // 验证：集群仍可用
    assert.True(t, cluster.IsAvailable())

    // 验证：消息仍能路由
    publisher := NewPublisher(cluster.Node("node1"))
    subscriber := NewSubscriber(cluster.Node("node3"))
    assert.NoError(t, TestMessageDelivery(publisher, subscriber))

    // 验证：node2恢复后能重新加入
    cluster.StartNode("node2")
    time.Sleep(5 * time.Second)
    assert.Equal(t, 3, cluster.ActiveNodes())
}
```

### 2. 渐进式失败模式

**目的**: 测试系统在逐步恶化条件下的行为

```go
func TestProgressiveDegradation(t *testing.T) {
    injector := chaos.GetGlobalInjector()
    injector.Enable()

    // 阶段1: 轻微延迟 (50ms)
    injector.InjectNetworkDelay(50 * time.Millisecond)
    metrics1 := CollectMetrics(30 * time.Second)
    assert.True(t, metrics1.Latency.P99 < 100)

    // 阶段2: 中等延迟 (100ms)
    injector.InjectNetworkDelay(100 * time.Millisecond)
    metrics2 := CollectMetrics(30 * time.Second)
    assert.True(t, metrics2.Latency.P99 < 200)

    // 阶段3: 高延迟 (500ms)
    injector.InjectNetworkDelay(500 * time.Millisecond)
    metrics3 := CollectMetrics(30 * time.Second)
    assert.True(t, metrics3.SuccessRate > 0.95)  // 仍需>95%成功

    // 验证：系统graceful degradation
    assert.True(t, metrics1.Latency.P99 < metrics2.Latency.P99)
    assert.True(t, metrics2.Latency.P99 < metrics3.Latency.P99)
}
```

### 3. 级联故障模式

**目的**: 测试多个故障同时发生时的系统行为

```go
func TestCascadeFailure(t *testing.T) {
    injector := chaos.GetGlobalInjector()
    injector.Enable()

    // 同时注入多种故障
    injector.InjectNetworkDelay(200 * time.Millisecond)
    injector.InjectNetworkLoss(0.15)  // 15%
    injector.InjectCPUStress(80)

    // 启动CPU压力
    ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
    defer cancel()
    injector.StartCPUStress(ctx)

    // 测试
    workload := NewWorkload(1000)  // 1000 messages
    result := workload.Run()

    // 验证：即使多重故障，核心功能仍可用
    assert.True(t, result.SuccessRate > 0.90)  // >90%成功
    assert.True(t, result.Latency.P99 < 1000)  // <1s延迟
}
```

### 4. 恢复测试模式

**目的**: 验证故障恢复后的系统行为

```go
func TestRecovery(t *testing.T) {
    injector := chaos.GetGlobalInjector()

    // Phase 1: 正常状态
    baseline := CollectMetrics(30 * time.Second)

    // Phase 2: 注入故障
    injector.Enable()
    injector.InjectNetworkLoss(0.30)  // 30% loss
    faultyMetrics := CollectMetrics(30 * time.Second)

    // Phase 3: 移除故障
    injector.Disable()
    time.Sleep(10 * time.Second)  // 给系统恢复时间
    recoveredMetrics := CollectMetrics(30 * time.Second)

    // 验证恢复
    assert.InDelta(t, baseline.Latency.P99, recoveredMetrics.Latency.P99, 20)
    assert.InDelta(t, baseline.SuccessRate, recoveredMetrics.SuccessRate, 0.01)

    // 验证没有残留影响
    assert.Equal(t, 0, countStuckSessions())
    assert.Equal(t, 0, countOrphanedResources())
}
```

---

## 📊 性能基准建立

### Baseline测试流程

```bash
#!/bin/bash
# 建立性能基准

# 1. 环境准备
./scripts/start-cluster.sh
sleep 10

# 2. 预热系统
for i in {1..100}; do
    ./bin/cluster-test > /dev/null 2>&1
done

# 3. 收集基准数据
echo "Collecting baseline metrics..."
BASELINE_DIR="baseline-$(date +%Y%m%d-%H%M%S)"
mkdir -p "$BASELINE_DIR"

# 运行多次测试
for run in {1..10}; do
    echo "Run $run/10..."
    ./bin/cluster-test > "$BASELINE_DIR/run-$run.log"
    for port in 8082 8084 8086; do
        curl -s http://localhost:$port/metrics > "$BASELINE_DIR/metrics-$port-run$run.txt"
    done
    sleep 5
done

# 4. 计算统计数据
python3 << 'EOF'
import json
import statistics

latencies = []
success_rates = []

for run in range(1, 11):
    with open(f'baseline-*/run-{run}.log') as f:
        # 解析延迟和成功率
        pass

baseline = {
    'latency_p50': statistics.median(latencies),
    'latency_p95': statistics.quantiles(latencies, n=20)[18],
    'latency_p99': statistics.quantiles(latencies, n=100)[98],
    'success_rate_avg': statistics.mean(success_rates),
    'success_rate_min': min(success_rates),
}

with open('BASELINE.json', 'w') as f:
    json.dump(baseline, f, indent=2)

print(json.dumps(baseline, indent=2))
EOF

echo "Baseline established and saved to BASELINE.json"
```

### 性能对比方法

```python
def compare_with_baseline(current_metrics, baseline):
    """对比当前性能与基准"""
    report = {
        'latency_degradation': (current_metrics['p99'] / baseline['latency_p99'] - 1) * 100,
        'success_rate_drop': (baseline['success_rate_avg'] - current_metrics['success_rate']) * 100,
        'verdict': 'PASS'
    }

    # 判定标准
    if report['latency_degradation'] > 100:  # 延迟增加超过2x
        report['verdict'] = 'FAIL'
        report['reason'] = 'Latency degraded beyond acceptable level'

    if report['success_rate_drop'] > 5:  # 成功率下降超过5%
        report['verdict'] = 'FAIL'
        report['reason'] = 'Success rate dropped too much'

    return report
```

---

## 🚀 渐进式测试策略

### Week 1: 基础场景
```
- baseline (无故障)
- network-delay-50ms
- network-delay-100ms
- network-loss-5%
- network-loss-10%
```

### Week 2: 中等强度
```
- network-delay-200ms
- network-loss-20%
- cpu-stress-70%
- combined-network (delay + loss)
```

### Week 3: 高强度
```
- network-delay-500ms
- network-loss-30%
- cpu-stress-90%
- extreme scenarios
```

### Week 4: 极限测试
```
- cascade-failure
- multi-node-failure
- long-duration-chaos (24h)
- production-like-workload
```

---

## 🎯 生产环境准备

### 预上线检查清单

- [ ] 所有混沌场景通过
- [ ] 性能指标在可接受范围
- [ ] 监控和告警配置完成
- [ ] 回滚计划准备就绪
- [ ] On-call团队培训完成
- [ ] Game Day演练完成

### Game Day计划

```markdown
# EMQX-Go Game Day - 2025-Q1

**日期**: 2025-01-15 10:00-16:00
**参与者**: 开发团队 + 运维团队
**目标**: 验证生产环境故障响应能力

## 时间表

10:00-10:30 准备和系统检查
10:30-11:00 场景1: 单节点故障
11:00-11:30 场景2: 网络分区
11:30-12:00 复盘和修复

12:00-13:00 午餐

13:00-13:30 场景3: 数据库故障
13:30-14:00 场景4: 级联故障
14:00-15:00 紧急修复演练
15:00-16:00 总结和改进计划

## 成功标准

- RTO < 5分钟
- RPO = 0
- 无数据丢失
- 用户影响 < 5%
```

---

## 🤖 自动化实践

### CI/CD集成

```yaml
# .github/workflows/chaos-daily.yml
name: Daily Chaos Tests

on:
  schedule:
    - cron: '0 2 * * *'

jobs:
  chaos-tests:
    runs-on: ubuntu-latest
    strategy:
      matrix:
        scenario:
          - baseline
          - network-delay
          - network-loss
          - cpu-stress
          - combined

    steps:
      - uses: actions/checkout@v2

      - name: Setup Go
        uses: actions/setup-go@v2
        with:
          go-version: '1.21'

      - name: Build
        run: |
          go build -o bin/emqx-go ./cmd/emqx-go
          go build -o bin/chaos-test-runner ./tests/chaos-test-runner

      - name: Run Chaos Test
        run: |
          ./bin/chaos-test-runner \
            -scenario ${{ matrix.scenario }} \
            -duration 120

      - name: Analyze Results
        run: |
          python3 scripts/analyze-chaos-results.py chaos-results-*

      - name: Upload Results
        uses: actions/upload-artifact@v2
        with:
          name: chaos-${{ matrix.scenario }}
          path: chaos-results-*

      - name: Check Thresholds
        run: |
          python3 scripts/check-thresholds.py chaos-results-*/analysis.json
```

### 自动告警

```yaml
# alertmanager-rules.yml
groups:
  - name: chaos_testing
    rules:
      - alert: ChaosTestFailed
        expr: chaos_test_success_rate < 0.9
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "Chaos test success rate below 90%"
          description: "Current rate: {{ $value }}%"

      - alert: PerformanceDegraded
        expr: chaos_test_latency_p99 > baseline_latency_p99 * 3
        for: 5m
        labels:
          severity: critical
        annotations:
          summary: "Performance degraded 3x from baseline"
```

---

## 📚 参考资料

### 推荐阅读

1. **Chaos Engineering**: Principles and Practices - Casey Rosenthal & Nora Jones
2. **Site Reliability Engineering** - Google
3. **Release It!** - Michael Nygard

### 相关工具

- [Chaos Mesh](https://chaos-mesh.org/)
- [Gremlin](https://www.gremlin.com/)
- [Chaos Toolkit](https://chaostoolkit.org/)
- [LitmusChaos](https://litmuschaos.io/)

### EMQX-Go文档

- [混沌测试实施报告](./CHAOS_IMPLEMENTATION_REPORT.md)
- [混沌测试使用指南](./CHAOS_TESTING_GUIDE.md)
- [混沌测试执行总结](./CHAOS_EXECUTION_SUMMARY.md)

---

**版本**: v1.0
**最后更新**: 2025-10-12
**维护者**: EMQX-Go团队
