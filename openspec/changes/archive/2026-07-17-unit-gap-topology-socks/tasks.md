## 1. Specs

- [x] 1.1 topology + socks-proxy deltas for empty list and bind-failure hygiene

## 2. Tests

- [x] 2.1 topology: ListAll empty — Scenario "Empty topology when no agents" (unit)
- [x] 2.2 controller: ListNodes empty + StartSocks port busy (unit)
- [x] 2.3 mcp: list_nodes empty + start_socks unknown node_id (unit)

## 3. Validate

- [x] 3.1 `go test ./pkg/topology/ ./pkg/controller/ ./pkg/mcp/ ./...`
- [x] 3.2 `openspec validate --all` + archive
