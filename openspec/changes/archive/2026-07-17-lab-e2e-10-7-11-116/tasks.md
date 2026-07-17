## 1. Prep

- [x] 1.1 Open change + rebuild controller/agent
- [x] 1.2 Ensure agent online (local if none)
- [x] 1.3 Probe lab `http://10.7.11.116/` direct (baseline)

## 2. SOCKS e2e

- [x] 2.1 `start_socks` on node → wait ready
- [x] 2.2 curl via SOCKS to lab; compare status/title
- [x] 2.3 Record results in design.md evidence

## 3. Script + specs

- [x] 3.1 Add `scripts/lab-e2e-socks.sh` + Go `TestE2E*`
- [x] 3.2 Spec delta socks-proxy lab/loopback scenario
- [x] 3.3 validate + archive
