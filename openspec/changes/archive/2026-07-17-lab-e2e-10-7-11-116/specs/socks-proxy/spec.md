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

#### Scenario: Lab or loopback HTTP via SOCKS
- **WHEN** a local agent is online and SOCKS is ready on the controller
- **THEN** an HTTP client using that SOCKS endpoint can retrieve content from a
  reachable target (loopback fixture in automated tests, or a configured lab URL
  such as `http://10.7.11.116/`)
