| Erlang Module | Go Analogue (Package) | Notes |
| :--- | :--- | :--- |
| `emqx_broker` | `pkg/broker` | Core broker logic, message routing, and dispatching. |
| `emqx_session` | `pkg/session` | Manages MQTT client sessions, subscriptions, and inflight messages. |
| `emqx_cm` | `pkg/cluster` | Cluster management, node discovery, and state synchronization. |
| `emqx_router` | `pkg/router` | Topic routing table (Trie) management. |
| `emqx_connection`| `pkg/transport` | Base module for handling network connections (TCP/TLS). |
| `emqx_channel` | `pkg/transport` | Handles the MQTT protocol state machine over a connection. |
| `emqx_listeners` | `pkg/transport` | Manages network listener sockets (e.g., TCP on port 1883). |
| `emqx_sup` | `pkg/supervisor` | Top-level supervisor for the entire application. Part of the mini-OTP framework. |
| `emqx_access_control` | `pkg/auth` | Authentication and authorization logic (ACLs). |
| `emqx_persistent_session` | `pkg/storage` | Handles the persistence of session data. |
| `emqx_shared_sub`| `pkg/subscription` | Logic for shared subscriptions. |
| `emqx_trie` | `pkg/router/trie` | The core Topic Trie data structure implementation. |
| `emqx_message` | `pkg/packets` | Defines the MQTT message structure. |
| `emqx_packet` | `pkg/packets` | MQTT packet parsing and serialization. |
| `emqx_ws_connection` | `pkg/transport/ws` | Handles WebSocket connections. |