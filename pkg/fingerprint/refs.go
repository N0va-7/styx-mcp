package fingerprint

import "strings"

// seedRefs maps normalized product → advisory links (hints only).
var seedRefs = map[string][]Ref{
	"weblogic": {
		{
			Type:      "cve",
			ID:        "CVE-2020-14882",
			URL:       "https://nvd.nist.gov/vuln/detail/CVE-2020-14882",
			Condition: "Oracle WebLogic console; verify exposure/version — not confirmed by scan",
		},
		{
			Type:      "cve",
			ID:        "CVE-2017-10271",
			URL:       "https://nvd.nist.gov/vuln/detail/CVE-2017-10271",
			Condition: "WebLogic WLS WSAT; verify version/path — not confirmed by scan",
		},
	},
	"redis": {
		{
			Type:      "advisory",
			ID:        "REDIS-UNAUTH",
			URL:       "https://redis.io/docs/latest/operate/oss_and_stack/management/security/",
			Condition: "Unauthenticated Redis is high risk if reachable; verify AUTH/bind",
		},
	},
	"ssh": {
		{
			Type:      "doc",
			ID:        "SSH-HARDENING",
			URL:       "https://www.ssh.com/academy/ssh/protocol",
			Condition: "Generic SSH exposure; review auth methods and banners",
		},
	},
	"openssh": {
		{
			Type:      "doc",
			ID:        "OPENSSH",
			URL:       "https://www.openssh.com/security.html",
			Condition: "Check OpenSSH version against current advisories",
		},
	},
	// Note: plain product "http"/"https" intentionally has no seed refs so
	// every web port does not flood summary.interesting; named products do.
	"thinkphp": {
		{
			Type:      "cve",
			ID:        "CVE-2018-20062",
			URL:       "https://nvd.nist.gov/vuln/detail/CVE-2018-20062",
			Condition: "ThinkPHP RCE class issues; verify framework version — not confirmed",
		},
	},
	"elasticsearch": {
		{
			Type:      "advisory",
			ID:        "ES-EXPOSURE",
			URL:       "https://www.elastic.co/guide/en/elasticsearch/reference/current/security-minimal-setup.html",
			Condition: "Open Elasticsearch APIs are sensitive; verify auth/network ACLs",
		},
	},
	"smb": {
		{
			Type:      "cve",
			ID:        "CVE-2017-0144",
			URL:       "https://nvd.nist.gov/vuln/detail/CVE-2017-0144",
			Condition: "EternalBlue-class SMB issues on unpatched hosts; verify OS patch level",
		},
	},
	"mysql": {
		{
			Type:      "doc",
			ID:        "MYSQL-EXPOSURE",
			URL:       "https://dev.mysql.com/doc/refman/8.0/en/security-guidelines.html",
			Condition: "MySQL on LAN; verify bind-address and credentials",
		},
	},
	"rdp": {
		{
			Type:      "cve",
			ID:        "CVE-2019-0708",
			URL:       "https://nvd.nist.gov/vuln/detail/CVE-2019-0708",
			Condition: "BlueKeep-class RDP issues on unpatched Windows; verify patch level",
		},
	},
}

// MatchRefs returns seed refs for a finding's product/service (empty if unknown).
// Generic http/https refs are only attached when product is exactly http/https
// with no stronger product classification — still useful as soft hints.
func MatchRefs(f Finding) []Ref {
	keys := []string{
		strings.ToLower(strings.TrimSpace(f.Product)),
		strings.ToLower(strings.TrimSpace(f.Service)),
	}
	// Prefer product over service; avoid double-adding same set.
	seen := make(map[string]struct{})
	var out []Ref
	for _, k := range keys {
		if k == "" {
			continue
		}
		// Skip ultra-generic "http"/"https" service when product is more specific.
		if (k == "http" || k == "https") && f.Product != "" &&
			strings.ToLower(f.Product) != "http" && strings.ToLower(f.Product) != "https" {
			continue
		}
		refs, ok := seedRefs[k]
		if !ok {
			continue
		}
		for _, r := range refs {
			id := r.Type + ":" + r.ID
			if _, dup := seen[id]; dup {
				continue
			}
			seen[id] = struct{}{}
			out = append(out, r)
		}
		// Product match is enough.
		if k == strings.ToLower(strings.TrimSpace(f.Product)) && k != "" {
			break
		}
	}
	return out
}
