#!/usr/bin/env node

const mqtt = require('mqtt');

console.log('🚀 MQTTX风格多客户端MQTT测试');
console.log('📡 模拟Publisher/Subscriber分离场景');
console.log('============================================\n');

// MQTT连接配置
const brokerUrl = 'mqtt://localhost:1883';
const baseOptions = {
    username: 'test',
    password: 'test',
    protocolVersion: 4,
    clean: true,
    keepalive: 60
};

// 测试场景配置
const scenarios = [
    {
        name: '传感器数据发布',
        topic: 'sensors/temperature/room1',
        message: '{"temperature": 23.5, "humidity": 45.2, "timestamp": "2025-10-01T15:30:00Z"}',
        qos: 1
    },
    {
        name: 'IoT设备状态',
        topic: 'devices/status/device001',
        message: '{"status": "online", "battery": 85, "signal": -45}',
        qos: 0
    },
    {
        name: '告警消息',
        topic: 'alerts/critical/fire',
        message: '{"type": "fire_alarm", "location": "building_a", "level": "critical"}',
        qos: 2
    },
    {
        name: '系统配置',
        topic: 'system/config/global',
        message: '{"update_interval": 30, "max_connections": 1000}',
        qos: 1,
        retain: true
    }
];

let publisherClient;
let subscriberClient;
let testResults = {};

async function createPublisher() {
    return new Promise((resolve, reject) => {
        const clientId = 'mqttx-publisher-' + Math.random().toString(16).substr(2, 8);
        const options = { ...baseOptions, clientId };

        console.log('📤 创建Publisher客户端:', clientId);

        const client = mqtt.connect(brokerUrl, options);

        client.on('connect', () => {
            console.log('✅ Publisher连接成功');
            resolve(client);
        });

        client.on('error', (error) => {
            console.error('❌ Publisher连接失败:', error.message);
            reject(error);
        });
    });
}

async function createSubscriber() {
    return new Promise((resolve, reject) => {
        const clientId = 'mqttx-subscriber-' + Math.random().toString(16).substr(2, 8);
        const options = { ...baseOptions, clientId };

        console.log('📥 创建Subscriber客户端:', clientId);

        const client = mqtt.connect(brokerUrl, options);

        client.on('connect', () => {
            console.log('✅ Subscriber连接成功');
            resolve(client);
        });

        client.on('error', (error) => {
            console.error('❌ Subscriber连接失败:', error.message);
            reject(error);
        });

        client.on('message', (topic, message, packet) => {
            const payload = message.toString();
            const qos = packet.qos;
            const retain = packet.retain;

            console.log(`📩 [QoS${qos}${retain ? ' RETAIN' : ''}] ${topic}: ${payload}`);

            // 记录测试结果
            testResults[topic] = {
                received: true,
                payload: payload,
                qos: qos,
                retain: retain,
                timestamp: new Date().toISOString()
            };
        });
    });
}

async function setupSubscriptions() {
    console.log('\n📋 设置订阅...');

    // 订阅具体主题
    const topicsToSubscribe = [
        { topic: 'sensors/+/+', qos: 1, description: '传感器数据（通配符）' },
        { topic: 'devices/status/+', qos: 0, description: '设备状态（通配符）' },
        { topic: 'alerts/critical/+', qos: 2, description: '紧急告警（通配符）' },
        { topic: 'system/config/global', qos: 1, description: '系统配置（具体主题）' }
    ];

    for (const sub of topicsToSubscribe) {
        await new Promise((resolve) => {
            subscriberClient.subscribe(sub.topic, { qos: sub.qos }, (error) => {
                if (error) {
                    console.error(`❌ 订阅失败 ${sub.topic}:`, error);
                } else {
                    console.log(`  ✓ 订阅 [QoS${sub.qos}] ${sub.topic} - ${sub.description}`);
                }
                resolve();
            });
        });
    }
}

async function publishMessages() {
    console.log('\n📤 开始发布消息...');

    for (const scenario of scenarios) {
        await new Promise((resolve) => {
            const options = {
                qos: scenario.qos,
                retain: scenario.retain || false
            };

            publisherClient.publish(scenario.topic, scenario.message, options, (error) => {
                if (error) {
                    console.error(`❌ 发布失败 ${scenario.name}:`, error);
                } else {
                    const retainText = options.retain ? ' [RETAIN]' : '';
                    console.log(`  ✓ 发布 [QoS${scenario.qos}]${retainText} ${scenario.name}: ${scenario.topic}`);
                }
                resolve();
            });
        });

        // 小延迟以观察消息顺序
        await new Promise(resolve => setTimeout(resolve, 500));
    }
}

async function testRetainedMessages() {
    console.log('\n🔄 测试保留消息 - 创建新订阅客户端...');

    const newClientId = 'mqttx-retained-test-' + Math.random().toString(16).substr(2, 8);
    const retainedClient = mqtt.connect(brokerUrl, { ...baseOptions, clientId: newClientId });

    return new Promise((resolve) => {
        retainedClient.on('connect', () => {
            console.log('✅ 保留消息测试客户端连接成功');

            retainedClient.on('message', (topic, message, packet) => {
                if (packet.retain) {
                    console.log(`📌 收到保留消息: ${topic} -> ${message.toString()}`);
                    testResults['retained_test'] = { success: true };
                }
            });

            // 订阅保留消息主题
            retainedClient.subscribe('system/config/global', (error) => {
                if (!error) {
                    console.log('  ✓ 订阅保留消息主题: system/config/global');
                }
            });

            setTimeout(() => {
                retainedClient.end();
                resolve();
            }, 2000);
        });
    });
}

async function showTestResults() {
    console.log('\n📊 测试结果统计:');
    console.log('============================================');

    const expectedTopics = scenarios.map(s => s.topic);
    let successCount = 0;

    for (const scenario of scenarios) {
        const result = testResults[scenario.topic];
        if (result && result.received) {
            console.log(`✅ ${scenario.name}: 成功`);
            console.log(`   主题: ${scenario.topic}`);
            console.log(`   QoS: ${result.qos}, 保留: ${result.retain ? '是' : '否'}`);
            console.log(`   时间: ${result.timestamp}`);
            successCount++;
        } else {
            console.log(`❌ ${scenario.name}: 失败`);
        }
        console.log('');
    }

    const retainedTest = testResults['retained_test'];
    if (retainedTest && retainedTest.success) {
        console.log('✅ 保留消息测试: 成功');
        successCount++;
    } else {
        console.log('❌ 保留消息测试: 失败');
    }

    console.log('============================================');
    console.log(`📈 总体成功率: ${successCount}/${scenarios.length + 1} (${Math.round(successCount/(scenarios.length + 1)*100)}%)`);

    // 显示broker连接统计
    console.log('\n📊 连接统计:');
    console.log(`Publisher客户端: ${publisherClient.options.clientId}`);
    console.log(`Subscriber客户端: ${subscriberClient.options.clientId}`);
    console.log(`总连接数: 3 (包括保留消息测试客户端)`);
}

async function cleanup() {
    console.log('\n🧹 清理连接...');

    if (publisherClient) {
        publisherClient.end();
        console.log('✓ Publisher客户端已断开');
    }

    if (subscriberClient) {
        subscriberClient.end();
        console.log('✓ Subscriber客户端已断开');
    }
}

// 主测试流程
async function runMQTTXTest() {
    try {
        // 创建客户端
        publisherClient = await createPublisher();
        subscriberClient = await createSubscriber();

        // 设置订阅
        await setupSubscriptions();

        // 等待订阅生效
        await new Promise(resolve => setTimeout(resolve, 1000));

        // 发布消息
        await publishMessages();

        // 等待消息传递
        await new Promise(resolve => setTimeout(resolve, 2000));

        // 测试保留消息
        await testRetainedMessages();

        // 显示结果
        await showTestResults();

        // 清理
        await cleanup();

        console.log('\n🎉 MQTTX风格测试完成!');

    } catch (error) {
        console.error('❌ 测试失败:', error);
        await cleanup();
        process.exit(1);
    }
}

// 启动测试
runMQTTXTest();