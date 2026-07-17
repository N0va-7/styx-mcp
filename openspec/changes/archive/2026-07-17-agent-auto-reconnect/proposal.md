## Why

Upstream drops (controller restart, NAT idle timeout, brief network blips) leave
agents dead until an operator restarts them. That breaks MCP body sense:
`list_nodes` goes empty, SOCKS/scan stop, and the same host often returns as a
**new** `node_id`. Wire types and `Topology.ReonlineNode` already exist, but the
agent path after `handleUpstream` ends is still a TODO.

## What Changes

- Active agents (`-c`) **default on** limited auto-reconnect after unexpected
  upstream loss: base interval + jitter, **max 3** attempts, counter **resets**
  after a successful online period.
- Reconnect uses existing UUID with `HIMess.IsReconnect=1`; controller
  **reonline**s so MCP `node_id` stays stable (via topology history).
- **Intentional** disconnect (`SHUTDOWN` / `shutdown_node`) sets do-not-reconnect;
  agent MUST exit without retrying.
- Controller **active** dial mode gets a matching retry max (symmetric knob);
  passive listen is unchanged (agents dial in).
- CLI: agent `-reconnect` default non-zero; `-reconnect-max` default 3;
  `0` disables. Controller exposes an equivalent max for its active connect path.

## Capabilities

### New Capabilities

- (none — extend existing domains)

### Modified Capabilities

- `transport`: agent reconnect loop, shutdown gate, dial retry knobs
- `topology`: reonline preserves numeric `node_id` after offline

## Non-goals

- Infinite / beacon-style always-on reconnect
- In-flight SOCKS, tunnel, scan, or file resume across reconnect
- Passive agent (`-l`) “calling back” to parent
- Multi-hop UPSTREAMOFFLINE cascade to children (v2)
- Heartbeat-based dead-peer detection (may follow; v1 uses TCP read/write failure)
- WebSocket transport

## Impact

- Agent lifecycle and CLI defaults (same-commit controller/agent for reonline path)
- Controller accept/handshake branch for `IsReconnect`
- Topology online set + history (no wire frame layout change if HI/UUID paths reuse existing types)
- README CLI table for `-reconnect` / `-reconnect-max`
