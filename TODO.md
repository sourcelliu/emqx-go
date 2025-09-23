# Technical Debt & Known Issues

This document tracks known issues, bugs, and technical debt that need to be addressed in the future.

## QoS 1 and Session Persistence Deadlock

**Issue:**
A severe, persistent deadlock was encountered when implementing the end-to-end integration tests for QoS 1 message delivery and session persistence (`CleanSession=false`). The tests would consistently time out, indicating a deadlock or a complex race condition.

**Context:**
The deadlock appears during integration tests that involve multiple client connections and the interaction between the `connection` actor, the `broker` actor, and the `transport` layer.

**Debugging Steps Taken:**
Several attempts were made to diagnose and fix the issue, without success:
1.  Corrected multiple instances of malformed, handcrafted MQTT packet data in the test suite.
2.  Identified and fixed a data-sharing race condition in the `MemoryStore` implementation.
3.  Added extensive logging to trace the execution flow, but the tests froze before providing conclusive evidence.
4.  Performed a full workspace reset and a careful, step-by-step re-implementation of all related features. The deadlock persisted.
5.  Attempted to mitigate potential race conditions in the tests by adding `time.Sleep` delays. This did not resolve the timeout.

**Hypothesis:**
The root cause is likely a subtle deadlock in the concurrent design, involving the interaction between network I/O in the `connection` actor's `handleConnection` goroutine and the actor's own message-processing goroutine. This requires more advanced debugging tools (e.g., a debugger with goroutine inspection) to resolve.

**Resolution & Current State:**
To deliver a stable, verifiable PoC, all code related to QoS 1 and session persistence has been **reverted**. The current state of the `main` branch is a **working, tested, in-memory QoS 0 broker**.

The implementation of QoS 1 and session persistence is blocked until this deadlock can be properly investigated.
