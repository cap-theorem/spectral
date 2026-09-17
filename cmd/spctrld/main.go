package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/cap-theorem/spectral/internal/api"
	"github.com/cap-theorem/spectral/internal/node"
)

func main() {
	if err := run(); err != nil {
		slog.Error("spctrld stopped", "error", err)
	}
}

func run() error {
	signalCtx, stop := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stop()

	apiLogger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{}))
	actorLogger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{}))
	slog.SetDefault(apiLogger)

	na := node.NewActor(actorLogger.With("component", "actor"))
	api := api.NewAPI(&na, apiLogger.With("component", "api"))

	apiErrors := make(chan error, 1)

	go func() {
		apiErrors <- api.Serve()
	}()

	var apiErr error
	serveExited := false

	select {
	case <-signalCtx.Done():
	case apiErr = <-apiErrors:
		serveExited = true
	}

	stop()

	shutdownCtx, cancel := context.WithTimeout(
		context.Background(),
		10*time.Second,
	)
	defer cancel()

	shutdownErr := api.Shutdown(shutdownCtx)
	if shutdownErr != nil {
		shutdownErr = errors.Join(shutdownErr, api.Close())
	}

	if !serveExited {
		apiErr = <-apiErrors
	}

	if errors.Is(apiErr, http.ErrServerClosed) {
		apiErr = nil
	}

	return errors.Join(apiErr, shutdownErr)
}
