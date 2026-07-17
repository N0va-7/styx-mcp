package scan

// DiscoverProgress is a snapshot during host discovery for observability.
type DiscoverProgress struct {
	// Stage: "icmp" | "tcp" | "icmp_done" | "tcp_done"
	Stage string `json:"stage"`
	// HostsTotal is expanded target count.
	HostsTotal int `json:"hosts_total"`
	// ICMPDone / ICMPTotal: ICMP probe progress (hosts completed / hosts).
	ICMPDone  int64 `json:"icmp_done"`
	ICMPTotal int   `json:"icmp_total,omitempty"`
	// ICMPAlive: hosts that answered ICMP so far.
	ICMPAlive int `json:"icmp_alive"`
	// TCPProbes: TCP/SYN open-check attempts so far.
	TCPProbes int64 `json:"tcp_probes,omitempty"`
	// AliveN: hosts currently classified alive (ICMP or TCP).
	AliveN int `json:"alive_n"`
	// Method: connect|syn for TCP phase.
	Method string `json:"method,omitempty"`
}

// DiscoverProgressFn is invoked periodically during Discover (best-effort; may skip).
type DiscoverProgressFn func(p DiscoverProgress)
