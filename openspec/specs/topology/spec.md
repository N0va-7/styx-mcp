# Topology Specification

## Purpose

Track online agents as a multi-hop tree, expose stable numeric node IDs to MCP
tools, and keep concurrent topology reads/writes consistent via a single task
queue (`Topology.Do`). Offline nodes MUST leave sparse IDs without shifting
other nodes' identities.
## Requirements
### Requirement: Online nodes are listable with numeric IDs
The system SHALL expose every currently online agent with a numeric `node_id`
usable by other MCP tools. When no agents are online, list APIs SHALL succeed
with an empty node set (not an error).

#### Scenario: Fresh agent appears in list
- **WHEN** an agent completes join handshake with the controller
- **THEN** `list_nodes` includes that agent with a numeric `node_id` and its UUID

#### Scenario: Empty topology when no agents
- **WHEN** the controller is running and no agents are connected
- **THEN** `list_nodes` returns success with an empty `nodes` collection and
  without treating emptiness as failure

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

### Requirement: Nodes report local addresses
Each online agent SHALL report its non-loopback IPv4 addresses to the controller.
MCP node listing and detail SHALL expose both the TCP peer address seen by the
controller and the agent-reported local addresses.

#### Scenario: Dual-homed agent lists LAN IPs
- **WHEN** an agent with addresses 172.16.23.20 and 10.10.5.20 joins
- **THEN** list_nodes/detail for that node include those addresses under
  `local_addrs` and still include the controller-observed `peer_ip` (or `ip`)

#### Scenario: Empty local list is non-fatal
- **WHEN** an agent cannot enumerate interfaces
- **THEN** it still joins; `local_addrs` is empty or omitted without failing handshake

### Requirement: Reonline preserves numeric node_id
When an agent that previously held UUID `U` and numeric `node_id` `N` leaves the
online set and later completes a reconnect/reonline join with the same UUID `U`,
the system MUST expose it again as `node_id` `N` (via topology history), not as a
newly allocated id that would renumber or consume the next free id unnecessarily.

#### Scenario: Same node_id after reconnect
- **WHEN** an agent is listed as `node_id` 0, then goes offline unexpectedly, then
  successfully reconnects with the same UUID
- **THEN** `list_nodes` again includes that agent as `node_id` 0 (not a new id
  such as 1 while 0 remains unused solely due to reconnect)

#### Scenario: True new agent still gets a new id
- **WHEN** a distinct agent process joins for the first time with a new UUID
- **THEN** it receives a new numeric `node_id` per normal allocation rules

### Requirement: Offline still removes from online set
Unexpected disconnect MUST remove the node from the online set promptly so
`list_nodes` does not show a dead peer as online, while retaining enough history
to satisfy reonline id stability.

#### Scenario: Drop then empty then reappear
- **WHEN** the sole online agent disconnects unexpectedly
- **THEN** `list_nodes` becomes empty (or omits that node) until reonline
  succeeds, after which the node reappears with its prior `node_id`

