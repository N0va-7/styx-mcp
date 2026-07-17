## MODIFIED Requirements

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
