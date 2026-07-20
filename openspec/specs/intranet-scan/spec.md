# intranet-scan Specification

## Purpose

Agent-side intranet recon: hybrid host discovery, port scan (connect or SYN when
available), light fingerprinting, and async task progress exposed over MCP
`start_scan` / `get_task_status`. Scans MUST exit via the selected online agent.
## Requirements
### Requirement: Scan exits via selected agent
Intranet scanning SHALL originate from the network stack of the agent identified
by `node_id` (or equivalent UUID routing), not from the controller host, unless
no such node exists in which case the tool SHALL fail clearly.

#### Scenario: Start scan on online node
- **WHEN** the operator invokes start_scan with a valid online `node_id` and
  valid targets
- **THEN** the system creates an async task and the agent for that node performs
  dial attempts toward the targets

#### Scenario: Unknown node rejected
- **WHEN** start_scan is invoked with an unknown or offline `node_id`
- **THEN** the call fails or the task ends failed with an actionable error, and
  no scan is claimed successful

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

### Requirement: Scan modes fast, normal, full, custom
The system SHALL support modes `fast`, `normal`, `full`, and `custom`:

- `fast`: built-in small high-value port set
- `normal`: built-in larger common port set
- `full`: ports 1–65535 subject to configured safety limits
- `custom`: operator-supplied port list/ranges; missing/invalid ports MUST fail

#### Scenario: fast mode uses built-in set
- **WHEN** start_scan runs with mode `fast` and no custom ports override
- **THEN** dials use only the built-in fast port set (not the full 1–65535 range)

#### Scenario: custom mode requires ports
- **WHEN** mode is `custom` and ports are empty or unparsable
- **THEN** the task or call fails without scanning

#### Scenario: full mode remains bounded
- **WHEN** mode is `full`
- **THEN** the implementation still enforces documented limits (e.g. max hosts,
  concurrency ceiling, and/or wall-clock timeout) rather than unbounded runtime

### Requirement: Two-phase pipeline port then fingerprint
Scanning SHALL first determine open TCP ports, then perform fingerprinting only
against ports found open (when fingerprinting is enabled). Fingerprinting MUST
NOT be required to run against closed ports.

#### Scenario: Fingerprint only open ports
- **WHEN** fingerprinting is enabled and a host has a subset of ports open
- **THEN** active fingerprint probes are attempted for open ports and are not
  required for ports that timed out or refused

#### Scenario: Ports-only job
- **WHEN** fingerprinting is disabled
- **THEN** the task completes after the port phase with open ports listed and
  without requiring product/refs fields to be populated

### Requirement: Light fingerprint fields
For fingerprinted open ports, the result SHOULD include when available:
`service`, `product`, optional `version`, `evidence` (truncated), and
`confidence`. Absence of version MUST be allowed.

#### Scenario: HTTP-like open port
- **WHEN** an open port speaks HTTP and returns a Server or title
- **THEN** the open entry includes evidence and a best-effort service/product
  classification for model consumption

### Requirement: Vulnerability reference links not exploits
When a product (and optional version) matches a seed rule, the result SHALL
attach zero or more `refs` entries with stable URLs (e.g. CVE/NVD or vendor
advisory). The system MUST NOT execute exploits or vulnerability POCs as part of
start_scan. Refs are hints for a human or model to verify.

#### Scenario: Known product attaches refs
- **WHEN** fingerprint classifies a service as a seeded product with ref rules
- **THEN** the open entry includes at least one ref with `id` and `url`

#### Scenario: Unknown product has empty refs
- **WHEN** a port is open but product is unknown or unmatched
- **THEN** the entry may omit refs or use an empty list without failing the job

### Requirement: Async task phases for scan
Scan jobs SHALL run as async tasks and expose phase values including at least
`scanning` and `fingerprinting` while in progress, and a terminal success or
failure state consumable via get_task_status (or equivalent).

#### Scenario: Phase advances
- **WHEN** a scan is running
- **THEN** get_task_status shows phase `scanning` and later `fingerprinting`
  (if fingerprint enabled) before terminal `done` or failed

### Requirement: Structured result for models
Completed scan tasks SHALL return structured data including a list of open
findings and a short `summary.interesting` (or equivalent) highlighting entries
with refs or other high-value signals, plus basic stats (counts, duration).

#### Scenario: Result shape
- **WHEN** a scan task completes successfully with at least one open port
- **THEN** task result includes an open list with ip/port/state and stats such
  as open count and duration, suitable for MCP JSON consumption

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

### Requirement: Hybrid host discovery
Default start_scan discovery SHALL combine best-effort ICMP and TCP/SYN probe
ports so that either signal can mark a host alive.

#### Scenario: Either signal marks alive
- **WHEN** a host fails TCP probes but ICMP succeeds (or the reverse)
- **THEN** the host is included in the alive set for subsequent port scanning

