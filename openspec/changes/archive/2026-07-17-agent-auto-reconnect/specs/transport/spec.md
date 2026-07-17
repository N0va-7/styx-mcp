## ADDED Requirements

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

## MODIFIED Requirements

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
