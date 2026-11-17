package config

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/alecthomas/kong"

	"github.com/cap-theorem/spectral/internal/version"
)

type CollectorCLIConfig struct {
	ListenAddr string `name:"listen-addr" default:":8100" help:"HTTP address for node reports and the API."`

	AnalyzeEvery time.Duration `name:"analyze-every" default:"1s" help:"Interval between analyses of the overlay."`
	StaleAfter   time.Duration `name:"stale-after" default:"90s" help:"Drop nodes that have not reported for this long. Nodes send a keepalive every ~30s."`

	LogLevel  slog.Level `name:"log-level" default:"info" help:"Log level: debug, info, warn, error."`
	LogFormat string     `name:"log-format" default:"text" enum:"text,json" help:"Log format."`

	DrainTimeout time.Duration `name:"drain-timeout" default:"10s" help:"Graceful shutdown timeout."`

	Version kong.VersionFlag `name:"version" help:"Print version and exit."`
}

func ParseCollectorCLI() CollectorCLIConfig {
	var cfg CollectorCLIConfig
	kong.Parse(&cfg, kong.Name("collector"), kong.Vars{"version": version.Version}, kong.Description("Collect spectral overlay topology and compute its spectral gap."))
	return cfg
}

func (c *CollectorCLIConfig) Validate() error {
	if c.AnalyzeEvery <= 0 {
		return fmt.Errorf("--analyze-every must be positive")
	}
	if c.StaleAfter < time.Minute {
		return fmt.Errorf("--stale-after must be at least 1m, or nodes are dropped between keepalives")
	}
	if c.DrainTimeout < 0 {
		return fmt.Errorf("--drain-timeout must not be negative")
	}
	return nil
}
