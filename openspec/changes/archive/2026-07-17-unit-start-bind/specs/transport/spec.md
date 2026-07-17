## MODIFIED Requirements

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
