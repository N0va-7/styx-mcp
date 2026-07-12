# SOCKS per-stream byte window (flow control)

**Date:** 2026-07-12  
**Status:** Approved  
**Repo:** styx-mcp

## Problem

Agent `handleSocksData` used non-blocking send with `default: drop` when `dataChan` was full. Silent byte loss breaks pentest assumptions (HTTP/RCE/shell/uploads). Blocking the mux read loop instead would cause head-of-line blocking across all streams on one agent link.

## Goal

- Never silently drop SOCKS TCP payload bytes
- Per-stream backpressure without stalling the control-plane mux
- Small, testable protocol extension; controller and agent must be upgraded together

## Design

### Protocol

Append a new message type **at the end of the iota list** (do not insert in the middle — preserves existing wire IDs):

```text
SOCKSTCPACK
SocksTCPAck { Seq uint64; Credit uint64 }
```

`Credit` is the number of bytes the peer may send again on that `Seq` (replenishment).

Existing `SOCKSTCPDATA` / `SOCKSTCPFIN` unchanged.

### Rules

- Each `Seq` has an independent **send credit** in each direction.
- Initial credit: **256 KiB** (`socksflow.InitialWindow`) when the stream is created.
- Sender may emit `SOCKSTCPDATA` only if `credit >= len(Data)`; then `credit -= len(Data)`.
- While credit is insufficient, sender **stops reading** the local/target socket (backpressure). Mux continues.
- Receiver, after successfully writing `n` bytes to the downstream TCP socket (or otherwise fully consuming them), sends `SOCKSTCPACK{Seq, Credit: n}`.
- If a peer sends more than the outstanding window allows (implementation-detected buffer overrun), reset the stream with `SOCKSTCPFIN` — never drop quietly.

### Components

| Piece | Role |
| :--- | :--- |
| `pkg/socksflow` | Shared window primitive: `Acquire(n)`, `Release(n)`, `Close()` |
| `pkg/protocol` | `SOCKSTCPACK` + `SocksTCPAck` + raw codec registration |
| `pkg/controller/socks.go` | Per-seq send window; ACK on local write; wait before send |
| `pkg/node/socks.go` | Symmetric; remove drop path; bounded ingest ≤ window |
| Tests | Credit gating + no-loss under slow consumer |

### Non-goals (this change)

- UDP SOCKS window
- Dynamic window sizing / BBR
- Applying the same scheme to forward/backward (follow-up)

### Compatibility

Breaking for SOCKS between mismatched builds. Document: **use matching controller and agent versions**.

### Success criteria

- `go test ./...` passes including new socksflow / SOCKS tests
- Slow consumer + fast producer: full payload integrity
- No `default: drop` on SOCKS data path
