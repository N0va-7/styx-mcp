package node

import (
	"flag"
	"fmt"
)

// Options holds command-line options for the node.
type Options struct {
	Listen      string
	Connect     string
	Secret      string
	Upstream    string
	Downstream  string
	TlsEnable   bool
	Domain      string
	Reconnect   int
	Socks5Proxy string
	Socks5User  string
	Socks5Pass  string
	HTTPProxy   string
}

// ParseOptions parses command-line flags.
func ParseOptions() *Options {
	opt := &Options{}

	flag.StringVar(&opt.Listen, "l", "", "passive mode listen address [ip]:<port>")
	flag.StringVar(&opt.Connect, "c", "", "active mode target address <ip>:<port>")
	flag.StringVar(&opt.Secret, "s", "", "shared secret for node communication")
	flag.StringVar(&opt.Upstream, "up", "raw", "upstream transport: raw only (ws not implemented)")
	flag.StringVar(&opt.Downstream, "down", "raw", "downstream transport: raw only (ws not implemented)")
	flag.BoolVar(&opt.TlsEnable, "tls-enable", false, "enable TLS for node communication")
	flag.StringVar(&opt.Domain, "domain", "", "TLS SNI domain")
	flag.IntVar(&opt.Reconnect, "reconnect", 0, "reconnect interval in seconds (0 to disable)")
	flag.StringVar(&opt.Socks5Proxy, "socks5-proxy", "", "socks5 proxy address")
	flag.StringVar(&opt.Socks5User, "socks5-proxyu", "", "socks5 proxy username")
	flag.StringVar(&opt.Socks5Pass, "socks5-proxyp", "", "socks5 proxy password")
	flag.StringVar(&opt.HTTPProxy, "http-proxy", "", "http proxy address")
	flag.Parse()

	if opt.Listen == "" && opt.Connect == "" {
		fmt.Println("[*] Either -l (listen) or -c (connect) must be specified")
		return nil
	}
	if err := validateTransport(opt.Upstream); err != nil {
		fmt.Println("[*]", err)
		return nil
	}
	if err := validateTransport(opt.Downstream); err != nil {
		fmt.Println("[*]", err)
		return nil
	}

	return opt
}

func validateTransport(name string) error {
	if name == "" || name == "raw" {
		return nil
	}
	if name == "ws" {
		return fmt.Errorf("websocket transport is not implemented; use raw for -up/-down")
	}
	return fmt.Errorf("unknown transport %q (only raw is supported)", name)
}

// Mode returns the connection mode.
func (opt *Options) Mode() string {
	if opt.Connect != "" {
		return "active"
	}
	return "passive"
}
