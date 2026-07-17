package fingerprint

import (
	"bufio"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// Confidence levels.
const (
	ConfidenceHigh   = "high"
	ConfidenceMedium = "medium"
	ConfidenceLow    = "low"
)

// Finding is one open port after optional fingerprinting.
type Finding struct {
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
	Refs       []Ref  `json:"refs,omitempty"`
}

// Ref is an advisory/CVE link (hints only, not confirmed vulns).
type Ref struct {
	Type      string `json:"type"` // cve | advisory | doc
	ID        string `json:"id"`
	URL       string `json:"url"`
	Condition string `json:"condition,omitempty"`
}

// Interesting is a short highlight for models.
type Interesting struct {
	IP    string `json:"ip"`
	Port  uint16 `json:"port"`
	Why   string `json:"why"`
	RefsN int    `json:"refs_n"`
}

// Dialer for fingerprint probes.
type Dialer interface {
	DialContext(ctx context.Context, network, address string) (net.Conn, error)
}

// Config for fingerprint phase.
type Config struct {
	Timeout     time.Duration
	Concurrency int
	Dialer      Dialer
}

const (
	defaultFPTimeout = 1200 * time.Millisecond
	maxEvidence      = 240
)

// ProbeOpen fingerprints open ports only.
func ProbeOpen(ctx context.Context, open []struct {
	IP   string
	Port uint16
}, cfg Config) []Finding {
	if cfg.Timeout <= 0 {
		cfg.Timeout = defaultFPTimeout
	}
	if cfg.Concurrency <= 0 {
		cfg.Concurrency = 20
	}
	if cfg.Dialer == nil {
		cfg.Dialer = &net.Dialer{Timeout: cfg.Timeout}
	}

	type job struct {
		ip   string
		port uint16
	}
	jobs := make(chan job, len(open))
	for _, o := range open {
		jobs <- job{ip: o.IP, port: o.Port}
	}
	close(jobs)

	outCh := make(chan Finding, len(open))
	workers := cfg.Concurrency
	if workers > len(open) && len(open) > 0 {
		workers = len(open)
	}
	if workers < 1 {
		workers = 1
	}
	done := make(chan struct{})
	for i := 0; i < workers; i++ {
		go func() {
			for j := range jobs {
				if ctx.Err() != nil {
					outCh <- Finding{IP: j.ip, Port: j.port, Proto: "tcp", State: "open"}
					continue
				}
				f := probeOne(ctx, j.ip, j.port, cfg)
				outCh <- f
			}
			done <- struct{}{}
		}()
	}
	go func() {
		for i := 0; i < workers; i++ {
			<-done
		}
		close(outCh)
	}()

	var findings []Finding
	for f := range outCh {
		findings = append(findings, f)
	}
	// Attach refs from seed table.
	for i := range findings {
		findings[i].Refs = MatchRefs(findings[i])
	}
	return findings
}

func probeOne(ctx context.Context, ip string, port uint16, cfg Config) Finding {
	f := Finding{
		IP:    ip,
		Port:  port,
		Proto: "tcp",
		State: "open",
	}
	addr := net.JoinHostPort(ip, fmt.Sprintf("%d", port))

	// Port heuristics first (cheap).
	switch port {
	case 22, 2222:
		return probeSSH(ctx, addr, cfg, f)
	case 6379:
		return probeRedis(ctx, addr, cfg, f)
	case 3306:
		return probeMySQL(ctx, addr, cfg, f)
	case 5432:
		return probeBannerService(ctx, addr, cfg, f, "postgresql", "postgresql", ConfidenceLow)
	case 27017:
		return probeBannerService(ctx, addr, cfg, f, "mongodb", "mongodb", ConfidenceLow)
	case 445, 139:
		f.Service = "smb"
		f.Product = "smb"
		f.Confidence = ConfidenceLow
		f.Evidence = "port heuristic"
		return f
	case 3389:
		f.Service = "rdp"
		f.Product = "rdp"
		f.Confidence = ConfidenceLow
		f.Evidence = "port heuristic"
		return f
	case 80, 81, 443, 8000, 8001, 8080, 8081, 8443, 8888, 9000, 9090, 7001, 7002, 4848, 9200, 5601, 9443, 7443:
		return probeHTTP(ctx, addr, port, cfg, f)
	}

	// Generic banner then optional HTTP guess.
	if fb := tryBanner(ctx, addr, cfg); fb != "" {
		f.Evidence = truncate(fb, maxEvidence)
		classifyBanner(&f, fb)
		if f.Service != "" {
			return f
		}
	}
	// Unknown high ports: try HTTP once.
	if port >= 8000 || port == 3000 || port == 5000 {
		return probeHTTP(ctx, addr, port, cfg, f)
	}
	f.Confidence = ConfidenceLow
	return f
}

func probeSSH(ctx context.Context, addr string, cfg Config, f Finding) Finding {
	banner := tryBanner(ctx, addr, cfg)
	f.Service = "ssh"
	f.Product = "ssh"
	f.Confidence = ConfidenceMedium
	if banner != "" {
		f.Evidence = truncate(banner, maxEvidence)
		f.Confidence = ConfidenceHigh
		if strings.Contains(strings.ToLower(banner), "openssh") {
			f.Product = "openssh"
			if v := extractVersion(banner, `OpenSSH[_\s]?([\d.]+)`); v != "" {
				f.Version = v
			}
		}
	}
	return f
}

func probeRedis(ctx context.Context, addr string, cfg Config, f Finding) Finding {
	f.Service = "redis"
	f.Product = "redis"
	f.Confidence = ConfidenceMedium
	dctx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()
	conn, err := cfg.Dialer.DialContext(dctx, "tcp", addr)
	if err != nil {
		f.Evidence = "connect failed after open"
		f.Confidence = ConfidenceLow
		return f
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(cfg.Timeout))
	_, _ = conn.Write([]byte("PING\r\n"))
	buf := make([]byte, 64)
	n, _ := conn.Read(buf)
	resp := string(buf[:n])
	if strings.Contains(resp, "+PONG") || strings.Contains(resp, "-NOAUTH") || strings.Contains(resp, "-ERR") {
		f.Evidence = truncate(resp, maxEvidence)
		f.Confidence = ConfidenceHigh
	} else if resp != "" {
		f.Evidence = truncate(resp, maxEvidence)
	}
	return f
}

func probeMySQL(ctx context.Context, addr string, cfg Config, f Finding) Finding {
	f.Service = "mysql"
	f.Product = "mysql"
	f.Confidence = ConfidenceMedium
	banner := tryBanner(ctx, addr, cfg)
	if banner != "" {
		f.Evidence = truncate(banner, maxEvidence)
		if strings.Contains(strings.ToLower(banner), "mariadb") {
			f.Product = "mariadb"
		}
		f.Confidence = ConfidenceHigh
	}
	return f
}

func probeBannerService(ctx context.Context, addr string, cfg Config, f Finding, service, product, conf string) Finding {
	f.Service = service
	f.Product = product
	f.Confidence = conf
	if b := tryBanner(ctx, addr, cfg); b != "" {
		f.Evidence = truncate(b, maxEvidence)
		if conf != ConfidenceHigh {
			f.Confidence = ConfidenceMedium
		}
	}
	return f
}

func probeHTTP(ctx context.Context, addr string, port uint16, cfg Config, f Finding) Finding {
	f.Service = "http"
	f.Product = "http"
	f.Confidence = ConfidenceLow

	tlsPorts := map[uint16]bool{443: true, 8443: true, 9443: true, 7443: true}
	useTLS := tlsPorts[port]

	// Try plain then TLS if needed.
	for _, secure := range []bool{useTLS, !useTLS} {
		if ctx.Err() != nil {
			break
		}
		scheme := "http"
		if secure {
			scheme = "https"
		}
		url := fmt.Sprintf("%s://%s/", scheme, addr)
		client := &http.Client{
			Timeout: cfg.Timeout,
			Transport: &http.Transport{
				DialContext: cfg.Dialer.DialContext,
				TLSClientConfig: &tls.Config{
					InsecureSkipVerify: true, //nolint:gosec // recon only
				},
				DisableKeepAlives: true,
			},
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 2 {
					return http.ErrUseLastResponse
				}
				return nil
			},
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			continue
		}
		req.Header.Set("User-Agent", "styx-scan/0.1")
		resp, err := client.Do(req)
		if err != nil {
			continue
		}
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		_ = resp.Body.Close()

		server := resp.Header.Get("Server")
		title := extractTitle(string(body))
		f.Title = title
		f.Service = "http"
		if secure {
			f.Service = "https"
		}
		parts := []string{fmt.Sprintf("status=%d", resp.StatusCode)}
		if server != "" {
			parts = append(parts, "Server="+server)
		}
		if title != "" {
			parts = append(parts, "Title="+title)
		}
		f.Evidence = truncate(strings.Join(parts, " "), maxEvidence)
		f.Confidence = ConfidenceMedium
		classifyHTTP(&f, server, title, string(body), port)
		return f
	}
	f.Evidence = "http probe failed"
	return f
}

func classifyHTTP(f *Finding, server, title, body string, port uint16) {
	lowS := strings.ToLower(server)
	lowT := strings.ToLower(title)
	lowB := strings.ToLower(body)
	switch {
	case strings.Contains(lowS, "weblogic") || strings.Contains(lowT, "weblogic") ||
		strings.Contains(lowB, "weblogic") || port == 7001 || port == 7002:
		f.Product = "weblogic"
		f.Confidence = ConfidenceMedium
		if strings.Contains(lowS, "weblogic") || strings.Contains(lowB, "weblogic") {
			f.Confidence = ConfidenceHigh
		}
	case strings.Contains(lowT, "thinkphp") || strings.Contains(lowB, "thinkphp"):
		f.Product = "thinkphp"
		f.Confidence = ConfidenceMedium
	case strings.Contains(lowS, "nginx"):
		f.Product = "nginx"
		f.Confidence = ConfidenceHigh
		if v := extractVersion(server, `nginx/([\d.]+)`); v != "" {
			f.Version = v
		}
	case strings.Contains(lowS, "apache"):
		f.Product = "apache"
		f.Confidence = ConfidenceHigh
	case strings.Contains(lowS, "microsoft-iis") || strings.Contains(lowS, "iis"):
		f.Product = "iis"
		f.Confidence = ConfidenceHigh
	case port == 9200 || strings.Contains(lowB, "elasticsearch") || strings.Contains(lowB, `"tagline"`):
		f.Product = "elasticsearch"
		f.Confidence = ConfidenceMedium
	case port == 5601 || strings.Contains(lowT, "kibana"):
		f.Product = "kibana"
		f.Confidence = ConfidenceMedium
	case strings.Contains(lowS, "tomcat") || strings.Contains(lowT, "tomcat"):
		f.Product = "tomcat"
		f.Confidence = ConfidenceMedium
	default:
		if server != "" {
			f.Product = "http"
		}
	}
}

func classifyBanner(f *Finding, banner string) {
	low := strings.ToLower(banner)
	switch {
	case strings.HasPrefix(low, "ssh-"):
		f.Service = "ssh"
		f.Product = "ssh"
		f.Confidence = ConfidenceHigh
		if strings.Contains(low, "openssh") {
			f.Product = "openssh"
		}
	case strings.Contains(low, "ftp"):
		f.Service = "ftp"
		f.Product = "ftp"
		f.Confidence = ConfidenceMedium
	case strings.Contains(low, "smtp") || strings.HasPrefix(low, "220 "):
		f.Service = "smtp"
		f.Product = "smtp"
		f.Confidence = ConfidenceLow
	}
}

func tryBanner(ctx context.Context, addr string, cfg Config) string {
	dctx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()
	conn, err := cfg.Dialer.DialContext(dctx, "tcp", addr)
	if err != nil {
		return ""
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(cfg.Timeout))
	r := bufio.NewReader(conn)
	// Some services send first; wait briefly.
	line, err := r.ReadString('\n')
	if err != nil && len(line) == 0 {
		// Try reading raw bytes without newline.
		buf := make([]byte, 256)
		n, _ := r.Read(buf)
		if n == 0 {
			return ""
		}
		return string(buf[:n])
	}
	return strings.TrimSpace(line)
}

func extractTitle(body string) string {
	re := regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)
	m := re.FindStringSubmatch(body)
	if len(m) < 2 {
		return ""
	}
	t := strings.TrimSpace(m[1])
	t = strings.Join(strings.Fields(t), " ")
	return truncate(t, 120)
}

func extractVersion(s, pattern string) string {
	re := regexp.MustCompile(`(?i)` + pattern)
	m := re.FindStringSubmatch(s)
	if len(m) >= 2 {
		return m[1]
	}
	return ""
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// BuildInteresting picks entries with refs or high-value ports/products.
func BuildInteresting(findings []Finding) []Interesting {
	highPorts := map[uint16]string{
		22: "ssh", 445: "smb", 3389: "rdp", 6379: "redis",
		3306: "mysql", 5432: "postgres", 27017: "mongodb",
		7001: "weblogic-port", 2375: "docker-api", 9200: "elasticsearch",
	}
	var out []Interesting
	for _, f := range findings {
		if len(f.Refs) > 0 {
			why := f.Product
			if why == "" {
				why = f.Service
			}
			if why == "" {
				why = "open"
			}
			out = append(out, Interesting{
				IP: f.IP, Port: f.Port,
				Why:   why + " + refs",
				RefsN: len(f.Refs),
			})
			continue
		}
		if label, ok := highPorts[f.Port]; ok {
			out = append(out, Interesting{
				IP: f.IP, Port: f.Port,
				Why:   label,
				RefsN: 0,
			})
			continue
		}
		switch strings.ToLower(f.Product) {
		case "weblogic", "thinkphp", "redis", "elasticsearch":
			out = append(out, Interesting{
				IP: f.IP, Port: f.Port,
				Why:   f.Product,
				RefsN: 0,
			})
		}
	}
	return out
}

// AttachRefs fills Refs for each finding (controller-side enrichment helper).
func AttachRefs(findings []Finding) {
	for i := range findings {
		findings[i].Refs = MatchRefs(findings[i])
	}
}
