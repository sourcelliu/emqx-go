# EMQX-Go 混沌工程测试计划

## 目录

1. [概述](#概述)
2. [测试目标](#测试目标)
3. [测试环境](#测试环境)
4. [Chaos Mesh 安装](#chaos-mesh-安装)
5. [测试场景](#测试场景)
6. [测试用例](#测试用例)
7. [监控与验证](#监控与验证)
8. [执行计划](#执行计划)
9. [故障恢复验证](#故障恢复验证)
10. [附录](#附录)

---

## 概述

### 什么是混沌工程？

混沌工程是一种通过在系统中主动注入故障来测试系统弹性和可靠性的方法。通过模拟真实世界中可能发生的各种故障场景，我们可以:

- 🔍 发现系统的薄弱环节
- 💪 验证容错机制的有效性
- 📊 建立系统可靠性基准
- 🛡️ 提高系统的整体韧性

### EMQX-Go 系统特点

EMQX-Go 是一个分布式 MQTT 消息代理,具有以下关键特性:

- **Actor 模型**: 使用 OTP 风格的 Actor 并发模型
- **分布式集群**: 支持多节点集群和自动服务发现
- **gRPC 通信**: 节点间使用 gRPC 进行消息路由和状态同步
- **Kubernetes 原生**: 支持 K8s 环境下的自动发现
- **会话管理**: 支持持久化会话和 QoS 保证

### 测试范围

本测试计划覆盖以下方面:

| 类别 | 测试内容 |
|------|---------|
| 🌐 **网络** | 网络延迟、丢包、分区、带宽限制 |
| 💻 **Pod** | Pod 故障、重启、删除 |
| 🖥️ **节点** | 节点故障、资源耗尽 |
| ⏰ **时间** | 时钟偏移、时间跳变 |
| 🔧 **压力** | CPU、内存、IO 压力 |
| ⚙️ **内核** | 系统调用故障 |

---

## 测试目标

### 主要目标

1. **可用性验证**
   - 验证集群在部分节点故障时能否继续提供服务
   - 验证消息路由的可靠性
   - 验证会话恢复机制

2. **性能影响评估**
   - 评估故障对消息延迟的影响
   - 评估故障对吞吐量的影响
   - 评估恢复时间目标 (RTO)

3. **故障恢复能力**
   - 验证 Actor Supervisor 的崩溃恢复
   - 验证集群自动重连机制
   - 验证数据一致性

4. **极限测试**
   - 确定系统的故障容忍阈值
   - 发现潜在的单点故障
   - 验证级联故障防护

### 成功标准

✅ **必须满足**:
- 单节点故障不影响整体服务可用性
- 消息零丢失(QoS 1/2)
- 会话状态正确恢复
- 集群自动恢复到健康状态

⚠️ **期望满足**:
- RTO < 30秒
- 故障期间消息延迟 < 500ms (P99)
- 网络分区恢复后数据一致

---

## 测试环境

### Kubernetes 集群要求

```yaml
最小配置:
  节点数: 3
  每节点:
    CPU: 2 cores
    内存: 4GB
    磁盘: 20GB
```

### EMQX-Go 部署配置

```yaml
集群配置:
  副本数: 3
  资源限制:
    requests:
      cpu: "500m"
      memory: "512Mi"
    limits:
      cpu: "1000m"
      memory: "1Gi"
```

### 监控组件

- **Prometheus**: 指标收集
- **Grafana**: 可视化监控
- **Loki**: 日志聚合(可选)

---

## Chaos Mesh 安装

### 1. 使用 Helm 安装

```bash
# 添加 Chaos Mesh 仓库
helm repo add chaos-mesh https://charts.chaos-mesh.org
helm repo update

# 创建命名空间
kubectl create ns chaos-mesh

# 安装 Chaos Mesh
helm install chaos-mesh chaos-mesh/chaos-mesh \
  --namespace=chaos-mesh \
  --set chaosDaemon.runtime=containerd \
  --set chaosDaemon.socketPath=/run/containerd/containerd.sock \
  --set dashboard.create=true \
  --version 2.6.0
```

### 2. 验证安装

```bash
# 检查 Chaos Mesh 组件状态
kubectl get pods -n chaos-mesh

# 访问 Chaos Dashboard (端口转发)
kubectl port-forward -n chaos-mesh svc/chaos-dashboard 2333:2333

# 浏览器访问: http://localhost:2333
```

### 3. 配置 RBAC

```bash
# 为测试命名空间创建角色绑定
kubectl apply -f - <<EOF
apiVersion: v1
kind: ServiceAccount
metadata:
  name: chaos-test-sa
  namespace: default
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: chaos-test-binding
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: chaos-mesh-cluster-manager
subjects:
- kind: ServiceAccount
  name: chaos-test-sa
  namespace: default
EOF
```

---

## 测试场景

### 场景分类

我们将混沌测试分为以下几类场景:

#### 🔴 高优先级场景 (P0)

这些场景模拟最常见、影响最大的故障:

1. **Pod 随机故障** - 模拟节点突然宕机
2. **网络分区** - 模拟脑裂场景
3. **网络延迟** - 模拟网络抖动
4. **CPU 压力** - 模拟高负载

#### 🟡 中优先级场景 (P1)

这些场景模拟次要但仍需关注的故障:

5. **内存压力** - 模拟内存泄漏
6. **网络丢包** - 模拟不稳定网络
7. **磁盘 IO 压力** - 模拟存储瓶颈
8. **时钟偏移** - 模拟时间同步问题

#### 🟢 低优先级场景 (P2)

这些场景模拟罕见但需要考虑的故障:

9. **DNS 故障** - 模拟服务发现问题
10. **系统调用故障** - 模拟内核异常
11. **容器重启** - 模拟 OOM Killer
12. **网络带宽限制** - 模拟拥塞

---

## 测试用例

### 测试用例 1: Pod Kill - 随机节点故障

**目标**: 验证集群在单节点突然故障时的弹性

**场景描述**:
- 随机杀死一个 EMQX-Go Pod
- 观察其他节点是否正常接管服务
- 验证客户端重连和消息路由

**Chaos 配置**:

```yaml
# chaos/01-pod-kill.yaml
apiVersion: chaos-mesh.org/v1alpha1
kind: PodChaos
metadata:
  name: emqx-pod-kill
  namespace: default
spec:
  action: pod-kill
  mode: one
  selector:
    namespaces:
      - default
    labelSelectors:
      app: emqx-go
  scheduler:
    cron: '@every 5m'
  duration: '10s'
```

**预期结果**:
- ✅ 其他节点继续服务
- ✅ 客户端自动重连到健康节点
- ✅ 消息路由正常
- ✅ Pod 自动重启并重新加入集群

**验证指标**:
```promql
# 集群可用性
sum(up{job="emqx-go"}) / count(up{job="emqx-go"}) >= 0.66

# 消息丢失率
rate(emqx_messages_dropped_total[1m]) == 0

# 重连成功率
rate(emqx_connections_success_total[1m]) > 0
```

**测试步骤**:

```bash
# 1. 部署 EMQX-Go 集群
kubectl apply -f k8s/statefulset.yaml

# 2. 启动测试负载
./scripts/start-test-load.sh

# 3. 应用混沌
kubectl apply -f chaos/01-pod-kill.yaml

# 4. 监控指标
kubectl port-forward svc/prometheus 9090:9090

# 5. 等待测试结束
sleep 600  # 10分钟

# 6. 清理
kubectl delete -f chaos/01-pod-kill.yaml
```

---

### 测试用例 2: Network Partition - 网络分区

**目标**: 验证集群在网络分区(脑裂)场景下的行为

**场景描述**:
- 将集群分为两个分区: Node1+Node2 vs Node3
- 模拟网络完全断开的情况
- 观察脑裂检测和恢复机制

**Chaos 配置**:

```yaml
# chaos/02-network-partition.yaml
apiVersion: chaos-mesh.org/v1alpha1
kind: NetworkChaos
metadata:
  name: emqx-network-partition
  namespace: default
spec:
  action: partition
  mode: all
  selector:
    namespaces:
      - default
    labelSelectors:
      app: emqx-go
  direction: both
  target:
    mode: one
    selector:
      namespaces:
        - default
      labelSelectors:
        app: emqx-go
      podPhaseSelectors:
        - Running
  duration: '2m'
```

**预期结果**:
- ✅ 多数派分区继续服务
- ✅ 少数派分区停止接受新连接
- ✅ 分区恢复后自动合并状态
- ⚠️ 可能出现短暂的消息重复(QoS 1)

**验证指标**:
```promql
# 检测分区
count(up{job="emqx-go"} == 1) < count(up{job="emqx-go"})

# 消息重复率
rate(emqx_messages_duplicate_total[1m])

# 集群状态不一致
emqx_cluster_inconsistent_nodes > 0
```

---

### 测试用例 3: Network Delay - 网络延迟

**目标**: 验证系统在高延迟网络环境下的性能

**场景描述**:
- 对集群节点间通信注入 100ms-500ms 延迟
- 模拟跨区域部署或网络拥塞
- 观察对消息延迟和吞吐量的影响

**Chaos 配置**:

```yaml
# chaos/03-network-delay.yaml
apiVersion: chaos-mesh.org/v1alpha1
kind: NetworkChaos
metadata:
  name: emqx-network-delay
  namespace: default
spec:
  action: delay
  mode: all
  selector:
    namespaces:
      - default
    labelSelectors:
      app: emqx-go
  delay:
    latency: "250ms"
    correlation: "50"
    jitter: "100ms"
  direction: both
  duration: '5m'
```

**预期结果**:
- ✅ 系统保持可用
- ⚠️ 消息延迟增加 (< 1s P99)
- ⚠️ 吞吐量下降 (>= 70% 正常水平)
- ✅ 无消息丢失

**验证指标**:
```promql
# 消息延迟 P99
histogram_quantile(0.99, rate(emqx_message_latency_bucket[1m]))

# 吞吐量
rate(emqx_messages_sent_total[1m])
```

---

### 测试用例 4: CPU Stress - CPU 压力测试

**目标**: 验证系统在 CPU 高负载下的行为

**场景描述**:
- 在一个节点上注入 80% CPU 压力
- 持续 3 分钟
- 观察 Actor 系统和消息处理的影响

**Chaos 配置**:

```yaml
# chaos/04-cpu-stress.yaml
apiVersion: chaos-mesh.org/v1alpha1
kind: StressChaos
metadata:
  name: emqx-cpu-stress
  namespace: default
spec:
  mode: one
  selector:
    namespaces:
      - default
    labelSelectors:
      app: emqx-go
  stressors:
    cpu:
      workers: 2
      load: 80
  duration: '3m'
```

**预期结果**:
- ✅ 系统保持响应
- ⚠️ 受影响节点的消息处理速度下降
- ✅ 其他节点正常工作
- ✅ CPU 压力消失后快速恢复

**验证指标**:
```promql
# CPU 使用率
rate(process_cpu_seconds_total{job="emqx-go"}[1m]) * 100

# Goroutine 数量
go_goroutines{job="emqx-go"}

# Actor 邮箱积压
emqx_actor_mailbox_size
```

---

### 测试用例 5: Memory Stress - 内存压力测试

**目标**: 验证系统在内存压力下的行为和 OOM 处理

**场景描述**:
- 在一个节点上分配大量内存
- 观察是否触发 OOM Killer
- 验证 Pod 重启后的恢复

**Chaos 配置**:

```yaml
# chaos/05-memory-stress.yaml
apiVersion: chaos-mesh.org/v1alpha1
kind: StressChaos
metadata:
  name: emqx-memory-stress
  namespace: default
spec:
  mode: one
  selector:
    namespaces:
      - default
    labelSelectors:
      app: emqx-go
  stressors:
    memory:
      workers: 4
      size: '512MB'
  duration: '3m'
```

**预期结果**:
- ✅ 内存使用触发 K8s 限制
- ✅ Pod 被 OOM Killer 终止后自动重启
- ✅ 重启后重新加入集群
- ✅ 持久化会话恢复

**验证指标**:
```promql
# 内存使用率
process_resident_memory_bytes{job="emqx-go"} / 1024 / 1024

# OOM 重启次数
kube_pod_container_status_restarts_total{pod=~"emqx-go.*"}

# 会话恢复
emqx_sessions_count
```

---

### 测试用例 6: Network Loss - 网络丢包

**目标**: 验证系统在丢包环境下的重传机制

**场景描述**:
- 注入 20% 网络丢包率
- 观察 TCP 重传和消息可靠性
- 验证 gRPC 连接的稳定性

**Chaos 配置**:

```yaml
# chaos/06-network-loss.yaml
apiVersion: chaos-mesh.org/v1alpha1
kind: NetworkChaos
metadata:
  name: emqx-network-loss
  namespace: default
spec:
  action: loss
  mode: all
  selector:
    namespaces:
      - default
    labelSelectors:
      app: emqx-go
  loss:
    loss: '20'
    correlation: '25'
  direction: both
  duration: '5m'
```

**预期结果**:
- ✅ TCP 层自动重传
- ✅ 消息最终送达
- ⚠️ 延迟增加
- ⚠️ 可能触发连接超时和重连

**验证指标**:
```promql
# TCP 重传率
rate(node_netstat_Tcp_RetransSegs[1m]) / rate(node_netstat_Tcp_OutSegs[1m])

# 连接断开
rate(emqx_connections_closed_total[1m])
```

---

### 测试用例 7: IO Stress - 磁盘 IO 压力

**目标**: 验证系统在磁盘 IO 压力下的性能

**场景描述**:
- 对节点注入高频磁盘读写
- 观察对会话持久化的影响
- 评估性能降级程度

**Chaos 配置**:

```yaml
# chaos/07-io-stress.yaml
apiVersion: chaos-mesh.org/v1alpha1
kind: IOChaos
metadata:
  name: emqx-io-stress
  namespace: default
spec:
  action: mixed-read-write
  mode: one
  selector:
    namespaces:
      - default
    labelSelectors:
      app: emqx-go
  volumePath: /data
  path: '/data/**/*'
  percent: 50
  duration: '3m'
  delay: '100ms'
```

**预期结果**:
- ✅ 系统保持可用
- ⚠️ 持久化操作延迟增加
- ⚠️ 整体吞吐量下降
- ✅ IO 压力消失后恢复正常

---

### 测试用例 8: Time Skew - 时钟偏移

**目标**: 验证系统对时间不同步的容忍度

**场景描述**:
- 将一个节点的时钟向前偏移 5 分钟
- 观察对超时判断和消息时间戳的影响
- 验证会话过期逻辑

**Chaos 配置**:

```yaml
# chaos/08-time-skew.yaml
apiVersion: chaos-mesh.org/v1alpha1
kind: TimeChaos
metadata:
  name: emqx-time-skew
  namespace: default
spec:
  mode: one
  selector:
    namespaces:
      - default
    labelSelectors:
      app: emqx-go
  timeOffset: '5m'
  clockIds:
    - CLOCK_REALTIME
  duration: '2m'
```

**预期结果**:
- ⚠️ 可能出现超时判断异常
- ⚠️ 消息时间戳不一致
- ✅ 不影响核心功能
- ✅ 时钟恢复后自动修正

---

### 测试用例 9: DNS Failure - DNS 故障

**目标**: 验证 DNS 解析失败对服务发现的影响

**场景描述**:
- 使 DNS 查询随机失败
- 观察节点发现和重连机制
- 验证 DNS 缓存的作用

**Chaos 配置**:

```yaml
# chaos/09-dns-failure.yaml
apiVersion: chaos-mesh.org/v1alpha1
kind: DNSChaos
metadata:
  name: emqx-dns-failure
  namespace: default
spec:
  action: random
  mode: all
  selector:
    namespaces:
      - default
    labelSelectors:
      app: emqx-go
  patterns:
    - emqx-go-*
    - '*.emqx-go-headless.default.svc.cluster.local'
  duration: '2m'
```

**预期结果**:
- ✅ 已建立的连接不受影响
- ⚠️ 新节点加入失败
- ✅ DNS 恢复后自动重试成功
- ⚠️ 可能影响 K8s 服务发现

---

### 测试用例 10: Container Kill - 容器级故障

**目标**: 验证容器层面故障的恢复能力

**场景描述**:
- 杀死容器进程但不删除 Pod
- 观察 K8s 重启容器
- 验证快速恢复

**Chaos 配置**:

```yaml
# chaos/10-container-kill.yaml
apiVersion: chaos-mesh.org/v1alpha1
kind: PodChaos
metadata:
  name: emqx-container-kill
  namespace: default
spec:
  action: container-kill
  mode: fixed
  value: '1'
  selector:
    namespaces:
      - default
    labelSelectors:
      app: emqx-go
  containerNames:
    - emqx-go
  duration: '30s'
```

**预期结果**:
- ✅ 容器自动重启
- ✅ 重启时间 < 10s
- ✅ 状态快速恢复
- ✅ 最小化服务中断

---

## 监控与验证

### 关键指标

#### 1. 可用性指标

```promql
# 集群整体可用性 (至少 2/3 节点在线)
(sum(up{job="emqx-go"}) / count(up{job="emqx-go"})) >= 0.66

# 服务响应成功率
sum(rate(http_requests_total{status="200"}[1m]))
/
sum(rate(http_requests_total[1m])) > 0.99
```

#### 2. 性能指标

```promql
# 消息延迟 P50, P95, P99
histogram_quantile(0.50, rate(emqx_message_latency_bucket[1m]))
histogram_quantile(0.95, rate(emqx_message_latency_bucket[1m]))
histogram_quantile(0.99, rate(emqx_message_latency_bucket[1m]))

# 消息吞吐量
sum(rate(emqx_messages_sent_total[1m]))
sum(rate(emqx_messages_received_total[1m]))
```

#### 3. 可靠性指标

```promql
# 消息丢失率 (应该为 0)
rate(emqx_messages_dropped_total[1m])

# 连接断开率
rate(emqx_connections_closed_total{reason="error"}[1m])

# Actor 重启次数
rate(emqx_actor_restarts_total[1m])
```

#### 4. 资源指标

```promql
# CPU 使用率
rate(process_cpu_seconds_total{job="emqx-go"}[1m]) * 100

# 内存使用
process_resident_memory_bytes{job="emqx-go"} / 1024 / 1024

# Goroutine 数量
go_goroutines{job="emqx-go"}

# GC 暂停时间
rate(go_gc_duration_seconds_sum[1m])
```

### Grafana 仪表板

创建一个专门的混沌测试仪表板:

```json
{
  "dashboard": {
    "title": "EMQX-Go Chaos Testing",
    "panels": [
      {
        "title": "Cluster Health",
        "targets": [
          {"expr": "sum(up{job=\"emqx-go\"})"}
        ]
      },
      {
        "title": "Message Latency (P99)",
        "targets": [
          {"expr": "histogram_quantile(0.99, rate(emqx_message_latency_bucket[1m]))"}
        ]
      },
      {
        "title": "Message Loss Rate",
        "targets": [
          {"expr": "rate(emqx_messages_dropped_total[1m])"}
        ]
      },
      {
        "title": "Actor Restarts",
        "targets": [
          {"expr": "rate(emqx_actor_restarts_total[1m])"}
        ]
      }
    ]
  }
}
```

### 日志聚合

使用 Loki 收集和查询日志:

```bash
# 查询故障期间的错误日志
{app="emqx-go"} |= "error" |= "chaos"

# 查询 Actor 重启日志
{app="emqx-go"} |= "Actor.*restarted"

# 查询集群状态变化
{app="emqx-go"} |= "cluster" |= "join\|leave\|partition"
```

---

## 执行计划

### 测试阶段

#### 阶段 1: 准备阶段 (Day 1-2)

**目标**: 搭建测试环境

- [ ] 部署 Kubernetes 集群
- [ ] 安装 Chaos Mesh
- [ ] 部署 EMQX-Go (3 节点)
- [ ] 配置 Prometheus + Grafana
- [ ] 准备测试脚本和负载生成器
- [ ] 建立基线指标

#### 阶段 2: 冒烟测试 (Day 3)

**目标**: 验证测试基础设施

- [ ] 运行测试用例 1 (Pod Kill) - 短时间
- [ ] 验证监控系统工作正常
- [ ] 验证指标收集完整
- [ ] 调整测试参数

#### 阶段 3: P0 测试 (Day 4-6)

**目标**: 执行高优先级测试

- [ ] 测试用例 1: Pod Kill
- [ ] 测试用例 2: Network Partition
- [ ] 测试用例 3: Network Delay
- [ ] 测试用例 4: CPU Stress
- [ ] 收集和分析结果
- [ ] 修复发现的问题

#### 阶段 4: P1 测试 (Day 7-9)

**目标**: 执行中优先级测试

- [ ] 测试用例 5: Memory Stress
- [ ] 测试用例 6: Network Loss
- [ ] 测试用例 7: IO Stress
- [ ] 测试用例 8: Time Skew
- [ ] 收集和分析结果

#### 阶段 5: P2 测试 (Day 10-11)

**目标**: 执行低优先级测试

- [ ] 测试用例 9: DNS Failure
- [ ] 测试用例 10: Container Kill
- [ ] 组合场景测试
- [ ] 极限测试

#### 阶段 6: 总结阶段 (Day 12-14)

**目标**: 整理结果和改进

- [ ] 汇总所有测试结果
- [ ] 编写测试报告
- [ ] 优先级排序发现的问题
- [ ] 制定改进计划
- [ ] 更新文档

### 测试时间表

```
Week 1:
  Mon-Tue:  环境准备
  Wed:      冒烟测试
  Thu-Fri:  P0 测试

Week 2:
  Mon-Wed:  P1 测试
  Thu:      P2 测试
  Fri:      总结报告
```

### 负载模式

在混沌测试期间,保持以下负载:

```yaml
测试负载:
  MQTT 客户端: 1000 个连接
  消息发布速率: 100 msg/s
  订阅主题数: 50 个
  消息大小: 256 bytes
  QoS 分布:
    QoS 0: 40%
    QoS 1: 50%
    QoS 2: 10%
```

---

## 故障恢复验证

### 自动恢复检查清单

每个测试用例都应验证以下恢复指标:

#### ✅ 集群级别

- [ ] **集群成员**: 所有节点重新加入集群
- [ ] **路由表**: 订阅路由正确同步
- [ ] **健康检查**: 所有健康检查通过
- [ ] **性能指标**: 恢复到基线的 90%+

#### ✅ 节点级别

- [ ] **进程状态**: 所有进程正常运行
- [ ] **端口监听**: MQTT/gRPC 端口正常监听
- [ ] **资源使用**: CPU/内存在正常范围
- [ ] **日志正常**: 无持续错误日志

#### ✅ 应用级别

- [ ] **连接状态**: 客户端成功重连
- [ ] **会话恢复**: 持久化会话正确恢复
- [ ] **消息路由**: 消息正确路由到订阅者
- [ ] **QoS 保证**: QoS 1/2 消息不丢失

#### ✅ 数据级别

- [ ] **消息完整性**: 无消息丢失
- [ ] **消息顺序**: 单客户端消息顺序正确
- [ ] **会话状态**: 订阅信息正确保存
- [ ] **Retained 消息**: Retained 消息正确保留

### 恢复时间目标 (RTO)

| 故障类型 | 目标 RTO | 可接受 RTO |
|---------|---------|-----------|
| Pod Kill | < 10s | < 30s |
| Container Kill | < 5s | < 15s |
| Network Partition | < 30s | < 60s |
| 节点故障 | < 60s | < 120s |

### 恢复验证脚本

```bash
#!/bin/bash
# chaos/verify-recovery.sh

echo "=== Verifying EMQX-Go Recovery ==="

# 1. 检查所有 Pod 运行
echo "Checking Pod status..."
READY_PODS=$(kubectl get pods -l app=emqx-go -o json | \
  jq '[.items[] | select(.status.phase=="Running")] | length')
TOTAL_PODS=$(kubectl get pods -l app=emqx-go --no-headers | wc -l)

if [ "$READY_PODS" -eq "$TOTAL_PODS" ]; then
  echo "✓ All pods are running ($READY_PODS/$TOTAL_PODS)"
else
  echo "✗ Some pods are not running ($READY_PODS/$TOTAL_PODS)"
  exit 1
fi

# 2. 检查服务端口
echo "Checking service endpoints..."
MQTT_ENDPOINTS=$(kubectl get endpoints emqx-go-mqtt -o json | \
  jq '.subsets[].addresses | length')

if [ "$MQTT_ENDPOINTS" -ge 2 ]; then
  echo "✓ MQTT endpoints available: $MQTT_ENDPOINTS"
else
  echo "✗ Insufficient MQTT endpoints: $MQTT_ENDPOINTS"
  exit 1
fi

# 3. 测试 MQTT 连接
echo "Testing MQTT connectivity..."
timeout 10 mosquitto_pub -h localhost -p 1883 \
  -t "chaos/test" -m "recovery-test" -q 1 \
  -u test -P test

if [ $? -eq 0 ]; then
  echo "✓ MQTT publish successful"
else
  echo "✗ MQTT publish failed"
  exit 1
fi

# 4. 检查 Prometheus 指标
echo "Checking Prometheus metrics..."
METRICS=$(curl -s http://localhost:9090/api/v1/query \
  --data-urlencode 'query=up{job="emqx-go"}' | \
  jq '.data.result | length')

if [ "$METRICS" -ge 2 ]; then
  echo "✓ Prometheus metrics available: $METRICS"
else
  echo "✗ Insufficient metrics: $METRICS"
  exit 1
fi

# 5. 检查错误日志
echo "Checking for error logs..."
ERROR_COUNT=$(kubectl logs -l app=emqx-go --tail=100 | \
  grep -i "error\|fatal\|panic" | wc -l)

if [ "$ERROR_COUNT" -lt 5 ]; then
  echo "✓ Low error count: $ERROR_COUNT"
else
  echo "⚠ High error count: $ERROR_COUNT (review logs)"
fi

echo ""
echo "=== Recovery Verification Complete ==="
echo "✓ All checks passed"
```

---

## 附录

### A. 快速开始指南

```bash
# 1. 克隆仓库
git clone https://github.com/turtacn/emqx-go.git
cd emqx-go

# 2. 创建混沌测试目录
mkdir -p chaos

# 3. 保存所有 chaos yaml 文件到 chaos/ 目录

# 4. 部署 EMQX-Go
kubectl apply -f k8s/

# 5. 等待 Pod 就绪
kubectl wait --for=condition=ready pod -l app=emqx-go --timeout=300s

# 6. 运行第一个混沌测试
kubectl apply -f chaos/01-pod-kill.yaml

# 7. 监控测试
kubectl get podchaos -w

# 8. 查看结果
kubectl logs -l app=emqx-go --tail=100

# 9. 清理
kubectl delete -f chaos/01-pod-kill.yaml
```

### B. 故障排查

#### 问题: Chaos 实验未生效

**原因**:
- RBAC 权限不足
- 选择器匹配不到 Pod
- Chaos Daemon 未运行

**解决方案**:
```bash
# 检查 Chaos Mesh 状态
kubectl get pods -n chaos-mesh

# 查看实验状态
kubectl describe podchaos emqx-pod-kill

# 检查事件
kubectl get events --sort-by='.lastTimestamp'
```

#### 问题: 测试后系统未恢复

**原因**:
- Duration 设置过长
- Pod 重启失败
- 资源配额限制

**解决方案**:
```bash
# 手动删除 Chaos 实验
kubectl delete podchaos --all
kubectl delete networkchaos --all
kubectl delete stresschaos --all

# 重启 Pod
kubectl rollout restart statefulset emqx-go

# 检查资源
kubectl describe node
```

#### 问题: 监控指标缺失

**原因**:
- ServiceMonitor 配置错误
- Prometheus 未抓取指标
- 网络策略阻止

**解决方案**:
```bash
# 检查 Prometheus targets
kubectl port-forward -n monitoring svc/prometheus 9090:9090
# 访问 http://localhost:9090/targets

# 测试指标端点
kubectl exec -it emqx-go-0 -- curl localhost:8082/metrics

# 检查 ServiceMonitor
kubectl get servicemonitor -A
```

### C. 测试负载生成器

创建一个简单的 MQTT 负载生成器:

```go
// chaos/load-generator/main.go
package main

import (
    "fmt"
    "log"
    "sync"
    "time"

    mqtt "github.com/eclipse/paho.mqtt.golang"
)

type LoadConfig struct {
    Broker      string
    ClientCount int
    MsgRate     int // messages per second
    Topics      []string
}

func main() {
    config := LoadConfig{
        Broker:      "tcp://localhost:1883",
        ClientCount: 100,
        MsgRate:     10,
        Topics:      []string{"test/topic1", "test/topic2", "test/topic3"},
    }

    var wg sync.WaitGroup

    // 启动订阅者
    for i := 0; i < config.ClientCount/2; i++ {
        wg.Add(1)
        go startSubscriber(&wg, config, i)
    }

    // 启动发布者
    for i := 0; i < config.ClientCount/2; i++ {
        wg.Add(1)
        go startPublisher(&wg, config, i)
    }

    wg.Wait()
}

func startSubscriber(wg *sync.WaitGroup, config LoadConfig, id int) {
    defer wg.Done()

    opts := mqtt.NewClientOptions()
    opts.AddBroker(config.Broker)
    opts.SetClientID(fmt.Sprintf("sub-%d", id))
    opts.SetUsername("test")
    opts.SetPassword("test")
    opts.SetAutoReconnect(true)

    client := mqtt.NewClient(opts)
    if token := client.Connect(); token.Wait() && token.Error() != nil {
        log.Printf("Subscriber %d failed to connect: %v", id, token.Error())
        return
    }

    for _, topic := range config.Topics {
        if token := client.Subscribe(topic, 1, nil); token.Wait() && token.Error() != nil {
            log.Printf("Subscriber %d failed to subscribe to %s: %v", id, topic, token.Error())
        }
    }

    log.Printf("Subscriber %d connected and subscribed", id)

    // 保持运行
    select {}
}

func startPublisher(wg *sync.WaitGroup, config LoadConfig, id int) {
    defer wg.Done()

    opts := mqtt.NewClientOptions()
    opts.AddBroker(config.Broker)
    opts.SetClientID(fmt.Sprintf("pub-%d", id))
    opts.SetUsername("test")
    opts.SetPassword("test")
    opts.SetAutoReconnect(true)

    client := mqtt.NewClient(opts)
    if token := client.Connect(); token.Wait() && token.Error() != nil {
        log.Printf("Publisher %d failed to connect: %v", id, token.Error())
        return
    }

    log.Printf("Publisher %d connected", id)

    ticker := time.NewTicker(time.Second / time.Duration(config.MsgRate))
    defer ticker.Stop()

    msgCount := 0
    for range ticker.C {
        topic := config.Topics[msgCount%len(config.Topics)]
        payload := fmt.Sprintf("msg-%d-%d-%d", id, msgCount, time.Now().Unix())

        token := client.Publish(topic, 1, false, payload)
        if token.Wait() && token.Error() != nil {
            log.Printf("Publisher %d failed to publish: %v", id, token.Error())
        }

        msgCount++
    }
}
```

### D. 自动化测试脚本

```bash
#!/bin/bash
# chaos/run-all-tests.sh

set -e

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

RESULTS_DIR="chaos-results-$(date +%Y%m%d-%H%M%S)"
mkdir -p "$RESULTS_DIR"

echo "=== Starting Chaos Testing Suite ==="
echo "Results will be saved to: $RESULTS_DIR"

for chaos_file in "${CHAOS_FILES[@]}"; do
  TEST_NAME=$(basename "$chaos_file" .yaml)
  echo ""
  echo "========================================"
  echo "Running: $TEST_NAME"
  echo "========================================"

  # 应用混沌
  kubectl apply -f "chaos/$chaos_file"

  # 等待测试完成 (根据 duration + buffer)
  sleep 420  # 7分钟

  # 收集结果
  echo "Collecting metrics..."
  kubectl get pods -l app=emqx-go -o wide > "$RESULTS_DIR/${TEST_NAME}-pods.txt"
  kubectl logs -l app=emqx-go --tail=500 > "$RESULTS_DIR/${TEST_NAME}-logs.txt"

  # 验证恢复
  echo "Verifying recovery..."
  ./chaos/verify-recovery.sh > "$RESULTS_DIR/${TEST_NAME}-recovery.txt" 2>&1

  if [ $? -eq 0 ]; then
    echo "✓ Test passed"
  else
    echo "✗ Test failed - check logs"
  fi

  # 清理
  kubectl delete -f "chaos/$chaos_file"

  # 等待系统稳定
  sleep 60
done

echo ""
echo "=== Chaos Testing Complete ==="
echo "Results saved to: $RESULTS_DIR"
```

### E. 参考资料

#### 官方文档

- [Chaos Mesh Documentation](https://chaos-mesh.org/docs/)
- [Kubernetes Best Practices](https://kubernetes.io/docs/concepts/cluster-administration/manage-deployment/)
- [MQTT Protocol Specification](https://mqtt.org/mqtt-specification/)

#### 相关文章

- [Principles of Chaos Engineering](https://principlesofchaos.org/)
- [Testing Microservices with Chaos Engineering](https://www.oreilly.com/library/view/chaos-engineering/9781492043850/)

#### 工具

- [Prometheus](https://prometheus.io/) - 监控和告警
- [Grafana](https://grafana.com/) - 可视化
- [MQTTX](https://mqttx.app/) - MQTT 客户端
- [K9s](https://k9scli.io/) - Kubernetes CLI

---

## 总结

本混沌测试计划为 EMQX-Go 系统提供了一个全面的弹性测试框架。通过系统性地注入各种故障,我们可以:

1. **验证系统的容错能力** - 确保在故障情况下系统仍然可用
2. **发现潜在问题** - 在生产环境出现问题之前发现薄弱环节
3. **建立信心** - 通过测试证明系统的可靠性
4. **持续改进** - 基于测试结果不断优化系统架构

### 下一步行动

1. ✅ 按照本文档部署测试环境
2. ✅ 执行冒烟测试验证基础设施
3. ✅ 按优先级执行测试用例
4. ✅ 分析结果并修复发现的问题
5. ✅ 将混沌测试集成到 CI/CD 流程

### 持续迭代

混沌工程是一个持续的过程,建议:

- **定期执行**: 每月执行一次完整测试套件
- **新场景**: 根据生产事故补充新的测试场景
- **自动化**: 逐步实现测试的完全自动化
- **文化建设**: 在团队中推广弹性工程文化

---

**文档版本**: v1.0
**最后更新**: 2025-10-10
**维护者**: EMQX-Go Team
**许可证**: Apache 2.0
