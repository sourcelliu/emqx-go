#!/usr/bin/env node

const mqtt = require('mqtt');

console.log('🩺 MQTT 通配符全面诊断工具');
console.log('请告诉我您使用的具体主题模式');
console.log('=====================================\n');

const client = mqtt.connect('mqtt://localhost:1883', {
    clientId: 'diagnostic-' + Math.random().toString(16).substr(2, 8),
    username: 'test',
    password: 'test',
    protocolVersion: 4,
    clean: true
});

client.on('connect', () => {
    console.log('✅ 连接成功');
    console.log('📝 客户端ID:', client.options.clientId);

    runDiagnostics();
});

client.on('error', (error) => {
    console.error('❌ 连接错误:', error.message);
    process.exit(1);
});

let messageCount = 0;
client.on('message', (topic, message, packet) => {
    messageCount++;
    const payload = message.toString();
    console.log(`📩 [${messageCount}] ${topic} (QoS ${packet.qos}) -> ${payload}`);
});

async function runDiagnostics() {
    console.log('\n🔍 测试常见的通配符模式:');

    const testCases = [
        {
            pattern: '#',
            description: '全局通配符',
            testTopics: ['test', 'test/sub', 'any/topic/here', 'level1/level2/level3']
        },
        {
            pattern: 'home/#',
            description: '前缀 + 多级通配符',
            testTopics: ['home', 'home/room1', 'home/room1/temperature', 'other/topic']
        },
        {
            pattern: 'sensor/+/temperature',
            description: '单级通配符',
            testTopics: ['sensor/room1/temperature', 'sensor/room2/temperature', 'sensor/kitchen/humidity']
        },
        {
            pattern: '+/+/status',
            description: '多个单级通配符',
            testTopics: ['device/room1/status', 'sensor/kitchen/status', 'device/room1/temperature']
        },
        {
            pattern: 'building/+/#',
            description: '混合通配符',
            testTopics: ['building/floor1', 'building/floor1/room1', 'building/floor2/room3/sensor']
        }
    ];

    for (let i = 0; i < testCases.length; i++) {
        const testCase = testCases[i];
        console.log(`\n🔸 测试 ${i + 1}: ${testCase.pattern} (${testCase.description})`);

        await new Promise((resolve) => {
            client.subscribe(testCase.pattern, { qos: 1 }, (error) => {
                if (error) {
                    console.error(`❌ 订阅失败:`, error);
                } else {
                    console.log(`✅ 成功订阅: ${testCase.pattern}`);
                }
                resolve();
            });
        });

        // 等待订阅生效
        await new Promise(resolve => setTimeout(resolve, 200));

        // 发布测试消息
        for (const topic of testCase.testTopics) {
            await new Promise((resolve) => {
                const message = `测试消息发送到 ${topic}`;
                client.publish(topic, message, { qos: 1 }, (error) => {
                    if (error) {
                        console.error(`❌ 发布失败 ${topic}:`, error);
                    } else {
                        console.log(`📤 已发布: ${topic}`);
                    }
                    resolve();
                });
            });
            await new Promise(resolve => setTimeout(resolve, 100));
        }

        // 取消订阅
        await new Promise((resolve) => {
            client.unsubscribe(testCase.pattern, () => {
                console.log(`🗑️ 已取消订阅: ${testCase.pattern}`);
                resolve();
            });
        });

        await new Promise(resolve => setTimeout(resolve, 500));
    }

    console.log('\n📊 诊断完成！');
    console.log(`总共收到 ${messageCount} 条消息`);

    console.log('\n💡 问题排查建议:');
    console.log('1. 确保 # 通配符位于主题的末尾');
    console.log('2. 检查主题名称是否完全匹配');
    console.log('3. 确认客户端和服务器的 QoS 设置');
    console.log('4. 验证订阅和发布的时间顺序');

    setTimeout(() => {
        client.end();
        console.log('\n👋 诊断工具关闭');
    }, 2000);
}