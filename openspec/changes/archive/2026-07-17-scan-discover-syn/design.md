## Pipeline

```text
targets expand
  → method = auto|syn|connect  (auto: SYN if CanRawTCP else connect)
  → [default] discover: probe ports on all hosts (short timeout, high concurrency)
       host alive iff any probe port open (TCP connect or SYN-ACK)
  → port scan: only alive hosts × mode ports
  → fingerprint open only
```

If discover disabled (`discover=false`): port-scan all hosts (legacy behavior).

If discover finds zero alive: result open=[] with stats; not a hard failure.

## Method selection

| Method | Behavior |
|--------|----------|
| `auto` | `CanRawTCP()` → SYN engine; else connect |
| `syn` | Require raw TCP; fail task if unavailable |
| `connect` | Always TCP connect |

`CanRawTCP`: Linux + (euid 0 or successful `ListenPacket("ip4:tcp")`).

Non-Linux: always connect (SYN stub).

## Discover probe ports

Default set (~10): 22, 80, 443, 445, 3389, 3306, 6379, 7001, 8080, 8443.
Optional override later via MCP; v1 freeze constants + tests.

Discover timeout: min(caller timeout, 400ms) by default so dark hosts die fast.

## SYN engine (Linux)

- `net.ListenPacket("ip4:tcp", "0.0.0.0")` (needs CAP_NET_RAW/root).
- Craft 20-byte TCP SYN; kernel supplies IPv4 header.
- Shared read loop matches SYN-ACK by dst/src port + remote IP.
- Concurrent probes via waiter map; per-probe deadline.
- Fallback: if engine setup fails mid-job, switch remaining work to connect and record method in stats.

## Wire

Append to `ScanReq` (same-commit rebuild):

- `Discover` uint16: 0=default on, 1=on, 2=off
- `MethodLen` + `Method` string: empty/auto/syn/connect

## Stats / result additions

```json
"stats": {
  "hosts_total": 254,
  "hosts_alive": 3,
  "hosts_with_open": 3,
  "method": "connect",
  "discover_ms": 1200,
  ...
}
```

## Risks

| Risk | Mitigation |
|------|------------|
| False negative on discover (host up but silent on probe ports) | Document; `discover=false` escape; probe set covers common services |
| SYN blocked by middlebox | auto falls back or operator forces connect |
| Spec said “connect only” | MODIFIED: connect always available; SYN optional |
