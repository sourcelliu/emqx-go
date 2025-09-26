# Phase 2 Risk Analysis: MQTT Basic Protocol PoC

This document outlines the potential risks and limitations of the minimal MQTT broker proof-of-concept (PoC) developed in Phase 2.

## 1. Single Point of Failure (SPOF)

*   **Risk**: The current implementation is a single-node broker. If this single instance crashes or becomes unavailable for any reason, the entire service goes down. All active client connections will be dropped, and no new connections can be made until the broker is manually restarted. This design has zero fault tolerance.

*   **Mitigation Strategy**: This is the primary risk that will be addressed in **Phase 3: Clustering & Routing PoC**. By implementing a clustered architecture, we can have multiple nodes running the broker. If one node fails, clients can reconnect to other nodes in the cluster, and the service can remain available.

## 2. Data Loss on Restart (In-Memory Storage)

*   **Risk**: All critical data, including session information and topic subscriptions, is stored in memory using `MemStore` and `topic.Store`. If the broker process is restarted, all of this state is lost. This means:
    *   Clients with non-clean sessions will lose their session state.
    *   All subscription information will be wiped out, and clients will need to re-subscribe.
    *   Any in-flight messages or retained messages (if they were implemented) would be lost.

*   **Mitigation Strategy**: This risk will be addressed in **Phase 4: Data Migration Design & Scripts**. We will replace the in-memory stores with a pluggable interface that can connect to persistent storage solutions like Redis (for caching/session data) and Postgres (for more durable storage). This will allow session and subscription data to survive restarts.

## 3. No Scalability

*   **Risk**: The current single-node architecture has a hard limit on the number of concurrent connections and message throughput it can handle, bound by the resources (CPU, memory) of a single machine. It cannot scale horizontally to accommodate a growing number of clients or higher message volumes.

*   **Mitigation Strategy**: This is also addressed by the move to a clustered architecture in **Phase 3**. A cluster will allow for horizontal scaling, where the load can be distributed across multiple nodes, significantly increasing the overall capacity of the system.

## 4. Simplified Topic Matching

*   **Risk**: The current `topic.Store` implementation only supports exact topic matching. It does not handle MQTT wildcards (`+` and `#`), which are a fundamental feature of the protocol. This limits the usefulness of the broker in any realistic scenario.

*   **Mitigation Strategy**: The `topic.Store` will need to be replaced with a more sophisticated implementation, likely based on a trie data structure, which is well-suited for efficient wildcard matching. This will be a necessary enhancement as we move towards a more feature-complete broker.