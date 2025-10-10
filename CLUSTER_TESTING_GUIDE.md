# 集群测试指南

本目录包含EMQX-Go集群功能的E2E测试工具和脚本。

## 快速开始

### 1. 启动3节点集群
```bash
./scripts/start-cluster.sh
```

这将启动3个EMQX-Go实例:
- Node1: MQTT端口1883, gRPC端口8081, Metrics端口8082
- Node2: MQTT端口1884, gRPC端口8083, Metrics端口8084
- Node3: MQTT端口1885, gRPC端口8085, Metrics端口8086

日志文件保存在 `./logs/` 目录下。

### 2. 运行测试
```bash
go run ./cmd/cluster-test/main.go
```

### 3. 停止集群
```bash
./scripts/stop-cluster.sh
```

## 测试说明

测试验证跨节点消息路由功能:
1. Subscriber连接到Node2并订阅主题 `cluster/test`
2. Publisher连接到Node1并发布消息到 `cluster/test`
3. 验证消息是否成功从Node1路由到Node2的subscriber

## 预期输出

成功的测试输出:
```
============================================================
EMQX-Go Cluster Cross-Node Messaging Test
============================================================

1. Creating subscriber connecting to node2 (port 1884)...
✓ Subscriber connected to node2

2. Subscribing to 'cluster/test'...
✓ Subscribed successfully

3. Waiting for route propagation (2 seconds)...

4. Creating publisher connecting to node1 (port 1883)...
✓ Publisher connected to node1

5. Publishing message from node1 to 'cluster/test'...
✓ Message published

6. Waiting for cross-node delivery (3 seconds)...
✓ RECEIVED: 'Hello from Node1 to Node2!' on topic 'cluster/test'

============================================================
Test Results:
============================================================
✓✓✓ SUCCESS! Cross-node messaging works!
✓ Message 'Hello from Node1 to Node2!' successfully routed from node1 to node2
============================================================
```

## Docker部署

也可以使用Docker Compose部署集群:
```bash
docker-compose -f docker-compose-cluster.yaml up -d
```

停止Docker集群:
```bash
docker-compose -f docker-compose-cluster.yaml down
```

## 查看日志

### 本地部署日志
```bash
tail -f ./logs/node1.log
tail -f ./logs/node2.log
tail -f ./logs/node3.log
```

### Docker部署日志
```bash
docker-compose -f docker-compose-cluster.yaml logs -f emqx-node1
docker-compose -f docker-compose-cluster.yaml logs -f emqx-node2
docker-compose -f docker-compose-cluster.yaml logs -f emqx-node3
```

## 故障排查

如果测试失败,请检查:

1. **节点是否成功启动**
   ```bash
   ps aux | grep emqx-go
   ```

2. **端口是否被占用**
   ```bash
   lsof -i :1883
   lsof -i :1884
   lsof -i :1885
   ```

3. **节点连接状态** - 查看日志中的"Successfully joined cluster"消息

4. **路由表同步** - 查看日志中的"Received BatchUpdateRoutes request"消息

## 技术细节

详细的技术说明和bug修复记录请参考:
- [完整测试报告](./CLUSTER_E2E_TEST_REPORT.md)
- [快速总结](./CLUSTER_TEST_SUMMARY.md)

## 已修复的关键Bug

本次测试发现并修复了2个关键bug:

1. **节点地址问题** (`cmd/emqx-go/main.go`): 节点使用nodeID作为主机名导致本地部署无法解析
2. **Context生命周期问题** (`pkg/cluster/server.go`): Join RPC处理器使用请求context导致反向连接立即取消

这两个bug的修复使得集群的跨节点消息路由功能得以正常工作。
