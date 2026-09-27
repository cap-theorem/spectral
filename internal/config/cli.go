package config

import "github.com/alecthomas/kong"

type DaemonCLIConfig struct {
	Port int `help:"Port to host local HTTP API on." default:"8000"`
}

func ParseDaemonCLI() DaemonCLIConfig {
	var cfg DaemonCLIConfig
	kong.Parse(&cfg)
	return cfg
}
