# SOCKS flow control Implementation Plan

> **For agentic workers:** Implement task-by-task. Check boxes as you go.

**Goal:** Per-stream byte-window flow control for SOCKS so payloads are never silently dropped and mux is not HOL-blocked.

**Architecture:** Shared `socksflow.Window`; new `SOCKSTCPACK` wire message; controller and agent wait on credit before send and ACK after downstream write.

**Tech Stack:** Go, existing styx-mcp protocol/raw codec

---

### Task 1: `pkg/socksflow` + tests

- [x] Add `Window` with `InitialWindow`, `Acquire`, `Release`, `Close`
- [x] Unit tests: block until credit, release unblocks, close fails acquire

### Task 2: Protocol

- [x] Append `SOCKSTCPACK` after `HEARTBEAT`
- [x] Add `SocksTCPAck` struct
- [x] Register in `raw.go` message factory

### Task 3: Controller SOCKS

- [x] Per-seq window; `Acquire` before `sendSocksData`
- [x] On inbound DATA write to client then send ACK
- [x] Handle inbound ACK → `Release`
- [x] Wire handlers in `controller.go`

### Task 4: Agent SOCKS

- [x] Remove drop `default` branch
- [x] Per-seq window; ACK after write to target
- [x] Handle inbound ACK
- [x] Bounded ingest / FIN on violation if needed

### Task 5: Verify

- [x] `go test ./...`
- [ ] Commit
