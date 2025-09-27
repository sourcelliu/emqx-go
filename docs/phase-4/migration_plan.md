# Phase 4: Data Migration Plan

## 1. Introduction

This document outlines the strategy for migrating data from the existing Erlang-based EMQX system (using Mnesia/ETS) to the new Go-based implementation with a PostgreSQL backend. The goal is to perform this migration with minimal downtime and ensure data integrity.

The migration will be conducted in three main phases:
1.  **Initial Data Seeding**: A one-time bulk load of existing data into the new database.
2.  **Dual-Write Period**: A period where the live system writes to both the old and new databases simultaneously to keep them in sync.
3.  **Verification and Cutover**: The final switch to make the new system the primary data store.

## 2. Schema Mapping

The following table maps the key data structures from the Erlang system to the new PostgreSQL schema.

| Erlang (Mnesia/ETS Table) | Key Fields                 | Value Fields                                 | PostgreSQL Table         | Key Columns               | Value Columns                                                              |
| ------------------------- | -------------------------- | -------------------------------------------- | ------------------------ | ------------------------- | -------------------------------------------------------------------------- |
| `emqx_session`            | `client_id`                | `clean_start`, `keepalive`, `created_at`     | `mqtt_sessions`          | `client_id`               | `clean_start`, `keepalive_interval`, `created_at`                          |
| `emqx_suboption`          | `topic_filter`, `client_id`  | `qos`, `no_local`, `retain_as_published`   | `mqtt_subscriptions`     | `topic_filter`, `client_id` | `qos`, `no_local`, `retain_as_published`                                   |
| `emqx_message` (Retained) | `topic`                    | `payload`, `qos`, `from_client`, `from_node` | `mqtt_retained_messages` | `topic`                   | `payload`, `qos`, `from_client_id`, `from_node`                            |
| `emqx_acl`                | `client_id`, `action`      | `permission`, `topic_filter`                 | `mqtt_acl_rules`         | `client_id`, `action`     | `permission`, `topic_filter`                                               |
| `emqx_connection`         | `client_id`                | `peer_host`, `peer_port`, `conn_state`       | `mqtt_connections`       | `client_id`               | `peer_host`, `peer_port`, `conn_state`                                     |

## 3. Migration Strategy

### Phase 1: Initial Data Seeding

1.  **Halt Writes (Briefly)**: Temporarily pause any administrative actions that would modify the data to be migrated (e.g., changing ACLs, creating durable sessions). Live message flow can continue.
2.  **Data Dump**: Take a complete dump of the relevant Mnesia/ETS tables using the existing Erlang system's tools. This dump will be in the format of `tests/fixtures/mnesia_dump.txt`.
3.  **Run Migration Script**: Execute the Go migration tool developed in this phase (`tools/migration/main.go`) to parse the dump file and generate the `migration.sql` script.
4.  **Import to PostgreSQL**: Apply the `migration.sql` script to the new PostgreSQL database to perform the initial data seeding.
5.  **Run Validator**: Execute the `Validator` component of the migration tool to perform initial checks (e.g., row counts) to ensure the bulk import was successful.

### Phase 2: Dual-Write Implementation

This is the most critical phase for a live migration.

1.  **Modify Erlang System**: The existing Erlang EMQX system must be modified to implement a dual-write mechanism. For every operation that writes to a local Mnesia/ETS table (e.g., creating a session, adding a subscription), it must also write the corresponding data to the new PostgreSQL database.
    *   This can be achieved by adding hooks or modifying the data-handling functions in the Erlang code.
    *   The write to the PostgreSQL database should be asynchronous or have a short timeout to avoid impacting the performance of the live system.
    *   Failures to write to PostgreSQL should be logged extensively for later reconciliation.
2.  **Deploy Dual-Write Code**: Deploy the updated Erlang code to the production environment.
3.  **Monitor**: Closely monitor both the old and new databases to ensure they are staying in sync. Run the `Validator` tool periodically to check for inconsistencies.

### Phase 3: Verification and Cutover

1.  **Final Verification**: Once the dual-write system has been running stably for a period (e.g., 24-48 hours), perform a final, comprehensive validation. This may involve more detailed checks, such as comparing individual records.
2.  **Traffic Shift**: Begin shifting live traffic from the old Erlang-based system to the new Go-based system. This can be done gradually (e.g., using a canary release) or all at once, depending on the risk tolerance.
3.  **Read from New System**: The new Go system will now read exclusively from the PostgreSQL database.
4.  **Decommission Dual-Write**: Once the new system is stable and handling all traffic, the dual-write mechanism in the Erlang system can be disabled.
5.  **Final Cutover**: The old Erlang system can now be decommissioned.

## 4. Rollback Plan

If a critical issue is discovered during or after the cutover, the following rollback plan will be executed:

1.  **Immediate Action**: Immediately shift all traffic back to the old Erlang-based system. The new Go system will be taken offline.
2.  **Disable Dual-Write**: If the dual-write mechanism is still active, it should be disabled to prevent further writes to the new database.
3.  **Data Reconciliation (If Necessary)**: If the new system was live for a period, there may be data in the PostgreSQL database that is not in the old Mnesia database. A reverse migration script may be needed to export this data and import it back into the Erlang system. This is the most complex part of a rollback and should be planned for carefully.
4.  **Root Cause Analysis**: Perform a thorough analysis to understand why the migration failed.
5.  **Reschedule Migration**: Do not attempt the migration again until the root cause has been identified and fixed.