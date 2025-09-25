# Phase 2: Risk Analysis for MQTT Basic Protocol PoC

This document outlines the risks associated with the Proof-of-Concept (PoC) for the basic MQTT broker.

## 1. Single Point of Failure (SPOF)

*   **Risk:** The current PoC runs as a single instance. If this pod or the node it runs on fails, the entire service becomes unavailable. There is no redundancy. All client connections will be dropped, and the service will be down until the pod is rescheduled and restarted.

*   **Likelihood:** High (Failures are inevitable in any distributed system).

*   **Impact:** High (Total loss of service availability).

*   **Mitigation Strategies:**
    1.  **Clustering (Phase 3):** The primary mitigation is to implement a clustering mechanism, which is the goal of Phase 3. This will involve multiple nodes that can take over for each other.
    2.  **Kubernetes Deployments:** Using a Kubernetes `Deployment` or `StatefulSet` with a `replica` count greater than 1 will ensure that Kubernetes automatically maintains a desired number of running instances. This is a prerequisite for high availability.

## 2. Data Loss on Restart

*   **Risk:** The broker's state (session information, subscriptions) is stored entirely in memory (`storage.MemStore`). When the pod restarts for any reason (crash, deployment, node failure), all of this state is lost. Clients will have to reconnect and re-subscribe, and any notion of a persistent session is non-existent.

*   **Likelihood:** High (Restarts are a normal part of a container's lifecycle).

*   **Impact:** Medium to High (Disrupts client sessions and violates MQTT QoS > 0 expectations for session persistence).

*   **Mitigation Strategies:**
    1.  **Persistent Storage Backend (Future Phase):** Replace the `MemStore` with a durable storage backend like Postgres, Redis, or a distributed key-value store. This is a planned activity for a later phase.
    2.  **Data Replication (Future Phase):** In a clustered environment, session state will need to be replicated across nodes so that if one node fails, another can take over the session seamlessly.

## 3. Incomplete Protocol Implementation

*   **Risk:** The PoC handles only the most basic MQTT packets (CONNECT, SUBSCRIBE, PUBLISH, PING, DISCONNECT) and does not handle all features like QoS levels above 0, retained messages, Last Will & Testament, etc. This limits its functionality and correctness according to the full MQTT spec.

*   **Likelihood:** Certain (It is intentionally simplified for a PoC).

*   **Impact:** Medium (The broker is functional for basic use cases but not feature-complete).

*   **Mitigation Strategies:**
    1.  **Iterative Implementation (Future Phases):** Subsequent phases will build upon this foundation, adding support for higher QoS levels, retained messages, and other essential MQTT features.
    2.  **Comprehensive Testing:** As new features are added, the contract tests must be expanded to validate their correctness.

## 4. Inefficient Message Routing

*   **Risk:** The current broker `routePublish` method uses a hardcoded client ID to route messages, which is only a placeholder for the test to pass. It does not implement a proper topic-based routing mechanism.

*   **Likelihood:** Certain.

*   **Impact:** High (The broker is not a functional pub/sub system yet).

*   **Mitigation Strategies:**
    1.  **Topic Trie Implementation (Future Phase):** The standard and most efficient way to handle MQTT topic routing is with a Trie data structure. The broker will maintain a global subscription Trie, allowing for very fast lookups of subscribers for any given topic, including wildcard support. This is a critical component to be built in a future phase.