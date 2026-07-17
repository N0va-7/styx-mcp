## 1. Spec

- [x] 1.1 Delta specs under this change (`transport`, `topology`)
- [x] 1.2 `openspec validate` for this change / `--all`

## 2. Agent reconnect loop

- [ ] 2.1 Options: default `-reconnect 10`, `-reconnect-max 3`; `0` disables
- [ ] 2.2 Outer lifecycle: unexpected drop → backoff+jitter → re-dial (max N)
- [ ] 2.3 On success, reset attempt counter; preserve UUID across reconnect
- [ ] 2.4 `SHUTDOWN` → do-not-reconnect → exit without retry
- [ ] 2.5 Tear down local SOCKS/tunnels/scan on drop (no resume)
- [ ] 2.6 Unit tests: backoff/jitter bounds; shutdown suppresses retry

## 3. Controller reonline

- [ ] 3.1 Accept path: `IsReconnect=1` reuses UUID, no new mint when valid
- [ ] 3.2 `ReonlineNode` + conn map replace; stable `node_id` via history
- [ ] 3.3 Offline generation guard (stale readLoop cannot DelNode new session)
- [ ] 3.4 `shutdown_node` always sends `SHUTDOWN` before close
- [ ] 3.5 Active controller dial: configurable max retries (default 3)
- [ ] 3.6 Unit/integration tests: reonline same id; shutdown no second join

## 4. Docs + validate

- [ ] 4.1 README / README_ZH CLI flags for reconnect defaults
- [ ] 4.2 `go test` packages touched; rebuild note if handshake path changes
