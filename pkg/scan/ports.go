package scan

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// Built-in port profiles. Freeze literals here; unit tests lock the sets.
var (
	// FastPorts: small high-value intranet set (~35).
	FastPorts = []uint16{
		21, 22, 23, 25, 53, 80, 110, 135, 139, 143,
		443, 445, 993, 995, 1433, 1521, 2181, 2375, 2379, 3306,
		3389, 5000, 5432, 5601, 5900, 5985, 6379, 6443, 7001, 8000,
		8080, 8443, 8888, 9000, 9090, 9200, 11211, 27017,
	}

	// NormalPorts: larger common intranet set (~110); supersets FastPorts ideas.
	NormalPorts = []uint16{
		21, 22, 23, 25, 53, 80, 81, 88, 110, 111,
		135, 139, 143, 389, 443, 445, 465, 502, 512, 513,
		514, 587, 636, 873, 993, 995, 1080, 1099, 1433, 1521,
		1723, 1883, 2049, 2082, 2083, 2086, 2087, 2095, 2096, 2181,
		2222, 2375, 2376, 2379, 2483, 2484, 3000, 3128, 3268, 3269,
		3306, 3389, 3690, 4000, 4369, 4443, 4505, 4506, 4848, 5000,
		5001, 5432, 5555, 5601, 5672, 5900, 5901, 5984, 5985, 5986,
		6000, 6379, 6443, 6666, 7001, 7002, 7070, 7071, 7180, 7443,
		7777, 8000, 8001, 8008, 8009, 8010, 8069, 8080, 8081, 8088,
		8089, 8090, 8161, 8443, 8480, 8880, 8888, 9000, 9001, 9042,
		9060, 9080, 9090, 9092, 9100, 9200, 9300, 9418, 9443, 9999,
		10000, 10050, 10051, 10250, 11211, 15672, 27017, 27018, 28017, 50000,
	}
)

// Mode names.
const (
	ModeFast   = "fast"
	ModeNormal = "normal"
	ModeFull   = "full"
	ModeCustom = "custom"
)

// ResolvePorts returns the port list for a mode (and optional custom ports string).
func ResolvePorts(mode, portsSpec string) ([]uint16, error) {
	mode = strings.ToLower(strings.TrimSpace(mode))
	if mode == "" {
		mode = ModeFast
	}
	switch mode {
	case ModeFast:
		return append([]uint16(nil), FastPorts...), nil
	case ModeNormal:
		return append([]uint16(nil), NormalPorts...), nil
	case ModeFull:
		out := make([]uint16, 65535)
		for i := 0; i < 65535; i++ {
			out[i] = uint16(i + 1)
		}
		return out, nil
	case ModeCustom:
		portsSpec = strings.TrimSpace(portsSpec)
		if portsSpec == "" {
			return nil, fmt.Errorf("custom mode requires ports")
		}
		return ParsePorts(portsSpec)
	default:
		return nil, fmt.Errorf("unknown scan mode %q (want fast|normal|full|custom)", mode)
	}
}

// ParsePorts parses "22,80,8000-8100" into a sorted unique list.
func ParsePorts(spec string) ([]uint16, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return nil, fmt.Errorf("empty ports")
	}
	seen := make(map[uint16]struct{})
	var out []uint16
	for _, part := range strings.Split(spec, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if strings.Contains(part, "-") {
			a, b, err := parseRange(part)
			if err != nil {
				return nil, err
			}
			if a > b {
				a, b = b, a
			}
			for p := a; p <= b; p++ {
				if _, ok := seen[p]; ok {
					continue
				}
				seen[p] = struct{}{}
				out = append(out, p)
			}
			continue
		}
		p, err := parsePort(part)
		if err != nil {
			return nil, err
		}
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no valid ports in %q", spec)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out, nil
}

func parseRange(s string) (uint16, uint16, error) {
	parts := strings.SplitN(s, "-", 2)
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("invalid port range %q", s)
	}
	a, err := parsePort(strings.TrimSpace(parts[0]))
	if err != nil {
		return 0, 0, err
	}
	b, err := parsePort(strings.TrimSpace(parts[1]))
	if err != nil {
		return 0, 0, err
	}
	return a, b, nil
}

func parsePort(s string) (uint16, error) {
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("invalid port %q", s)
	}
	if n < 1 || n > 65535 {
		return 0, fmt.Errorf("port out of range: %d", n)
	}
	return uint16(n), nil
}
