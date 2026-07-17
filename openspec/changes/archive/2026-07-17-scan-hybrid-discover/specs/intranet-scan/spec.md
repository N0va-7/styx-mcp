## MODIFIED Requirements

### Requirement: No hard dependency on ICMP/ping
Host discovery MUST NOT require ICMP echo to succeed. WHEN ICMP is unavailable or
filtered, discovery SHALL still use TCP (or SYN) probes. WHEN ICMP is available,
a host that answers ICMP SHALL be treated as alive even if TCP probes fail.
Default discovery SHALL treat a host as alive if **ICMP succeeds OR** at least one
probe port is open.

#### Scenario: Scan with ping disabled
- **WHEN** targets are scanned where ICMP is blocked
- **THEN** hosts with open TCP probe ports are still discovered and the job does
  not fail solely due to missing ping replies

#### Scenario: ICMP-only host still discovered
- **WHEN** a host answers ICMP but all default TCP probe ports are closed
- **THEN** default discovery still classifies the host as alive for the mode
  port-scan phase (best-effort ICMP)

#### Scenario: Discover finds no alive hosts
- **WHEN** discover is enabled and no host answers ICMP or TCP probes
- **THEN** the implementation SHALL fall back to scanning all expanded targets
  (or document equivalent full coverage) and record a warning in the result
  rather than silently returning an empty open list without attempting ports

## ADDED Requirements

### Requirement: Hybrid host discovery
Default start_scan discovery SHALL combine best-effort ICMP and TCP/SYN probe
ports so that either signal can mark a host alive.

#### Scenario: Either signal marks alive
- **WHEN** a host fails TCP probes but ICMP succeeds (or the reverse)
- **THEN** the host is included in the alive set for subsequent port scanning
