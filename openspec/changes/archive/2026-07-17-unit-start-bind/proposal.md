## Why

Transport scenarios "Free port binds" and "Port already in use" only had
message-level unit tests (`listenBindError`), not end-to-end `Controller.Start()`.
Close that gap so MCP healthy-without-listen regressions fail in `go test`.

## What Changes

- Unit tests calling `Start()` for free bind and EADDRINUSE hard-fail.
- Spec wording: bind failure surfaces via `Start()` (not only helper formatting).

## Capabilities

### New Capabilities

(none)

### Modified Capabilities

- `transport`: passive listen scenarios pin `Start()` behavior

## Non-goals

- Full agent join over the new listen socket
- Active-mode `Start()` tests

## Impact

- `pkg/controller` tests only
