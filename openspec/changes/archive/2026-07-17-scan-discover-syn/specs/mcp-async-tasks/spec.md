## ADDED Requirements

### Requirement: Scan discover phase
Async tasks of type start_scan SHALL may report phase `discovering` while host
alive probes run, then `scanning` for the mode port set.

#### Scenario: Discover then scan phases
- **WHEN** start_scan runs with discover enabled
- **THEN** get_task_status may show phase `discovering` before `scanning`
