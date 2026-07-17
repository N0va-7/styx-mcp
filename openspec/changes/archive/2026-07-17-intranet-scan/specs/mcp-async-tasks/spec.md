## ADDED Requirements

### Requirement: Scan tasks use progressive phases
Async tasks of type start_scan (or equivalent) SHALL record phase strings while
running so clients can distinguish port scanning from fingerprinting.

#### Scenario: Scan phase visible
- **WHEN** a start_scan task is in the port-connect stage
- **THEN** get_task_status includes phase `scanning` (or a documented synonym)

#### Scenario: Fingerprint phase visible
- **WHEN** fingerprinting is enabled and the job has moved to probing open ports
- **THEN** get_task_status includes phase `fingerprinting` (or a documented synonym)
