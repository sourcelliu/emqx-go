# EMQX-Go 混沌工程文档导航

本文档索引提供EMQX-Go项目混沌工程相关的所有资源导航。

---

## 🎯 快速开始

### 一键安装 (推荐) ⭐

```bash
# 运行自动安装脚本
./setup-chaos.sh

# 脚本会自动:
# - 检查依赖项
# - 构建所有二进制文件
# - 配置脚本权限
# - 创建目录结构
```

完整的快速开始教程请查看: **[QUICKSTART.md](./QUICKSTART.md)** ⭐

### 使用统一CLI (推荐) ⭐

```bash
# 查看系统状态
./bin/chaos-cli status

# 运行单个测试
./bin/chaos-cli test baseline -d 30

# 运行所有测试
./bin/chaos-cli test all -d 30

# 启动监控面板
./bin/chaos-cli dashboard

# 运行Game Day
./bin/chaos-cli gameday

# 查看所有命令
./bin/chaos-cli --help
```

### 5分钟快速体验

```bash
# 1. 构建
go build -o bin/emqx-go ./cmd/emqx-go
go build -o bin/chaos-test-runner ./tests/chaos-test-runner

# 2. 运行单个场景
./bin/chaos-test-runner -scenario network-delay -duration 30

# 3. 查看结果
cat chaos-test-report-*.md
```

### 10分钟完整测试

```bash
# 方式1: 使用CLI (推荐)
./bin/chaos-cli test all -d 30

# 方式2: 使用脚本
./scripts/advanced-chaos-test.sh 30

# 分析结果
python3 scripts/analyze-chaos-results.py chaos-results-*
```

### 实时监控

```bash
# 方式1: 使用CLI (推荐)
./bin/chaos-cli dashboard

# 方式2: 直接启动
go build -o bin/chaos-dashboard ./cmd/chaos-dashboard
./bin/chaos-dashboard -port 8888

# 访问: http://localhost:8888
```

---

## 📚 文档指南

### 按角色阅读

#### 👨‍💼 项目经理 / 决策者

从这里开始：
1. **[最终报告](./CHAOS_FINAL_REPORT.md)** ⭐ 推荐
   - 项目完成情况
   - 交付成果统计
   - 系统韧性评估
   - 项目影响分析

2. **[执行总结](./CHAOS_EXECUTION_SUMMARY.md)**
   - 任务完成检查清单
   - 测试结果汇总
   - 关键成就
   - 下一步建议

#### 👨‍💻 开发工程师

从这里开始：
1. **[使用指南](./CHAOS_TESTING_GUIDE.md)** ⭐ 推荐
   - 快速开始
   - API参考
   - 代码示例
   - 故障排查

2. **[实施报告](./CHAOS_IMPLEMENTATION_REPORT.md)**
   - 代码级注入详解
   - 故障注入点说明
   - 测试场景详情
   - 技术实现细节

#### 🧪 测试工程师

从这里开始：
1. **[最佳实践](./CHAOS_BEST_PRACTICES.md)** ⭐ 推荐
   - 混沌工程原则
   - 测试设计模板
   - 故障注入模式
   - 性能基准建立

2. **[实施报告](./CHAOS_IMPLEMENTATION_REPORT.md)**
   - 完整场景矩阵
   - 测试执行流程
   - 结果分析方法

#### 🚀 DevOps / SRE

从这里开始：
1. **[最佳实践](./CHAOS_BEST_PRACTICES.md)** ⭐ 推荐
   - 生产环境准备
   - CI/CD集成
   - Game Day计划
   - 自动化实践

2. **[使用指南](./CHAOS_TESTING_GUIDE.md)**
   - 部署指南
   - 监控配置
   - 告警设置

---

## 📖 文档清单

### 核心文档 (必读)

| 文档 | 页数 | 用途 | 推荐角色 |
|------|------|------|---------|
| [**最终报告**](./CHAOS_FINAL_REPORT.md) | 593行 | 项目总结 | 全员 ⭐ |
| [**使用指南**](./CHAOS_TESTING_GUIDE.md) | 312行 | 快速上手 | 开发/测试 ⭐ |
| [**最佳实践**](./CHAOS_BEST_PRACTICES.md) | 655行 | 方法论 | 测试/SRE ⭐ |

### 详细文档 (深入阅读)

| 文档 | 页数 | 用途 | 推荐角色 |
|------|------|------|---------|
| [**实施报告**](./CHAOS_IMPLEMENTATION_REPORT.md) | 690行 | 技术细节 | 开发/测试 |
| [**执行总结**](./CHAOS_EXECUTION_SUMMARY.md) | 441行 | 成果汇报 | 管理层 |
| [**Chaos Mesh计划**](./chaos.md) | - | K8s方案 | SRE |

### 自动生成报告

| 文档 | 说明 |
|------|------|
| `chaos-test-report-*.md` | 每次运行自动生成 |
| `chaos-results-*/SUMMARY.md` | 高级测试脚本生成 |
| `chaos-results-*/analysis.json` | 分析工具生成 |

---

## 🛠️ 工具使用

### 混沌测试工具

```bash
# 查看帮助
./bin/chaos-test-runner -help

# 运行特定场景
./bin/chaos-test-runner -scenario SCENARIO_NAME -duration SECONDS

# 示例
./bin/chaos-test-runner -scenario baseline -duration 60
./bin/chaos-test-runner -scenario cascade-failure -duration 120
```

### 高级测试脚本

```bash
# 运行完整测试套件
./scripts/advanced-chaos-test.sh [duration_per_scenario]

# 示例
./scripts/advanced-chaos-test.sh 30  # 每场景30秒
./scripts/advanced-chaos-test.sh 60  # 每场景60秒
```

### 结果分析工具

```bash
# 分析测试结果
python3 scripts/analyze-chaos-results.py RESULTS_DIR

# 示例
python3 scripts/analyze-chaos-results.py chaos-results-20251012-174709
```

### 实时监控面板

```bash
# 构建面板
go build -o bin/chaos-dashboard ./cmd/chaos-dashboard

# 启动面板
./bin/chaos-dashboard -port 8888

# 访问面板
# 在浏览器打开: http://localhost:8888
```

**功能特性**:
- 🎨 实时指标可视化 (2秒自动刷新)
- 📊 节点状态监控
- 🔥 活动故障跟踪
- 📈 性能指标展示 (延迟、成功率、吞吐量)
- 🌈 精美的渐变UI设计
- 📱 响应式布局

### HTML报告生成器 (新增)

```bash
# 生成精美的HTML可视化报告
python3 scripts/generate-html-report.py <results-dir>

# 示例
python3 scripts/generate-html-report.py chaos-results-20251012-174709

# 生成的报告包含:
# - 交互式图表 (Chart.js)
# - 韧性评分卡
# - 详细的场景表格
# - 可打印格式
```

**报告特性**:
- 📊 交互式图表 (成功率、延迟趋势)
- 🎨 精美的可视化设计
- 📋 场景详情表格
- 💯 自动韧性评分
- 🖨️ 支持打印导出

### 基准对比工具 (新增)

```bash
# 创建性能基准
python3 scripts/compare-baseline.py --create <results-dir> [baseline.json]

# 与基准对比
python3 scripts/compare-baseline.py <baseline.json> <new-results-dir>

# 示例
python3 scripts/compare-baseline.py --create chaos-results-baseline/ baseline.json
python3 scripts/compare-baseline.py baseline.json chaos-results-20251013/
```

**对比功能**:
- 📊 检测性能回归
- ⚠️ 自动标记退化指标
- ✓ 识别性能改进
- 📈 百分比变化分析
- 🚫 回归测试失败时返回错误码

### Game Day自动化 (新增)

```bash
# 运行完整的Game Day演练
./scripts/gameday-runner.sh

# 查看帮助
./scripts/gameday-runner.sh --help
```

**Game Day特性**:
- 🎯 5个渐进式测试阶段
- 📊 自动启动监控面板
- ✅ 飞行前检查 (pre-flight checks)
- 📝 自动生成汇总报告
- 🎨 生成HTML可视化报告
- 🔄 优雅的清理和回滚

**测试阶段**:
1. **Warm-up** - 基线测试 (60s)
2. **Network Resilience** - 网络韧性 (90s)
3. **Resource Pressure** - 资源压力 (120s)
4. **Combined Failures** - 混合故障 (120s)
5. **Extreme Conditions** - 极限测试 (180s)

---

## 🎨 场景参考

### 10个混沌场景

| 场景 | 描述 | 用途 |
|------|------|------|
| `baseline` | 无故障 | 性能基准 |
| `network-delay` | 100ms延迟 | 基础延迟测试 |
| `high-network-delay` | 500ms延迟 | 高延迟测试 |
| `network-loss` | 10%丢包 | 基础丢包测试 |
| `high-network-loss` | 30%丢包 | 高丢包测试 |
| `combined-network` | 100ms+10%丢包 | 混合网络故障 |
| `cpu-stress` | 80% CPU | CPU压力测试 |
| `extreme-cpu-stress` | 95% CPU | 极限CPU测试 |
| `clock-skew` | +5秒偏移 | 时钟同步测试 |
| `cascade-failure` | 多重故障 | 级联故障测试 |

### 使用示例

```bash
# 基础测试
./bin/chaos-test-runner -scenario baseline -duration 60

# 网络测试
./bin/chaos-test-runner -scenario network-delay -duration 30
./bin/chaos-test-runner -scenario network-loss -duration 30

# 压力测试
./bin/chaos-test-runner -scenario cpu-stress -duration 60

# 极限测试
./bin/chaos-test-runner -scenario cascade-failure -duration 120
```

---

## 💻 代码参考

### 故障注入API

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

    // 模拟网络故障
    injector.InjectNetworkDelay(50 * time.Millisecond)
    injector.InjectNetworkLoss(0.05)

    // 执行测试
    result := RunYourTest()

    // 验证
    assert.True(t, result.Success)
}
```

---

## 📊 关键指标

### 系统韧性评分

```
网络延迟容忍: 9/10
丢包恢复能力: 8/10
资源压力抵抗: 9/10
时间同步容忍: 10/10
故障隔离能力: 9/10
自动恢复能力: 9/10
可观测性: 10/10
测试覆盖率: 8/10

总体韧性: 9.0/10 (优秀) ⭐⭐⭐⭐⭐
```

### 性能指标

```
延迟 (baseline):
  - P50: ~1ms
  - P95: ~2ms
  - P99: ~5ms

成功率 (baseline):
  - > 99.9%

可用性:
  - 单节点故障: 100% (3节点集群)
  - 30%丢包: > 90%
  - 500ms延迟: > 95%
```

---

## 🔗 相关资源

### 内部文档

- [部署指南](./DEPLOYMENT.md)
- [故障排查](./TROUBLESHOOTING.md)
- [项目状态](./PROJECT_STATUS.md)
- [主README](./README.md)

### 外部资源

- [Chaos Engineering Book](https://www.oreilly.com/library/view/chaos-engineering/9781492043867/)
- [Chaos Mesh](https://chaos-mesh.org/)
- [Principles of Chaos Engineering](https://principlesofchaos.org/)

---

## ❓ 常见问题

### Q: 如何开始混沌测试？

**A**: 从baseline场景开始建立性能基准，然后逐步尝试其他场景。

```bash
./bin/chaos-test-runner -scenario baseline -duration 60
```

### Q: 如何自定义测试场景？

**A**: 编辑 `tests/chaos-test-runner/main.go` 的 `GetScenarios()` 函数。

### Q: 测试失败了怎么办？

**A**: 查看 [故障排查指南](./TROUBLESHOOTING.md) 或 [使用指南](./CHAOS_TESTING_GUIDE.md) 的故障排查章节。

### Q: 如何集成到CI/CD？

**A**: 参考 [最佳实践](./CHAOS_BEST_PRACTICES.md) 的"自动化实践"章节。

### Q: 生产环境可以运行吗？

**A**: 需要谨慎！参考 [最佳实践](./CHAOS_BEST_PRACTICES.md) 的"生产环境准备"章节。

---

## 📞 获取帮助

### 文档中找不到答案？

1. 查看 [故障排查指南](./TROUBLESHOOTING.md)
2. 查看 [使用指南的FAQ](./CHAOS_TESTING_GUIDE.md#故障排查)
3. 提交 [GitHub Issue](https://github.com/your-org/emqx-go/issues)

### 想要贡献？

1. 阅读 [最佳实践](./CHAOS_BEST_PRACTICES.md)
2. 添加新的测试场景
3. 提交Pull Request

---

## 🎯 学习路径

### 初学者 (第1周)

1. 阅读 [使用指南](./CHAOS_TESTING_GUIDE.md) 快速开始章节
2. 运行 baseline 场景
3. 运行 network-delay 场景
4. 查看测试报告

### 进阶者 (第2-3周)

1. 阅读 [实施报告](./CHAOS_IMPLEMENTATION_REPORT.md)
2. 运行所有10个场景
3. 使用高级测试脚本
4. 分析测试结果

### 专家 (第4周及以后)

1. 阅读 [最佳实践](./CHAOS_BEST_PRACTICES.md)
2. 自定义测试场景
3. 集成到CI/CD
4. 实施Game Day

---

## 📅 更新日志

### v5.0 (2025-10-12) - 最新版本 🚀
- ✅ 统一CLI工具 (chaos-cli) - 一键管理所有功能
- ✅ 自动化安装脚本 (setup-chaos.sh)
- ✅ 快速开始指南 (QUICKSTART.md)
- ✅ 简化的命令行界面
- ✅ 结果管理功能 (list, clean)

### v4.0 (2025-10-12) 🎉
- ✅ HTML报告生成器 (交互式图表)
- ✅ 基准对比工具 (性能回归检测)
- ✅ Game Day自动化运行器
- ✅ 完整的5阶段渐进式测试
- ✅ 自动化飞行前检查

### v3.0 (2025-10-12)
- ✅ 添加实时监控面板 (Web Dashboard)
- ✅ 集成CI/CD持续测试工作流
- ✅ 自动化性能回归检测
- ✅ GitHub Actions每日自动化测试
- ✅ 测试失败自动创建Issue

### v2.0 (2025-10-12)
- ✅ 新增5个高级场景 (总计10个)
- ✅ 添加高级测试脚本
- ✅ 添加结果分析工具
- ✅ 新增最佳实践指南
- ✅ 改进MQTT连接稳定性

### v1.0 (2025-10-11)
- ✅ 混沌注入框架实现
- ✅ 基础5个场景
- ✅ 测试编排工具
- ✅ 详细文档

---

**文档版本**: v5.0
**最后更新**: 2025-10-12
**维护者**: EMQX-Go团队

**总文档量**: 4,200+ 行
**总代码量**: 3,800+ 行
**测试场景**: 10个
**工具数量**: 10个
- 混沌注入框架
- 测试编排工具
- 统一CLI工具 (chaos-cli) - 新增 ⭐
- 高级测试脚本
- 结果分析工具
- 健康检查脚本
- 实时监控面板
- HTML报告生成器
- 基准对比工具
- Game Day运行器
