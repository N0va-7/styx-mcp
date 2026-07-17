## Context

Today:

- `-reconnect N` only retries **initial** active dial/handshake failure.
- After a live session dies, `handleUpstreamOffline` logs and stops (TODO).
- Controller always mints a new UUID on join → new sparse `node_id`.
- `Topology.ReonlineNode` + `history[uuid]→idNum` already support stable IDs.
- `HIMess.IsReconnect`, `SHUTDOWN`, `NODEREONLINE` exist on the wire.

Product choices (session agreement):

1. Default **on**, fault-driven only (not periodic beacon while healthy).
2. **Max 3** attempts with **jitter** (and light backoff).
3. Counter **resets** after successful re-online (so daily blips do not exhaust the budget forever).
4. Controller can configure the same **max** for its own **active** dial retries.
5. Controller **intentional** node kill → agent **must not** reconnect.

## Goals / Non-Goals

**Goals**

- Unexpected disconnect → agent re-dials, re-handshakes with prior UUID, reappears under the **same** `node_id`.
- Intentional `shutdown_node` → process ends, no reconnect loop.
- Defaults good for lab/MCP UX; operators can set `-reconnect 0` to silence.

**Non-goals**

- Resume streams/tasks; children UPSTREAM* fan-out; infinite retry; passive reverse reconnect.

## Decisions

### D1 — Trigger

Reconnect runs only when the upstream session ends **without** a prior
`SHUTDOWN` (or equivalent intentional close flag). Triggers: read EOF/error,
fatal write error. No timer while the connection is healthy.

### D2 — Agent state machine

```text
online → (unexpected drop) → cleanup local streams
       → if doNotReconnect || max==0 → exit
       → attempt = 1..max:
            sleep(backoff_with_jitter(attempt))
            dial + TLS? + preauth + HI(IsReconnect=1, UUID)
            MYINFO → online; reset attempt counter; resume read loop
       → exhausted → exit non-zero
```

On `SHUTDOWN` message: set `doNotReconnect=true`, close parent, exit (no loop).

### D3 — Defaults and knobs

| Knob | Default | Meaning |
|------|---------|---------|
| Agent `-reconnect` | `10` | Base delay seconds; `0` = disable all auto-reconnect |
| Agent `-reconnect-max` | `3` | Max attempts after a drop (ignored if reconnect=0) |
| Controller active dial max | `3` | Symmetric for controller `-c` path; `0` = single try only |

Backoff (normative intent, implement simply):

- Sleep ≈ `base * 2^(attempt-1)` clamped, plus **uniform jitter** in
  `[0, base]` seconds (or ±50% of the computed delay). Exact formula may live
  in code comments + unit tests; must not be a fixed zero-jitter interval.

Successful online (MYINFO accepted / read loop running) **resets** the attempt
counter for the next future drop.

### D4 — UUID / reonline

**First join:** unchanged — controller assigns UUID; agent stores it.

**Reconnect:**

1. Agent HI with `IsReconnect=1` and previous UUID (not JoinUUID-only identity).
2. Controller does **not** allocate a new UUID; may confirm with existing UUID
   path or skip UUIDMess if agent already holds identity (pick one path in
   implementation; tests lock behavior).
3. Agent sends MYINFO with that UUID.
4. Controller replaces `conns[uuid]`, runs `ReonlineNode` (or equivalent) so
   `history` yields the **same** numeric id, then `Calculate`.
5. Offline path still `DelNode` from the online set but **keeps** history.

### D5 — Race: old readLoop vs new session

When replacing a connection, close the old conn and ignore late `nodeOffline`
from the superseded generation (conn generation counter or “only offline if
`conns[uuid]` still points at this conn”).

### D6 — In-flight work

On unexpected drop: local SOCKS/forward/backward/scan/file state fails or is
torn down; no cross-reconnect resume. MCP tasks already surface failure via
status.

### D7 — Controller “reconnect max”

Applies only when the **controller dials** an agent (`-c`). Passive
`-l` accept path does not dial agents. No “controller periodically connects to
agent” behavior.

### D8 — Multi-hop

v1: agent whose **direct** parent/controller link drops. Child notification via
`UPSTREAMOFFLINE` is **out of scope**; middle-node process may still reconnect
upstream on its own if it is active to its parent.

## Risks / Trade-offs

| Risk | Mitigation |
|------|------------|
| OPSEC: reconnect noise | Default max 3 + jitter; not infinite; `-reconnect 0` off |
| Mis-detect intentional close | Always send SHUTDOWN before close on `shutdown_node` |
| node_id churn | Mandatory reonline + history |
| Double offline | Generation-guarded offline |

## Migration

- Same-commit controller + agent after this change.
- Document new defaults in README CLI tables (EN/ZH).
- Existing deployments that relied on “reconnect defaults to 0” must pass
  `-reconnect 0` to keep silent agents.

## Open questions (resolved for v1)

- Heartbeat kick: **deferred**.
- Passive agent reconnect: **deferred**.
- Exact jitter formula: **implementation + unit test**, not wire-visible.
