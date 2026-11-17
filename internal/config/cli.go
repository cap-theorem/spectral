package config

import (
	"fmt"
	"log/slog"
	"net"
	"net/netip"
	"net/url"
	"time"

	"github.com/alecthomas/kong"

	"github.com/cap-theorem/spectral/internal/version"
)

type DaemonCLIConfig struct {
	APIPort int `name:"api-port" help:"Port to host local HTTP API on." default:"8000"`
	// TODO: add advertise addr when implementing support for nodes behind NATs
	ListenAddr netip.AddrPort `name:"listen-addr" help:"QUIC listen IP and port." required:""`

	TLSCert string `name:"tls-cert" type:"existingfile" help:"Node certificate file."`
	TLSKey  string `name:"tls-key" type:"existingfile" help:"Node private key file."`
	TLSCA   string `name:"tls-ca" type:"existingfile" help:"Trusted CA bundle."`

	Bootstrap []string `name:"bootstrap" sep:"none" help:"Existing peer host:port to contact when joining. May be repeated."`
	Overlay   string   `name:"overlay" default:"hrw" enum:"hrw,coin" help:"Overlay link rule: hrw or coin."`

	CollectorURL string `name:"collector-url" help:"Topology collector base URL (e.g. http://collector:8100). Unset disables topology reports."`
	OTLPEndpoint string `name:"otlp-endpoint" env:"OTEL_EXPORTER_OTLP_ENDPOINT" help:"OTLP gRPC endpoint for metrics (e.g. http://otel-collector:4317). Unset disables metrics export."`

	LogLevel  slog.Level `name:"log-level" default:"info" help:"Log level: debug, info, warn, error."`
	LogFormat string     `name:"log-format" default:"text" enum:"text,json" help:"Log format."`

	DrainTimeout time.Duration `name:"drain-timeout" default:"30s" help:"Graceful shutdown timeout."`

	Version kong.VersionFlag `name:"version" help:"Print version and exit."`
}

func ParseDaemonCLI() DaemonCLIConfig {
	var cfg DaemonCLIConfig
	kong.Parse(&cfg, kong.Name("spectral"), kong.Vars{"version": version.Version}, kong.Description("Run a spectral node."))
	return cfg
}

func (d *DaemonCLIConfig) Validate() error {
	if d.APIPort < 1 || d.APIPort > 65535 {
		return fmt.Errorf("--api-port must be between 1-65535; got %d", d.APIPort)
	}

	if !d.ListenAddr.IsValid() {
		return fmt.Errorf("--listen-addr must contain a valid IP and port (i.e 127.0.0.1:8000)")
	}

	if d.ListenAddr.Addr().IsUnspecified() || d.ListenAddr.Port() == 0 {
		return fmt.Errorf("--listen-addr must be a specific IP and port that peers can dial (not 0.0.0.0, :: or port 0)")
	}

	if d.DrainTimeout < 0 {
		return fmt.Errorf("--drain-timeout must be a positive value")
	}

	if d.CollectorURL != "" {
		u, err := url.Parse(d.CollectorURL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return fmt.Errorf("--collector-url must be an http(s) URL with a host")
		}
	}

	if d.OTLPEndpoint != "" {
		u, err := url.Parse(d.OTLPEndpoint)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return fmt.Errorf("--otlp-endpoint must be an http(s) URL with a host")
		}
	}

	for _, addr := range d.Bootstrap {
		host, port, err := net.SplitHostPort(addr)
		if err != nil || host == "" || port == "" {
			return fmt.Errorf("--bootstrap address must have valid host and port.")
		}
	}

	return nil
}
