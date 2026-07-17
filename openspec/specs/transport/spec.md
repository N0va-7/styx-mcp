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
- **WHEN** another process holds the configured listen address and
  `Controller.Start()` is called in passive mode
- **THEN** `Start()` returns a readable bind/port-in-use error (including a
  STYX_LISTEN-oriented hint when applicable) and does not report success

#### Scenario: Free port binds
- **WHEN** the listen address is free and `Controller.Start()` is called in
  passive mode
- **THEN** `Start()` returns nil so agents can connect on that address

### Requirement: Raw transport is the supported path
The system SHALL implement the `raw` upstream/downstream transport for
controller↔agent links. Operators using raw SHALL be able to complete join and
exchange control messages.

#### Scenario: Agent joins over raw
- **WHEN** controller and agent share the same secret and raw transport settings
  from a compatible build
- **THEN** the agent appears in topology after handshake

### Requirement: WebSocket transport is rejected explicitly
WebSocket (`ws`) transport MUST NOT pretend to work. Controller and agent CLIs
and protocol paths SHALL reject `ws` with an error stating it is not implemented
and that raw should be used.

#### Scenario: Agent rejects -up/-down ws
- **WHEN** an agent is started with websocket upstream or downstream mode
- **THEN** startup fails with a clear "not implemented; use raw" style error

#### Scenario: Controller rejects -down ws
- **WHEN** a controller is started with websocket downstream mode
- **THEN** startup fails with a clear "not implemented; use raw" style error

### Requirement: Wire identity is styx-owned
Handshake and framing identity strings SHALL use this project's styx identity
(not third-party product names) while remaining consistent between controller
and agent of the same build.

#### Scenario: Mismatched identity fails join
- **WHEN** a peer speaks a different wire identity than the controller expects
- **THEN** the join does not produce a trusted online topology entry

### Requirement: Same-commit after protocol changes
After changes to handshake, encryption, framing, **or reconnect/reonline
handshake behavior**, controller and agent MUST be built from the same revision
before interoperability is claimed.

#### Scenario: Protocol change deploy
- **WHEN** wire protocol or join/reonline code changes
- **THEN** operators rebuild both binaries from the same commit before e2e use

#### Scenario: Reconnect path deploy
- **WHEN** agent reconnect or controller reonline handling changes
- **THEN** operators rebuild both binaries from the same commit before relying
  on stable `node_id` after reconnect

### Requirement: Active agent reconnects after unexpected upstream loss
An active-mode agent (`-c`) SHALL treat loss of the upstream session without a
prior intentional shutdown as a recoverable fault. When reconnect is enabled, it
SHALL re-establish dial, preauth, and handshake using its existing node UUID and
`IsReconnect` semantics, up to a configured maximum number of attempts, with
delay that includes jitter (not a fixed zero-jitter period only).

#### Scenario: Default limited reconnect after drop
- **WHEN** an active agent is online with default reconnect settings and the
  upstream TCP session ends without `SHUTDOWN`
- **THEN** the agent attempts to reconnect at most three times with jittered
  delays before giving up and exiting

#### Scenario: Reconnect disabled
- **WHEN** reconnect is configured to off (base interval `0`)
- **THEN** after unexpected upstream loss the agent does not re-dial and exits
  (or otherwise ends the process) without a reconnect loop

#### Scenario: Successful online resets attempt budget
- **WHEN** an agent exhausts some reconnect attempts, then successfully comes
  online again, then later drops unexpectedly once more
- **THEN** it again has a full default attempt budget for the new drop (counter
  was reset after success)

### Requirement: Intentional shutdown disables reconnect
When the controller intentionally stops a node via the control-plane shutdown
path, the agent MUST NOT auto-reconnect for the remainder of that process
lifetime.

#### Scenario: shutdown_node then no rejoin
- **WHEN** the operator invokes `shutdown_node` (or equivalent) for an online
  agent and the agent receives `SHUTDOWN` before the link is closed
- **THEN** the agent closes and does not perform reconnect attempts even if
  reconnect is enabled

#### Scenario: Unexpected drop still reconnects
- **WHEN** reconnect is enabled and the upstream fails with read/write error
  without a prior `SHUTDOWN`
- **THEN** the agent enters the reconnect attempt loop subject to max attempts

### Requirement: Controller active dial respects retry max
When the controller is configured to actively dial a peer, failed dials or
dropped active sessions SHALL retry according to a configurable maximum attempt
count (default aligned with the agent default of three). Passive listen mode does
not dial agents and is unaffected.

#### Scenario: Controller active dial gives up after max
- **WHEN** controller active connect is enabled with max retries three and the
  peer is unreachable
- **THEN** the controller stops dialing after three failed attempts (or the
  configured max) rather than retrying forever

