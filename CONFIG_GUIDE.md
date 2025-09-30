# EMQX-Go 用户配置管理指南

EMQX-Go 现在支持通过配置文件来管理用户认证。您可以使用 YAML 或 JSON 格式的配置文件来添加、修改和管理 MQTT 用户。

## 配置文件格式

### YAML 格式 (推荐)

```yaml
broker:
  node_id: "emqx-go-node"
  mqtt_port: ":1883"
  grpc_port: ":8081"
  metrics_port: ":8082"

  auth:
    enabled: true
    users:
      - username: "admin"
        password: "admin123"
        algorithm: "bcrypt"    # bcrypt, sha256, 或 plain
        enabled: true

      - username: "user1"
        password: "password123"
        algorithm: "sha256"
        enabled: true

      - username: "guest"
        password: "guest123"
        algorithm: "sha256"
        enabled: false         # 禁用用户
```

### JSON 格式

```json
{
  "broker": {
    "node_id": "emqx-go-node",
    "mqtt_port": ":1883",
    "grpc_port": ":8081",
    "metrics_port": ":8082",
    "auth": {
      "enabled": true,
      "users": [
        {
          "username": "admin",
          "password": "admin123",
          "algorithm": "bcrypt",
          "enabled": true
        }
      ]
    }
  }
}
```

## 启动方式

### 1. 使用配置文件启动

```bash
# 使用 YAML 配置文件
./bin/emqx-go -config=config.yaml

# 使用 JSON 配置文件
./bin/emqx-go -config=config.json

# 不指定配置文件（使用默认配置）
./bin/emqx-go
```

### 2. 生成示例配置文件

```bash
# 生成 YAML 配置文件
./bin/emqx-go -generate-config=my-config.yaml

# 生成 JSON 配置文件
./bin/emqx-go -generate-config=my-config.json
```

## 用户管理 CLI 工具

EMQX-Go 提供了专门的 CLI 工具来管理用户，无需手动编辑配置文件。

### 编译用户管理工具

```bash
go build -o bin/emqx-user ./cmd/emqx-user
```

### 用户管理命令

#### 1. 查看所有用户

```bash
./bin/emqx-user -cmd=list -config=config.yaml
```

输出示例：
```
USERNAME   ALGORITHM  ENABLED  PASSWORD
--------   ---------  -------  --------
admin      bcrypt     ✓        ad****23
user1      sha256     ✓        pass****d123
guest      sha256     ✗        gu****23
```

#### 2. 添加新用户

```bash
# 添加用户（默认启用，使用 bcrypt 加密）
./bin/emqx-user -cmd=add -user=newuser -pass=mypassword -config=config.yaml

# 指定加密算法
./bin/emqx-user -cmd=add -user=device001 -pass=secret123 -algo=sha256 -config=config.yaml

# 添加但默认禁用的用户
./bin/emqx-user -cmd=add -user=testuser -pass=testpass -enabled=false -config=config.yaml
```

#### 3. 更新用户密码

```bash
# 更新密码
./bin/emqx-user -cmd=update -user=admin -pass=newpassword -config=config.yaml

# 更新密码和加密算法
./bin/emqx-user -cmd=update -user=user1 -pass=newpass -algo=bcrypt -config=config.yaml
```

#### 4. 启用/禁用用户

```bash
# 禁用用户
./bin/emqx-user -cmd=disable -user=guest -config=config.yaml

# 启用用户
./bin/emqx-user -cmd=enable -user=guest -config=config.yaml
```

#### 5. 删除用户

```bash
./bin/emqx-user -cmd=remove -user=olduser -config=config.yaml
```

#### 6. 生成配置文件

```bash
./bin/emqx-user -cmd=generate -config=new-config.yaml
```

## 密码加密算法

EMQX-Go 支持三种密码加密算法：

### 1. bcrypt (推荐用于生产环境)
- **优点**: 最安全，自带盐值，抗彩虹表攻击
- **缺点**: 计算密集，速度较慢
- **使用场景**: 生产环境，管理员账户

```yaml
- username: "admin"
  password: "admin123"
  algorithm: "bcrypt"
```

### 2. sha256 (平衡选择)
- **优点**: 计算快速，安全性好
- **缺点**: 需要盐值防止彩虹表攻击
- **使用场景**: 大量IoT设备认证

```yaml
- username: "device001"
  password: "device_secret"
  algorithm: "sha256"
```

### 3. plain (仅用于测试)
- **优点**: 明文存储，便于调试
- **缺点**: 完全不安全
- **使用场景**: 仅限开发和测试环境

```yaml
- username: "test"
  password: "test"
  algorithm: "plain"
```

## 认证配置选项

### 启用/禁用认证

```yaml
auth:
  enabled: false  # 完全禁用认证，允许匿名连接
```

### 用户状态控制

```yaml
users:
  - username: "admin"
    password: "admin123"
    algorithm: "bcrypt"
    enabled: false    # 禁用此用户，拒绝连接
```

## 实际使用示例

### 示例 1: IoT 场景配置

```yaml
broker:
  node_id: "iot-gateway-01"
  mqtt_port: ":1883"

  auth:
    enabled: true
    users:
      # 管理员账户
      - username: "admin"
        password: "secure_admin_password"
        algorithm: "bcrypt"
        enabled: true

      # IoT 设备账户
      - username: "sensor_001"
        password: "sensor_secret_key_001"
        algorithm: "sha256"
        enabled: true

      - username: "actuator_001"
        password: "actuator_secret_key_001"
        algorithm: "sha256"
        enabled: true

      # 应用程序账户
      - username: "app_backend"
        password: "backend_service_key"
        algorithm: "bcrypt"
        enabled: true

      # 临时禁用的设备
      - username: "sensor_002"
        password: "sensor_secret_key_002"
        algorithm: "sha256"
        enabled: false
```

### 示例 2: 测试环境配置

```yaml
broker:
  node_id: "test-broker"
  mqtt_port: ":1883"

  auth:
    enabled: true
    users:
      - username: "test"
        password: "test"
        algorithm: "plain"
        enabled: true

      - username: "demo"
        password: "demo123"
        algorithm: "plain"
        enabled: true
```

## 故障排除

### 1. 配置文件格式错误

**错误**: `Failed to parse config file: yaml: unmarshal errors`

**解决**: 检查 YAML 缩进是否正确，确保使用空格而不是制表符。

### 2. 用户无法连接

**排查步骤**:
1. 检查用户是否存在：`./bin/emqx-user -cmd=list`
2. 检查用户是否启用：查看 `enabled` 字段
3. 检查密码是否正确
4. 查看服务器日志中的认证信息

### 3. 配置文件权限问题

**错误**: `Failed to read config file: permission denied`

**解决**: 确保配置文件有正确的读取权限：
```bash
chmod 644 config.yaml
```

### 4. 重新加载配置

目前需要重启服务来加载新配置：
```bash
# 停止服务 (Ctrl+C)
# 重新启动
./bin/emqx-go -config=config.yaml
```

## 安全最佳实践

1. **生产环境**: 使用 `bcrypt` 算法
2. **密码强度**: 使用复杂密码，至少 12 位字符
3. **配置文件权限**: 设置适当的文件权限 (644 或 600)
4. **定期更新**: 定期更新用户密码
5. **禁用测试账户**: 生产环境中禁用或删除测试用户
6. **监控日志**: 定期检查认证失败日志

## 集成示例

### Python 客户端

```python
import paho.mqtt.client as mqtt

client = mqtt.Client()
client.username_pw_set("admin", "admin123")
client.connect("localhost", 1883, 60)
client.publish("test/topic", "Hello EMQX-Go")
```

### Go 客户端

```go
opts := mqtt.NewClientOptions()
opts.AddBroker("tcp://localhost:1883")
opts.SetUsername("admin")
opts.SetPassword("admin123")
opts.SetClientID("go-client")

client := mqtt.NewClient(opts)
token := client.Connect()
```

### mosquitto 命令行

```bash
# 发布消息
mosquitto_pub -h localhost -p 1883 -u admin -P admin123 -t test/topic -m "Hello"

# 订阅消息
mosquitto_sub -h localhost -p 1883 -u admin -P admin123 -t test/topic
```