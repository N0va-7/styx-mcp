## Context

Lab operators saw four friction points:

1. `list_nodes` only showed controller-observed peer IPs (often NAT), not agent LAN IPs.
2. SOCKS HTTP clients that half-close after the request sometimes got empty replies.
3. File upload/download used a single FileData payload — large binaries hit limits or stalled.
4. Async MCP tasks stayed `running` with no intermediate phase.

Wire format for MyInfo and FileData is extended; controller and agent must be built from the **same commit**.

## Goals / Non-Goals

**Goals**

- Report non-loopback IPv4 addresses on join / MYINFO; expose `peer_ip` + `local_addrs`.
- SOCKS: local EOF must not kill remote→local drain before agent FIN.
- Chunked FileData (SliceIndex / SliceTotal); reassemble in order.
- Task `phase` via SetPhase for start_*/upload/pull/cmd.

**Non-goals**

- Secret argv hygiene (deferred).
- Agent auto-reconnect defaults.
- Changing SOCKS flow-control window protocol.

## Decisions

### Local addresses

- Collect with `utils.LocalIPv4Addrs()` (up, non-loopback, non-link-local IPv4).
- Wire as comma-separated string on MyInfo (`LocalAddrsLen` + `LocalAddrs`).
- Topology stores `[]string`; MCP keeps legacy `ip` and adds `peer_ip` / `local_addrs`.

### SOCKS half-close

- Controller: on local Read EOF, do **not** send FIN immediately; wait for agent FIN (or timeout).
- `handleFin` closes the inbox so drain finishes buffered chunks; **must not** `CloseWrite` before drain (that raced and truncated HTTP bodies).
- `removeConn` still sends FIN + closes local conn after drain completes.

### File chunks

- Chunk size 512 KiB both upload (MCP) and download (agent).
- FileData: `SliceIndex` (0-based), `SliceTotal` (>=1); `SliceTotal==0` treated as 1 for legacy single-slice.
- Receivers reject out-of-order slices and clear pending state.

### Task phase

- `Task.Phase` string; `SetPhase`; `ToMap` always includes `phase`.
- Phases are short labels (`wait-ack`, `sending`, `receiving`, `done`, `…-error`).

## Risks / Migration

- **Breaking wire**: old agent + new controller (or reverse) fails MYINFO / FILEDATA parse → rebuild both.
- SOCKS long wait (5m) if agent never FINs after client half-close; timeout then teardown.
- Upload still fire-and-forget after slices (no per-slice agent ACK); failures surface as agent-side logs unless download path.

## Testing

- Unit: LocalIPv4Addrs/Join/Split, topology LocalAddrs, task phase, multi-slice reassembly, out-of-order reject.
- E2E: `TestE2ESOCKSLocalAgentHTTP` (requires `make build` agent binary matching tree).
