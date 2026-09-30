package config

import (
	"fmt"
	"log/slog"
	"net"
	"net/netip"
	"time"

	"github.com/alecthomas/kong"
)

type DaemonCLIConfig struct {
	APIPort    int            `name:"api-port" help:"Port to host local HTTP API on." default:"8000"`
	ListenAddr netip.AddrPort `name:"listen-addr" help:"QUIC listen IP and port." required:""`

	TLSCert string `name:"tls-cert" type:"existingfile" help:"Node certificate file."`
	TLSKey  string `name:"tls-key" type:"existingfile" help:"Node private key file."`
	TLSCA   string `name:"tls-ca" type:"existingfile" help:"Trusted CA bundle."`

	Bootstrap []string `name:"bootstrap" sep:"none" help:"Existing peer host:port to contact when joining. May be repeated."`

	LogLevel  slog.Level `name:"log-level" default:"info" help:"Log level: debug, info, warn, error."`
	LogFormat string     `name:"log-format" default:"text" enum:"text,json" help:"Log format."`

	DrainTimeout time.Duration `name:"drain-timeout" default:"30s" help:"Graceful shutdown timeout."`
}

func ParseDaemonCLI() DaemonCLIConfig {
	var cfg DaemonCLIConfig
	kong.Parse(&cfg, kong.Name("spectral"), kong.Description("Run a spectral node."))
	return cfg
}

func (d *DaemonCLIConfig) Validate() error {
	if d.APIPort < 1 || d.APIPort > 65535 {
		return fmt.Errorf("--api-port must be between 1-65535; got %d", d.APIPort)
	}

	if !d.ListenAddr.IsValid() {
		return fmt.Errorf("--listen-addr must contain a valid IP and port (i.e 127.0.0.1:8080)")
	}

	if d.DrainTimeout < 0 {
		return fmt.Errorf("--drain-timeout must be a positive value")
	}

	for _, addr := range d.Bootstrap {
		host, port, err := net.SplitHostPort(addr)
		if err != nil || host == "" || port == "" {
			return fmt.Errorf("--bootstrap address must have valid host and port.")
		}
	}

	return nil
}
