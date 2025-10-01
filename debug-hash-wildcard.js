#!/usr/bin/env node

const mqtt = require('mqtt');

console.log('🔍 专门测试多级通配符 # 功能');
console.log('=====================================\n');

const client = mqtt.connect('mqtt://localhost:1883', {
    clientId: 'hash-wildcard-test-' + Math.random().toString(16).substr(2, 8),
    username: 'test',
    password: 'test',
    protocolVersion: 4,
    clean: true
});

let receivedMessages = [];

client.on('connect', () => {
    console.log('✅ 连接成功');
    console.log('📝 客户端ID:', client.options.clientId);

    // 订阅多级通配符
    console.log('\n🔸 测试场景1: 订阅 test/#');
    client.subscribe('test/#', (error) => {
        if (error) {
            console.error('❌ 订阅失败:', error);
        } else {
            console.log('✅ 成功订阅 test/#');

            setTimeout(() => {
                console.log('\n📤 发布测试消息...');
                publishTestMessages();
            }, 1000);
        }
    });
});

client.on('error', (error) => {
    console.error('❌ 连接错误:', error.message);
    process.exit(1);
});

client.on('message', (topic, message) => {
    const payload = message.toString();
    console.log(`📩 收到: ${topic} -> ${payload}`);
    receivedMessages.push({ topic, payload });
});

function publishTestMessages() {
    const messages = [
        { topic: 'test', payload: '根级别消息' },
        { topic: 'test/level1', payload: '一级消息' },
        { topic: 'test/level1/level2', payload: '二级消息' },
        { topic: 'test/sensors/temperature/room1', payload: '深层级消息' },
        { topic: 'other/topic', payload: '不匹配的消息' }
    ];

    let index = 0;
    const publishNext = () => {
        if (index < messages.length) {
            const msg = messages[index];
            client.publish(msg.topic, msg.payload, (error) => {
                if (error) {
                    console.error(`❌ 发布失败 ${msg.topic}:`, error);
                } else {
                    console.log(`✅ 已发布: ${msg.topic}`);
                }
                index++;
                setTimeout(publishNext, 500);
            });
        } else {
            setTimeout(showResults, 2000);
        }
    };

    publishNext();
}

function showResults() {
    console.log('\n📊 测试结果:');
    console.log('=====================================');

    const expectedTopics = ['test', 'test/level1', 'test/level1/level2', 'test/sensors/temperature/room1'];

    console.log(`✅ 收到 ${receivedMessages.length} 条消息:`);
    receivedMessages.forEach((msg, i) => {
        console.log(`  ${i+1}. ${msg.topic} -> ${msg.payload}`);
    });

    console.log('\n🎯 期望匹配的主题:');
    expectedTopics.forEach(topic => {
        const received = receivedMessages.some(msg => msg.topic === topic);
        console.log(`  ${received ? '✅' : '❌'} ${topic}`);
    });

    console.log('\n❌ 不应该匹配的主题:');
    const shouldNotMatch = receivedMessages.filter(msg => msg.topic === 'other/topic');
    if (shouldNotMatch.length === 0) {
        console.log('  ✅ other/topic (正确未匹配)');
    } else {
        console.log('  ❌ other/topic (错误匹配了)');
    }

    const successRate = expectedTopics.filter(topic =>
        receivedMessages.some(msg => msg.topic === topic)
    ).length;

    console.log(`\n📈 成功率: ${successRate}/${expectedTopics.length} (${Math.round(successRate/expectedTopics.length*100)}%)`);

    setTimeout(() => {
        client.end();
        console.log('\n👋 测试完成');
        process.exit(0);
    }, 1000);
}