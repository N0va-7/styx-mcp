## 1. Audit artifacts

- [x] 1.1 Write proposal + design matrix (Scenario → unit/MCP/lab)
- [x] 1.2 Record lab `10.7.11.116` fingerprint in design

## 2. Spec deltas

- [x] 2.1 MODIFIED transport WebSocket requirement (controller + agent)
- [x] 2.2 Fix dev-workflow Purpose text

## 3. Cheap unit coverage (harden existing scenarios)

- [x] 3.1 topology: UpdateMemo set/read — Scenario "Set and read memo" (unit)
- [x] 3.2 node + controller: validateTransport ws/raw — Scenario "Agent rejects -up/-down ws" (unit)

## 4. Validate

- [x] 4.1 `go test ./...`
- [x] 4.2 `openspec validate --all`
- [x] 4.3 Archive change into main specs
