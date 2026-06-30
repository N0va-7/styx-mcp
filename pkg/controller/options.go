package controller

import (
	"flag"
	"fmt"
)

// Options holds command-line options for the controller.
type Options struct {
	Listen     string
	Connect    string
	Secret     string
	Downstream string
	TlsEnable  bool
	Domain     string
	Heartbeat  bool
}

// ParseOptions parses command-line flags.
func ParseOptions() *Options {
	opt := &Options{}

	flag.StringVar(&opt.Listen, "l", "", "passive mode listen address [ip]:<port>")
	flag.StringVar(&opt.Connect, "c", "", "active mode target address <ip>:<port>")
	flag.StringVar(&opt.Secret, "s", "", "shared secret for node communication")
	flag.StringVar(&opt.Downstream, "down", "raw", "downstream transport type: raw/ws")
	flag.BoolVar(&opt.TlsEnable, "tls-enable", false, "enable TLS for node communication")
	flag.StringVar(&opt.Domain, "domain", "", "TLS SNI or WebSocket domain")
	flag.BoolVar(&opt.Heartbeat, "heartbeat", false, "enable heartbeat to first node")
	flag.Parse()

	if opt.Listen == "" && opt.Connect == "" {
		fmt.Println("[*] Either -l (listen) or -c (connect) must be specified")
		return nil
	}

	return opt
}

// Mode returns the connection mode.
func (opt *Options) Mode() string {
	if opt.Connect != "" {
		return "active"
	}
	return "passive"
}
