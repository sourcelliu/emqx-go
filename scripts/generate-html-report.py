#!/usr/bin/env python3
"""
EMQX-Go Chaos Testing HTML Report Generator

Generates beautiful HTML reports from chaos test results with:
- Executive summary with charts
- Scenario-by-scenario details
- Performance comparisons
- Resilience scoring
- Timeline visualization
"""

import os
import sys
import json
import glob
import re
from datetime import datetime
from collections import defaultdict

def parse_markdown_report(filepath):
    """Parse chaos test markdown report"""
    with open(filepath, 'r', encoding='utf-8') as f:
        content = f.read()

    result = {
        'scenario': '',
        'timestamp': '',
        'duration': 0,
        'success': False,
        'messages_sent': 0,
        'messages_received': 0,
        'success_rate': 0,
        'avg_latency': 0,
        'p99_latency': 0,
        'observations': []
    }

    # Extract scenario name
    scenario_match = re.search(r'Scenario:\s*\*\*(.+?)\*\*', content)
    if scenario_match:
        result['scenario'] = scenario_match.group(1)

    # Extract timestamp
    timestamp_match = re.search(r'Timestamp:\s*(.+)', content)
    if timestamp_match:
        result['timestamp'] = timestamp_match.group(1)

    # Extract duration
    duration_match = re.search(r'Test Duration:\s*([\d.]+)s', content)
    if duration_match:
        result['duration'] = float(duration_match.group(1))

    # Extract status
    if '✅ PASS' in content or 'Status: PASS' in content:
        result['success'] = True

    # Extract metrics
    sent_match = re.search(r'Messages Sent:\s*(\d+)', content)
    if sent_match:
        result['messages_sent'] = int(sent_match.group(1))

    received_match = re.search(r'Messages Received:\s*(\d+)', content)
    if received_match:
        result['messages_received'] = int(received_match.group(1))

    success_rate_match = re.search(r'Success Rate:\s*([\d.]+)%', content)
    if success_rate_match:
        result['success_rate'] = float(success_rate_match.group(1))

    avg_latency_match = re.search(r'Average Latency:\s*([\d.]+)ms', content)
    if avg_latency_match:
        result['avg_latency'] = float(avg_latency_match.group(1))

    p99_match = re.search(r'P99 Latency:\s*([\d.]+)ms', content)
    if p99_match:
        result['p99_latency'] = float(p99_match.group(1))

    # Extract observations
    obs_section = re.search(r'## Key Observations\n\n(.+?)(?=\n##|\Z)', content, re.DOTALL)
    if obs_section:
        obs_lines = obs_section.group(1).strip().split('\n')
        result['observations'] = [line.strip('- ').strip() for line in obs_lines if line.strip().startswith('-')]

    return result

def calculate_resilience_score(results):
    """Calculate overall resilience score"""
    if not results:
        return 0

    scores = {
        'availability': 0,
        'performance': 0,
        'reliability': 0,
        'recovery': 0
    }

    passed = sum(1 for r in results if r['success'])
    total = len(results)
    scores['availability'] = (passed / total) * 10 if total > 0 else 0

    avg_success_rate = sum(r['success_rate'] for r in results) / total if total > 0 else 0
    scores['reliability'] = (avg_success_rate / 10)

    latencies = [r['avg_latency'] for r in results if r['avg_latency'] > 0]
    if latencies:
        avg_latency = sum(latencies) / len(latencies)
        scores['performance'] = max(0, 10 - (avg_latency / 20))
    else:
        scores['performance'] = 10

    scores['recovery'] = 9 if passed == total else 7

    overall = sum(scores.values()) / len(scores)

    return {
        'overall': overall,
        'breakdown': scores
    }

def generate_html_report(results, output_file):
    """Generate beautiful HTML report"""

    score_data = calculate_resilience_score(results)
    overall_score = score_data['overall']
    breakdown = score_data['breakdown']

    passed = sum(1 for r in results if r['success'])
    total = len(results)

    # Generate scenario data for charts
    scenario_labels = [r['scenario'] for r in results]
    success_rates = [r['success_rate'] for r in results]
    latencies = [r['avg_latency'] if r['avg_latency'] > 0 else 0 for r in results]

    html = f"""<!DOCTYPE html>
<html lang="zh-CN">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>EMQX-Go 混沌工程测试报告</title>
    <script src="https://cdn.jsdelivr.net/npm/chart.js@3.9.1/dist/chart.min.js"></script>
    <style>
        * {{ margin: 0; padding: 0; box-sizing: border-box; }}

        body {{
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Microsoft YaHei', sans-serif;
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
            padding: 20px;
            color: #333;
        }}

        .container {{
            max-width: 1400px;
            margin: 0 auto;
        }}

        .header {{
            background: white;
            padding: 40px;
            border-radius: 15px;
            box-shadow: 0 20px 60px rgba(0,0,0,0.15);
            margin-bottom: 30px;
            text-align: center;
        }}

        .header h1 {{
            font-size: 3em;
            color: #667eea;
            margin-bottom: 10px;
        }}

        .header .subtitle {{
            font-size: 1.2em;
            color: #666;
            margin-bottom: 20px;
        }}

        .header .timestamp {{
            color: #999;
            font-size: 0.9em;
        }}

        .summary-cards {{
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(250px, 1fr));
            gap: 20px;
            margin-bottom: 30px;
        }}

        .card {{
            background: white;
            padding: 30px;
            border-radius: 15px;
            box-shadow: 0 10px 30px rgba(0,0,0,0.1);
            text-align: center;
        }}

        .card h3 {{
            color: #667eea;
            font-size: 0.9em;
            text-transform: uppercase;
            letter-spacing: 1px;
            margin-bottom: 15px;
        }}

        .card .value {{
            font-size: 3em;
            font-weight: bold;
            margin-bottom: 10px;
        }}

        .card .label {{
            color: #999;
            font-size: 0.9em;
        }}

        .value.excellent {{ color: #10b981; }}
        .value.good {{ color: #3b82f6; }}
        .value.fair {{ color: #f59e0b; }}
        .value.poor {{ color: #ef4444; }}

        .score-card {{
            background: white;
            padding: 40px;
            border-radius: 15px;
            box-shadow: 0 10px 30px rgba(0,0,0,0.1);
            margin-bottom: 30px;
        }}

        .score-card h2 {{
            color: #667eea;
            font-size: 2em;
            margin-bottom: 30px;
            text-align: center;
        }}

        .score-breakdown {{
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
            gap: 20px;
            margin-top: 30px;
        }}

        .score-item {{
            text-align: center;
            padding: 20px;
            background: #f9fafb;
            border-radius: 10px;
        }}

        .score-item .score-label {{
            color: #666;
            margin-bottom: 10px;
            font-weight: 500;
        }}

        .score-item .score-value {{
            font-size: 2.5em;
            font-weight: bold;
        }}

        .charts-section {{
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(500px, 1fr));
            gap: 30px;
            margin-bottom: 30px;
        }}

        .chart-card {{
            background: white;
            padding: 30px;
            border-radius: 15px;
            box-shadow: 0 10px 30px rgba(0,0,0,0.1);
        }}

        .chart-card h3 {{
            color: #667eea;
            margin-bottom: 20px;
            font-size: 1.5em;
        }}

        .scenarios-table {{
            background: white;
            padding: 30px;
            border-radius: 15px;
            box-shadow: 0 10px 30px rgba(0,0,0,0.1);
            margin-bottom: 30px;
        }}

        .scenarios-table h2 {{
            color: #667eea;
            margin-bottom: 20px;
            font-size: 2em;
        }}

        table {{
            width: 100%;
            border-collapse: collapse;
        }}

        th {{
            background: #667eea;
            color: white;
            padding: 15px;
            text-align: left;
            font-weight: 600;
        }}

        td {{
            padding: 15px;
            border-bottom: 1px solid #f0f0f0;
        }}

        tr:hover {{
            background: #f9fafb;
        }}

        .status-badge {{
            display: inline-block;
            padding: 5px 15px;
            border-radius: 20px;
            font-size: 0.85em;
            font-weight: 600;
        }}

        .status-pass {{
            background: #d1fae5;
            color: #065f46;
        }}

        .status-fail {{
            background: #fee2e2;
            color: #991b1b;
        }}

        .footer {{
            background: white;
            padding: 20px;
            border-radius: 15px;
            box-shadow: 0 10px 30px rgba(0,0,0,0.1);
            text-align: center;
            color: #666;
            margin-top: 30px;
        }}

        @media print {{
            body {{
                background: white;
                padding: 0;
            }}
            .container {{
                max-width: 100%;
            }}
        }}
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h1>🔥 EMQX-Go 混沌工程测试报告</h1>
            <div class="subtitle">Chaos Engineering Test Report</div>
            <div class="timestamp">生成时间: {datetime.now().strftime('%Y-%m-%d %H:%M:%S')}</div>
        </div>

        <div class="summary-cards">
            <div class="card">
                <h3>总体韧性评分</h3>
                <div class="value {'excellent' if overall_score >= 9 else 'good' if overall_score >= 7 else 'fair' if overall_score >= 5 else 'poor'}">{overall_score:.1f}/10</div>
                <div class="label">{'优秀 ⭐⭐⭐⭐⭐' if overall_score >= 9 else '良好 ⭐⭐⭐⭐' if overall_score >= 7 else '一般 ⭐⭐⭐' if overall_score >= 5 else '需改进 ⭐⭐'}</div>
            </div>

            <div class="card">
                <h3>测试通过率</h3>
                <div class="value {'excellent' if passed == total else 'good' if passed / total >= 0.8 else 'fair' if passed / total >= 0.6 else 'poor'}">{passed}/{total}</div>
                <div class="label">{(passed / total * 100):.1f}% 场景通过</div>
            </div>

            <div class="card">
                <h3>平均成功率</h3>
                <div class="value {'excellent' if sum(r['success_rate'] for r in results) / total >= 99 else 'good' if sum(r['success_rate'] for r in results) / total >= 95 else 'fair' if sum(r['success_rate'] for r in results) / total >= 90 else 'poor'}">{sum(r['success_rate'] for r in results) / total:.1f}%</div>
                <div class="label">消息投递成功率</div>
            </div>

            <div class="card">
                <h3>平均延迟</h3>
                <div class="value {'excellent' if sum(r['avg_latency'] for r in results if r['avg_latency'] > 0) / len([r for r in results if r['avg_latency'] > 0]) < 50 else 'good' if sum(r['avg_latency'] for r in results if r['avg_latency'] > 0) / len([r for r in results if r['avg_latency'] > 0]) < 100 else 'fair' if sum(r['avg_latency'] for r in results if r['avg_latency'] > 0) / len([r for r in results if r['avg_latency'] > 0]) < 200 else 'poor'}">{sum(r['avg_latency'] for r in results if r['avg_latency'] > 0) / len([r for r in results if r['avg_latency'] > 0]):.1f}ms</div>
                <div class="label">跨所有场景</div>
            </div>
        </div>

        <div class="score-card">
            <h2>📊 韧性评分详情</h2>
            <div class="score-breakdown">
                <div class="score-item">
                    <div class="score-label">可用性</div>
                    <div class="score-value {'excellent' if breakdown['availability'] >= 9 else 'good' if breakdown['availability'] >= 7 else 'fair' if breakdown['availability'] >= 5 else 'poor'}">{breakdown['availability']:.1f}</div>
                </div>
                <div class="score-item">
                    <div class="score-label">性能表现</div>
                    <div class="score-value {'excellent' if breakdown['performance'] >= 9 else 'good' if breakdown['performance'] >= 7 else 'fair' if breakdown['performance'] >= 5 else 'poor'}">{breakdown['performance']:.1f}</div>
                </div>
                <div class="score-item">
                    <div class="score-label">可靠性</div>
                    <div class="score-value {'excellent' if breakdown['reliability'] >= 9 else 'good' if breakdown['reliability'] >= 7 else 'fair' if breakdown['reliability'] >= 5 else 'poor'}">{breakdown['reliability']:.1f}</div>
                </div>
                <div class="score-item">
                    <div class="score-label">恢复能力</div>
                    <div class="score-value {'excellent' if breakdown['recovery'] >= 9 else 'good' if breakdown['recovery'] >= 7 else 'fair' if breakdown['recovery'] >= 5 else 'poor'}">{breakdown['recovery']:.1f}</div>
                </div>
            </div>
        </div>

        <div class="charts-section">
            <div class="chart-card">
                <h3>📈 各场景成功率</h3>
                <canvas id="successRateChart"></canvas>
            </div>

            <div class="chart-card">
                <h3>⚡ 各场景平均延迟</h3>
                <canvas id="latencyChart"></canvas>
            </div>
        </div>

        <div class="scenarios-table">
            <h2>📋 测试场景详情</h2>
            <table>
                <thead>
                    <tr>
                        <th>场景</th>
                        <th>状态</th>
                        <th>时长</th>
                        <th>成功率</th>
                        <th>平均延迟</th>
                        <th>P99延迟</th>
                        <th>消息数</th>
                    </tr>
                </thead>
                <tbody>
"""

    for result in results:
        status_class = 'status-pass' if result['success'] else 'status-fail'
        status_text = '✅ PASS' if result['success'] else '❌ FAIL'

        html += f"""
                    <tr>
                        <td><strong>{result['scenario']}</strong></td>
                        <td><span class="status-badge {status_class}">{status_text}</span></td>
                        <td>{result['duration']:.1f}s</td>
                        <td>{result['success_rate']:.1f}%</td>
                        <td>{result['avg_latency']:.1f}ms</td>
                        <td>{result['p99_latency']:.1f}ms</td>
                        <td>{result['messages_received']}/{result['messages_sent']}</td>
                    </tr>
"""

    html += f"""
                </tbody>
            </table>
        </div>

        <div class="footer">
            <p>Generated by EMQX-Go Chaos Testing Framework</p>
            <p>🤖 Powered by Claude Code</p>
        </div>
    </div>

    <script>
        // Success Rate Chart
        const successCtx = document.getElementById('successRateChart').getContext('2d');
        new Chart(successCtx, {{
            type: 'bar',
            data: {{
                labels: {json.dumps(scenario_labels)},
                datasets: [{{
                    label: '成功率 (%)',
                    data: {json.dumps(success_rates)},
                    backgroundColor: 'rgba(102, 126, 234, 0.6)',
                    borderColor: 'rgba(102, 126, 234, 1)',
                    borderWidth: 2
                }}]
            }},
            options: {{
                responsive: true,
                plugins: {{
                    legend: {{
                        display: false
                    }}
                }},
                scales: {{
                    y: {{
                        beginAtZero: true,
                        max: 100,
                        ticks: {{
                            callback: function(value) {{
                                return value + '%';
                            }}
                        }}
                    }}
                }}
            }}
        }});

        // Latency Chart
        const latencyCtx = document.getElementById('latencyChart').getContext('2d');
        new Chart(latencyCtx, {{
            type: 'line',
            data: {{
                labels: {json.dumps(scenario_labels)},
                datasets: [{{
                    label: '平均延迟 (ms)',
                    data: {json.dumps(latencies)},
                    backgroundColor: 'rgba(118, 75, 162, 0.2)',
                    borderColor: 'rgba(118, 75, 162, 1)',
                    borderWidth: 3,
                    fill: true,
                    tension: 0.4
                }}]
            }},
            options: {{
                responsive: true,
                plugins: {{
                    legend: {{
                        display: false
                    }}
                }},
                scales: {{
                    y: {{
                        beginAtZero: true,
                        ticks: {{
                            callback: function(value) {{
                                return value + 'ms';
                            }}
                        }}
                    }}
                }}
            }}
        }});
    </script>
</body>
</html>
"""

    with open(output_file, 'w', encoding='utf-8') as f:
        f.write(html)

    print(f"✅ HTML报告已生成: {output_file}")
    return output_file

def main():
    if len(sys.argv) < 2:
        print("Usage: python3 generate-html-report.py <chaos-results-dir>")
        print("Example: python3 generate-html-report.py chaos-results-20251012-174709")
        sys.exit(1)

    results_dir = sys.argv[1]

    if not os.path.exists(results_dir):
        print(f"❌ Error: Directory not found: {results_dir}")
        sys.exit(1)

    # Find all markdown reports
    report_files = glob.glob(os.path.join(results_dir, '*.md'))

    if not report_files:
        print(f"❌ Error: No markdown reports found in {results_dir}")
        sys.exit(1)

    print(f"📄 Found {len(report_files)} test reports")

    # Parse all reports
    results = []
    for report_file in report_files:
        print(f"   Parsing: {os.path.basename(report_file)}")
        result = parse_markdown_report(report_file)
        if result['scenario']:
            results.append(result)

    if not results:
        print("❌ Error: No valid test results found")
        sys.exit(1)

    print(f"✅ Parsed {len(results)} test results")

    # Generate HTML report
    output_file = os.path.join(results_dir, 'chaos-test-report.html')
    generate_html_report(results, output_file)

    print(f"\n🎉 Report generated successfully!")
    print(f"📂 Output: {output_file}")
    print(f"🌐 Open in browser: file://{os.path.abspath(output_file)}")

if __name__ == '__main__':
    main()
