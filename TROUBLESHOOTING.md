# EMQX-Go 故障排查指南

本文档提供EMQX-Go常见问题的诊断和解决方案。

---

## 目录

- [快速诊断](#快速诊断)
- [集群问题](#集群问题)
- [连接问题](#连接问题)
- [性能问题](#性能问题)
- [监控问题](#监控问题)
- [日志分析](#日志分析)

---

## 快速诊断

### 一键健康检查

```bash
# 运行健康检查
./scripts/health-check-simple.sh

# 如果发现问题，查看详细日志
tail -f logs/*.log
```

###常见问题快速自检清单

| 问题 | 检查命令 | 正常输出 |
|------|---------|---------|
| 进程运行？ | `pgrep -f emqx-go` | 显示PID |
| 端口监听？ | `nc -z localhost 1883` | 成功连接 |
| 集群连通？ | `./bin/cluster-test` | SUCCESS |
| 内存使用？ | `ps aux \| grep emqx-go` | < 2GB |
| 文件描述符？ | `lsof -p <PID> \| wc -l` | < ulimit |

---

## 集群问题

### 问题1: 节点无法加入集群

**症状**:
```
Failed to join cluster: connection refused
```

**原因分析**:
1. gRPC端口未开放
2. 防火墙阻止
3. 网络不可达
4. 节点地址配置错误

**诊断步骤**:

```bash
# 1. 检查gRPC端口
nc -zv localhost 8081
nc -zv localhost 8083
nc -zv localhost 8085

# 2. 检查防火墙
sudo ufw status  # Ubuntu
sudo firewall-cmd --list-all  # CentOS

# 3. 检查网络连通性
ping <peer-node-ip>
telnet <peer-node-ip> 8081

# 4. 检查节点配置
cat logs/node*.log | grep "PEER_NODES"
```

**解决方案**:

```bash
# 1. 开放gRPC端口
sudo ufw allow 8081/tcp
sudo ufw allow 8083/tcp
sudo ufw allow 8085/tcp

# 2. 确认PEER_NODES配置正确
# 应该是: PEER_NODES=localhost:8081 (对于node2)
# 或: PEER_NODES=localhost:8081,localhost:8083 (对于node3)

# 3. 重启节点
./scripts/stop-cluster.sh
./scripts/start-cluster.sh
```

### 问题2: 集群分裂 (Split Brain)

**症状**:
```
Multiple cluster IDs detected
```

**诊断**:
```bash
# 检查每个节点的cluster ID
for port in 8082 8084 8086; do
  curl -s http://localhost:$port/metrics | grep cluster_id
done
```

**解决方案**:
```bash
# 1. 停止所有节点
./scripts/stop-cluster.sh

# 2. 清理状态（如果有持久化）
rm -rf /var/lib/emqx-go/cluster-state

# 3. 重新启动集群
./scripts/start-cluster.sh

# 4. 验证cluster ID一致
./scripts/health-check-simple.sh
```

### 问题3: 节点间路由不同步

**症状**:
```
Message not received on subscriber
```

**诊断**:
```bash
# 检查路由同步日志
grep "BatchUpdateRoutes" logs/node*.log

# 检查gRPC连接
grep "Successfully connected" logs/node*.log
```

**解决方案**:
```bash
# 1. 检查网络延迟
ping -c 10 <peer-node>

# 2. 重启问题节点
pkill -f "NODE_ID=node2"
NODE_ID=node2 MQTT_PORT=:1884 GRPC_PORT=:8083 \
  METRICS_PORT=:8084 DASHBOARD_PORT=:18084 \
  PEER_NODES=localhost:8081 \
  ./bin/emqx-go &

# 3. 验证路由同步
./bin/cluster-test
```

---

## 连接问题

### 问题4: MQTT客户端无法连接

**症状**:
```
Connection refused / Connection timeout
```

**诊断**:
```bash
# 1. 检查端口监听
lsof -i :1883
netstat -tulpn | grep 1883

# 2. 测试本地连接
mosquitto_pub -h localhost -p 1883 -t test -m "test"

# 3. 测试远程连接
mosquitto_pub -h <server-ip> -p 1883 -t test -m "test"

# 4. 检查防火墙
sudo iptables -L -n | grep 1883
```

**解决方案**:
```bash
# 1. 确认服务运行
ps aux | grep emqx-go

# 2. 检查绑定地址
# 确保没有只绑定127.0.0.1，应该绑定0.0.0.0

# 3. 开放防火墙
sudo ufw allow 1883/tcp

# 4. 检查日志
tail -f logs/node1.log | grep "MQTT broker listening"
```

### 问题5: 认证失败

**症状**:
```
Connection refused: Not authorized
```

**诊断**:
```bash
# 1. 检查认证配置
cat config.yaml | grep -A 10 "auth:"

# 2. 测试不同用户
mosquitto_pub -h localhost -p 1883 \
  -u admin -P admin123 \
  -t test -m "test"

# 3. 查看认证日志
grep "Authentication" logs/node1.log
```

**解决方案**:
```bash
# 1. 验证用户配置
./bin/emqx-go -config config.yaml

# 2. 检查密码哈希
# bcrypt密码需要正确的哈希值

# 3. 临时禁用认证测试
# 修改config.yaml: auth.enabled: false

# 4. 查看详细日志
grep -i "auth" logs/node1.log
```

### 问题6: 连接频繁断开

**症状**:
```
Client disconnecting frequently
```

**诊断**:
```bash
# 1. 检查keepalive设置
grep "KeepAlive" logs/node1.log

# 2. 监控连接数
watch -n 1 'curl -s http://localhost:8082/metrics | grep emqx_connections_count'

# 3. 检查网络质量
ping -i 0.2 <server-ip>

# 4. 查看断开原因
grep "disconnected" logs/node1.log
```

**解决方案**:
```bash
# 1. 增加keepalive时间
# 客户端: set keepalive to 60 seconds

# 2. 检查网络稳定性
# 使用更稳定的网络连接

# 3. 增加TCP缓冲区
sudo sysctl -w net.core.rmem_max=16777216
sudo sysctl -w net.core.wmem_max=16777216

# 4. 检查系统资源
free -h
df -h
```

---

## 性能问题

### 问题7: 高延迟

**症状**:
```
Message delivery latency > 100ms
```

**诊断**:
```bash
# 1. 运行性能测试
./scripts/performance-test.sh

# 2. 检查系统负载
top
vmstat 1

# 3. 检查网络延迟
ping -c 100 <peer-node>

# 4. 查看消息队列
curl -s http://localhost:8082/metrics | grep queue
```

**解决方案**:
```bash
# 1. 优化TCP参数
sudo sysctl -w net.ipv4.tcp_nodelay=1
sudo sysctl -w net.ipv4.tcp_low_latency=1

# 2. 增加处理器数
export GOMAXPROCS=8

# 3. 禁用调试日志
# 减少日志输出

# 4. 使用更快的存储
# 将日志写入内存文件系统或SSD
```

### 问题8: 高CPU使用率

**症状**:
```
CPU usage > 80%
```

**诊断**:
```bash
# 1. 查看CPU使用
top -p $(pgrep -f emqx-go)

# 2. 性能分析
go tool pprof http://localhost:8082/debug/pprof/profile

# 3. 检查goroutine数量
curl http://localhost:8082/debug/pprof/goroutine?debug=1

# 4. 查看连接数
curl -s http://localhost:8082/metrics | grep connections
```

**解决方案**:
```bash
# 1. 增加GOMAXPROCS
export GOMAXPROCS=<cpu-cores>

# 2. 优化代码热点
# 使用pprof识别热点函数

# 3. 限制连接数
# 配置max_connections

# 4. 水平扩展
# 添加更多节点
```

### 问题9: 内存泄漏

**症状**:
```
Memory usage continuously increasing
```

**诊断**:
```bash
# 1. 监控内存使用
watch -n 5 'ps aux | grep emqx-go'

# 2. 内存分析
go tool pprof http://localhost:8082/debug/pprof/heap

# 3. 检查goroutine泄漏
curl http://localhost:8082/debug/pprof/goroutine?debug=2

# 4. 查看会话数
curl -s http://localhost:8082/metrics | grep sessions_count
```

**解决方案**:
```bash
# 1. 检查会话清理
# 确保cleanSession正常工作

# 2. 调整GC参数
export GOGC=50  # 更频繁的GC

# 3. 重启服务
sudo systemctl restart emqx-go

# 4. 升级到最新版本
# 可能包含内存泄漏修复
```

### 问题10: 消息丢失

**症状**:
```
Published messages not received
```

**诊断**:
```bash
# 1. 检查丢弃消息数
curl -s http://localhost:8082/metrics | grep messages_dropped

# 2. 检查订阅
grep "SUBSCRIBE" logs/node*.log

# 3. 检查路由
grep "route" logs/node*.log

# 4. 测试QoS级别
mosquitto_pub -h localhost -p 1883 -t test -m "test" -q 1
mosquitto_sub -h localhost -p 1883 -t test -q 1
```

**解决方案**:
```bash
# 1. 使用QoS 1或2
# 确保消息可靠传递

# 2. 检查主题过滤器
# 确保订阅主题匹配发布主题

# 3. 增加消息缓冲区
# 配置更大的缓冲区

# 4. 检查集群路由
./bin/cluster-test
```

---

## 监控问题

### 问题11: Prometheus无法抓取指标

**症状**:
```
Metrics endpoint not accessible
```

**诊断**:
```bash
# 1. 测试指标端点
curl http://localhost:8082/metrics

# 2. 检查端口
nc -zv localhost 8082

# 3. 查看Prometheus日志
docker logs prometheus

# 4. 检查Prometheus配置
cat prometheus.yml
```

**解决方案**:
```bash
# 1. 确认metrics端口配置
METRICS_PORT=:8082 ./bin/emqx-go

# 2. 更新Prometheus配置
# prometheus.yml中的targets应包含正确的地址

# 3. 重启Prometheus
docker restart prometheus

# 4. 验证抓取
curl http://localhost:9090/api/v1/targets
```

### 问题12: Grafana显示"No Data"

**症状**:
```
Dashboard panels show "No Data"
```

**诊断**:
```bash
# 1. 测试Prometheus数据源
# Grafana → Configuration → Data Sources → Test

# 2. 检查指标名称
curl -s http://localhost:8082/metrics | grep emqx

# 3. 在Prometheus中查询
# http://localhost:9090/graph
# 查询: emqx_connections_count

# 4. 检查时间范围
# 确保Grafana时间范围包含数据
```

**解决方案**:
```bash
# 1. 确认数据源配置
# URL应该是: http://localhost:9090

# 2. 检查指标是否存在
curl http://localhost:9090/api/v1/label/__name__/values | grep emqx

# 3. 刷新仪表板
# Dashboard → Settings → Refresh

# 4. 重新导入仪表板
# 使用最新的grafana-dashboard.json
```

### 问题13: 告警未触发

**症状**:
```
Alert conditions met but no notifications
```

**诊断**:
```bash
# 1. 检查告警规则
curl http://localhost:9090/api/v1/rules

# 2. 查看告警状态
curl http://localhost:9090/api/v1/alerts

# 3. 检查AlertManager
curl http://localhost:9093/api/v1/alerts

# 4. 查看AlertManager日志
docker logs alertmanager
```

**解决方案**:
```bash
# 1. 验证规则语法
promtool check rules monitoring/prometheus-alerts.yml

# 2. 重启Prometheus
docker restart prometheus

# 3. 测试AlertManager配置
amtool check-config alertmanager.yml

# 4. 手动触发测试告警
curl -H "Content-Type: application/json" -d '[{
  "labels": {"alertname":"test","severity":"warning"}
}]' http://localhost:9093/api/v1/alerts
```

---

## 日志分析

### 日志级别

| 级别 | 用途 | 示例 |
|------|------|------|
| DEBUG | 详细调试信息 | `[DEBUG] Received packet type: 3` |
| INFO | 一般信息 | `[INFO] Client connected` |
| WARN | 警告信息 | `[WARN] High memory usage` |
| ERROR | 错误信息 | `[ERROR] Failed to connect` |
| FATAL | 致命错误 | `[FATAL] Cannot start server` |

### 常用日志查询

```bash
# 查找错误
grep -i "error\|fatal\|panic" logs/*.log

# 查找连接问题
grep -i "connect\|disconnect" logs/*.log

# 查找认证问题
grep -i "auth" logs/*.log

# 查找集群问题
grep -i "cluster\|join\|peer" logs/*.log

# 查找性能问题
grep -i "timeout\|slow\|lag" logs/*.log

# 实时监控
tail -f logs/node1.log | grep --color -i "error\|warn"

# 统计错误
grep -c "ERROR" logs/*.log

# 按时间过滤
grep "2025/10/11 08:" logs/node1.log
```

### 日志分析工具

```bash
# 使用jq分析JSON日志（如果有）
tail -f logs/node1.log | jq '.'

# 使用awk统计
awk '/ERROR/{print $0}' logs/node1.log | wc -l

# 使用sed提取
sed -n '/ERROR/,/INFO/p' logs/node1.log
```

---

## 紧急恢复流程

### 完全集群故障

```bash
# 1. 停止所有节点
./scripts/stop-cluster.sh

# 2. 备份日志和配置
tar -czf backup-$(date +%Y%m%d-%H%M%S).tar.gz logs/ config.yaml

# 3. 清理可能损坏的状态
rm -f /tmp/emqx-go-*

# 4. 重新启动
./scripts/start-cluster.sh

# 5. 验证
./scripts/health-check-simple.sh

# 6. 恢复流量
# 逐步增加负载
```

### 数据恢复

```bash
# 1. 从备份恢复
tar -xzf backup-20250110-120000.tar.gz

# 2. 恢复配置
cp backup/config.yaml /etc/emqx-go/

# 3. 重启服务
sudo systemctl restart emqx-go

# 4. 验证
./bin/cluster-test
```

---

## 获取帮助

### 收集诊断信息

运行此脚本收集所有诊断信息：

```bash
#!/bin/bash
# collect-diagnostics.sh

DIAG_DIR="diagnostics-$(date +%Y%m%d-%H%M%S)"
mkdir -p "$DIAG_DIR"

# 系统信息
uname -a > "$DIAG_DIR/system.txt"
free -h >> "$DIAG_DIR/system.txt"
df -h >> "$DIAG_DIR/system.txt"

# 进程信息
ps aux | grep emqx-go > "$DIAG_DIR/processes.txt"
pgrep -f emqx-go | xargs lsof -p > "$DIAG_DIR/open-files.txt"

# 网络信息
netstat -tulpn | grep emqx-go > "$DIAG_DIR/network.txt"
ss -s >> "$DIAG_DIR/network.txt"

# 日志
cp -r logs "$DIAG_DIR/"

# 配置
cp config.yaml "$DIAG_DIR/" 2>/dev/null

# 指标
for port in 8082 8084 8086; do
  curl -s http://localhost:$port/metrics > "$DIAG_DIR/metrics-$port.txt"
done

# 健康检查
./scripts/health-check-simple.sh > "$DIAG_DIR/health-check.txt" 2>&1

# 打包
tar -czf "$DIAG_DIR.tar.gz" "$DIAG_DIR"
rm -rf "$DIAG_DIR"

echo "Diagnostics collected: $DIAG_DIR.tar.gz"
```

### 联系支持

提交Issue时请包含：
1. EMQX-Go版本
2. 操作系统和版本
3. 部署方式（本地/Docker/K8s）
4. 问题描述和复现步骤
5. 诊断信息包

---

## 附录

### 常用命令速查

```bash
# 健康检查
./scripts/health-check-simple.sh

# 启动集群
./scripts/start-cluster.sh

# 停止集群
./scripts/stop-cluster.sh

# 查看日志
tail -f logs/node1.log

# 测试连接
./bin/cluster-test

# 查看指标
curl http://localhost:8082/metrics

# 查看进程
ps aux | grep emqx-go

# 查看端口
netstat -tulpn | grep emqx-go

# 重启节点
sudo systemctl restart emqx-go
```

---

**文档版本**: v1.0
**最后更新**: 2025-10-11
**维护者**: EMQX-Go团队
