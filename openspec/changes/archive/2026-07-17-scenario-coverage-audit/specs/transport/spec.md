## MODIFIED Requirements

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
