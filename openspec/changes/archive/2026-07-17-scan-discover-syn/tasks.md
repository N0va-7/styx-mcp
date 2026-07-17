## 1. Spec + core

- [x] 1.1 OpenSpec delta: discover phase, optional SYN, connect fallback
- [x] 1.2 `pkg/scan`: PortChecker, Discover, probe port constants + tests
- [x] 1.3 Linux SYN engine + stub for !linux; auto method selection
- [x] 1.4 `Run` only scans hosts list (caller passes alive); stats helpers

## 2. Agent / protocol / MCP

- [x] 2.1 ScanReq Discover + Method fields
- [x] 2.2 Agent: discovering → scanning → fingerprinting; stats.method
- [x] 2.3 MCP start_scan: discover, method knobs + tool text

## 3. Validate

- [x] 3.1 Unit tests green; `openspec validate --all`
- [x] 3.2 Rebuild agent+controller note
