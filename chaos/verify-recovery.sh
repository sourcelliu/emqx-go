#!/bin/bash
# chaos/verify-recovery.sh
# 验证 EMQX-Go 集群恢复状态

set -e

echo "=== Verifying EMQX-Go Recovery ==="

# 1. 检查所有 Pod 运行
echo ""
echo "1. Checking Pod status..."
READY_PODS=$(kubectl get pods -l app=emqx-go -o json | \
  jq '[.items[] | select(.status.phase=="Running" and .status.conditions[]? | select(.type=="Ready" and .status=="True"))] | length')
TOTAL_PODS=$(kubectl get pods -l app=emqx-go --no-headers | wc -l | tr -d ' ')

if [ "$READY_PODS" -eq "$TOTAL_PODS" ]; then
  echo "   ✓ All pods are running ($READY_PODS/$TOTAL_PODS)"
else
  echo "   ✗ Some pods are not ready ($READY_PODS/$TOTAL_PODS)"
  kubectl get pods -l app=emqx-go
  exit 1
fi

# 2. 检查服务端点
echo ""
echo "2. Checking service endpoints..."
MQTT_SERVICE=$(kubectl get service emqx-go-headless -o json 2>/dev/null || echo "{}")
if [ "$MQTT_SERVICE" != "{}" ]; then
  MQTT_ENDPOINTS=$(kubectl get endpoints emqx-go-headless -o json | \
    jq '.subsets[]?.addresses | length' 2>/dev/null || echo "0")
else
  # 如果使用不同的服务名,尝试查找
  MQTT_ENDPOINTS=$(kubectl get pods -l app=emqx-go -o json | \
    jq '[.items[] | select(.status.phase=="Running")] | length')
fi

if [ "$MQTT_ENDPOINTS" -ge 2 ]; then
  echo "   ✓ Service endpoints available: $MQTT_ENDPOINTS"
else
  echo "   ⚠ Low endpoint count: $MQTT_ENDPOINTS (expected >= 2)"
fi

# 3. 测试 MQTT 连接 (如果本地有 mosquitto_pub)
echo ""
echo "3. Testing MQTT connectivity..."
if command -v mosquitto_pub &> /dev/null; then
  # 尝试端口转发
  kubectl port-forward service/emqx-go-headless 1883:1883 &
  PF_PID=$!
  sleep 2

  timeout 10 mosquitto_pub -h localhost -p 1883 \
    -t "chaos/recovery-test" -m "recovery-test-$(date +%s)" -q 1 \
    -u test -P test 2>&1

  if [ $? -eq 0 ]; then
    echo "   ✓ MQTT publish successful"
  else
    echo "   ⚠ MQTT publish failed (this may be expected if auth is different)"
  fi

  kill $PF_PID 2>/dev/null || true
else
  echo "   ⚠ mosquitto_pub not found, skipping MQTT test"
fi

# 4. 检查容器重启次数
echo ""
echo "4. Checking container restarts..."
MAX_RESTARTS=$(kubectl get pods -l app=emqx-go -o json | \
  jq '[.items[].status.containerStatuses[]?.restartCount] | max // 0')

if [ "$MAX_RESTARTS" -lt 3 ]; then
  echo "   ✓ Container restarts within acceptable range: $MAX_RESTARTS"
else
  echo "   ⚠ High container restart count: $MAX_RESTARTS"
fi

# 5. 检查错误日志
echo ""
echo "5. Checking for critical error logs..."
ERROR_COUNT=$(kubectl logs -l app=emqx-go --tail=100 --since=2m 2>/dev/null | \
  grep -i "fatal\|panic" | wc -l | tr -d ' ')

if [ "$ERROR_COUNT" -eq 0 ]; then
  echo "   ✓ No critical errors found"
else
  echo "   ⚠ Found $ERROR_COUNT critical errors (review logs)"
  kubectl logs -l app=emqx-go --tail=20 --since=2m | grep -i "fatal\|panic" || true
fi

# 6. 检查资源使用
echo ""
echo "6. Checking resource usage..."
kubectl top pods -l app=emqx-go 2>/dev/null || echo "   ⚠ Metrics server not available"

echo ""
echo "=== Recovery Verification Summary ==="

if [ "$READY_PODS" -eq "$TOTAL_PODS" ] && [ "$MAX_RESTARTS" -lt 3 ] && [ "$ERROR_COUNT" -eq 0 ]; then
  echo "✓✓✓ All checks passed - System fully recovered"
  exit 0
else
  echo "⚠⚠⚠ Some checks failed - Review output above"
  exit 1
fi
