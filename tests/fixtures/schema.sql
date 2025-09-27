-- EMQX-Go Migration Target Schema for PostgreSQL
-- This schema represents the equivalent structures for EMQX data in PostgreSQL

-- Enable UUID extension
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- Sessions table
CREATE TABLE IF NOT EXISTS mqtt_sessions (
    client_id VARCHAR(255) PRIMARY KEY,
    clean_start BOOLEAN NOT NULL DEFAULT true,
    keepalive_interval INTEGER NOT NULL DEFAULT 60,
    session_state JSONB DEFAULT '{}',
    subscriptions JSONB DEFAULT '{}',
    inflight_messages JSONB DEFAULT '[]',
    created_at BIGINT NOT NULL,
    updated_at BIGINT DEFAULT EXTRACT(epoch FROM NOW()),
    node_id VARCHAR(100),
    INDEX idx_sessions_node (node_id),
    INDEX idx_sessions_created (created_at)
);

-- Subscriptions table
CREATE TABLE IF NOT EXISTS mqtt_subscriptions (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    topic_filter VARCHAR(1000) NOT NULL,
    client_id VARCHAR(255) NOT NULL,
    qos SMALLINT NOT NULL CHECK (qos >= 0 AND qos <= 2),
    no_local BOOLEAN DEFAULT false,
    retain_as_published BOOLEAN DEFAULT false,
    retain_handling SMALLINT DEFAULT 0 CHECK (retain_handling >= 0 AND retain_handling <= 2),
    created_at BIGINT DEFAULT EXTRACT(epoch FROM NOW()),
    UNIQUE(topic_filter, client_id),
    FOREIGN KEY (client_id) REFERENCES mqtt_sessions(client_id) ON DELETE CASCADE
);

CREATE INDEX idx_subscriptions_topic ON mqtt_subscriptions(topic_filter);
CREATE INDEX idx_subscriptions_client ON mqtt_subscriptions(client_id);

-- Routes table (topic to subscribers mapping)
CREATE TABLE IF NOT EXISTS mqtt_routes (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    topic VARCHAR(1000) NOT NULL,
    client_id VARCHAR(255) NOT NULL,
    node_id VARCHAR(100),
    created_at BIGINT DEFAULT EXTRACT(epoch FROM NOW()),
    INDEX idx_routes_topic (topic),
    INDEX idx_routes_client (client_id),
    INDEX idx_routes_node (node_id),
    UNIQUE(topic, client_id)
);

-- Retained messages table
CREATE TABLE IF NOT EXISTS mqtt_retained_messages (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    message_id VARCHAR(255),
    qos SMALLINT NOT NULL CHECK (qos >= 0 AND qos <= 2),
    topic VARCHAR(1000) NOT NULL UNIQUE,
    from_node VARCHAR(100),
    from_client_id VARCHAR(255),
    properties JSONB DEFAULT '{}',
    payload BYTEA,
    created_at BIGINT NOT NULL,
    expires_at BIGINT,
    INDEX idx_retained_topic (topic),
    INDEX idx_retained_expires (expires_at)
);

-- Connection states table
CREATE TABLE IF NOT EXISTS mqtt_connections (
    client_id VARCHAR(255) PRIMARY KEY,
    peer_host INET NOT NULL,
    peer_port INTEGER NOT NULL,
    node_id VARCHAR(100) NOT NULL,
    conn_state VARCHAR(20) NOT NULL CHECK (conn_state IN ('connected', 'disconnected', 'connecting')),
    connected_at BIGINT NOT NULL,
    properties JSONB DEFAULT '{}',
    last_seen BIGINT DEFAULT EXTRACT(epoch FROM NOW()),
    INDEX idx_connections_node (node_id),
    INDEX idx_connections_state (conn_state),
    INDEX idx_connections_last_seen (last_seen)
);

-- Cluster nodes table
CREATE TABLE IF NOT EXISTS mqtt_cluster_nodes (
    node_id VARCHAR(100) PRIMARY KEY,
    node_status VARCHAR(20) NOT NULL CHECK (node_status IN ('running', 'stopped', 'joining', 'leaving')),
    started_at BIGINT NOT NULL,
    node_host INET NOT NULL,
    last_heartbeat BIGINT DEFAULT EXTRACT(epoch FROM NOW()),
    metadata JSONB DEFAULT '{}',
    INDEX idx_nodes_status (node_status),
    INDEX idx_nodes_heartbeat (last_heartbeat)
);

-- Topic and client metrics table
CREATE TABLE IF NOT EXISTS mqtt_metrics (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    metric_type VARCHAR(20) NOT NULL CHECK (metric_type IN ('topic_metrics', 'client_metrics')),
    resource_name VARCHAR(1000) NOT NULL,
    message_count BIGINT DEFAULT 0,
    byte_count BIGINT DEFAULT 0,
    recorded_at BIGINT NOT NULL,
    node_id VARCHAR(100),
    INDEX idx_metrics_type_resource (metric_type, resource_name),
    INDEX idx_metrics_recorded (recorded_at),
    INDEX idx_metrics_node (node_id)
);

-- ACL (Access Control List) table
CREATE TABLE IF NOT EXISTS mqtt_acl_rules (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    client_id VARCHAR(255) NOT NULL,
    action VARCHAR(10) NOT NULL CHECK (action IN ('publish', 'subscribe', 'all')),
    permission VARCHAR(10) NOT NULL CHECK (permission IN ('allow', 'deny')),
    topic_filter VARCHAR(1000) NOT NULL,
    scope VARCHAR(10) DEFAULT 'all',
    created_at BIGINT DEFAULT EXTRACT(epoch FROM NOW()),
    INDEX idx_acl_client (client_id),
    INDEX idx_acl_action (action),
    INDEX idx_acl_topic (topic_filter),
    UNIQUE(client_id, action, topic_filter)
);

-- Message queue for persistent sessions (QoS 1/2 messages)
CREATE TABLE IF NOT EXISTS mqtt_message_queue (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    client_id VARCHAR(255) NOT NULL,
    message_id VARCHAR(255) NOT NULL,
    topic VARCHAR(1000) NOT NULL,
    qos SMALLINT NOT NULL CHECK (qos >= 1 AND qos <= 2),
    payload BYTEA NOT NULL,
    properties JSONB DEFAULT '{}',
    state VARCHAR(20) DEFAULT 'pending' CHECK (state IN ('pending', 'inflight', 'acked', 'expired')),
    created_at BIGINT DEFAULT EXTRACT(epoch FROM NOW()),
    expires_at BIGINT,
    retry_count INTEGER DEFAULT 0,
    INDEX idx_queue_client (client_id),
    INDEX idx_queue_state (state),
    INDEX idx_queue_expires (expires_at),
    FOREIGN KEY (client_id) REFERENCES mqtt_sessions(client_id) ON DELETE CASCADE
);

-- Sequence numbers for message ordering
CREATE SEQUENCE IF NOT EXISTS mqtt_packet_id_seq START 1;

-- Views for common queries
CREATE VIEW active_sessions AS
SELECT s.*, c.conn_state, c.peer_host, c.last_seen
FROM mqtt_sessions s
LEFT JOIN mqtt_connections c ON s.client_id = c.client_id
WHERE c.conn_state = 'connected' OR c.conn_state IS NULL;

CREATE VIEW subscription_summary AS
SELECT
    topic_filter,
    COUNT(*) as subscriber_count,
    COUNT(DISTINCT SUBSTRING(client_id, 1, POSITION('@' IN client_id || '@')-1)) as unique_clients
FROM mqtt_subscriptions
GROUP BY topic_filter;

-- Triggers for automatic timestamp updates
CREATE OR REPLACE FUNCTION update_timestamp()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = EXTRACT(epoch FROM NOW());
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER update_sessions_timestamp
    BEFORE UPDATE ON mqtt_sessions
    FOR EACH ROW EXECUTE FUNCTION update_timestamp();

CREATE TRIGGER update_connections_last_seen
    BEFORE UPDATE ON mqtt_connections
    FOR EACH ROW EXECUTE FUNCTION update_timestamp();