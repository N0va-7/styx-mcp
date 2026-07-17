## Why

Real-lab use showed operators cannot see agent LAN addresses, SOCKS sometimes
truncates HTTP responses when clients half-close, large file transfer is
single-slice only, and async tasks lack phase/error detail.

## What Changes

- Nodes report local IPv4 addresses; MCP list/detail expose `peer_ip` + `local_addrs`.
- SOCKS: tolerate local half-close so remote responses can finish (HTTP-friendly).
- File upload/download: multi-slice chunks with progress on tasks.
- Tasks: `phase` field + SetPhase; richer failure messages.

## Capabilities

### New Capabilities

- `file-transfer`: chunked upload/download behavior

### Modified Capabilities

- `topology`: local address reporting
- `socks-proxy`: half-close / response completeness
- `mcp-async-tasks`: phase observability

## Non-goals

- Secret argv hygiene (deferred)
- WebSocket transport
- Agent auto-reconnect defaults

## Impact

- Wire format: MyInfo + FileData extended (same-commit controller/agent)
- MCP JSON fields additive (`peer_ip`, `local_addrs`, task `phase`)
