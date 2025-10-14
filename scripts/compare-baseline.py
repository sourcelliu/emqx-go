#!/usr/bin/env python3
"""
EMQX-Go Chaos Testing Baseline Comparison Tool

Compares current test results against baseline to detect:
- Performance regression
- Reliability degradation
- Capacity reduction
"""

import os
import sys
import json
import glob
import re
from datetime import datetime

class BaselineComparator:
    def __init__(self, baseline_file):
        self.baseline = self.load_baseline(baseline_file)

    def load_baseline(self, filepath):
        """Load baseline data from JSON file"""
        if not os.path.exists(filepath):
            raise FileNotFoundError(f"Baseline file not found: {filepath}")

        with open(filepath, 'r') as f:
            return json.load(f)

    def parse_markdown_report(self, filepath):
        """Parse chaos test markdown report"""
        with open(filepath, 'r', encoding='utf-8') as f:
            content = f.read()

        result = {
            'scenario': '',
            'success': False,
            'success_rate': 0,
            'avg_latency': 0,
            'p99_latency': 0,
            'messages_sent': 0,
            'messages_received': 0,
        }

        # Extract scenario
        scenario_match = re.search(r'Scenario:\s*\*\*(.+?)\*\*', content)
        if scenario_match:
            result['scenario'] = scenario_match.group(1)

        # Extract status
        result['success'] = '✅ PASS' in content or 'Status: PASS' in content

        # Extract metrics
        success_rate_match = re.search(r'Success Rate:\s*([\d.]+)%', content)
        if success_rate_match:
            result['success_rate'] = float(success_rate_match.group(1))

        avg_latency_match = re.search(r'Average Latency:\s*([\d.]+)ms', content)
        if avg_latency_match:
            result['avg_latency'] = float(avg_latency_match.group(1))

        p99_match = re.search(r'P99 Latency:\s*([\d.]+)ms', content)
        if p99_match:
            result['p99_latency'] = float(p99_match.group(1))

        sent_match = re.search(r'Messages Sent:\s*(\d+)', content)
        if sent_match:
            result['messages_sent'] = int(sent_match.group(1))

        received_match = re.search(r'Messages Received:\s*(\d+)', content)
        if received_match:
            result['messages_received'] = int(received_match.group(1))

        return result

    def compare_scenario(self, scenario_name, current_data):
        """Compare current scenario results with baseline"""

        if scenario_name not in self.baseline:
            return {
                'status': 'NO_BASELINE',
                'message': f"No baseline data for scenario '{scenario_name}'"
            }

        baseline = self.baseline[scenario_name]
        comparison = {
            'scenario': scenario_name,
            'status': 'PASS',
            'regressions': [],
            'improvements': [],
            'metrics': {}
        }

        # Compare success rate
        success_rate_diff = current_data['success_rate'] - baseline.get('success_rate', 0)
        comparison['metrics']['success_rate'] = {
            'baseline': baseline.get('success_rate', 0),
            'current': current_data['success_rate'],
            'diff': success_rate_diff,
            'diff_pct': (success_rate_diff / baseline.get('success_rate', 1)) * 100 if baseline.get('success_rate', 0) > 0 else 0
        }

        if success_rate_diff < -5:  # More than 5% drop
            comparison['regressions'].append({
                'metric': 'success_rate',
                'severity': 'HIGH',
                'message': f"Success rate dropped by {abs(success_rate_diff):.1f}% ({baseline.get('success_rate', 0):.1f}% → {current_data['success_rate']:.1f}%)"
            })
            comparison['status'] = 'REGRESSION'
        elif success_rate_diff > 2:
            comparison['improvements'].append({
                'metric': 'success_rate',
                'message': f"Success rate improved by {success_rate_diff:.1f}%"
            })

        # Compare latency
        latency_diff = current_data['avg_latency'] - baseline.get('avg_latency', 0)
        comparison['metrics']['avg_latency'] = {
            'baseline': baseline.get('avg_latency', 0),
            'current': current_data['avg_latency'],
            'diff': latency_diff,
            'diff_pct': (latency_diff / baseline.get('avg_latency', 1)) * 100 if baseline.get('avg_latency', 0) > 0 else 0
        }

        if current_data['avg_latency'] > baseline.get('avg_latency', 0) * 1.5:  # 50% increase
            comparison['regressions'].append({
                'metric': 'avg_latency',
                'severity': 'MEDIUM',
                'message': f"Average latency increased by {latency_diff:.1f}ms ({baseline.get('avg_latency', 0):.1f}ms → {current_data['avg_latency']:.1f}ms)"
            })
            if comparison['status'] == 'PASS':
                comparison['status'] = 'REGRESSION'
        elif latency_diff < -10:
            comparison['improvements'].append({
                'metric': 'avg_latency',
                'message': f"Average latency improved by {abs(latency_diff):.1f}ms"
            })

        # Compare P99 latency
        p99_diff = current_data['p99_latency'] - baseline.get('p99_latency', 0)
        comparison['metrics']['p99_latency'] = {
            'baseline': baseline.get('p99_latency', 0),
            'current': current_data['p99_latency'],
            'diff': p99_diff,
            'diff_pct': (p99_diff / baseline.get('p99_latency', 1)) * 100 if baseline.get('p99_latency', 0) > 0 else 0
        }

        if current_data['p99_latency'] > baseline.get('p99_latency', 0) * 2:  # 100% increase
            comparison['regressions'].append({
                'metric': 'p99_latency',
                'severity': 'HIGH',
                'message': f"P99 latency doubled ({baseline.get('p99_latency', 0):.1f}ms → {current_data['p99_latency']:.1f}ms)"
            })
            comparison['status'] = 'REGRESSION'

        return comparison

    def generate_report(self, comparisons):
        """Generate comparison report"""

        report = []
        report.append("=" * 80)
        report.append("EMQX-Go Chaos Testing - Baseline Comparison Report")
        report.append("=" * 80)
        report.append(f"Generated: {datetime.now().strftime('%Y-%m-%d %H:%M:%S')}")
        report.append("")

        # Summary
        total = len(comparisons)
        regressions = sum(1 for c in comparisons if c['status'] == 'REGRESSION')
        passes = sum(1 for c in comparisons if c['status'] == 'PASS')
        no_baseline = sum(1 for c in comparisons if c['status'] == 'NO_BASELINE')

        report.append("SUMMARY")
        report.append("-" * 80)
        report.append(f"Total Scenarios:        {total}")
        report.append(f"Passed:                 {passes} ✓")
        report.append(f"Regressions Detected:   {regressions} {'⚠' if regressions > 0 else ''}")
        report.append(f"No Baseline:            {no_baseline}")
        report.append("")

        if regressions > 0:
            report.append("⚠️  WARNING: Performance regressions detected!")
            report.append("")

        # Detailed results
        report.append("DETAILED RESULTS")
        report.append("-" * 80)
        report.append("")

        for comparison in comparisons:
            if comparison['status'] == 'NO_BASELINE':
                report.append(f"Scenario: {comparison['scenario']}")
                report.append(f"  Status: NO BASELINE DATA")
                report.append("")
                continue

            status_symbol = "✓" if comparison['status'] == 'PASS' else "⚠"
            report.append(f"{status_symbol} Scenario: {comparison['scenario']}")
            report.append(f"  Status: {comparison['status']}")
            report.append("")

            # Metrics
            report.append("  Metrics:")
            for metric_name, metric_data in comparison['metrics'].items():
                report.append(f"    {metric_name}:")
                report.append(f"      Baseline: {metric_data['baseline']:.2f}")
                report.append(f"      Current:  {metric_data['current']:.2f}")
                report.append(f"      Diff:     {metric_data['diff']:+.2f} ({metric_data['diff_pct']:+.1f}%)")

            report.append("")

            # Regressions
            if comparison['regressions']:
                report.append("  ⚠️  REGRESSIONS:")
                for reg in comparison['regressions']:
                    report.append(f"    [{reg['severity']}] {reg['message']}")
                report.append("")

            # Improvements
            if comparison['improvements']:
                report.append("  ✓ IMPROVEMENTS:")
                for imp in comparison['improvements']:
                    report.append(f"    {imp['message']}")
                report.append("")

        report.append("=" * 80)

        # Overall verdict
        if regressions == 0:
            report.append("✅ OVERALL VERDICT: NO REGRESSIONS DETECTED")
        else:
            report.append(f"❌ OVERALL VERDICT: {regressions} REGRESSION(S) DETECTED - INVESTIGATION REQUIRED")

        report.append("=" * 80)

        return "\n".join(report)

def create_baseline(results_dir, output_file):
    """Create a new baseline from test results"""

    report_files = glob.glob(os.path.join(results_dir, '*.md'))

    if not report_files:
        print(f"❌ Error: No markdown reports found in {results_dir}")
        sys.exit(1)

    print(f"📄 Found {len(report_files)} test reports")

    baseline = {}
    comparator = BaselineComparator.__new__(BaselineComparator)

    for report_file in report_files:
        result = comparator.parse_markdown_report(report_file)
        if result['scenario']:
            baseline[result['scenario']] = {
                'success_rate': result['success_rate'],
                'avg_latency': result['avg_latency'],
                'p99_latency': result['p99_latency'],
                'messages_sent': result['messages_sent'],
                'messages_received': result['messages_received'],
                'created_at': datetime.now().isoformat()
            }
            print(f"   Added: {result['scenario']}")

    with open(output_file, 'w') as f:
        json.dump(baseline, f, indent=2)

    print(f"\n✅ Baseline created: {output_file}")
    print(f"📊 Scenarios: {len(baseline)}")

def main():
    if len(sys.argv) < 2:
        print("EMQX-Go Chaos Testing - Baseline Comparison Tool")
        print("")
        print("Usage:")
        print("  Create baseline:")
        print("    python3 compare-baseline.py --create <results-dir> [baseline.json]")
        print("")
        print("  Compare with baseline:")
        print("    python3 compare-baseline.py <baseline.json> <results-dir>")
        print("")
        print("Examples:")
        print("  python3 compare-baseline.py --create chaos-results-20251012/ baseline.json")
        print("  python3 compare-baseline.py baseline.json chaos-results-20251013/")
        sys.exit(1)

    # Create baseline mode
    if sys.argv[1] == '--create':
        if len(sys.argv) < 3:
            print("❌ Error: Missing results directory")
            sys.exit(1)

        results_dir = sys.argv[2]
        output_file = sys.argv[3] if len(sys.argv) > 3 else 'baseline.json'

        create_baseline(results_dir, output_file)
        sys.exit(0)

    # Compare mode
    baseline_file = sys.argv[1]
    results_dir = sys.argv[2]

    if not os.path.exists(baseline_file):
        print(f"❌ Error: Baseline file not found: {baseline_file}")
        sys.exit(1)

    if not os.path.exists(results_dir):
        print(f"❌ Error: Results directory not found: {results_dir}")
        sys.exit(1)

    print(f"📊 Comparing results against baseline: {baseline_file}")
    print("")

    # Load comparator
    comparator = BaselineComparator(baseline_file)

    # Find test reports
    report_files = glob.glob(os.path.join(results_dir, '*.md'))

    if not report_files:
        print(f"❌ Error: No markdown reports found in {results_dir}")
        sys.exit(1)

    print(f"📄 Found {len(report_files)} test reports")
    print("")

    # Compare each report
    comparisons = []
    for report_file in report_files:
        result = comparator.parse_markdown_report(report_file)
        if result['scenario']:
            print(f"   Comparing: {result['scenario']}...", end=' ')
            comparison = comparator.compare_scenario(result['scenario'], result)
            comparisons.append(comparison)

            if comparison['status'] == 'PASS':
                print("✓")
            elif comparison['status'] == 'REGRESSION':
                print("⚠ REGRESSION")
            else:
                print("⊘ NO BASELINE")

    print("")

    # Generate report
    report = comparator.generate_report(comparisons)
    print(report)

    # Save report
    output_file = os.path.join(results_dir, 'baseline-comparison.txt')
    with open(output_file, 'w') as f:
        f.write(report)

    print(f"\n💾 Report saved: {output_file}")

    # Exit with error code if regressions found
    regressions = sum(1 for c in comparisons if c['status'] == 'REGRESSION')
    sys.exit(1 if regressions > 0 else 0)

if __name__ == '__main__':
    main()
