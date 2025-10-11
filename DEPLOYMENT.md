# EMQX-Go 部署指南

本文档提供EMQX-Go MQTT Broker集群的完整部署指南，涵盖本地开发、测试和生产环境部署。

---

## 目录

- [快速开始](#快速开始)
- [环境要求](#环境要求)
- [本地部署](#本地部署)
- [Docker部署](#docker部署)
- [Kubernetes部署](#kubernetes部署)
- [生产环境配置](#生产环境配置)
- [监控部署](#监控部署)
- [安全配置](#安全配置)
- [性能调优](#性能调优)

---

## 快速开始

### 5分钟快速体验

```bash
# 1. 克隆代码
git clone https://github.com/your-org/emqx-go.git
cd emqx-go

# 2. 构建
go build -o bin/emqx-go ./cmd/emqx-go

# 3. 启动集群
./scripts/start-cluster.sh

# 4. 验证
./scripts/health-check-simple.sh

# 5. 测试
./bin/cluster-test
```

---

## 环境要求

### 最低要求

| 组件 | 版本 | 说明 |
|------|------|------|
| Go | 1.21+ | 编译器 |
| CPU | 2 核心 | 最小配置 |
| 内存 | 2GB | 最小配置 |
| 磁盘 | 10GB | 日志和数据 |
| 操作系统 | Linux/macOS | 推荐Linux |

### 推荐配置

| 组件 | 版本 | 说明 |
|------|------|------|
| Go | 1.21+ | 最新稳定版 |
| CPU | 4+ 核心 | 生产环境 |
| 内存 | 8GB+ | 生产环境 |
| 磁盘 | 100GB+ SSD | 高性能存储 |
| 操作系统 | Ubuntu 22.04 LTS | 推荐 |

### 依赖工具

```bash
# macOS
brew install go git mosquitto

# Ubuntu/Debian
sudo apt-get update
sudo apt-get install -y golang git mosquitto-clients

# CentOS/RHEL
sudo yum install -y golang git mosquitto
```

---

## 本地部署

### 单节点部署

```bash
# 1. 编译
go build -o bin/emqx-go ./cmd/emqx-go

# 2. 运行
./bin/emqx-go

# 3. 测试连接
mosquitto_pub -h localhost -p 1883 -t "test/topic" -m "Hello EMQX-Go"
mosquitto_sub -h localhost -p 1883 -t "test/topic"
```

### 3节点集群部署

```bash
# 使用自动化脚本
./scripts/start-cluster.sh

# 或手动启动
# Node 1
NODE_ID=node1 MQTT_PORT=:1883 GRPC_PORT=:8081 \
  METRICS_PORT=:8082 DASHBOARD_PORT=:18083 \
  ./bin/emqx-go &

# Node 2
NODE_ID=node2 MQTT_PORT=:1884 GRPC_PORT=:8083 \
  METRICS_PORT=:8084 DASHBOARD_PORT=:18084 \
  PEER_NODES=localhost:8081 \
  ./bin/emqx-go &

# Node 3
NODE_ID=node3 MQTT_PORT=:1885 GRPC_PORT=:8085 \
  METRICS_PORT=:8086 DASHBOARD_PORT=:18085 \
  PEER_NODES=localhost:8081,localhost:8083 \
  ./bin/emqx-go &
```

### 配置文件部署

1. **生成配置文件**:
```bash
./bin/emqx-go -generate-config config.yaml
```

2. **编辑配置** (`config.yaml`):
```yaml
broker:
  node_id: "node1"
  mqtt_port: ":1883"
  grpc_port: ":8081"
  metrics_port: ":8082"
  dashboard_port: ":18083"
  auth:
    enabled: true
    users:
      - username: "admin"
        password: "your-secure-password"
        algorithm: "bcrypt"
        enabled: true
```

3. **启动**:
```bash
./bin/emqx-go -config config.yaml
```

---

## Docker部署

### 构建镜像

```bash
# 创建 Dockerfile
cat > Dockerfile << 'EOF'
FROM golang:1.21-alpine AS builder

WORKDIR /build
COPY . .
RUN go build -o emqx-go ./cmd/emqx-go

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /app
COPY --from=builder /build/emqx-go .

EXPOSE 1883 8081 8082 18083

CMD ["./emqx-go"]
EOF

# 构建
docker build -t emqx-go:latest .
```

### 单容器运行

```bash
docker run -d \
  --name emqx-go \
  -p 1883:1883 \
  -p 8082:8082 \
  -p 18083:18083 \
  emqx-go:latest
```

### Docker Compose集群

创建 `docker-compose.yml`:

```yaml
version: '3.8'

services:
  node1:
    image: emqx-go:latest
    container_name: emqx-node1
    environment:
      - NODE_ID=node1
      - MQTT_PORT=:1883
      - GRPC_PORT=:8081
      - METRICS_PORT=:8082
      - DASHBOARD_PORT=:18083
    ports:
      - "1883:1883"
      - "8081:8081"
      - "8082:8082"
      - "18083:18083"
    networks:
      - emqx-net

  node2:
    image: emqx-go:latest
    container_name: emqx-node2
    environment:
      - NODE_ID=node2
      - MQTT_PORT=:1883
      - GRPC_PORT=:8081
      - METRICS_PORT=:8082
      - DASHBOARD_PORT=:18083
      - PEER_NODES=node1:8081
    ports:
      - "1884:1883"
      - "8083:8081"
      - "8084:8082"
      - "18084:18083"
    networks:
      - emqx-net
    depends_on:
      - node1

  node3:
    image: emqx-go:latest
    container_name: emqx-node3
    environment:
      - NODE_ID=node3
      - MQTT_PORT=:1883
      - GRPC_PORT=:8081
      - METRICS_PORT=:8082
      - DASHBOARD_PORT=:18083
      - PEER_NODES=node1:8081,node2:8081
    ports:
      - "1885:1883"
      - "8085:8081"
      - "8086:8082"
      - "18085:18083"
    networks:
      - emqx-net
    depends_on:
      - node1
      - node2

networks:
  emqx-net:
    driver: bridge
```

启动：
```bash
docker-compose up -d
```

---

## Kubernetes部署

### 准备工作

```bash
# 创建命名空间
kubectl create namespace emqx

# 创建配置映射
kubectl create configmap emqx-config \
  --from-file=config.yaml \
  -n emqx
```

### StatefulSet部署

创建 `k8s-deployment.yaml`:

```yaml
apiVersion: v1
kind: Service
metadata:
  name: emqx-go-headless
  namespace: emqx
spec:
  clusterIP: None
  selector:
    app: emqx-go
  ports:
    - name: mqtt
      port: 1883
      targetPort: 1883
    - name: grpc
      port: 8081
      targetPort: 8081
    - name: metrics
      port: 8082
      targetPort: 8082
---
apiVersion: v1
kind: Service
metadata:
  name: emqx-go-lb
  namespace: emqx
spec:
  type: LoadBalancer
  selector:
    app: emqx-go
  ports:
    - name: mqtt
      port: 1883
      targetPort: 1883
---
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: emqx-go
  namespace: emqx
spec:
  serviceName: emqx-go-headless
  replicas: 3
  selector:
    matchLabels:
      app: emqx-go
  template:
    metadata:
      labels:
        app: emqx-go
    spec:
      containers:
      - name: emqx-go
        image: emqx-go:latest
        imagePullPolicy: IfNotPresent
        env:
        - name: NODE_ID
          valueFrom:
            fieldRef:
              fieldPath: metadata.name
        - name: POD_NAMESPACE
          valueFrom:
            fieldRef:
              fieldPath: metadata.namespace
        - name: MQTT_PORT
          value: ":1883"
        - name: GRPC_PORT
          value: ":8081"
        - name: METRICS_PORT
          value: ":8082"
        - name: DASHBOARD_PORT
          value: ":18083"
        ports:
        - containerPort: 1883
          name: mqtt
        - containerPort: 8081
          name: grpc
        - containerPort: 8082
          name: metrics
        - containerPort: 18083
          name: dashboard
        resources:
          requests:
            cpu: "500m"
            memory: "512Mi"
          limits:
            cpu: "2000m"
            memory: "2Gi"
        livenessProbe:
          tcpSocket:
            port: 1883
          initialDelaySeconds: 30
          periodSeconds: 10
        readinessProbe:
          tcpSocket:
            port: 1883
          initialDelaySeconds: 5
          periodSeconds: 5
```

部署：
```bash
kubectl apply -f k8s-deployment.yaml
```

验证：
```bash
# 检查Pod状态
kubectl get pods -n emqx

# 检查服务
kubectl get svc -n emqx

# 查看日志
kubectl logs -f emqx-go-0 -n emqx
```

---

## 生产环境配置

### 系统优化

```bash
# /etc/sysctl.conf
# 增加文件描述符限制
fs.file-max = 2097152
fs.nr_open = 2097152

# TCP优化
net.ipv4.tcp_max_syn_backlog = 8192
net.core.somaxconn = 8192
net.core.netdev_max_backlog = 8192

# 连接追踪
net.netfilter.nf_conntrack_max = 1048576
net.nf_conntrack_max = 1048576

# 应用配置
sudo sysctl -p
```

```bash
# /etc/security/limits.conf
* soft nofile 1048576
* hard nofile 1048576
* soft nproc unlimited
* hard nproc unlimited
```

### 日志配置

```bash
# 使用logrotate管理日志
cat > /etc/logrotate.d/emqx-go << 'EOF'
/var/log/emqx-go/*.log {
    daily
    rotate 7
    compress
    delaycompress
    missingok
    notifempty
    create 0640 emqx emqx
    sharedscripts
    postrotate
        systemctl reload emqx-go > /dev/null 2>&1 || true
    endscript
}
EOF
```

### Systemd服务

创建 `/etc/systemd/system/emqx-go.service`:

```ini
[Unit]
Description=EMQX-Go MQTT Broker
After=network.target

[Service]
Type=simple
User=emqx
Group=emqx
WorkingDirectory=/opt/emqx-go
Environment="NODE_ID=node1"
Environment="MQTT_PORT=:1883"
Environment="GRPC_PORT=:8081"
Environment="METRICS_PORT=:8082"
Environment="DASHBOARD_PORT=:18083"
ExecStart=/opt/emqx-go/bin/emqx-go -config /etc/emqx-go/config.yaml
Restart=always
RestartSec=10
StandardOutput=journal
StandardError=journal
SyslogIdentifier=emqx-go

# 资源限制
LimitNOFILE=1048576
LimitNPROC=unlimited

[Install]
WantedBy=multi-user.target
```

启用服务：
```bash
sudo systemctl daemon-reload
sudo systemctl enable emqx-go
sudo systemctl start emqx-go
sudo systemctl status emqx-go
```

---

## 监控部署

### Prometheus配置

创建 `prometheus.yml`:

```yaml
global:
  scrape_interval: 15s
  evaluation_interval: 15s

rule_files:
  - "monitoring/prometheus-alerts.yml"

scrape_configs:
  - job_name: 'emqx-go'
    static_configs:
      - targets:
        - 'localhost:8082'
        - 'localhost:8084'
        - 'localhost:8086'
        labels:
          cluster: 'emqx-prod'
```

启动：
```bash
prometheus --config.file=prometheus.yml
```

### Grafana配置

```bash
# 安装
brew install grafana  # macOS
sudo apt-get install grafana  # Ubuntu

# 启动
brew services start grafana  # macOS
sudo systemctl start grafana  # Linux

# 访问 http://localhost:3000
# 默认账号: admin/admin

# 添加数据源
# Configuration → Data Sources → Add data source
# 选择 Prometheus
# URL: http://localhost:9090

# 导入仪表板
# Dashboard → Import
# 上传: monitoring/grafana-dashboard.json
```

### AlertManager配置

创建 `alertmanager.yml`:

```yaml
global:
  resolve_timeout: 5m
  smtp_smarthost: 'smtp.gmail.com:587'
  smtp_from: 'alerts@your-domain.com'
  smtp_auth_username: 'your-email@gmail.com'
  smtp_auth_password: 'your-app-password'

route:
  group_by: ['alertname', 'cluster', 'severity']
  group_wait: 10s
  group_interval: 10s
  repeat_interval: 12h
  receiver: 'email-notifications'
  routes:
    - match:
        severity: critical
      receiver: 'pagerduty'
      continue: true
    - match:
        severity: warning
      receiver: 'email-notifications'

receivers:
  - name: 'email-notifications'
    email_configs:
      - to: 'team@your-domain.com'
        headers:
          Subject: '[EMQX-Go] {{ .GroupLabels.alertname }}'

  - name: 'pagerduty'
    pagerduty_configs:
      - service_key: 'your-pagerduty-key'
```

---

## 安全配置

### TLS/SSL配置

1. **生成证书**:
```bash
# 自签名证书（仅用于测试）
openssl req -x509 -newkey rsa:4096 \
  -keyout key.pem -out cert.pem \
  -days 365 -nodes \
  -subj "/CN=emqx-go.local"
```

2. **配置EMQX-Go** (TODO: 需要在代码中添加TLS支持):
```yaml
broker:
  mqtt_port: ":1883"
  mqtts_port: ":8883"
  tls:
    enabled: true
    cert_file: "/path/to/cert.pem"
    key_file: "/path/to/key.pem"
```

### 认证配置

编辑 `config.yaml`:

```yaml
broker:
  auth:
    enabled: true
    users:
      - username: "admin"
        password: "$2a$10$..."  # bcrypt hash
        algorithm: "bcrypt"
        enabled: true
      - username: "device001"
        password: "device-token-001"
        algorithm: "sha256"
        enabled: true
```

生成bcrypt密码：
```bash
# 使用在线工具或自己编写Go程序
# https://bcrypt-generator.com/
```

### 防火墙配置

```bash
# Ubuntu/Debian (ufw)
sudo ufw allow 1883/tcp  # MQTT
sudo ufw allow 8883/tcp  # MQTTS
sudo ufw allow 8081/tcp  # gRPC (仅内网)
sudo ufw allow 8082/tcp  # Metrics (仅内网)

# CentOS/RHEL (firewalld)
sudo firewall-cmd --permanent --add-port=1883/tcp
sudo firewall-cmd --permanent --add-port=8883/tcp
sudo firewall-cmd --reload
```

---

## 性能调优

### 连接数优化

```yaml
# config.yaml
broker:
  max_connections: 100000
  max_packet_size: 268435456  # 256MB
  keepalive_backoff: 0.75
```

### 内存优化

```bash
# 设置Go运行时
export GOGC=100  # GC触发百分比
export GOMAXPROCS=8  # 使用CPU核心数
```

### 网络优化

```bash
# TCP参数优化
sudo sysctl -w net.ipv4.tcp_tw_reuse=1
sudo sysctl -w net.ipv4.tcp_fin_timeout=30
sudo sysctl -w net.ipv4.tcp_keepalive_time=300
sudo sysctl -w net.ipv4.tcp_keepalive_probes=3
sudo sysctl -w net.ipv4.tcp_keepalive_intvl=30
```

### 磁盘I/O优化

```bash
# 使用SSD
# 禁用访问时间更新
mount -o noatime,nodiratime /dev/sda1 /mnt/data

# 调整I/O调度器
echo noop > /sys/block/sda/queue/scheduler
```

---

## 备份和恢复

### 数据备份

```bash
# 备份脚本
#!/bin/bash
BACKUP_DIR=/backup/emqx-go/$(date +%Y%m%d)
mkdir -p $BACKUP_DIR

# 备份配置
cp /etc/emqx-go/config.yaml $BACKUP_DIR/

# 备份日志
cp -r /var/log/emqx-go $BACKUP_DIR/logs/

# 压缩
tar -czf $BACKUP_DIR.tar.gz $BACKUP_DIR
rm -rf $BACKUP_DIR
```

### 恢复

```bash
# 解压备份
tar -xzf /backup/emqx-go/20250110.tar.gz -C /tmp/

# 恢复配置
cp /tmp/20250110/config.yaml /etc/emqx-go/

# 重启服务
sudo systemctl restart emqx-go
```

---

## 故障排查

参见 [TROUBLESHOOTING.md](./TROUBLESHOOTING.md) 获取详细的故障排查指南。

---

## 升级指南

### 滚动升级

```bash
# 1. 备份当前配置和数据
./scripts/backup.sh

# 2. 逐个节点升级
# 停止node3
systemctl stop emqx-go@node3

# 更新二进制文件
cp new/emqx-go /opt/emqx-go/bin/

# 启动node3
systemctl start emqx-go@node3

# 验证
./scripts/health-check-simple.sh

# 重复node2和node1
```

---

## 附录

### 端口列表

| 端口 | 协议 | 用途 | 访问控制 |
|------|------|------|---------|
| 1883 | MQTT | MQTT连接 | 公网 |
| 8883 | MQTTS | MQTT over TLS | 公网 |
| 8081 | gRPC | 集群通信 | 内网 |
| 8082 | HTTP | Prometheus指标 | 内网 |
| 18083 | HTTP | Dashboard | 内网 |

### 配置参数参考

| 参数 | 默认值 | 说明 |
|------|--------|------|
| NODE_ID | "local-node" | 节点唯一标识 |
| MQTT_PORT | ":1883" | MQTT监听端口 |
| GRPC_PORT | ":8081" | gRPC监听端口 |
| METRICS_PORT | ":8082" | 指标监听端口 |
| DASHBOARD_PORT | ":18083" | Dashboard端口 |
| PEER_NODES | "" | 集群对等节点 |

---

**文档版本**: v1.0
**最后更新**: 2025-10-11
**维护者**: EMQX-Go团队
