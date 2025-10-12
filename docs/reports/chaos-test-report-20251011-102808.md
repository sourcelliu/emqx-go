# CHAOS TEST EXECUTION REPORT

**Execution Time**: 2025-10-11T10:28:08+08:00

---

## Scenario: network-delay

- **Status**: ✅ PASS
- **Duration**: 10.385109ms
- **Messages Sent**: 0
- **Messages Received**: 0
- **Messages Lost**: 0

**Observations**:
- Message success rate: NaN%

**Errors**:
- Subscribe failed: write tcp [::1]:60720->[::1]:1883: use of closed network connection

---

## Scenario: network-loss

- **Status**: ✅ PASS
- **Duration**: 9.906777ms
- **Messages Sent**: 0
- **Messages Received**: 0
- **Messages Lost**: 0

**Observations**:
- Message success rate: NaN%

**Errors**:
- Subscribe failed: write tcp [::1]:60749->[::1]:1883: use of closed network connection

---

## Scenario: high-network-loss

- **Status**: ✅ PASS
- **Duration**: 11.98406ms
- **Messages Sent**: 0
- **Messages Received**: 0
- **Messages Lost**: 0

**Observations**:
- Message success rate: NaN%

**Errors**:
- Subscribe failed: write tcp [::1]:60773->[::1]:1883: use of closed network connection

---

## Scenario: cpu-stress

- **Status**: ✅ PASS
- **Duration**: 7.298829ms
- **Messages Sent**: 0
- **Messages Received**: 0
- **Messages Lost**: 0

**Observations**:
- Message success rate: NaN%

**Errors**:
- Subscribe failed: write tcp [::1]:60809->[::1]:1883: use of closed network connection

---

## Scenario: clock-skew

- **Status**: ✅ PASS
- **Duration**: 5.885776ms
- **Messages Sent**: 0
- **Messages Received**: 0
- **Messages Lost**: 0

**Observations**:
- Message success rate: NaN%

**Errors**:
- Subscribe failed: write tcp [::1]:60836->[::1]:1883: use of closed network connection

---

## Summary

- **Total Tests**: 5
- **Successful**: 5
- **Failed**: 0
- **Success Rate**: 100.0%
