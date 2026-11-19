package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/cap-theorem/spectral/internal/collector"
	"github.com/cap-theorem/spectral/internal/config"
	"github.com/golang-cz/devslog"
	otlpmetricgrpc "go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"golang.org/x/sync/errgroup"
)

func main() {
	cfg := config.ParseCollectorCLI()
	logger := newLogger(cfg.LogFormat, cfg.LogLevel)

	if err := run(cfg, logger); err != nil {
		logger.Error("collector stopped", "error", err)
		os.Exit(1)
	}
}

func run(cfg config.CollectorCLIConfig, logger *slog.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Configured by OTEL_EXPORTER_OTLP_* env vars, same as spctrld.
	exporterCtx, cancelExporter := context.WithTimeout(ctx, 5*time.Second)
	exporter, err := otlpmetricgrpc.New(exporterCtx)
	cancelExporter()
	if err != nil {
		return fmt.Errorf("initialize metrics exporter: %w", err)
	}
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(sdkmetric.NewPeriodicReader(exporter)))
	meter := provider.Meter("github.com/cap-theorem/spectral/internal/collector")

	graph := collector.NewGraph()
	analyzer := collector.NewAnalyzer(collector.AnalyzerConfig{
		Interval:   cfg.AnalyzeEvery,
		StaleAfter: cfg.StaleAfter,
	}, graph, logger.With("component", "analyzer"))

	metrics, err := collector.NewMetrics(meter)
	if err != nil {
		return fmt.Errorf("initialize collector metrics: %w", err)
	}
	if err := collector.RegisterSummaryGauges(meter, analyzer); err != nil {
		return fmt.Errorf("initialize summary gauges: %w", err)
	}

	server := collector.NewServer(collector.ServerConfig{ListenAddr: cfg.ListenAddr}, graph, analyzer, metrics, logger.With("component", "server"))

	g, gctx := errgroup.WithContext(ctx)
	g.Go(server.Serve)
	g.Go(func() error {
		if err := server.RunFlows(gctx); !errors.Is(err, context.Canceled) {
			return err
		}
		return nil
	})
	g.Go(func() error {
		if err := analyzer.Run(gctx); !errors.Is(err, context.Canceled) {
			return err
		}
		return nil
	})
	g.Go(func() error {
		<-gctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.DrainTimeout)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			return errors.Join(fmt.Errorf("server shutdown: %w", err), server.Close())
		}
		return nil
	})

	runErr := g.Wait()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.DrainTimeout)
	defer cancel()
	if err := provider.Shutdown(shutdownCtx); err != nil {
		runErr = errors.Join(runErr, fmt.Errorf("metrics shutdown: %w", err))
	}
	return runErr
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
