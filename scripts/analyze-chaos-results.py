#!/usr/bin/env python3
"""
Chaos Testing Metrics Analyzer and Visualizer
Analyzes chaos test results and generates visual reports
"""

import json
import os
import sys
from datetime import datetime
from pathlib import Path
import re

def parse_metrics_file(filepath):
    """Parse Prometheus metrics file"""
    metrics = {}
    try:
        with open(filepath, 'r') as f:
            for line in f:
                line = line.strip()
                if line and not line.startswith('#'):
                    parts = line.split()
                    if len(parts) >= 2:
                        metric_name = parts[0]
                        metric_value = parts[1]
                        try:
                            metrics[metric_name] = float(metric_value)
                        except ValueError:
                            metrics[metric_name] = metric_value
    except Exception as e:
        print(f"Error parsing {filepath}: {e}", file=sys.stderr)
    return metrics

def analyze_scenario(scenario_dir):
    """Analyze a single chaos scenario"""
    result = {
        'name': os.path.basename(scenario_dir),
        'cluster_test_passed': False,
        'metrics': {},
        'observations': []
    }

    # Check cluster test result
    cluster_test_log = os.path.join(scenario_dir, 'cluster-test.log')
    if os.path.exists(cluster_test_log):
        with open(cluster_test_log, 'r') as f:
            content = f.read()
            result['cluster_test_passed'] = 'SUCCESS' in content

    # Parse metrics from all nodes
    for port in [8082, 8084, 8086]:
        metrics_file = os.path.join(scenario_dir, f'metrics-{port}.txt')
        if os.path.exists(metrics_file):
            node_metrics = parse_metrics_file(metrics_file)
            result['metrics'][f'node_{port}'] = node_metrics

    # Calculate aggregate metrics
    total_connections = 0
    total_messages_received = 0
    total_messages_sent = 0
    total_messages_dropped = 0

    for node_metrics in result['metrics'].values():
        total_connections += node_metrics.get('emqx_connections_count', 0)
        total_messages_received += node_metrics.get('emqx_messages_received_total', 0)
        total_messages_sent += node_metrics.get('emqx_messages_sent_total', 0)
        total_messages_dropped += node_metrics.get('emqx_messages_dropped_total', 0)

    result['aggregate'] = {
        'total_connections': total_connections,
        'total_messages_received': total_messages_received,
        'total_messages_sent': total_messages_sent,
        'total_messages_dropped': total_messages_dropped,
        'message_loss_rate': (total_messages_dropped / total_messages_sent * 100) if total_messages_sent > 0 else 0
    }

    # Observations
    if total_messages_dropped > 0:
        result['observations'].append(f"Message loss detected: {total_messages_dropped} messages")
    if not result['cluster_test_passed']:
        result['observations'].append("Cluster test failed")
    if total_connections == 0:
        result['observations'].append("No active connections")

    return result

def generate_report(results_dir):
    """Generate comprehensive chaos testing report"""
    print("="*80)
    print("CHAOS TESTING ANALYSIS REPORT")
    print("="*80)
    print(f"\nResults Directory: {results_dir}")
    print(f"Analysis Time: {datetime.now().strftime('%Y-%m-%d %H:%M:%S')}")
    print()

    # Find all scenario directories
    scenarios = []
    if os.path.exists(results_dir):
        for item in os.listdir(results_dir):
            item_path = os.path.join(results_dir, item)
            if os.path.isdir(item_path) and item != '.':
                scenarios.append(item_path)

    if not scenarios:
        print("No scenario results found")
        return

    # Analyze each scenario
    results = []
    for scenario_dir in sorted(scenarios):
        result = analyze_scenario(scenario_dir)
        results.append(result)

    # Print results table
    print("="*80)
    print("SCENARIO RESULTS")
    print("="*80)
    print(f"{'Scenario':<25} {'Status':<10} {'Connections':<15} {'Msg Recv':<12} {'Msg Drop':<10}")
    print("-"*80)

    passed = 0
    failed = 0

    for result in results:
        status = "✓ PASS" if result['cluster_test_passed'] else "✗ FAIL"
        connections = int(result['aggregate']['total_connections'])
        msg_recv = int(result['aggregate']['total_messages_received'])
        msg_drop = int(result['aggregate']['total_messages_dropped'])

        print(f"{result['name']:<25} {status:<10} {connections:<15} {msg_recv:<12} {msg_drop:<10}")

        if result['cluster_test_passed']:
            passed += 1
        else:
            failed += 1

    print("-"*80)
    print(f"\nTotal: {len(results)} scenarios | Passed: {passed} | Failed: {failed}")
    print(f"Success Rate: {passed/len(results)*100:.1f}%")

    # Print observations
    print("\n" + "="*80)
    print("OBSERVATIONS")
    print("="*80)
    for result in results:
        if result['observations']:
            print(f"\n{result['name']}:")
            for obs in result['observations']:
                print(f"  - {obs}")

    # System resilience score
    print("\n" + "="*80)
    print("SYSTEM RESILIENCE SCORE")
    print("="*80)

    # Calculate scores based on different criteria
    availability_score = (passed / len(results)) * 10

    # Calculate based on message loss across all scenarios
    total_loss_rate = sum(r['aggregate']['message_loss_rate'] for r in results) / len(results)
    reliability_score = max(0, 10 - (total_loss_rate / 10))

    # Overall score
    overall_score = (availability_score * 0.6 + reliability_score * 0.4)

    print(f"Availability Score: {availability_score:.1f}/10")
    print(f"Reliability Score: {reliability_score:.1f}/10")
    print(f"Overall Resilience: {overall_score:.1f}/10")

    if overall_score >= 9:
        grade = "Excellent ⭐⭐⭐⭐⭐"
    elif overall_score >= 7:
        grade = "Good ⭐⭐⭐⭐"
    elif overall_score >= 5:
        grade = "Fair ⭐⭐⭐"
    else:
        grade = "Needs Improvement ⭐⭐"

    print(f"\nGrade: {grade}")

    print("\n" + "="*80)

    # Generate JSON report
    json_report = {
        'timestamp': datetime.now().isoformat(),
        'results_dir': results_dir,
        'scenarios': results,
        'summary': {
            'total': len(results),
            'passed': passed,
            'failed': failed,
            'success_rate': passed/len(results)*100,
            'availability_score': availability_score,
            'reliability_score': reliability_score,
            'overall_score': overall_score,
            'grade': grade
        }
    }

    json_file = os.path.join(results_dir, 'analysis.json')
    with open(json_file, 'w') as f:
        json.dump(json_report, f, indent=2)

    print(f"JSON report saved to: {json_file}")

if __name__ == '__main__':
    if len(sys.argv) < 2:
        print("Usage: python3 analyze-chaos-results.py <results_directory>")
        sys.exit(1)

    results_dir = sys.argv[1]
    if not os.path.exists(results_dir):
        print(f"Error: Directory not found: {results_dir}")
        sys.exit(1)

    generate_report(results_dir)
