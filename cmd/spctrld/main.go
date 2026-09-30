package main

import (
	"log/slog"
	"os"

	"github.com/cap-theorem/spectral/internal/config"
	"github.com/cap-theorem/spectral/internal/daemon"
	"github.com/golang-cz/devslog"
)

func main() {
	cfg := config.ParseDaemonCLI()
	logger := newLogger(cfg.LogFormat, cfg.LogLevel)

	if err := daemon.Run(cfg, logger); err != nil {
		logger.Error("spctrld stopped", "error", err)
		os.Exit(1)
	}
}

func newLogger(format string, level slog.Level) *slog.Logger {
	opts := &slog.HandlerOptions{Level: level}

	switch format {
	case "json":
		return slog.New(slog.NewJSONHandler(os.Stdout, opts))
	default:
		return slog.New(devslog.NewHandler(os.Stdout, &devslog.Options{
			HandlerOptions: opts,
			SortKeys:       true,
		}))
	}
}
