## ADDED Requirements

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
