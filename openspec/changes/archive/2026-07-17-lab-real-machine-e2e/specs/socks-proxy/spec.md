## ADDED Requirements

### Requirement: Multiple nodes can host independent SOCKS exits
The system SHALL allow concurrent SOCKS listeners on the controller that exit via
different online nodes (distinct local addresses and node_ids).

#### Scenario: Two real agents two SOCKS ports
- **WHEN** two agents are online and `start_socks` is called once per node with
  distinct local addresses
- **THEN** both tasks complete with `ready: true` and HTTP clients can reach
  destinations via each SOCKS independently
