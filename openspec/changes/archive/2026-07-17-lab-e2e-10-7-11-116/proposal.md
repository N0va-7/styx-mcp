## Why

Scenario coverage audit deferred lab e2e. New entry host `10.7.11.116` is up
(HTTP 200, EyouCMS). Need a repeatable path: controller → agent → SOCKS → lab.

## What Changes

- Run and record MCP lab e2e against `10.7.11.116` (list_nodes, start_socks,
  curl via SOCKS).
- Add a small shell checklist script under `scripts/` for re-runs (local agent).
- Spec delta: socks-proxy successful path notes lab verification method.

## Capabilities

### New Capabilities

(none)

### Modified Capabilities

- `socks-proxy`: Successful SOCKS scenario notes optional lab target verification

## Non-goals

- Exploiting EyouCMS / foothold on the lab host
- Full multi-hop child topology
- CI automation of remote lab (environment-dependent)

## Impact

- `scripts/lab-e2e-socks.sh` (optional helper)
- Archive evidence under change folder
