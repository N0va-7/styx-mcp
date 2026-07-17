## ADDED Requirements

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
Port scanning SHALL use TCP connect-style dials only. The implementation MUST
NOT require raw sockets, SYN/half-open scans, or elevated/admin privileges.

#### Scenario: Unprivileged agent can scan
- **WHEN** an agent process runs as a non-admin user (e.g. www-data)
- **THEN** start_scan in `fast` or `normal` mode can still report open ports for
  reachable services without requiring privilege escalation

### Requirement: No hard dependency on ICMP/ping
Host discovery MUST NOT require ICMP echo. Disabled ping or filtered ICMP MUST
NOT block scanning. A host MAY be treated as having open services only when at
least one scanned TCP port is open.

#### Scenario: Scan with ping disabled
- **WHEN** targets are scanned in an environment where ICMP is blocked
- **THEN** open TCP ports are still reported for responsive services and the job
  does not fail solely due to missing ping replies

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
