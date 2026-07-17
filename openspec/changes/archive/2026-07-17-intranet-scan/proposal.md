## Why

After foothold, operators need a **fast, structured** view of the LAN from a
chosen agent: open ports, light fingerprints, and **vulnerability reference
links** so the MCP-driving model can decide next steps. Today that means
uploading fscan or improvising with `start_cmd`—noisy, unstructured, and
poorly tied to topology/`node_id`.

styx should ship a **lightweight agent-side probe**, not a second fscan
(no brute force, no exploit, no full POC engine).

## What Changes

- MCP `start_scan` (async task): targets + mode (`fast`|`normal`|`full`|`custom`)
  exit via `node_id`.
- Agent: TCP **connect** port scan (no SYN, no admin), **no ICMP/ping hard
  dependency**; then fingerprint **only open ports**; attach `refs[]`.
- Wire messages for scan request/progress/result (same-commit controller/agent).
- Structured task result for the model: `open[]`, `summary.interesting[]`, refs.

## Capabilities

### New Capabilities

- `intranet-scan`: port modes, two-phase probe, fingerprint + vuln refs, MCP API

### Modified Capabilities

- `mcp-async-tasks`: scan tasks use `phase` (`scanning` / `fingerprinting` / …)

## Non-goals

- SYN / half-open scan, raw sockets, admin-only paths
- Weak-password spraying, exploit/POC execution, Redis/MS17 weaponization
- Embedding full fscan or FingerprintHub wholesale
- UDP sweep (optional later)
- Agent auto-reconnect / secret argv (separate changes)

## Impact

- New protocol message types; rebuild controller **and** agent together
- New MCP tool(s); additive JSON only
- Agent binary grows modestly (connect scan + small rule tables)
