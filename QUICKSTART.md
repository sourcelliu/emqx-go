# EMQX-Go Chaos Engineering - Quick Start

快速上手EMQX-Go混沌工程测试系统。

## 🚀 一键安装

```bash
# 克隆项目（如果还没有）
git clone https://github.com/your-org/emqx-go.git
cd emqx-go

# 运行安装脚本
./setup-chaos.sh
```

安装脚本将自动：
- ✅ 检查依赖项（Go, Python3, Git）
- ✅ 构建所有二进制文件
- ✅ 配置脚本权限
- ✅ 创建必要的目录

## 📋 系统要求

- **Go**: 1.21+
- **Python**: 3.8+
- **操作系统**: Linux, macOS, Windows (WSL)
- **内存**: 4GB+
- **磁盘**: 1GB+

可选：
- **Docker**: 用于Kubernetes部署
- **Kubernetes**: 用于Chaos Mesh集成

## 🎯 5分钟快速体验

### 方式1: 使用CLI工具 (推荐)

```bash
# 1. 检查系统状态
./bin/chaos-cli status

# 2. 运行单个测试
./bin/chaos-cli test baseline -d 30

# 3. 查看结果
cat chaos-test-report-baseline.md
```

### 方式2: 直接使用测试工具

```bash
# 1. 构建
go build -o bin/chaos-test-runner ./tests/chaos-test-runner

# 2. 运行
./bin/chaos-test-runner -scenario baseline -duration 30

# 3. 查看结果
cat chaos-test-report-baseline.md
```

## 🔥 常用命令

### 运行测试

```bash
# 运行单个场景
./bin/chaos-cli test network-delay -d 30

# 运行所有场景
./bin/chaos-cli test all -d 30

# 运行特定场景
./bin/chaos-cli test cascade-failure -d 60
```

### 启动监控面板

```bash
# 启动Web监控面板
./bin/chaos-cli dashboard

# 指定端口
./bin/chaos-cli dashboard -p 9999

# 然后访问: http://localhost:8888
```

### Game Day演练

```bash
# 运行完整的Game Day（约10分钟）
./bin/chaos-cli gameday
```

### 生成报告

```bash
# 生成HTML可视化报告
./bin/chaos-cli report chaos-results-20251012/

# 报告将保存为: chaos-results-20251012/chaos-test-report.html
```

### 性能基准对比

```bash
# 创建基准
./bin/chaos-cli baseline create chaos-results-baseline/ baseline.json

# 对比新结果
./bin/chaos-cli baseline compare baseline.json chaos-results-20251013/
```

### 管理结果

```bash
# 列出所有测试结果
./bin/chaos-cli list

# 清理30天前的结果
./bin/chaos-cli clean -d 30
```

## 📊 10个测试场景

| 场景 | 描述 | 推荐时长 |
|------|------|----------|
| `baseline` | 无故障（性能基准） | 60s |
| `network-delay` | 100ms网络延迟 | 30s |
| `high-network-delay` | 500ms高延迟 | 30s |
| `network-loss` | 10%丢包 | 30s |
| `high-network-loss` | 30%高丢包 | 60s |
| `combined-network` | 延迟+丢包混合 | 60s |
| `cpu-stress` | 80% CPU压力 | 60s |
| `extreme-cpu-stress` | 95%极限CPU | 120s |
| `clock-skew` | +5秒时钟偏移 | 30s |
| `cascade-failure` | 级联故障 | 120s |

## 🎨 完整工作流示例

### 场景1: 开发阶段快速验证

```bash
# 1. 运行baseline建立基准
./bin/chaos-cli test baseline -d 60

# 2. 测试网络韧性
./bin/chaos-cli test network-delay -d 30
./bin/chaos-cli test network-loss -d 30

# 3. 生成报告
./bin/chaos-cli report chaos-results-*/
```

### 场景2: 发布前完整测试

```bash
# 1. 运行所有场景
./bin/chaos-cli test all -d 60

# 2. 生成HTML报告
./bin/chaos-cli report chaos-results-*/

# 3. 创建性能基准
./bin/chaos-cli baseline create chaos-results-*/ baseline-v1.0.json
```

### 场景3: Game Day演练

```bash
# 1. 启动监控面板（新终端）
./bin/chaos-cli dashboard

# 2. 运行Game Day（主终端）
./bin/chaos-cli gameday

# 3. 查看结果
./bin/chaos-cli list
```

### 场景4: CI/CD集成

```bash
# 在CI/CD pipeline中
./bin/chaos-cli test all -d 30
./bin/chaos-cli baseline compare baseline.json chaos-results-*/
# 如果检测到回归，返回 exit 1
```

## 🔍 故障排查

### 问题1: 二进制文件未找到

```bash
# 重新构建所有二进制
./setup-chaos.sh

# 或手动构建
go build -o bin/chaos-cli ./cmd/chaos-cli
go build -o bin/chaos-test-runner ./tests/chaos-test-runner
go build -o bin/chaos-dashboard ./cmd/chaos-dashboard
```

### 问题2: Python脚本失败

```bash
# 检查Python版本
python3 --version

# 安装所需的库（如果有）
pip3 install -r requirements.txt  # 如果有requirements.txt
```

### 问题3: 测试连接失败

```bash
# 检查MQTT broker是否运行
ps aux | grep emqx-go

# 手动启动broker
./bin/emqx-go &

# 等待启动
sleep 5

# 然后运行测试
./bin/chaos-cli test baseline -d 30
```

### 问题4: 端口被占用

```bash
# 检查端口占用
lsof -i :8888

# 使用不同端口
./bin/chaos-cli dashboard -p 9999
```

## 📚 下一步

- 📖 阅读 [使用指南](./CHAOS_TESTING_GUIDE.md) 了解详细用法
- 🎯 阅读 [最佳实践](./CHAOS_BEST_PRACTICES.md) 学习方法论
- 📊 阅读 [实施报告](./CHAOS_IMPLEMENTATION_REPORT.md) 了解技术细节
- 🏆 阅读 [最终报告](./CHAOS_FINAL_REPORT.md) 查看项目总结

## 💡 提示和技巧

### 提示1: 使用别名简化命令

```bash
# 添加到 ~/.bashrc 或 ~/.zshrc
alias chaos='./bin/chaos-cli'

# 然后可以直接使用
chaos test baseline -d 30
chaos dashboard
chaos gameday
```

### 提示2: 后台运行监控面板

```bash
# 后台启动dashboard
nohup ./bin/chaos-cli dashboard > dashboard.log 2>&1 &

# 查看日志
tail -f dashboard.log
```

### 提示3: 结合watch实时监控

```bash
# 实时查看测试结果目录
watch -n 1 'ls -lth chaos-results-* | head -20'
```

### 提示4: 自动化每日测试

```bash
# 添加到crontab
# 每天凌晨2点运行
0 2 * * * cd /path/to/emqx-go && ./bin/chaos-cli test all -d 30
```

## 🆘 获取帮助

```bash
# 查看所有命令
./bin/chaos-cli --help

# 查看特定命令帮助
./bin/chaos-cli test --help
./bin/chaos-cli dashboard --help
./bin/chaos-cli baseline --help
```

## 🎉 成功案例

完成快速开始后，你应该能够：

- ✅ 运行混沌测试场景
- ✅ 查看实时监控面板
- ✅ 生成HTML可视化报告
- ✅ 检测性能回归
- ✅ 执行Game Day演练

恭喜你已经掌握了EMQX-Go混沌工程的基础！

---

**版本**: v4.0
**最后更新**: 2025-10-12
**维护者**: EMQX-Go团队
