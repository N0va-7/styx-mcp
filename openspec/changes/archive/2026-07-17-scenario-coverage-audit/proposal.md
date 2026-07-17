## Why

OpenSpec scenarios are the contract, but coverage vs unit/MCP/lab tests was never
mapped. Without an audit, “tests map to Scenarios” stays aspirational.

## What Changes

- Produce a full Scenario → test/type matrix (gaps explicit).
- Close a few **cheap unit** gaps for transport WS reject + topology memo.
- Clarify transport WS rejection applies to **controller and agent** CLIs.
- Fix `dev-workflow` Purpose (was TBD after archive).
- Record current lab entry `10.7.11.116` for the next e2e change (not automated here).

## Capabilities

### New Capabilities

(none)

### Modified Capabilities

- `transport`: WebSocket rejection wording covers controller `-down ws` and agent
  `-up`/`-down ws`.
- `dev-workflow`: Purpose text only (clarity).

## Non-goals

- Full MCP/lab e2e harness in this change
- Implementing reconnect / real WebSocket
- Covering every orphan test (crypto, socksflow, upload) as new capabilities yet

## Impact

- Contributors know which scenarios are unit-backed vs lab-only
- Follow-up changes prioritized by gap severity
- Lab IP for next session: `10.7.11.116` (HTTP/EyouCMS, PHP 5.5, ports 22/80)
