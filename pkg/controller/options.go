package controller

import (
	"flag"
	"fmt"
)

// Options holds command-line options for the controller.
type Options struct {
	Listen       string
	Connect      string
	Secret       string
	Downstream   string
	TlsEnable    bool
	Domain       string
	Heartbeat    bool
	ReconnectMax int // max attempts for active (-c) dial; 0 = single try only
}

// ParseOptions parses command-line flags.
func ParseOptions() *Options {
	opt := &Options{}

	flag.StringVar(&opt.Listen, "l", "", "passive mode listen address [ip]:<port>")
	flag.StringVar(&opt.Connect, "c", "", "active mode target address <ip>:<port>")
	flag.StringVar(&opt.Secret, "s", "", "shared secret for node communication")
	flag.StringVar(&opt.Downstream, "down", "raw", "downstream transport: raw only (ws not implemented)")
	flag.BoolVar(&opt.TlsEnable, "tls-enable", false, "enable TLS for node communication")
	flag.StringVar(&opt.Domain, "domain", "", "TLS SNI domain")
	flag.BoolVar(&opt.Heartbeat, "heartbeat", false, "enable heartbeat to first node")
	flag.IntVar(&opt.ReconnectMax, "reconnect-max", 3, "max active dial attempts (0 = single try only)")
	flag.Parse()

	if opt.Listen == "" && opt.Connect == "" {
		fmt.Println("[*] Either -l (listen) or -c (connect) must be specified")
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
		return fmt.Errorf("websocket transport (-down ws) is not implemented; use raw")
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
