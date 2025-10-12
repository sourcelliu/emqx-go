# EMQX-Go 混沌测试使用指南

本指南说明如何使用EMQX-Go的内置混沌工程测试功能。

---

## 🎯 快速开始

### 1. 构建程序

```bash
# 构建主程序（包含混沌注入能力）
go build -o bin/emqx-go ./cmd/emqx-go

# 构建混沌测试工具
go build -o bin/chaos-test-runner ./tests/chaos-test-runner
```

### 2. 运行混沌测试

```bash
# 运行所有场景（默认60秒）
./bin/chaos-test-runner

# 指定测试时长
./bin/chaos-test-runner -duration 120

# 运行特定场景
./bin/chaos-test-runner -scenario network-delay -duration 30

# 详细输出
./bin/chaos-test-runner -verbose
```

### 3. 查看结果

测试完成后会生成两个文件：
- `chaos-test-report-YYYYMMDD-HHMMSS.md` - 详细测试报告
- `chaos-test-execution.log` - 执行日志

---

## 📋 可用的混沌场景

| 场景名称 | 描述 | 故障类型 |
|---------|------|---------|
| `network-delay` | 注入100ms网络延迟 | 网络故障 |
| `network-loss` | 注入10%丢包率 | 网络故障 |
| `high-network-loss` | 注入30%丢包率 | 网络故障 |
| `cpu-stress` | 注入80% CPU压力 | 资源压力 |
| `clock-skew` | 注入5秒时钟偏移 | 时间故障 |

---

## 🔧 编程式使用

### 在代码中启用混沌注入

```go
import "github.com/turtacn/emqx-go/pkg/chaos"

func main() {
    // 获取全局注入器
    injector := chaos.GetGlobalInjector()

    // 启用混沌注入
    injector.Enable()
    defer injector.Disable()

    // 注入网络延迟
    injector.InjectNetworkDelay(100 * time.Millisecond)

    // 注入丢包
    injector.InjectNetworkLoss(0.10)  // 10%

    // 注入CPU压力
    injector.InjectCPUStress(80)  // 80%
    ctx := context.Background()
    injector.StartCPUStress(ctx)

    // 运行你的测试...
}
```

### 在测试中使用

```go
func TestWithChaos(t *testing.T) {
    injector := chaos.GetGlobalInjector()
    injector.Enable()
    defer injector.Disable()

    // 模拟网络延迟
    injector.InjectNetworkDelay(50 * time.Millisecond)

    // 执行测试
    client := mqtt.NewClient(opts)
    token := client.Connect()

    // 验证系统在延迟下的行为
    assert.True(t, token.WaitTimeout(2*time.Second))
}
```

---

## 🎨 故障注入点

混沌故障在以下关键点自动注入：

### 1. 集群加入 (`pkg/cluster/client.go:Join`)
- 网络延迟
- 丢包模拟

**影响**: 节点加入集群的过程

### 2. 路由同步 (`pkg/cluster/client.go:BatchUpdateRoutes`)
- 网络延迟
- 丢包模拟

**影响**: 集群路由表同步

### 3. 消息转发 (`pkg/cluster/client.go:ForwardPublish`)
- 网络延迟
- 丢包模拟

**影响**: 跨节点消息传递

---

## 📊 测试输出示例

```
================================================================================
CHAOS TEST RESULTS
================================================================================

Scenario: network-delay
Status: ✓ PASS
Duration: 30.5s
Messages: Sent=300, Received=295, Lost=5
Average Latency: 120ms
Observations:
  - Message success rate: 98.33%
  - Average latency: 120ms

Scenario: network-loss
Status: ✓ PASS
Duration: 30.2s
Messages: Sent=300, Received=270, Lost=30
Observations:
  - Message success rate: 90.00%

================================================================================
Overall Success Rate: 2/2 (100.0%)
================================================================================
```

---

## 🛠️ 高级配置

### 自定义测试场景

编辑 `tests/chaos-test-runner/main.go`:

```go
scenarios := []TestScenario{
    {
        Name:        "custom-scenario",
        Description: "My custom chaos scenario",
        Duration:    60 * time.Second,
        Setup: func(i *chaos.Injector) error {
            i.Enable()
            // 自定义故障注入
            i.InjectNetworkDelay(200 * time.Millisecond)
            i.InjectCPUStress(90)
            return nil
        },
        Validate: func() error {
            // 自定义验证逻辑
            return nil
        },
        Cleanup: func(i *chaos.Injector) {
            i.Disable()
        },
    },
}
```

### 组合多种故障

```go
func (i *Injector) InjectComplexFailure() {
    i.Enable()
    i.InjectNetworkDelay(100 * time.Millisecond)  // 延迟
    i.InjectNetworkLoss(0.05)                      // 5%丢包
    i.InjectCPUStress(70)                          // 70% CPU
    i.InjectClockSkew(2 * time.Second)             // 2秒偏移
}
```

---

## 📈 性能监控

在混沌测试期间，可以监控系统指标：

```bash
# 在另一个终端监控Prometheus指标
watch -n 1 'curl -s http://localhost:8082/metrics | grep emqx_connections_count'

# 监控系统资源
top -pid $(pgrep emqx-go)

# 查看实时日志
tail -f logs/node*.log | grep CHAOS
```

---

## ⚠️ 注意事项

1. **仅用于测试环境**
   - 混沌注入会影响系统性能
   - 不要在生产环境启用

2. **故障恢复**
   - 测试工具会自动清理故障注入
   - 如果手动中断，需要重启集群

3. **资源消耗**
   - CPU压力测试会占用真实CPU
   - 建议在专用测试机器运行

4. **测试时长**
   - 建议每个场景至少运行30秒
   - 短时间测试可能无法观察到明显效果

---

## 🐛 故障排查

### 问题1: 测试快速失败

**症状**: 测试在几毫秒内结束
**原因**: MQTT客户端连接失败
**解决**:
```bash
# 确认集群正在运行
./scripts/health-check-simple.sh

# 检查端口是否监听
nc -z localhost 1883 1884 1885
```

### 问题2: 没有观察到故障效果

**症状**: 混沌日志显示故障注入，但无明显影响
**原因**: 测试时长太短或工作负载太轻
**解决**:
```bash
# 增加测试时长
./bin/chaos-test-runner -duration 120

# 增加并发（需要修改代码）
```

### 问题3: 集群无法恢复

**症状**: 测试后节点状态异常
**解决**:
```bash
# 停止并重启集群
./scripts/stop-cluster.sh
./scripts/start-cluster.sh

# 验证健康状态
./scripts/health-check-simple.sh
```

---

## 📚 相关文档

- [完整实施报告](./CHAOS_IMPLEMENTATION_REPORT.md) - 详细的混沌工程实施文档
- [混沌测试计划](./chaos.md) - 原始的Chaos Mesh测试计划
- [故障排查指南](./TROUBLESHOOTING.md) - 系统故障排查手册

---

## 🤝 贡献

欢迎添加更多混沌场景！步骤：

1. 在 `pkg/chaos/chaos.go` 添加新的故障类型
2. 在 `tests/chaos-test-runner/main.go` 添加测试场景
3. 更新本文档
4. 提交Pull Request

---

## 📞 获取帮助

如有问题，请查看：
- [GitHub Issues](https://github.com/your-org/emqx-go/issues)
- [完整文档](./README.md)

---

**最后更新**: 2025-10-11
**版本**: v1.0
