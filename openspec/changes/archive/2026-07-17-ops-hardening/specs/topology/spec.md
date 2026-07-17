## ADDED Requirements

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
