## MODIFIED Requirements

### Requirement: TCP connect only without admin privileges
Port scanning SHALL always support unprivileged TCP connect-style probes so
agents without raw-socket capability can still scan. The implementation MUST
NOT require elevated privileges for the default path.

WHEN the agent process has raw IPv4 TCP capability (e.g. root or CAP_NET_RAW on
Linux) and method is `auto` or `syn`, the implementation MAY use SYN/half-open
probes for discovery and/or port scanning. WHEN SYN is unavailable or method is
`connect`, the implementation SHALL use TCP connect. Method `syn` when
capability is missing MUST fail clearly rather than silently claiming SYN.

#### Scenario: Unprivileged agent can scan
- **WHEN** an agent process runs as a non-admin user (e.g. www-data)
- **THEN** start_scan in `fast` or `normal` mode with method auto/connect can
  still report open ports without requiring privilege escalation

#### Scenario: Privileged auto prefers SYN
- **WHEN** the agent can open a raw IPv4 TCP packet socket and method is `auto`
- **THEN** discovery and/or port probes use the SYN path and task stats record
  method `syn` (or equivalent)

#### Scenario: Forced SYN without capability fails
- **WHEN** method is `syn` and the agent cannot use raw TCP
- **THEN** the task or call fails with an actionable error

### Requirement: No hard dependency on ICMP/ping
Host discovery MUST NOT require ICMP echo. Disabled ping or filtered ICMP MUST
NOT block scanning. Default host discovery SHALL use TCP (or SYN) probes to a
small built-in port set. A host is treated as alive when at least one probe port
responds open (connect success or SYN-ACK). Final open ports still come from the
mode port scan (or discover-only empties).

#### Scenario: Scan with ping disabled
- **WHEN** targets are scanned in an environment where ICMP is blocked
- **THEN** open TCP ports are still reported for responsive services and the job
  does not fail solely due to missing ping replies

#### Scenario: Discover finds no alive hosts
- **WHEN** discover is enabled and no probe port answers on any target
- **THEN** the job completes successfully with an empty open list and stats
  reflecting zero alive hosts (not a hard error)

## ADDED Requirements

### Requirement: Host discovery before full port set
By default, start_scan SHALL run a discovery phase against all expanded targets
using a small high-value probe port set, then run the configured mode port scan
only against hosts classified alive. Operators MUST be able to disable discovery
to force scanning every host with the full mode port set (previous behavior).

#### Scenario: Default discover then port-scan alive only
- **WHEN** start_scan runs with default discover on a multi-host CIDR
- **THEN** the mode port set is not required to be dialed against hosts that
  failed all discovery probes

#### Scenario: Discover disabled scans all hosts
- **WHEN** discover is explicitly disabled
- **THEN** the mode port set is applied to all expanded target hosts (subject to
  existing caps)

### Requirement: Discovering phase visible
While host discovery is running, async scan tasks SHALL expose phase
`discovering` (or a documented synonym) before `scanning`.

#### Scenario: Discover phase visible
- **WHEN** a start_scan task is in host discovery
- **THEN** get_task_status includes phase `discovering`
