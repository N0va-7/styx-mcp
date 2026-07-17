## ADDED Requirements

### Requirement: Tasks expose phase while running
Async tasks SHALL record a short `phase` string while in progress (and may keep
the last phase on completion). `get_task_status` SHALL include `phase` so clients
can distinguish bind / wait-ack / transfer / exec stages.

#### Scenario: Upload reports transfer phase
- **WHEN** an upload_file task is sending file slices
- **THEN** get_task_status shows status running (or done) and a phase such as
  `sending` or `stat` rather than an empty phase for the whole lifetime
