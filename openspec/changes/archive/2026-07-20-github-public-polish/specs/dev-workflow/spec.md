## ADDED Requirements

### Requirement: Public docs name supported MCP hosts and toolchain floor
Public README surfaces SHALL document the MCP host configurations the project
actively supports (at least Cursor and Codex) and SHALL state a Go version floor
consistent with `go.mod` (badge or equivalent).

#### Scenario: Reader finds Codex setup
- **WHEN** a contributor opens the English or Chinese README
- **THEN** they can find a Codex `config.toml` MCP snippet in addition to Cursor

#### Scenario: Badge matches module Go version
- **WHEN** the README Go badge (or version claim) is published
- **THEN** it is not lower than the major.minor floor declared in `go.mod`
