# styx-mcp — project agent rules

This repo is an **MCP-native multi-hop proxy** (controller + agent). Multi-hop ideas are inspired by [Stowaway](https://github.com/ph4ntonn/Stowaway); wire identity, crypto, and the control plane are **styx-mcp’s own**. Keep attribution links in docs; do not reintroduce foreign handshake strings or dead “compat” stubs without a reason.

**Authorized use only** (labs, CTF/exam ranges, RoE-covered work).

---

## Fix / change workflow (required)

When implementing features or bugfixes, follow this loop. Do not skip steps.

```text
1. Scope     — fix only what was asked; no drive-by refactors
2. Implement — minimal, correct change; fail visibly on hard errors
3. Unit      — go test ./...  (and new tests when logic is testable)
4. Real e2e  — if the change touches proxy/control plane/handshake/listen, run the target lab path below
5. Commit    — only after tests that apply have passed
6. Push      — only if the user explicitly asks
```

### Scope

| Do | Don't |
|----|--------|
| Touch files related to the defect/feature | Unrelated renames, drive-by cleanup |
| One logical theme per commit (or a tight P0 set) | Mix P0 + P2 + docs rewrite in one dump |
| Call out wire/compat breaks in the commit message | Silently break agent↔controller compatibility |

If you discover a larger issue while fixing: record it for the user; **do not expand the PR** unless it blocks verification.

### Code quality bar

- Prefer simple control flow over clever abstraction.
- **Hard failures must be visible**: e.g. controller listen bind failure must not leave MCP “healthy” with an empty topology — exit non-zero or surface a clear error to the client.
- **Never panic on peer/MCP input**: type-assert with comma-ok; log and skip/fail the request.
- Wrap errors with context (`fmt.Errorf("…: %w", err)`).
- Secrets: never commit `.grok/styx.secret`, real `STYX_SECRET`, or lab keys. Prefer env / gitignored local config (see `.grok/config.toml.example`).

### Wire / versioning

- Controller and agent must be built from the **same commit** when handshake, crypto, or framing changes.
- Identity constants live in `pkg/protocol` (`ControllerUUID`, `JoinUUID`, `HelloFromAgent`, `HelloFromController`, …). Do not reintroduce legacy Stowaway greetings/UUIDs.
- Message type numeric IDs are append-only when possible (keep existing wire IDs stable).

---

## Testing

### Always

```bash
go test ./...
go build ./...   # or make build / make build-all when shipping agents
```

### Real-scenario e2e (when required)

Run this path when the change affects **listen, preauth, handshake, routing, SOCKS, MCP tools, or packaging**:

1. Rebuild controller (host OS) + agent (`linux/amd64` for typical lab targets).
2. Ensure **one** controller owns the agent port (default `19137`); free the port if another MCP instance holds it.
3. Foothold on authorized target (e.g. lab entry) → deploy **new** agent → connect with shared secret.
4. MCP: `list_nodes` shows the node.
5. MCP: `start_socks` on controller (`127.0.0.1:<port>`).
6. Prove pivot: **direct** to internal service fails or is unreachable from the attacker host; **via SOCKS** reaches the intended internal target (e.g. WebLogic console).

If e2e fails: fix and re-run; **do not commit**.

Unit-only is enough for pure refactors that cannot affect the data plane (e.g. comment/docs-only), but prefer e2e when unsure.

### Smoke commands (examples)

```bash
# After start_socks(node_id, "127.0.0.1:10801")
curl -sS -m 3 -I http://<internal-host>:<port>/          # often timeout from attacker
curl -sS -m 15 --socks5-hostname 127.0.0.1:10801 -I \
  http://<internal-host>:<port>/console/                 # expect service response
```

---

## Git commits

- Commit only with a clean verification story for the change.
- Message style (match repo history): short subject, imperative; body explains **why** if non-obvious.
  - Examples: `fix: …`, `feat: …`, `refactor: …`
- Do not commit: `release/`, secrets, local `.grok/config.toml` (example file is OK).
- Do not `git push` or amend published history unless the user asks.

---

## Layout (quick)

| Path | Role |
|------|------|
| `cmd/controller` | Controller + MCP stdio entry |
| `cmd/agent` | Agent entry |
| `pkg/mcp` | MCP tool surface |
| `pkg/controller` | Control plane, SOCKS/backward |
| `pkg/node` | Agent handlers |
| `pkg/protocol` | Wire protocol + identity |
| `scripts/styx-mcp-wrapper.sh` | Cursor/Grok MCP launcher |
| `.grok/config.toml.example` | Project MCP template |

---

## Ops notes for agents

- Prefer **styx-mcp MCP tools** for tunnels (`start_socks` / `start_backward` / `start_forward`); do not invent chisel/frp unless asked or MCP is down.
- `start_socks`: SOCKS listens on **controller**; traffic exits via `node_id`.
- `start_forward`: listens on **agent**; not a local SOCKS substitute.
- Multiple MCP clients (Cursor + Grok) must not fight over the same listen port — one controller instance at a time.
