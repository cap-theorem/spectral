package daemon

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/cap-theorem/spectral/internal/api"
	"github.com/cap-theorem/spectral/internal/collector"
	"github.com/cap-theorem/spectral/internal/config"
	"github.com/cap-theorem/spectral/internal/node"
	"github.com/cap-theorem/spectral/internal/telemetry"
	"github.com/cap-theorem/spectral/internal/transport"
	otlpmetricgrpc "go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/metric/noop"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"golang.org/x/sync/errgroup"
)

func Run(cfg config.DaemonCLIConfig, logger *slog.Logger) error {
	tr, err := transport.NewUDP(
		transport.UDPConfig{
			ListenAddr: cfg.ListenAddr,
		},
		logger.With("component", "udp"),
	)
	if err != nil {
		logger.Error("failed to initialize udp transport", "err", err)
		return err
	}

	sigCtx, stop := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)

	var meterProvider metric.MeterProvider = noop.NewMeterProvider()
	var sdkProvider *sdkmetric.MeterProvider
	if cfg.OTLPEndpoint != "" {
		exporterCtx, cancelExporter := context.WithTimeout(sigCtx, 5*time.Second)
		exporter, err := otlpmetricgrpc.New(exporterCtx, otlpmetricgrpc.WithEndpointURL(cfg.OTLPEndpoint))
		cancelExporter()
		if err != nil {
			return fmt.Errorf("initialize metrics exporter: %w", err)
		}
		sdkProvider = sdkmetric.NewMeterProvider(
			sdkmetric.WithReader(sdkmetric.NewPeriodicReader(exporter)),
		)
		meterProvider = sdkProvider
	}
	meter := meterProvider.Meter("github.com/cap-theorem/spectral/internal/node")
	pongsReceived, err := meter.Int64Counter(
		"spectral.pong.recieved",
		metric.WithDescription("Pongs recieved by this node"),
		metric.WithUnit("{pong}"),
	)
	if err != nil {
		return fmt.Errorf("initalize pongsReceived meter: %w", err)
	}

	overlay, err := node.ParseOverlay(cfg.Overlay)
	if err != nil {
		return fmt.Errorf("parse overlay %q: %w", cfg.Overlay, err)
	}

	nodeCtx, stopNode := context.WithCancel(context.Background())

	var reports chan collector.Report
	if cfg.CollectorURL != "" {
		reports = make(chan collector.Report, 1)
	}

	node := node.NewNode(
		node.NodeConfig{
			ListenAddr: cfg.ListenAddr,
			Seeds:      cfg.Bootstrap,
			Overlay:    overlay,
			Metrics: node.NodeMetrics{
				PongsReceived: pongsReceived,
			},
			Telemetry: reports,
		},
		tr,
		logger.With("component", "node"),
	)
	api := api.NewAPI(
		api.APIConfig{
			Port: cfg.APIPort,
		},
		node,
		logger.With("component", "api"),
	)

	g, stopping := errgroup.WithContext(sigCtx)

	g.Go(supervise("node", stopping, func() error {
		node.Run(nodeCtx)
		return nil
	}))
	g.Go(supervise("api", stopping, api.Serve))
	if reports != nil {
		exporter := telemetry.NewExporter(
			telemetry.ExporterConfig{CollectorURL: cfg.CollectorURL},
			reports,
			logger.With("component", "telemetry"),
		)
		g.Go(supervise("telemetry", stopping, func() error {
			return exporter.Run(stopping)
		}))
	}

	var shutdownErr error
	g.Go(func() error {
		<-stopping.Done()

		stop()

		ctx, cancel := context.WithTimeout(
			context.Background(),
			cfg.DrainTimeout,
		)
		defer cancel()

		if err := api.Shutdown(ctx); err != nil {
			shutdownErr = errors.Join(
				shutdownErr,
				fmt.Errorf("api shutdown: %w", err),
				api.Close(),
			)
		}

		stopNode()

		if err := tr.Close(); err != nil {
			shutdownErr = errors.Join(
				shutdownErr,
				fmt.Errorf("transport close: %w", err),
			)
		}

		return nil
	})

	runErr := g.Wait()

	if sdkProvider != nil {
		metricsCtx, cancelMetrics := context.WithTimeout(context.Background(), 2*time.Second)
		if err := sdkProvider.Shutdown(metricsCtx); err != nil {
			logger.Warn("final metrics flush failed", "err", err)
		}
		cancelMetrics()
	}

	return errors.Join(runErr, shutdownErr)
}

func supervise(
	name string,
	stopping context.Context,
	run func() error,
) func() error {
	return func() error {
		err := run()

		if stopping.Err() != nil {
			switch {
			case err == nil, errors.Is(err, context.Canceled), errors.Is(err, http.ErrServerClosed):
				return nil
			}
		}

		if err == nil {
			return fmt.Errorf("%s exited unexpectedly", name)
		}

		return fmt.Errorf("%s: %w", name, err)
	}
}
