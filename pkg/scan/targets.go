package scan

import (
	"fmt"
	"net"
	"strings"
)

// Default and hard host caps (full mode still bounded by hosts + wall clock).
const (
	DefaultMaxHosts = 1024
	HardMaxHosts    = 4096
)

// ParseTargets expands IPs, CIDRs, and comma-separated lists into unique IPv4 hosts.
// IPv6 is deferred. Hosts are capped at maxHosts (clamped to HardMaxHosts).
func ParseTargets(spec string, maxHosts int) ([]string, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return nil, fmt.Errorf("empty targets")
	}
	if maxHosts <= 0 {
		maxHosts = DefaultMaxHosts
	}
	if maxHosts > HardMaxHosts {
		maxHosts = HardMaxHosts
	}

	seen := make(map[string]struct{})
	var out []string

	// Also accept whitespace / newline separation.
	spec = strings.ReplaceAll(spec, "\n", ",")
	spec = strings.ReplaceAll(spec, " ", ",")
	for _, part := range strings.Split(spec, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if strings.Contains(part, "/") {
			hosts, err := expandCIDR(part)
			if err != nil {
				return nil, err
			}
			for _, h := range hosts {
				if _, ok := seen[h]; ok {
					continue
				}
				if len(out) >= maxHosts {
					return nil, fmt.Errorf("too many hosts (max %d)", maxHosts)
				}
				seen[h] = struct{}{}
				out = append(out, h)
			}
			continue
		}
		ip := net.ParseIP(part)
		if ip == nil {
			return nil, fmt.Errorf("invalid target %q", part)
		}
		ip4 := ip.To4()
		if ip4 == nil {
			return nil, fmt.Errorf("IPv6 not supported: %q", part)
		}
		h := ip4.String()
		if _, ok := seen[h]; ok {
			continue
		}
		if len(out) >= maxHosts {
			return nil, fmt.Errorf("too many hosts (max %d)", maxHosts)
		}
		seen[h] = struct{}{}
		out = append(out, h)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no valid targets in %q", spec)
	}
	return out, nil
}

func expandCIDR(cidr string) ([]string, error) {
	_, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return nil, fmt.Errorf("invalid CIDR %q: %w", cidr, err)
	}
	if ipNet.IP.To4() == nil {
		return nil, fmt.Errorf("IPv6 CIDR not supported: %q", cidr)
	}
	// Skip network and broadcast for /24 and tighter when mask < 31.
	ones, bits := ipNet.Mask.Size()
	var hosts []string
	for ip := ipNet.IP.Mask(ipNet.Mask); ipNet.Contains(ip); incIP(ip) {
		ip4 := make(net.IP, len(ip))
		copy(ip4, ip)
		// For typical subnets, skip network (.0) and broadcast.
		if ones < 31 && bits == 32 && isNetworkOrBroadcast(ip4, ipNet) {
			continue
		}
		hosts = append(hosts, ip4.String())
		// Safety for huge ranges: caller enforces maxHosts when collecting.
		if len(hosts) > HardMaxHosts+2 {
			break
		}
	}
	return hosts, nil
}

func isNetworkOrBroadcast(ip net.IP, n *net.IPNet) bool {
	ip4 := ip.To4()
	if ip4 == nil {
		return false
	}
	// Network address.
	if ip4.Equal(n.IP.To4()) {
		return true
	}
	// Broadcast: host bits all 1.
	bcast := make(net.IP, 4)
	for i := 0; i < 4; i++ {
		bcast[i] = n.IP.To4()[i] | ^n.Mask[i]
	}
	return ip4.Equal(bcast)
}

func incIP(ip net.IP) {
	for j := len(ip) - 1; j >= 0; j-- {
		ip[j]++
		if ip[j] > 0 {
			break
		}
	}
}
