package scan

import (
	"encoding/json"
	"testing"
)

// Freeze result JSON field names used by agent/controller/MCP consumers.
func TestResultJSONFieldNames(t *testing.T) {
	type openEntry struct {
		IP         string `json:"ip"`
		Port       uint16 `json:"port"`
		Proto      string `json:"proto"`
		State      string `json:"state"`
		Service    string `json:"service,omitempty"`
		Product    string `json:"product,omitempty"`
		Version    string `json:"version,omitempty"`
		Title      string `json:"title,omitempty"`
		Evidence   string `json:"evidence,omitempty"`
		Confidence string `json:"confidence,omitempty"`
		Refs       []struct {
			Type      string `json:"type"`
			ID        string `json:"id"`
			URL       string `json:"url"`
			Condition string `json:"condition,omitempty"`
		} `json:"refs,omitempty"`
	}
	type result struct {
		ViaNodeID int    `json:"via_node_id"`
		Mode      string `json:"mode"`
		Stats     struct {
			HostsTotal    int   `json:"hosts_total"`
			HostsWithOpen int   `json:"hosts_with_open"`
			PortsTried    int64 `json:"ports_tried"`
			Open          int   `json:"open"`
			DurationMs    int64 `json:"duration_ms"`
		} `json:"stats"`
		Open    []openEntry `json:"open"`
		Summary struct {
			Interesting []struct {
				IP    string `json:"ip"`
				Port  uint16 `json:"port"`
				Why   string `json:"why"`
				RefsN int    `json:"refs_n"`
			} `json:"interesting"`
		} `json:"summary"`
	}

	sample := result{
		ViaNodeID: 0,
		Mode:      ModeFast,
		Open: []openEntry{{
			IP: "172.16.23.20", Port: 7001, Proto: "tcp", State: "open",
			Product: "weblogic",
			Refs: []struct {
				Type      string `json:"type"`
				ID        string `json:"id"`
				URL       string `json:"url"`
				Condition string `json:"condition,omitempty"`
			}{{Type: "cve", ID: "CVE-2020-14882", URL: "https://nvd.nist.gov/vuln/detail/CVE-2020-14882"}},
		}},
	}
	sample.Stats.HostsTotal = 1
	sample.Stats.Open = 1
	sample.Summary.Interesting = []struct {
		IP    string `json:"ip"`
		Port  uint16 `json:"port"`
		Why   string `json:"why"`
		RefsN int    `json:"refs_n"`
	}{{IP: "172.16.23.20", Port: 7001, Why: "weblogic + refs", RefsN: 1}}

	b, err := json.Marshal(sample)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"via_node_id", "mode", "stats", "open", "summary"} {
		if _, ok := m[key]; !ok {
			t.Errorf("missing top-level key %q", key)
		}
	}
	stats := m["stats"].(map[string]interface{})
	for _, key := range []string{"hosts_total", "hosts_with_open", "ports_tried", "open", "duration_ms"} {
		if _, ok := stats[key]; !ok {
			t.Errorf("missing stats.%s", key)
		}
	}
	sum := m["summary"].(map[string]interface{})
	if _, ok := sum["interesting"]; !ok {
		t.Error("missing summary.interesting")
	}
}
