# EMQX Core Functionality Tests for emqx-go

本目录包含从EMQX官方测试套件转化而来的核心功能测试，用于验证emqx-go实现的MQTT协议兼容性和核心功能正确性。

## 测试文件说明

### 1. broker_test.go - 消息路由与发布/订阅测试
源自: `emqx/apps/emqx/test/emqx_broker_SUITE.erl`

测试内容：
- **TestBasicPubSub**: 基础发布/订阅功能
- **TestMultipleSubscribers**: 消息扇出到多个订阅者
- **TestSharedSubscription**: 共享订阅（负载均衡）
- **TestNoSubscriberMessageDrop**: 无订阅者时消息丢弃
- **TestTopicLevels**: 多级主题订阅（通配符 #）
- **TestSingleLevelWildcard**: 单级通配符订阅（通配符 +）

核心验证：
- 消息正确路由到所有订阅者
- 共享订阅组内只有一个订阅者收到消息
- 通配符订阅正确匹配主题
- 消息顺序保证

### 2. connection_test.go - 连接管理测试
源自: `emqx/apps/emqx/test/emqx_cm_SUITE.erl` 和 `emqx_connection_SUITE.erl`

测试内容：
- **TestClientConnect**: 基础客户端连接/断开
- **TestCleanSession**: Clean Session标志行为
- **TestSessionTakeover**: 会话接管（重复客户端ID）
- **TestMultipleConnections**: 多并发连接
- **TestKeepAlive**: MQTT保活机制
- **TestWillMessage**: 遗嘱消息（Last Will and Testament）
- **TestConnectionRetry**: 连接重试机制
- **TestMaxConnections**: 最大连接数压力测试
- **TestClientIDValidation**: 客户端ID验证

核心验证：
- 连接状态管理正确
- 持久会话跨连接保持
- 会话接管机制正常工作
- 遗嘱消息在异常断开时发送
- 并发连接处理能力

### 3. qos_test.go - QoS服务质量测试
源自: `emqx/apps/emqx/test/emqx_mqueue_SUITE.erl` 和 `emqx_inflight_SUITE.erl`

测试内容：
- **TestQoS0AtMostOnce**: QoS 0 - 最多一次传递
- **TestQoS1AtLeastOnce**: QoS 1 - 至少一次传递
- **TestQoS2ExactlyOnce**: QoS 2 - 恰好一次传递
- **TestQoSDowngrade**: QoS降级机制
- **TestMixedQoSSubscriptions**: 混合QoS订阅
- **TestQoSWithRetainedMessage**: QoS与保留消息
- **TestQoSOrderPreservation**: 消息顺序保证

核心验证：
- QoS 0: 尽力传递，可能丢失
- QoS 1: 至少传递一次，可能重复
- QoS 2: 恰好传递一次，四次握手协议
- QoS降级规则：min(发布QoS, 订阅QoS)
- 同一客户端消息顺序保持

### 4. subscription_test.go - 订阅管理测试
源自: `emqx/apps/emqx/test/emqx_session_SUITE.erl` 和 `emqx_router_SUITE.erl`

测试内容：
- **TestBasicSubscription**: 基础订阅/取消订阅
- **TestMultipleSubscriptions**: 多主题订阅
- **TestSubscriptionPersistence**: 订阅持久化
- **TestResubscribe**: 重新订阅（QoS变更）
- **TestUnsubscribeBehavior**: 取消订阅行为
- **TestOverlappingSubscriptions**: 重叠订阅
- **TestSubscriptionWithDifferentClients**: 多客户端同主题订阅
- **TestSubscriptionFilters**: 订阅过滤器模式

核心验证：
- 订阅状态正确管理
- 持久会话保留订阅
- 通配符订阅正确匹配
- 取消订阅立即生效
- 订阅过滤器按MQTT规范工作

## 测试覆盖的MQTT核心功能

### MQTT 3.1.1 协议功能
- ✅ 基础连接管理（CONNECT, CONNACK, DISCONNECT）
- ✅ 发布/订阅（PUBLISH, SUBSCRIBE, UNSUBSCRIBE）
- ✅ QoS 0, 1, 2 三种服务质量等级
- ✅ 保活机制（PINGREQ, PINGRESP）
- ✅ 遗嘱消息（Last Will and Testament）
- ✅ 保留消息（Retained Messages）
- ✅ Clean Session 标志
- ✅ 会话接管（Session Takeover）
- ✅ 主题通配符（+, #）
- ✅ 共享订阅（$share/group/topic）

### 核心broker功能
- ✅ 消息路由引擎
- ✅ 主题树管理
- ✅ 订阅管理
- ✅ 会话管理
- ✅ 消息队列（QoS > 0）
- ✅ Inflight窗口管理
- ✅ 消息扇出（Fanout）
- ✅ 多订阅者消息分发

## 运行测试

### 前置条件
1. 启动emqx-go broker，带配置文件：
```bash
./bin/emqx-go -config config.yaml
```

2. 确保broker配置了测试用户认证：
```yaml
broker:
  auth:
    enabled: true
    users:
    - username: admin
      password: admin123
      algorithm: bcrypt
      enabled: true
```

### 运行所有测试
```bash
cd tests/emqx
go test -v
```

### 运行特定测试文件
```bash
go test -v -run TestBasicPubSub
go test -v -run "QoS"           # 运行所有QoS相关测试
go test -v -run "Subscription"  # 运行所有订阅相关测试
```

### 运行特定测试套件
```bash
# Broker测试
go test -v ./broker_test.go

# 连接管理测试
go test -v ./connection_test.go

# QoS测试
go test -v ./qos_test.go

# 订阅管理测试
go test -v ./subscription_test.go
```

## 测试设计原则

### 1. 忠实于EMQX原始测试
- 保持测试场景与EMQX官方测试一致
- 验证相同的MQTT协议行为
- 覆盖相同的边界条件和异常情况

### 2. Go语言习惯
- 使用testify断言库
- 遵循Go测试命名约定（Test*）
- 使用Go并发原语（channels, goroutines）

### 3. 测试隔离
- 每个测试使用独立的客户端ID
- 使用不同的测试主题避免干扰
- 测试结束时清理资源（defer cleanup）

### 4. 异步行为验证
- 使用channels和超时机制
- 明确等待broker处理完成
- 处理并发场景的竞争条件

## 测试结果参考

### 预期通过的测试
所有测试都应通过，表示emqx-go实现符合MQTT 3.1.1规范和EMQX核心功能要求。

### 测试失败可能的原因
1. **Broker未启动或未正确配置**
   - 检查broker进程是否运行
   - 验证配置文件加载正确

2. **认证失败**
   - 确认测试用户（admin/admin123）已配置
   - 检查认证算法是否匹配

3. **功能未实现**
   - 某些高级功能可能尚未实现（如共享订阅）
   - 参考测试日志确定具体缺失功能

4. **时序问题**
   - 增加sleep时间等待broker处理
   - 检查是否有并发竞争条件

## 与EMQX测试的对应关系

| emqx-go测试文件 | EMQX测试套件 | 测试行数 |
|----------------|-------------|---------|
| broker_test.go | emqx_broker_SUITE.erl | ~350 |
| connection_test.go | emqx_cm_SUITE.erl, emqx_connection_SUITE.erl | ~400 |
| qos_test.go | emqx_mqueue_SUITE.erl, emqx_inflight_SUITE.erl | ~350 |
| subscription_test.go | emqx_session_SUITE.erl, emqx_router_SUITE.erl | ~400 |
| **总计** | **4个测试套件** | **~1500行** |

## 扩展测试建议

### 短期扩展
- [ ] MQTT 5.0功能测试（用户属性、主题别名等）
- [ ] 规则引擎集成测试
- [ ] 数据桥接测试
- [ ] 认证授权测试

### 中期扩展
- [ ] 集群测试（跨节点消息路由）
- [ ] 性能基准测试
- [ ] 并发压力测试
- [ ] 故障注入测试

### 长期扩展
- [ ] 混沌工程测试
- [ ] 长时间稳定性测试
- [ ] 网络分区测试
- [ ] 内存泄漏检测

## 参考文档

- [MQTT 3.1.1 规范](http://docs.oasis-open.org/mqtt/mqtt/v3.1.1/os/mqtt-v3.1.1-os.html)
- [MQTT 5.0 规范](https://docs.oasis-open.org/mqtt/mqtt/v5.0/mqtt-v5.0.html)
- [EMQX测试架构分析](../../../docs/testing/e2e.md)
- [EMQX官方GitHub](https://github.com/emqx/emqx)

## 许可证

Apache License 2.0

## 维护者

emqx-go测试团队

---

**测试版本**: 1.0
**最后更新**: 2025-10-14
**基于EMQX版本**: 5.x
