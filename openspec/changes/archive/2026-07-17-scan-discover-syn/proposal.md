## Why

Full-CIDR connect scan without host discovery is slow (~254×ports×timeout).
fscan-like speed needs: **alive probe first**, then port-scan only live hosts;
prefer **SYN** when the agent has CAP_NET_RAW/root, else **TCP connect**.

## What Changes

- Default **discover** phase: small high-value probe ports (TCP or SYN).
- Port phase runs **only on hosts that answered** (unless discover disabled).
- Method `auto` (default): try SYN if privileged, else connect.
- Task phases: `discovering` → `scanning` → `fingerprinting` → `done`.
- Stats: hosts_alive, method, discover timing.
- Spec: unprivileged path remains required; SYN is optional acceleration.

## Capabilities

### Modified Capabilities

- `intranet-scan`: discover + optional SYN; connect fallback
- `mcp-async-tasks`: phase `discovering`

## Non-goals

- ICMP-only discovery as hard dependency
- Windows SYN
- Weak-password / exploit modules
