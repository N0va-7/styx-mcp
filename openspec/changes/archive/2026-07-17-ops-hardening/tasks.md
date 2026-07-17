## 1. Topology / MyInfo

- [x] 1.1 Collect local IPv4 addrs on agent; extend MyInfo wire fields
- [x] 1.2 Topology stores LocalAddrs; MCP list/detail expose peer_ip + local_addrs
- [x] 1.3 Unit tests for addr collection + list fields

## 2. Tasks

- [x] 2.1 Add Phase + SetPhase; ToMap includes phase
- [x] 2.2 MCP start_*/upload/pull/cmd set phases

## 3. File transfer

- [x] 3.1 FileData SliceIndex/SliceTotal; chunked upload + download
- [x] 3.2 Tests for multi-slice reassembly path

## 4. SOCKS

- [x] 4.1 Controller half-close: local EOF does not kill remote→local drain
- [x] 4.2 Unit/regression where possible (E2E SOCKS local + lab)

## 5. Validate

- [x] 5.1 go test ./...
- [x] 5.2 openspec validate + archive
