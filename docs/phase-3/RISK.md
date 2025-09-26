# Phase 3: Risk Analysis for Cluster & Routing PoC

This document outlines the risks associated with the Proof-of-Concept (PoC) for the clustering and routing mechanism.

## 1. Network Partitions (Split Brain)

*   **Risk:** If the Kubernetes network has a partition, the cluster can split into two or more groups that cannot communicate with each other. In this "split-brain" scenario, each partition might believe it is the only active group. This can lead to inconsistent routing tables. For example, a client connected to Partition A might not receive messages published by a client in Partition B.

*   **Likelihood:** Medium (Network partitions are a known failure mode in distributed systems).

*   **Impact:** High (Leads to message loss and inconsistent state across the cluster).

*   **Mitigation Strategies:**
    1.  **Consensus Protocol:** A robust implementation would use a consensus algorithm like Raft (via an etcd or Consul-like backend) to manage a consistent view of the cluster membership and routing table. This ensures that there is always a single source of truth.
    2.  **Quorum:** The system should require a quorum (a majority of nodes) to be present to make changes to the routing table. Partitions without a quorum would operate in a read-only or degraded mode.
    3.  **Liveness Probes:** Aggressive Kubernetes liveness and readiness probes can help to quickly restart pods that are isolated or unhealthy, reducing the duration of a partition.

## 2. Incomplete Route Synchronization

*   **Risk:** The current PoC simulates broadcasting route updates but doesn't have a robust mechanism to ensure that a new node receives the *entire* current routing table when it joins the cluster. It only receives *new* updates. This means a newly joined node will have an incomplete view of the network until every topic is subscribed to again.

*   **Likelihood:** Certain (This is a known simplification in the PoC).

*   **Impact:** High (New nodes cannot route messages correctly, leading to message loss).

*   **Mitigation Strategies:**
    1.  **State Reconciliation:** When a new node joins (or a peer connection is established), it should perform an initial state reconciliation. The new node should request a full dump of the current routing table from an existing node.
    2.  **Versioned Routing Table:** The routing table can be versioned. Each update increments the version. Nodes can then easily check if they have the latest version or request missing updates from their peers.

## 3. gRPC Connection Management

*   **Risk:** The current PoC opens gRPC client connections to peers but has very basic error handling. If a connection to a peer drops, there is no automatic reconnection logic. The node will simply stop receiving or sending route updates to that peer until the discovery loop runs again and re-establishes the connection.

*   **Likelihood:** High.

*   **Impact:** Medium (Temporary message loss and routing inconsistencies during the disconnection period).

*   **Mitigation Strategies:**
    1.  **Exponential Backoff:** Implement an exponential backoff and retry mechanism for gRPC connections. When a connection fails, the client should wait for a short, increasing amount of time before trying to reconnect.
    2.  **Connection Health Checks:** The gRPC clients should periodically send keepalive pings to ensure the connection to the peer is still active. If pings fail, the client should proactively try to reconnect.

## 4. Scalability of Broadcast Updates

*   **Risk:** The current model implies that every subscription change is broadcast to every other node in the cluster. In a large cluster with many topics and frequent client connections/disconnections, this can create a huge amount of internal cluster traffic, potentially overwhelming the gRPC servers.

*   **Likelihood:** High (In a high-churn environment).

*   **Impact:** Medium (Increased network latency, higher CPU usage on all nodes).

*   **Mitigation Strategies:**
    1.  **Topic Sharding/Partitioning:** Instead of every node knowing about every topic, the topic space can be sharded across the nodes. Each node would only be responsible for a subset of topics. When a message is published, the broker would use a consistent hashing algorithm to determine which node is responsible for that topic and forward the message only to that node.
    2.  **Batching Updates:** Instead of sending a gRPC message for every single subscription change, updates can be batched together and sent periodically (e.g., every 100ms). This reduces the number of RPC calls significantly.