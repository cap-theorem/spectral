package main

import (
	"log/slog"

	"github.com/cap-theorem/spectral/internal/config"
	"github.com/cap-theorem/spectral/internal/daemon"
)

func main() {
	cfg := config.ParseDaemonCLI()

	if err := daemon.Run(cfg); err != nil {
		slog.Error("spctrld stopped", "error", err)
	}
}
