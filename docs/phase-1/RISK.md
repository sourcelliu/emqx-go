# Phase 1 Risk Analysis: Mini-OTP Runtime

This document outlines the potential risks and trade-offs associated with the mini-OTP (Actor and Supervisor) runtime developed in Phase 1.

## 1. Unbounded Mailbox Queues

*   **Risk**: The current `Mailbox` implementation is based on a buffered Go channel. If a producer actor sends messages faster than a consumer actor can process them, the buffered channel can fill up. When this happens, any subsequent `Send` calls will block the producer indefinitely, potentially causing a system-wide stall or deadlock. While the buffer provides some back-pressure, a consistently slow consumer can still lead to problems.

*   **Mitigation Strategies**:
    *   **Bounded Channels (Current Approach)**: We have used buffered channels, which provide a basic level of back-pressure. The size of the buffer is a critical tuning parameter.
    *   **Non-Blocking Sends**: We could introduce a non-blocking `Send` variant (e.g., `SendNB`) that returns an error if the mailbox is full. This would shift the responsibility of handling a full mailbox to the producer.
    *   **Metrics and Monitoring**: Expose the depth of each actor's mailbox as a Prometheus metric (e.g., `emqx_go_mailbox_depth`). This would allow for monitoring and alerting on actors that are consistently falling behind, indicating a performance bottleneck.
    *   **Message Dropping**: For non-critical messages, a strategy could be implemented to drop messages when the mailbox is full.

## 2. Performance of Channel-Based Communication

*   **Risk**: While Go channels are highly optimized, they are not zero-cost. In a high-throughput scenario with millions of messages per second, the overhead of channel operations (sending, receiving, and context switching) could become a performance bottleneck compared to more specialized message queue implementations.

*   **Mitigation Strategies**:
    *   **Benchmarking**: In later phases, we must conduct rigorous performance testing to measure the throughput and latency of the actor system under heavy load and compare it against the baseline.
    *   **Batching**: Introduce a mechanism for actors to send and receive messages in batches. This can significantly reduce the overhead of channel operations per message.
    *   **Alternative Queues**: If channel performance proves insufficient, explore more advanced, lock-free queue implementations from the Go ecosystem.

## 3. Lack of "Selective Receive"

*   **Risk**: The current `Mailbox` only supports FIFO (First-In, First-Out) message processing. Erlang's `receive` statement allows for "selective receive," where an actor can wait for a specific type of message, ignoring others in the mailbox. This is a powerful pattern for handling state machines and specific protocols. Our current implementation lacks this feature.

*   **Mitigation Strategies**:
    *   **State-Driven Processing**: The actor can manage its own internal state and ignore or re-queue messages that are not relevant to its current state. This simulates selective receive but can add complexity to the actor's logic.
    *   **Dedicated Channels**: For different message types that require different handling, multiple channels could be used within a single actor, and the actor could `select` on all of them. This can also increase complexity.
    *   **Future Enhancement**: If selective receive becomes a critical requirement, a more advanced mailbox implementation could be designed, but this would add significant complexity to the runtime. For now, the simpler FIFO approach is sufficient for the PoC.

## 4. Supervisor Restart Storms

*   **Risk**: If a child actor panics due to a persistent, non-transient error (e.g., a misconfiguration that it reads on startup), a `RestartPermanent` strategy could lead to a "restart storm," where the actor continuously panics and is restarted, consuming significant CPU and flooding the logs.

*   **Mitigation Strategies**:
    *   **Restart Backoff**: The current implementation includes a simple `1-second` delay. A more robust solution would be to implement an exponential backoff strategy, where the delay between restarts increases with each consecutive failure.
    *   **Max Restarts**: A supervisor could be configured with a maximum number of restarts within a given time period. If this limit is exceeded, the supervisor would stop trying to restart the child and could either shut down itself or escalate the failure.
    *   **Health Checks**: Introduce an optional health check mechanism where a supervisor can probe an actor's health before considering it "successfully started."