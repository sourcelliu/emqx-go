# Chaos Testing for EMQX-Go

This directory contains Chaos Mesh experiments and automation scripts for testing the resilience of EMQX-Go.

## Quick Start

### 1. Prerequisites

```bash
# Install Chaos Mesh
helm repo add chaos-mesh https://charts.chaos-mesh.org
helm install chaos-mesh chaos-mesh/chaos-mesh \
  --namespace=chaos-mesh --create-namespace \
  --set chaosDaemon.runtime=containerd \
  --set chaosDaemon.socketPath=/run/containerd/containerd.sock

# Deploy EMQX-Go
kubectl apply -f ../k8s/
```

### 2. Run a Single Test

```bash
# Apply a chaos experiment
kubectl apply -f 01-pod-kill.yaml

# Watch the chaos
kubectl get podchaos -w

# Verify recovery
./verify-recovery.sh

# Clean up
kubectl delete -f 01-pod-kill.yaml
```

### 3. Run All Tests

```bash
# This will run all 10 tests automatically
./run-all-tests.sh

# Results will be saved to chaos-results-YYYYMMDD-HHMMSS/
```

## Available Tests

| # | Test Name | Type | Description | Duration |
|---|-----------|------|-------------|----------|
| 01 | pod-kill | PodChaos | Kill random pod | 10s |
| 02 | network-partition | NetworkChaos | Simulate network partition | 2m |
| 03 | network-delay | NetworkChaos | Add 250ms latency | 5m |
| 04 | cpu-stress | StressChaos | 80% CPU load | 3m |
| 05 | memory-stress | StressChaos | Allocate 512MB memory | 3m |
| 06 | network-loss | NetworkChaos | 20% packet loss | 5m |
| 07 | io-stress | IOChaos | 100ms disk latency | 3m |
| 08 | time-skew | TimeChaos | +5 minutes time offset | 2m |
| 09 | dns-failure | DNSChaos | Random DNS failures | 2m |
| 10 | container-kill | PodChaos | Kill container process | 30s |

## Test Files

```
chaos/
├── 01-pod-kill.yaml           # Pod failure test
├── 02-network-partition.yaml  # Network partition test
├── 03-network-delay.yaml      # Network latency test
├── 04-cpu-stress.yaml         # CPU pressure test
├── 05-memory-stress.yaml      # Memory pressure test
├── 06-network-loss.yaml       # Packet loss test
├── 07-io-stress.yaml          # Disk IO test
├── 08-time-skew.yaml          # Time synchronization test
├── 09-dns-failure.yaml        # DNS failure test
├── 10-container-kill.yaml     # Container kill test
├── verify-recovery.sh         # Recovery verification script
├── run-all-tests.sh           # Test automation script
└── README.md                  # This file
```

## Usage Examples

### Run Pod Kill Test

```bash
# Start the test
kubectl apply -f 01-pod-kill.yaml

# Watch pods
kubectl get pods -l app=emqx-go -w

# Check chaos status
kubectl get podchaos emqx-pod-kill

# Verify system recovered
./verify-recovery.sh

# Clean up
kubectl delete -f 01-pod-kill.yaml
```

### Run Network Partition Test

```bash
# This test simulates a network split
kubectl apply -f 02-network-partition.yaml

# Monitor cluster status
watch kubectl get pods -l app=emqx-go

# Wait for duration (2 minutes)
sleep 120

# Verify recovery
./verify-recovery.sh

# Clean up
kubectl delete -f 02-network-partition.yaml
```

### Custom Test Duration

Edit the YAML file to change duration:

```yaml
spec:
  duration: '10m'  # Change from default
```

## Monitoring During Tests

### Watch Pods

```bash
kubectl get pods -l app=emqx-go -w
```

### View Logs

```bash
kubectl logs -l app=emqx-go --tail=100 -f
```

### Check Events

```bash
kubectl get events --sort-by='.lastTimestamp' | grep emqx
```

### View Chaos Dashboard

```bash
kubectl port-forward -n chaos-mesh svc/chaos-dashboard 2333:2333
# Open http://localhost:2333
```

## Recovery Verification

The `verify-recovery.sh` script checks:

- ✅ All pods are running
- ✅ Service endpoints available
- ✅ MQTT connectivity (if mosquitto_pub available)
- ✅ Container restart count < 3
- ✅ No critical errors in logs

Example output:

```
=== Verifying EMQX-Go Recovery ===

1. Checking Pod status...
   ✓ All pods are running (3/3)

2. Checking service endpoints...
   ✓ Service endpoints available: 3

3. Testing MQTT connectivity...
   ✓ MQTT publish successful

4. Checking container restarts...
   ✓ Container restarts within acceptable range: 1

5. Checking for critical error logs...
   ✓ No critical errors found

6. Checking resource usage...
   NAME          CPU(cores)   MEMORY(bytes)
   emqx-go-0     125m         256Mi
   emqx-go-1     130m         245Mi
   emqx-go-2     120m         260Mi

=== Recovery Verification Summary ===
✓✓✓ All checks passed - System fully recovered
```

## Automated Testing

### Run All Tests

```bash
./run-all-tests.sh
```

This will:
1. Check prerequisites
2. Collect baseline metrics
3. Run all 10 tests sequentially
4. Collect results for each test
5. Verify recovery after each test
6. Generate a comprehensive report

### Test Results

Results are saved in `chaos-results-YYYYMMDD-HHMMSS/`:

```
chaos-results-20251010-120000/
├── baseline/
│   ├── pods.txt
│   ├── services.txt
│   ├── resources.txt
│   └── logs.txt
├── 01-pod-kill/
│   ├── start_time.txt
│   ├── pods_final.txt
│   ├── logs.txt
│   ├── recovery.txt
│   └── result.txt
├── 02-network-partition/
│   └── ...
└── REPORT.md
```

### Test Report

The `REPORT.md` includes:
- Summary table of all tests
- Pass/fail status
- Success rate
- Recommendations for failed tests

## Troubleshooting

### Chaos Not Applied

**Problem**: Chaos experiment doesn't take effect

**Solutions**:
```bash
# Check Chaos Mesh is running
kubectl get pods -n chaos-mesh

# Check RBAC permissions
kubectl auth can-i create podchaos --as=system:serviceaccount:default:default

# Verify label selector matches pods
kubectl get pods -l app=emqx-go
```

### System Not Recovering

**Problem**: Pods stuck after chaos experiment

**Solutions**:
```bash
# Delete all chaos experiments
kubectl delete podchaos --all
kubectl delete networkchaos --all
kubectl delete stresschaos --all

# Force restart StatefulSet
kubectl rollout restart statefulset emqx-go

# Check resource constraints
kubectl describe nodes
kubectl describe pods -l app=emqx-go
```

### Permission Denied

**Problem**: Cannot create chaos experiments

**Solutions**:
```bash
# Create service account with proper permissions
kubectl create serviceaccount chaos-admin
kubectl create clusterrolebinding chaos-admin \
  --clusterrole=cluster-admin \
  --serviceaccount=default:chaos-admin
```

## Best Practices

### 1. Start Small

Begin with simple tests like pod-kill before complex scenarios:

```bash
# Test 1: Pod kill (simple)
kubectl apply -f 01-pod-kill.yaml

# Test 2: Network delay (medium)
kubectl apply -f 03-network-delay.yaml

# Test 3: Network partition (complex)
kubectl apply -f 02-network-partition.yaml
```

### 2. Monitor Continuously

Keep monitoring dashboards open during tests:
- Kubernetes Dashboard
- Chaos Mesh Dashboard
- Grafana (if available)
- Application logs

### 3. Test in Isolation

Run tests one at a time to understand individual impact:

```bash
# Good: Sequential testing
kubectl apply -f 01-pod-kill.yaml
# Wait for completion and recovery
kubectl delete -f 01-pod-kill.yaml
sleep 60
kubectl apply -f 02-network-partition.yaml

# Bad: Overlapping chaos
kubectl apply -f 01-pod-kill.yaml
kubectl apply -f 02-network-partition.yaml  # Don't do this
```

### 4. Establish Baseline

Always collect baseline metrics before chaos testing:

```bash
# Collect baseline
kubectl get pods -l app=emqx-go -o wide > baseline-pods.txt
kubectl top pods -l app=emqx-go > baseline-resources.txt

# Run tests
./run-all-tests.sh

# Compare with baseline
diff baseline-pods.txt chaos-results-*/baseline/pods.txt
```

### 5. Document Findings

Keep notes on:
- What failed and why
- Recovery time observed
- Performance degradation
- Unexpected behaviors
- Required fixes or improvements

## Integration with CI/CD

### GitHub Actions Example

```yaml
name: Chaos Testing

on:
  schedule:
    - cron: '0 2 * * 0'  # Weekly on Sunday

jobs:
  chaos-test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v2

      - name: Setup Kubernetes
        uses: helm/kind-action@v1

      - name: Install Chaos Mesh
        run: |
          helm repo add chaos-mesh https://charts.chaos-mesh.org
          helm install chaos-mesh chaos-mesh/chaos-mesh -n chaos-mesh --create-namespace

      - name: Deploy EMQX-Go
        run: kubectl apply -f k8s/

      - name: Run Chaos Tests
        run: cd chaos && ./run-all-tests.sh

      - name: Upload Results
        uses: actions/upload-artifact@v2
        with:
          name: chaos-results
          path: chaos/chaos-results-*/
```

## Safety Considerations

⚠️ **NEVER run chaos tests in production without:**
1. Proper authorization from stakeholders
2. Backup and recovery procedures in place
3. Monitoring and alerting configured
4. Incident response team on standby
5. Maintenance window scheduled

✅ **Safe environments for chaos testing:**
- Development clusters
- Staging/QA environments
- Dedicated chaos testing clusters
- Production (only with Game Days and proper planning)

## Further Reading

- [Chaos Mesh Documentation](https://chaos-mesh.org/docs/)
- [Principles of Chaos Engineering](https://principlesofchaos.org/)
- [EMQX-Go Chaos Testing Plan](../chaos.md)

## Support

For issues or questions:
- Check the main [chaos testing documentation](../chaos.md)
- Review [EMQX-Go documentation](../README.md)
- Open an issue on GitHub
