# MQTTX Comprehensive Test Suite for emqx-go

This directory contains a comprehensive test suite with **1000+ test cases** designed to validate the emqx-go MQTT broker implementation using MQTTX client testing methodology.

## Overview

The test suite is organized into 13 test modules covering all aspects of MQTT functionality:

### Original Test Suites (500 test cases)
1. **Basic Operations** (100 tests) - Connection, publish/subscribe, QoS, sessions
2. **Protocol Compliance** (80 tests) - MQTT 3.1.1 and 5.0 specification compliance
3. **Topic Management** (75 tests) - Wildcards, hierarchies, validation
4. **Concurrency Performance** (60 tests) - Load testing, throughput, scalability
5. **Security Auth** (50 tests) - Authentication, authorization, SSL/TLS
6. **Error Handling** (60 tests) - Edge cases, error recovery, resilience
7. **Real World Scenarios** (75 tests) - IoT simulations, practical use cases

### Additional Test Suites (500 test cases)
8. **Advanced MQTT 5.0** (60 tests) - Enhanced features, properties, flow control
9. **WebSocket Transport** (50 tests) - WebSocket MQTT transport testing
10. **Protocol Edge Cases** (60 tests) - Boundary conditions, limits, special cases
11. **Large Scale Integration** (80 tests) - Massive client loads, performance benchmarks
12. **IoT Scenarios** (120 tests) - Smart home, industrial IoT, vehicle telematics
13. **Security Compliance** (130 tests) - Authentication, encryption, regulatory compliance

## Quick Start

### Prerequisites
- Node.js 14.0.0 or higher
- Running emqx-go broker on localhost:1883
- npm or yarn package manager

### Installation
```bash
cd tests/e2e
npm install
```

### Running Tests

#### Run All Tests (1000+ test cases)
```bash
npm run test
# or
npm run test:comprehensive
```

#### Run Tests in Parallel (faster execution)
```bash
npm run test:parallel
```

#### Run Individual Test Suites
```bash
npm run test:basic           # Basic operations (100 tests)
npm run test:protocol        # Protocol compliance (80 tests)
npm run test:topics          # Topic management (75 tests)
npm run test:performance     # Performance tests (60 tests)
npm run test:security        # Security tests (50 tests)
npm run test:errors          # Error handling (60 tests)
npm run test:realworld       # Real-world scenarios (75 tests)
npm run test:mqtt5           # Advanced MQTT 5.0 (60 tests)
npm run test:websocket       # WebSocket transport (50 tests)
npm run test:edge-cases      # Protocol edge cases (60 tests)
npm run test:large-scale     # Large scale integration (80 tests)
npm run test:iot             # IoT scenarios (120 tests)
npm run test:compliance      # Security compliance (130 tests)
```

#### Debug Commands
```bash
npm run debug               # Debug payload format issues
npm run test:stop-on-failure # Stop on first test failure
```

## Test Configuration

### Broker Configuration
The tests expect the emqx-go broker to be running with:
- **Host**: localhost
- **Port**: 1883 (MQTT), 8883 (MQTTS), 8083 (WebSocket)
- **Authentication**: Username `test`, Password `test`
- **Protocol**: MQTT 3.1.1 and MQTT 5.0 support

### Test Categories

#### Core Functionality
- Connection establishment and management
- Message publishing and subscription
- QoS level handling (0, 1, 2)
- Topic wildcards and filters
- Session management (clean/persistent)

#### Advanced Features
- MQTT 5.0 enhanced features
- WebSocket transport
- SSL/TLS security
- Large-scale performance
- Error recovery and resilience

#### IoT Scenarios
- Smart home automation
- Industrial IoT monitoring
- Vehicle telematics
- Environmental monitoring
- Predictive maintenance

#### Security & Compliance
- Authentication and authorization
- End-to-end encryption
- Data privacy (GDPR, CCPA)
- Industry compliance (HIPAA, PCI-DSS, etc.)

## Test Reports

The test runner generates comprehensive reports in multiple formats:

### Console Output
Real-time test execution progress with:
- Suite-by-suite results
- Pass/fail statistics
- Performance metrics
- Error summaries

### File Reports
- **JSON Report**: `test-reports/mqttx-test-report-{timestamp}.json`
- **HTML Report**: `test-reports/mqttx-test-report-{timestamp}.html`

### Report Contents
- Overall test statistics
- Category breakdowns
- Individual suite results
- Performance benchmarks
- Failure analysis
- Recommendations for fixes

## Test Structure

Each test file follows this structure:
```javascript
class TestSuite {
  constructor() {
    this.helpers = new TestHelpers();
    this.clients = [];
  }

  async runAllTests() {
    // Execute test groups
    await this.runTestGroup1();
    await this.runTestGroup2();
    return this.helpers.generateReport();
  }

  async cleanup() {
    // Clean up resources
    await disconnectAll(this.clients);
  }
}
```

## Contributing

### Adding New Tests
1. Create test files following the naming convention: `mqttx-{category}-{description}.js`
2. Implement the test class with proper cleanup
3. Add test scripts to `package.json`
4. Update the comprehensive runner to include new suites

### Test Guidelines
- Use descriptive test names and categories
- Implement proper resource cleanup
- Include performance measurements where relevant
- Add error handling for network issues
- Document complex test scenarios

## Troubleshooting

### Common Issues

#### Connection Failures
- Ensure emqx-go broker is running on localhost:1883
- Check authentication credentials (test/test)
- Verify firewall settings allow MQTT connections

#### Test Timeouts
- Increase test timeouts for slow networks
- Run tests sequentially instead of parallel
- Check broker resource limits

#### Memory Issues
- Run fewer concurrent tests
- Increase Node.js heap size: `--max-old-space-size=4096`
- Monitor broker memory usage

### Debug Mode
Use the debug script to isolate specific issues:
```bash
npm run debug
```

This runs a minimal test to verify basic broker functionality and message format handling.

## Performance Expectations

### Test Execution Time
- **Full Suite**: ~30-45 minutes (sequential)
- **Full Suite**: ~15-25 minutes (parallel)
- **Individual Suites**: 2-5 minutes each

### Resource Requirements
- **Memory**: 2-4 GB available RAM
- **Network**: Stable localhost connection
- **CPU**: Multi-core recommended for parallel execution

### Pass Rate Targets
- **Development**: >80% pass rate acceptable
- **Staging**: >95% pass rate required
- **Production**: 100% pass rate required

## Integration with CI/CD

### GitHub Actions Example
```yaml
- name: Run MQTTX Tests
  run: |
    cd tests/e2e
    npm install
    npm run test:stop-on-failure
```

### Test Results Integration
The JSON report format can be integrated with:
- GitHub Actions test reporting
- Jenkins test result plugins
- SonarQube quality gates
- Custom monitoring dashboards

## License

Apache License 2.0 - See LICENSE file for details.

## Support

For issues related to:
- **Test Failures**: Check emqx-go source code implementation
- **Test Framework**: Create issues in the emqx-go repository
- **MQTT Protocol**: Refer to MQTT 3.1.1 and 5.0 specifications
- **Performance**: Review broker configuration and system resources