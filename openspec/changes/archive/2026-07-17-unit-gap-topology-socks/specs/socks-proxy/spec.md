## MODIFIED Requirements

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

### Requirement: Bind and dial failures surface as task errors
If the controller cannot bind SOCKS, or the agent rejects listen/forward, the
system SHALL mark the async task failed with a readable error instead of leaving
it stuck in running forever. A failed local bind MUST NOT leave a SOCKS service
registered for that node.

#### Scenario: Local SOCKS port busy
- **WHEN** `start_socks` targets an address already in use on the controller
- **THEN** the operation fails with a bind error and no SOCKS service remains
  registered for the requested node
