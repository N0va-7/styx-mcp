## 1. Spec freeze

- [x] 1.1 Confirm `fast`/`normal` port lists in design → code constants + unit tests
- [x] 1.2 Freeze result JSON field names (`open`, `summary.interesting`, `refs`)

## 2. Core scan engine (`pkg/scan`)

- [x] 2.1 Parse targets (IP, CIDR, list) with host caps — unit tests
- [x] 2.2 Parse ports / modes (`fast`|`normal`|`full`|`custom`) — unit tests
- [x] 2.3 Worker pool TCP connect dialer interface + fake dialer tests
- [x] 2.4 Stats: ports_tried, open count, duration (no ICMP required)

## 3. Fingerprint + refs (`pkg/fingerprint`)

- [x] 3.1 Banner / HTTP title-Server / SSH / few DB heuristics — fixture tests
- [x] 3.2 Product normalize + seed refs table (WebLogic, generic redis/ssh/http, …)
- [x] 3.3 `interesting` summary helper

## 4. Wire + agent/controller

- [x] 4.1 Protocol SCANREQ / SCANPROG / SCANRES (+ chunking if needed)
- [x] 4.2 Agent job runner: scanning then fingerprinting phases
- [x] 4.3 Controller: dispatch by node UUID, SetPhase, SetResult/SetError
- [x] 4.4 Same-commit note; rebuild agent+controller for lab

## 5. MCP

- [x] 5.1 `start_scan` tool + validation (mode, custom ports, limits)
- [x] 5.2 Task type `start_scan`; phases `scanning`|`fingerprinting`|`done`
- [x] 5.3 Tool description: authorized use, full-mode cost, refs are hints

## 6. Validate

- [x] 6.1 `go test` for `pkg/scan`, `pkg/fingerprint`, protocol/node/controller as touched
- [x] 6.2 Optional lab: `start_scan` via node → open ports + at least one ref seed
- [x] 6.3 `openspec validate --all` then archive when implementation complete
