# SOCKS and Port Forward Specification

## Purpose

Provide two complementary ways to push traffic through an agent: controller-side
SOCKS5 (`start_socks`) for tools that run on the controller host, and agent-side
listen+forward (`start_forward`) for a foothold port. These MUST NOT be confused
with each other.
## Requirements
### Requirement: SOCKS listens on the controller
`start_socks` SHALL bind a SOCKS5 listener on the controller host at the given
address and SHALL exit proxied connections via the selected online node.

#### Scenario: Successful SOCKS ready
- **WHEN** `start_socks` is called with a valid `node_id` and free local address
- **THEN** the async task completes with `ready: true` and clients on the
  controller can open SOCKS sessions that reach destinations via that node

#### Scenario: Unknown node rejected
- **WHEN** `start_socks` is called with a `node_id` not in the online topology
- **THEN** the tool fails immediately with a clear "node not found" style error
  and does not bind a local listener for that request

#### Scenario: Lab or loopback HTTP via SOCKS
- **WHEN** a local agent is online and SOCKS is ready on the controller
- **THEN** an HTTP client using that SOCKS endpoint can retrieve content from a
  reachable target (loopback fixture in automated tests, or a configured lab URL
  such as `http://10.7.11.116/`)

### Requirement: Forward listens on the agent
`start_forward` SHALL instruct the selected agent to listen on `listen_address`
and forward accepted connections to `target_address`. The controller host MUST
NOT be required to bind `listen_address`.

#### Scenario: Agent-side forward ready
- **WHEN** `start_forward` is called with a valid online node and addresses the
  agent can bind and dial
- **THEN** the async task completes with `ready: true` only after the agent
  acknowledges the forward is up

#### Scenario: Not a local SOCKS substitute
- **WHEN** an operator needs a SOCKS endpoint on the controller for local tools
- **THEN** they MUST use `start_socks`, not `start_forward`

### Requirement: Bind and dial failures surface as task errors
If the controller cannot bind SOCKS, or the agent rejects listen/forward, the
system SHALL mark the async task failed with a readable error instead of leaving
it stuck in running forever. A failed local bind MUST NOT leave a SOCKS service
registered for that node.

#### Scenario: Local SOCKS port busy
- **WHEN** `start_socks` targets an address already in use on the controller
- **THEN** the operation fails with a bind error and no SOCKS service remains
  registered for the requested node

