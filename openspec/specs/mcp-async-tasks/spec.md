# MCP Async Tasks Specification

## Purpose

Long-running control actions (listen, connect child, SOCKS, forward) return a
`task_id` immediately over MCP stdio, then complete asynchronously. Clients MUST
be able to observe success (`ready: true`) or failure without false "healthy"
signals when the agent never acknowledged.
## Requirements
### Requirement: start tools return task_id promptly
`start_listener`, `connect_node`, `start_socks`, and `start_forward` SHALL return
success with a `task_id` without blocking the MCP request on agent completion.

#### Scenario: Immediate ack with task id
- **WHEN** a client calls one of the start_* tools with valid arguments
- **THEN** the tool response includes `success: true` and a non-empty `task_id`
  before the agent finishes the work

### Requirement: ready only after agent or local success
For agent-side start actions (`start_listener`, `connect_node`, `start_forward`),
the task result SHALL include `ready: true` only after a positive ACK from the
agent (listen/connect/forward ready). For `start_socks`, `ready: true` SHALL
mean the controller has successfully bound the SOCKS listener.

#### Scenario: Listener ready after ACK
- **WHEN** `start_listener` is sent and the agent binds successfully
- **THEN** task status becomes completed with `ready: true` and the listen address

#### Scenario: Agent rejects listen
- **WHEN** the agent cannot bind the requested listen address
- **THEN** the task ends in error (rejected), not `ready: true`

#### Scenario: ACK timeout
- **WHEN** no ACK arrives within the controller's wait timeout
- **THEN** the task ends in error and MUST NOT report `ready: true`

### Requirement: Waiter armed before send
The controller SHALL arm the ACK waiter before (or atomically with) sending the
request so a fast agent response cannot be lost to a race.

#### Scenario: Fast agent ACK still completes
- **WHEN** the agent responds with success immediately after receiving the request
- **THEN** the waiter still receives the ACK and the task can complete successfully

### Requirement: Tasks expose phase while running
Async tasks SHALL record a short `phase` string while in progress (and may keep
the last phase on completion). `get_task_status` SHALL include `phase` so clients
can distinguish bind / wait-ack / transfer / exec stages.

#### Scenario: Upload reports transfer phase
- **WHEN** an upload_file task is sending file slices
- **THEN** get_task_status shows status running (or done) and a phase such as
  `sending` or `stat` rather than an empty phase for the whole lifetime

### Requirement: Scan tasks use progressive phases
Async tasks of type start_scan (or equivalent) SHALL record phase strings while
running so clients can distinguish port scanning from fingerprinting.

#### Scenario: Scan phase visible
- **WHEN** a start_scan task is in the port-connect stage
- **THEN** get_task_status includes phase `scanning` (or a documented synonym)

#### Scenario: Fingerprint phase visible
- **WHEN** fingerprinting is enabled and the job has moved to probing open ports
- **THEN** get_task_status includes phase `fingerprinting` (or a documented synonym)

### Requirement: Scan discover phase
Async tasks of type start_scan SHALL may report phase `discovering` while host
alive probes run, then `scanning` for the mode port set.

#### Scenario: Discover then scan phases
- **WHEN** start_scan runs with discover enabled
- **THEN** get_task_status may show phase `discovering` before `scanning`

