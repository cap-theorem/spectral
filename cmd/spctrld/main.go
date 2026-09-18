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
	"github.com/cap-theorem/spectral/internal/metrics"
	"github.com/cap-theorem/spectral/internal/node"
	"github.com/golang-cz/devslog"
)

func main() {
	if err := run(); err != nil {
		slog.Error("spctrld stopped", "error", err)
	}
}

func newLogger() *slog.Logger {
	// Makes a logger which can output in text for humans and json for containers.

	slogOptions := &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}

	var handler slog.Handler

	switch os.Getenv("LOG_FORMAT") {
	case "json":
		handler = slog.NewJSONHandler(os.Stdout, slogOptions)
	default:
		handler = devslog.NewHandler(os.Stdout, &devslog.Options{
			HandlerOptions: slogOptions,
			SortKeys:       true,
			DebugColor:     devslog.Blue,
			InfoColor:      devslog.Green,
			WarnColor:      devslog.Yellow,
			ErrorColor:     devslog.Red,
		})
	}

	return slog.New(handler)
}

func run() error {
	signalCtx, stop := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stop()

	apiLogger := newLogger()
	actorLogger := newLogger()
	metricsLogger := newLogger()
	na := node.NewActor(actorLogger.With("component", "actor"))
	api := api.NewAPI(&na, apiLogger.With("component", "api"))

	metricsPipeline, err := metrics.NewPipeline(
		signalCtx,
		metricsLogger.With("component", "metrics"),
	)
	if err != nil {
		return err
	}
	meter := metricsPipeline.Meter("github.com/cap-theorem/spectral")

	starts, err := meter.Int64Counter(
		"spectral.process.starts",
	)
	if err != nil {
		return err
	}
	starts.Add(signalCtx, 1)

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

	metricsErr := metricsPipeline.Shutdown(shutdownCtx)

	return errors.Join(apiErr, shutdownErr, metricsErr)
}
