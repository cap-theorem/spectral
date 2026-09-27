package main

import (
	"log/slog"
	"os"

	"github.com/cap-theorem/spectral/internal/config"
	"github.com/cap-theorem/spectral/internal/daemon"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("spctrld couldn't init", "error", err)
		os.Exit(1)
	}

	if err = daemon.Run(cfg); err != nil {
		slog.Error("spctrld stopped", "error", err)
	}
}
