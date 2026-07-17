## Context

styx-mcp is a multi-hop jump host: controller (stdio MCP) + agents that exit
traffic. Lab use already has dual agents with `local_addrs`; operators next
want **intranet port scan + light fingerprint + CVE/advisory links** from a
selected `node_id`.

Inspiration (pipeline only): [fscan](https://github.com/shadow1ng/fscan)
discover → port → fingerprint. **Not** fscan’s brute/exploit/POC surface.

## Goals / Non-Goals

**Goals**

- Fast structured recon from agent network stack (no admin privileges).
- Modes: `fast` | `normal` | `full` | `custom`.
- Phase 1: TCP connect scan → open set (ICMP/ping not required).
- Phase 2: fingerprint open ports only → `product` / optional `version`.
- Attach **reference links** (`refs[]`), not exploit payloads.
- Async MCP task + `phase` for model observability.
- Hard limits so a jump box is not turned into a DDoS source.

**Non-goals**

- Second fscan; full-port-as-default; SYN scan; mandatory host ping.
- Credential attacks; automated exploitation.

## Pipeline

```text
MCP start_scan(node_id, targets, mode, ports?, fingerprint?=true, …)
  → Controller validates limits, creates task, sends SCANREQ to agent
  → Agent phase scanning: TCP DialTimeout only
  → Agent phase fingerprinting: probes on open ports only
  → Controller (or shared lib) maps product → refs[]
  → SCANPROG (optional) + SCANRES → task.result
```

**Alive host without ping:** a host is “up” if **any** scanned port is `open`.
Hosts with all timeouts/refused stay out of fingerprint phase; stats may count
them as dark. ICMP is never required; if later added, it must be optional and
failure-ignored.

## Scan modes

| Mode | Ports | Notes |
|------|-------|--------|
| `fast` | Built-in high-value set (~20–40): e.g. 22,80,443,445,3389,6379,3306,5432,27017,7001,8080,8443, … | Default profile |
| `normal` | Larger common intranet set (~80–130); may be **trimmed** from fscan-like common lists | Default for thorough-but-bounded |
| `full` | 1–65535 | Strict caps: max hosts, concurrency, wall time; tool description MUST warn cost |
| `custom` | Caller `ports` string (`22,80,8000-8100`) | Invalid/empty ports → fail task |

Shared knobs (defaults + hard ceilings in implementation):

- `concurrency` (workers), `timeout_ms` per dial
- Max hosts per job, max ports per host (full excepted but wall-clock capped)
- Max total dials / max duration

`fingerprint=false` → ports-only (phase ends after scanning).

## Privilege model

- **Only TCP connect** (`net.Dial` / equivalent). No raw sockets, no SYN.
- Works as unprivileged user (www-data, oracle, etc.).
- No dependency on CAP_NET_RAW or admin.

## Fingerprint (light)

On each open port (budgeted concurrency):

1. Optional short banner read.
2. Port/protocol heuristics (e.g. 22 → SSH line; 80/443/8080/7001 → HTTP(S)).
3. HTTP(S): status, `Server`, `Title`, limited body bytes; optional path hints later.
4. Normalize to `service` + `product` (+ `version` when cheap).
5. `evidence` truncated string; `confidence` high|medium|low.

Do **not** ship a multi-thousand-rule CMS DB in v1. Seed high-value products
(e.g. generic http/ssh/redis/mysql, weblogic, thinkphp-ish title clues) and grow
tables deliberately.

## Vulnerability references

After fingerprint:

```text
match(product, version?, port?, evidence?) → refs[]
```

Each ref:

- `type`: `cve` | `advisory` | `doc`
- `id`, `url` (prefer NVD / vendor stable URLs)
- `condition`: human/model note (“verify console path”, “version range uncertain”)

**No** exploit code, no “confirmed vulnerable” claim. Wording is advisory.
Rule tables live in shared Go package (prefer evaluate on controller so agent
stays thinner; same embed file OK if dual-linked).

## Result schema (task.result)

```json
{
  "via_node_id": 0,
  "mode": "fast",
  "stats": {
    "hosts_total": 254,
    "hosts_with_open": 3,
    "ports_tried": 5000,
    "open": 12,
    "duration_ms": 42000
  },
  "open": [
    {
      "ip": "172.16.23.20",
      "port": 7001,
      "proto": "tcp",
      "state": "open",
      "service": "http",
      "product": "weblogic",
      "version": "",
      "title": "",
      "evidence": "",
      "confidence": "medium",
      "refs": [
        {
          "type": "cve",
          "id": "CVE-2020-14882",
          "url": "https://nvd.nist.gov/vuln/detail/CVE-2020-14882",
          "condition": "Oracle WebLogic console; verify exposure/version"
        }
      ]
    }
  ],
  "summary": {
    "interesting": [
      {
        "ip": "172.16.23.20",
        "port": 7001,
        "why": "weblogic + refs",
        "refs_n": 2
      }
    ]
  }
}
```

`interesting`: open entries with non-empty `refs`, or high-value ports/products.

## Task phases

| phase | meaning |
|-------|---------|
| `scanning` | Connect scan in progress |
| `fingerprinting` | Probing open ports |
| `done` | Finished OK |
| `failed` / `*-error` | Limits, node offline, protocol error |

Optional progressive `result` updates via SCANPROG (open count / partial open list).

## Wire protocol (sketch)

New message types (names flexible in impl, must be versioned with same-commit builds):

- `SCANREQ`: task_id, targets blob, mode, ports, flags (fingerprint), concurrency, timeout_ms, limits
- `SCANPROG`: task_id, phase, stats, optional partial hits (chunked if large)
- `SCANRES`: task_id, ok, final payload or error string (chunked if needed)

Reuse existing framing/crypto. Large results: multi-frame or reuse file-chunk
patterns; do not single-frame multi-megabyte JSON.

## MCP tools

- **`start_scan`** (required): `node_id`, `targets` (string or list), `mode`, optional `ports`, `fingerprint`, concurrency/timeout if exposed.
- Reuse **`get_task_status`** for phase + result.
- Optional later: `stop_scan(task_id)`.

Tool description MUST state: authorized environments only; `full` is expensive;
results are hints not confirmed vulns.

## Package layout (implementation guide)

| Package | Role |
|---------|------|
| `pkg/scan` | Expand CIDR/ports, scheduler, dial abstraction (unit-testable) |
| `pkg/fingerprint` | Probes + product normalize + refs table |
| `pkg/protocol` | SCAN* structs |
| `pkg/node` | Run scan job on agent |
| `pkg/controller` | Route SCAN*, fill task |
| `pkg/mcp` | `start_scan` |

## Risks / trade-offs

| Risk | Mitigation |
|------|------------|
| full mode burns time/CPU on agent | Caps + default fast + docs |
| False product → wrong refs | low confidence; condition text; no auto-exploit |
| Concurrent SOCKS + scan | Bound scan workers |
| Wire break | same-commit rebuild note in AGENTS/PR |
| Agent size | Small seed rules; no full FingerprintHub |

## Testing strategy

- Unit: port parse, CIDR expand, mode port sets, scheduler with fake dialer, refs match.
- Unit: fingerprint classifiers on canned banners/HTTP fixtures.
- Integration: local agent + controller, scan 127.0.0.1 or lab via real node.
- Lab (optional): node on 172.16.23.10 scan `172.16.23.0/24` fast → expect :80 / :7001-ish + refs seeds.

## Open questions (resolve in impl if needed)

1. Exact `fast`/`normal` port literals (freeze in `pkg/scan/ports.go` + tests).
2. Whether refs matching runs only on controller (preferred) or both.
3. `stop_scan` in v1 or v1.1.
4. IPv6: defer unless trivial.
