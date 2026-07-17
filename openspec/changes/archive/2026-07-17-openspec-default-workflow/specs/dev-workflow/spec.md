## ADDED Requirements

### Requirement: All product work uses an OpenSpec change
Feature work, bug fixes, and test backfill that touch behavior or code under
`cmd/` or `pkg/` SHALL be tracked as an OpenSpec change under
`openspec/changes/<kebab-name>/` with at least `proposal.md` and `tasks.md`.

#### Scenario: Bug fix opens a change
- **WHEN** a contributor fixes a defect in controller, agent, MCP, or protocol code
- **THEN** they create or update a named change with proposal and tasks before
  treating the work as complete

#### Scenario: Slim change is allowed
- **WHEN** the fix only hardens behavior already described by an existing Scenario
- **THEN** the change MAY omit a specs delta and design, but MUST still include
  proposal and tasks that name the scenario being hardened

### Requirement: Specs are the behavioral contract
Normative product behavior SHALL live in `openspec/specs/<capability>/spec.md`.
Code and tests MUST NOT be the only place a required behavior is documented.

#### Scenario: Missing scenario before new test pin
- **WHEN** a new test asserts product behavior not covered by any Scenario
- **THEN** the change ADDs or MODIFIESs the matching requirement so the Scenario
  exists in the change delta (and later in main specs after archive)

### Requirement: Tests map to Scenarios
Automated and manual verification for a change SHALL map to one or more Scenarios
from the relevant capability specs (existing or added in the same change).

#### Scenario: Task lists the scenario and test type
- **WHEN** tasks.md lists an implementation step that needs verification
- **THEN** the task names the Scenario (or requirement) and whether coverage is
  unit, MCP+agent, or full lab path

### Requirement: Validate before commit
Before committing a change, the contributor SHALL run `openspec validate --all`
successfully and run package tests for touched code.

#### Scenario: Validation and tests green
- **WHEN** the change is ready to commit
- **THEN** `openspec validate --all` passes and relevant `go test` packages pass
