## 1. Specs

- [x] 1.1 MODIFIED transport passive listen requirement (Start() hard-fail / free bind)

## 2. Tests

- [x] 2.1 Start free port — Scenario "Free port binds" (unit)
- [x] 2.2 Start EADDRINUSE — Scenario "Port already in use" (unit)

## 3. Validate

- [x] 3.1 `go test ./pkg/controller/ ./...`
- [x] 3.2 `openspec validate --all` + archive
