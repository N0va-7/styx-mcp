# Topology Specification

## Purpose

Track online agents as a multi-hop tree, expose stable numeric node IDs to MCP
tools, and keep concurrent topology reads/writes consistent via a single task
queue (`Topology.Do`). Offline nodes MUST leave sparse IDs without shifting
other nodes' identities.

## Requirements

### Requirement: Online nodes are listable with numeric IDs
The system SHALL expose every currently online agent with a numeric `node_id`
usable by other MCP tools.

#### Scenario: Fresh agent appears in list
- **WHEN** an agent completes join handshake with the controller
- **THEN** `list_nodes` includes that agent with a numeric `node_id` and its UUID

#### Scenario: Empty topology when no agents
- **WHEN** the controller is running and no agents are connected
- **THEN** `list_nodes` returns an empty online set without error

### Requirement: Node IDs stay sparse after offline
When a node goes offline, the system MUST remove it from the online set and MUST
NOT renumber remaining nodes so that existing `node_id` values keep referring to
the same agents until those agents themselves leave.

#### Scenario: Middle node offline leaves gaps
- **WHEN** nodes with IDs 0, 1, 2 are online and node 1 goes offline
- **THEN** `list_nodes` no longer includes node 1 and still reports 0 and 2 with
  their original IDs (sparse list, not compacted to 0, 1)

### Requirement: Topology mutations are serialized
All topology state changes and UUID lookups SHALL go through a single serialized
API so concurrent MCP tool calls cannot corrupt the tree or race on replies.

#### Scenario: Concurrent list and detail
- **WHEN** multiple clients call `list_nodes` and node detail/memo updates at once
- **THEN** each call receives a consistent snapshot and no panic or stuck waiter occurs

### Requirement: Memo and detail per node
The system SHALL allow attaching a human-readable memo to a node and retrieving
node detail for operators without requiring a reconnect.

#### Scenario: Set and read memo
- **WHEN** an operator sets a memo on a valid `node_id`
- **THEN** subsequent list/detail for that node includes the memo text
