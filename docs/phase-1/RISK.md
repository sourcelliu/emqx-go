# Phase 1: Risk Analysis for mini-OTP Runtime

This document outlines the risks associated with the Proof-of-Concept (PoC) implementation of the mini-OTP runtime, which includes the `actor` and `supervisor` packages.

## 1. Unbounded Mailbox Growth

*   **Risk:** The `actor.Mailbox` is implemented using a buffered Go channel. If a producer sends messages faster than the actor can process them, the buffer will fill up. Subsequent `Send` calls will block. If the producer is a critical part of the system (like a network listener), this could lead to a cascading failure where the whole system becomes unresponsive. While the current implementation uses a buffered channel, a naive implementation could have used an unbuffered one, or a very large buffer, which can hide performance issues.

*   **Likelihood:** High (It's a common problem in actor systems).

*   **Impact:** Medium to High (Can cause performance degradation or a total system freeze).

*   **Mitigation Strategies:**
    1.  **Bounded Channels (Current Implementation):** The `NewMailbox` function already accepts a size, creating a bounded channel. This is the primary mitigation.
    2.  **Back-Pressure:** The blocking nature of a full channel is a form of back-pressure. The system must be designed to handle this gracefully. For example, a network connection actor might stop reading from the TCP socket if its mailbox is full.
    3.  **Metrics and Monitoring:** Expose the mailbox length as a Prometheus metric (`emqx_go_actor_mailbox_length`). This allows for monitoring and alerting when mailboxes are consistently full, indicating a performance bottleneck in the actor.
    4.  **Message Dropping:** For non-critical messages, an actor could be designed to drop messages if its mailbox is full, although this is not suitable for MQTT.

## 2. Supervisor Restart Loops

*   **Risk:** The `one_for_one` supervisor will relentlessly restart a child actor that fails, as long as its restart strategy is `Permanent` or `Transient`. If an actor fails immediately on startup (e.g., due to a misconfiguration that isn't going away), the supervisor will enter a tight loop, consuming CPU and spamming logs.

*   **Likelihood:** Medium (Configuration errors or persistent external resource failures are common).

*   **Impact:** Medium (Can lead to high CPU usage and make logs unusable).

*   **Mitigation Strategies:**
    1.  **Restart Rate Limiting (Not Implemented):** A more robust supervisor (like Erlang's) allows specifying a maximum restart frequency (e.g., 5 restarts in 10 seconds). If this limit is exceeded, the supervisor stops trying to restart the child and escalates the failure to its own supervisor. This is a critical feature for a production-grade system.
    2.  **Delayed Restarts (Partially Implemented):** The current PoC has a hardcoded `1 * time.Second` delay before restarting. This is a simple but effective way to prevent a CPU-spinning tight loop. This should be made configurable.

## 3. Graceful Shutdown Complexity

*   **Risk:** Graceful shutdown in a complex, supervised actor system is non-trivial. The current implementation relies on cancelling a top-level context. However, actors might be in the middle of critical operations. A simple context cancellation might not be enough to ensure a clean shutdown (e.g., persisting state to disk).

*   **Likelihood:** High.

*   **Impact:** Low to High (Depends on the actor's responsibility. Can range from lost messages to data corruption).

*   **Mitigation Strategies:**
    1.  **Shutdown Protocols:** Actors should handle the `ctx.Done()` signal gracefully. This may involve a two-phase shutdown: first, stop accepting new messages; second, process remaining messages in the mailbox before terminating.
    2.  **Configurable Shutdown Timeouts:** The `Spec` includes a `Shutdown` duration. This is not yet fully implemented in the supervisor logic but is the correct place to define how long to wait for a child to terminate gracefully before forcefully killing it (if possible). The supervisor should manage this timeout process.

## 4. Lack of Actor Isolation

*   **Risk:** Unlike Erlang processes, Go goroutines do not have their own isolated, preemptively scheduled heaps. A misbehaving actor (e.g., one that allocates a huge amount of memory or enters an infinite, non-blocking loop) can impact the performance of the entire Go runtime and all other actors.

*   **Likelihood:** Medium.

*   **Impact:** High (Can cause the entire application to become slow or crash with an Out-Of-Memory error).

*   **Mitigation Strategies:**
    1.  **Careful Code Review and Testing:** This is the primary defense. The logic of each actor must be scrutinized for unbounded resource consumption.
    2.  **Resource Pooling:** Use pools for large objects to manage memory allocation more carefully.
    3.  **Runtime Monitoring:** Monitor the overall memory and CPU usage of the Go process. While this doesn't isolate the faulty actor, it can alert operators to a problem.