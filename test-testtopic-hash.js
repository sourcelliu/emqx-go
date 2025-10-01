#!/usr/bin/env node

const mqtt = require('mqtt');

console.log('🎯 专门测试 testtopic/# 通配符');
console.log('模拟 MQTTX 桌面版的行为');
console.log('=====================================\n');

const client = mqtt.connect('mqtt://localhost:1883', {
    clientId: 'testtopic-test-' + Math.random().toString(16).substr(2, 8),
    username: 'test',
    password: 'test',
    protocolVersion: 4,
    clean: true,
    keepalive: 60
});

let receivedMessages = [];

client.on('connect', () => {
    console.log('✅ 连接成功');
    console.log('📝 客户端ID:', client.options.clientId);

    // 订阅 testtopic/#
    console.log('\n🔸 订阅: testtopic/#');
    client.subscribe('testtopic/#', { qos: 1 }, (error) => {
        if (error) {
            console.error('❌ 订阅失败:', error);
        } else {
            console.log('✅ 成功订阅 testtopic/#');
            console.log('📋 QoS 级别: 1');

            // 等待1秒确保订阅生效
            setTimeout(() => {
                console.log('\n📤 开始发布测试消息...');
                publishTestMessages();
            }, 1000);
        }
    });
});

client.on('error', (error) => {
    console.error('❌ 连接错误:', error.message);
    process.exit(1);
});

client.on('message', (topic, message, packet) => {
    const payload = message.toString();
    console.log(`📩 [QoS${packet.qos}] ${topic} -> ${payload}`);
    receivedMessages.push({ topic, payload, qos: packet.qos });
});

function publishTestMessages() {
    const messages = [
        // 应该匹配 testtopic/# 的消息
        { topic: 'testtopic', payload: 'testtopic 根消息' },
        { topic: 'testtopic/sub1', payload: 'testtopic 一级子消息' },
        { topic: 'testtopic/sub1/sub2', payload: 'testtopic 二级子消息' },
        { topic: 'testtopic/device/sensor/data', payload: 'testtopic 深层级消息' },

        // 不应该匹配的消息
        { topic: 'anothertopic', payload: '不匹配的消息1' },
        { topic: 'test', payload: '不匹配的消息2' },
        { topic: 'testtopic_similar', payload: '相似但不匹配的消息' }
    ];

    let index = 0;
    const publishNext = () => {
        if (index < messages.length) {
            const msg = messages[index];
            client.publish(msg.topic, msg.payload, { qos: 1, retain: false }, (error) => {
                if (error) {
                    console.error(`❌ 发布失败 ${msg.topic}:`, error);
                } else {
                    console.log(`📤 已发布: ${msg.topic} -> ${msg.payload}`);
                }
                index++;
                setTimeout(publishNext, 800); // 稍微慢一点，模拟手动发布
            });
        } else {
            setTimeout(showResults, 2000);
        }
    };

    publishNext();
}

function showResults() {
    console.log('\n📊 测试结果分析:');
    console.log('=====================================');

    const expectedTopics = [
        'testtopic',
        'testtopic/sub1',
        'testtopic/sub1/sub2',
        'testtopic/device/sensor/data'
    ];

    const shouldNotMatch = [
        'anothertopic',
        'test',
        'testtopic_similar'
    ];

    console.log(`✅ 总共收到 ${receivedMessages.length} 条消息:`);
    receivedMessages.forEach((msg, i) => {
        console.log(`  ${i+1}. [QoS${msg.qos}] ${msg.topic} -> ${msg.payload}`);
    });

    console.log('\n🎯 应该匹配 testtopic/# 的主题:');
    expectedTopics.forEach(topic => {
        const received = receivedMessages.some(msg => msg.topic === topic);
        console.log(`  ${received ? '✅' : '❌'} ${topic}`);
    });

    console.log('\n❌ 不应该匹配的主题:');
    shouldNotMatch.forEach(topic => {
        const wronglyReceived = receivedMessages.some(msg => msg.topic === topic);
        console.log(`  ${wronglyReceived ? '❌ 错误匹配' : '✅ 正确未匹配'} ${topic}`);
    });

    const successCount = expectedTopics.filter(topic =>
        receivedMessages.some(msg => msg.topic === topic)
    ).length;

    console.log(`\n📈 testtopic/# 匹配成功率: ${successCount}/${expectedTopics.length} (${Math.round(successCount/expectedTopics.length*100)}%)`);

    if (successCount === expectedTopics.length) {
        console.log('\n🎉 testtopic/# 通配符工作完全正常！');
        console.log('💡 如果在 MQTTX 桌面版中仍然看不到消息，请检查:');
        console.log('   1. MQTTX 的订阅时间是否在消息发布之前');
        console.log('   2. MQTTX 的 QoS 设置是否正确');
        console.log('   3. MQTTX 的 Clean Session 设置');
        console.log('   4. 是否有任何过滤器设置');
    } else {
        console.log('\n⚠️ 发现问题，某些预期的消息没有收到');
    }

    setTimeout(() => {
        client.end();
        console.log('\n👋 测试完成');
        process.exit(0);
    }, 1000);
}