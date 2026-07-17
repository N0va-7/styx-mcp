## Scenario coverage matrix

Legend: **unit** = `go test`; **MCP** = stdio tools + real agent; **lab** = remote
target path; **docs** = process/docs only; **—** = no automated coverage.

### topology

| Scenario | Coverage | Evidence / gap |
|----------|----------|----------------|
| Fresh agent appears in list | MCP / lab | No unit; needs live join |
| Empty topology when no agents | — | Easy unit via `ListNodes` / empty topo |
| Middle node offline leaves gaps | **unit** | `TestListAllSkipsSparseIDs` |
| Concurrent list and detail | **unit** (partial) | `TestDoCorrelatesConcurrentResults` (GetUUID/GetNode, not MCP list+memo) |
| Set and read memo | **unit** (this change) | `TestUpdateMemo` |

### socks-proxy

| Scenario | Coverage | Evidence / gap |
|----------|----------|----------------|
| Successful SOCKS ready | MCP / lab | No unit (needs bind + optional dial) |
| Unknown node rejected | — | MCP path in `handleStartSocks`; unit mockable |
| Agent-side forward ready | MCP / lab | Depends on AfterSendWait + agent ACK |
| Not a local SOCKS substitute | **docs** | README / AGENTS; no test needed |
| Local SOCKS port busy | — | Should unit-test `StartSocks` bind failure |

### mcp-async-tasks

| Scenario | Coverage | Evidence / gap |
|----------|----------|----------------|
| Immediate ack with task id | — | `pkg/mcp` has no tests |
| Listener ready after ACK | **unit** (partial) | `TestAfterSendWaitOK` (not full MCP task result) |
| Agent rejects listen | **unit** (partial) | `TestAfterSendWaitReject` |
| ACK timeout | **unit** (partial) | `TestAfterSendWaitTimeout` |
| Fast agent ACK still completes | **unit** (partial) | AfterSendWait arms then send |

### transport

| Scenario | Coverage | Evidence / gap |
|----------|----------|----------------|
| Port already in use | **unit** (partial) | `listen_error_test` messages; not full `Start()` |
| Free port binds | — | Integration-style unit with `:0` easy |
| Agent joins over raw | **unit** (partial) + MCP | `preauth` mutual auth; full join is MCP |
| Agent rejects -up/-down ws | **unit** (this change) | `validateTransport` tests |
| Controller rejects -down ws | **unit** (this change) | same |
| Mismatched identity fails join | — | Needs crafted peer / protocol fixture |
| Protocol change deploy | **docs** | AGENTS / process |

### dev-workflow

| Scenario | Coverage | Evidence / gap |
|----------|----------|----------------|
| All process scenarios | **docs** | Enforced by convention + this audit change |

## Orphan tests (behavior without capability specs)

| Area | Tests | Recommendation |
|------|-------|----------------|
| crypto / HKDF / TLS cert | `pkg/crypto/*_test` | Future capability `wire-crypto` if we document guarantees |
| raw frame size limits | `pkg/protocol/raw_test` | Fold into `transport` or `wire-framing` later |
| upload path sanitize | `pkg/node/file_test` | Future `file-transfer` |
| socksflow window/queue | `pkg/socksflow/*_test` | Implementation detail of socks; optional sub-req under socks-proxy |

## Follow-up change order (recommended)

1. **`unit-gap-topology-socks`** — empty list_nodes, SOCKS port busy, unknown node_id
2. **`unit-start-bind`** — controller `Start()` free port + EADDRINUSE end-to-end
3. **`lab-e2e-10-7-11-116`** — local agent or foothold → SOCKS → lab HTTP (see below)
4. **`spec-file-transfer`** — only if upload/download becomes a product focus

## Current lab (2026-07-17)

| Field | Value |
|-------|--------|
| Entry IP | `10.7.11.116` |
| Reachable | yes (ICMP + TCP) |
| Open ports (light scan) | **22**, **80** |
| HTTP | Apache/2.4.7 (Ubuntu), **PHP/5.5.9**, session cookie |
| App | **EyouCMS** (meta generator), site title 闻道基金 |
| Note | Previous lab `10.7.8.202` shut down; do not assume WebLogic path |

E2E script (next change, not this one):

1. Controller listen + agent join (same secret / commit)
2. `list_nodes` → node_id
3. `start_socks` → curl via SOCKS to `http://10.7.11.116/`
4. Optional: foothold agent on lab host if RCE path is used later
