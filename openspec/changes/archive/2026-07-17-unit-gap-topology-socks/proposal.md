## Why

Scenario coverage audit flagged unit gaps for empty topology, unknown
`node_id` on `start_socks`, and local SOCKS bind failure. Close them before
lab e2e so failures are cheap to catch.

## What Changes

- Unit tests: empty `ListNodes` / list_nodes, unknown node rejects without
  binding, SOCKS port busy returns error and leaves no service.
- Spec deltas: clarify empty-list success shape; SOCKS bind failure leaves no
  registered service.

## Capabilities

### New Capabilities

(none)

### Modified Capabilities

- `topology`: empty list is success with empty nodes set
- `socks-proxy`: port-busy / unknown-node leave no controller SOCKS listener

## Non-goals

- Full SOCKS traffic e2e or agent ACK success path
- MCP async task_id timing tests

## Impact

- `pkg/topology`, `pkg/controller`, `pkg/mcp` tests only
