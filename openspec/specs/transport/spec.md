# Transport and Controller Startup Specification

## Purpose

Define how controller and agent establish the control plane: secret-based
preauth, styx wire identity, raw transport framing, and honest capability
boundaries (no silent WebSocket). Controller listen bind failures MUST fail
startup so MCP clients never see a healthy session with no inbound path for
agents.

## Requirements

### Requirement: Passive listen hard-fails on bind error
In passive mode the controller MUST bind its agent listen address during
`Start()` and MUST return an error if the bind fails (including address already
in use). The process MUST NOT proceed to serve MCP as if networking were up.

#### Scenario: Port already in use
- **WHEN** another process holds `STYX_LISTEN` / configured listen address
- **THEN** controller start fails with a readable bind/port-in-use error and does
  not open a successful MCP session on that broken config

#### Scenario: Free port binds
- **WHEN** the listen address is free
- **THEN** `Start()` returns nil and agents can connect on that address

### Requirement: Raw transport is the supported path
The system SHALL implement the `raw` upstream/downstream transport for
controller↔agent links. Operators using raw SHALL be able to complete join and
exchange control messages.

#### Scenario: Agent joins over raw
- **WHEN** controller and agent share the same secret and raw transport settings
  from a compatible build
- **THEN** the agent appears in topology after handshake

### Requirement: WebSocket transport is rejected explicitly
WebSocket (`ws`) transport MUST NOT pretend to work. CLI and protocol paths
SHALL reject `ws` with an error stating it is not implemented and that raw
should be used.

#### Scenario: Agent rejects -up/-down ws
- **WHEN** an agent is started with websocket upstream or downstream mode
- **THEN** startup fails with a clear "not implemented; use raw" style error

### Requirement: Wire identity is styx-owned
Handshake and framing identity strings SHALL use this project's styx identity
(not third-party product names) while remaining consistent between controller
and agent of the same build.

#### Scenario: Mismatched identity fails join
- **WHEN** a peer speaks a different wire identity than the controller expects
- **THEN** the join does not produce a trusted online topology entry

### Requirement: Same-commit after protocol changes
After changes to handshake, encryption, or framing, controller and agent MUST be
built from the same revision before interoperability is claimed.

#### Scenario: Protocol change deploy
- **WHEN** wire protocol code changes
- **THEN** operators rebuild both binaries from the same commit before e2e use
