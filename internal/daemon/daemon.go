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
	"github.com/cap-theorem/spectral/internal/config"
	"github.com/cap-theorem/spectral/internal/node"
	"github.com/cap-theorem/spectral/internal/transport"
	otlpmetricgrpc "go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/metric"
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
	nodeCtx, stopNode := context.WithCancel(context.Background())

	// exporters for metrics
	exporterCtx, cancelExporter := context.WithTimeout(sigCtx, 5*time.Second)
	exporter, err := otlpmetricgrpc.New(exporterCtx)
	cancelExporter()
	if err != nil {
		return fmt.Errorf("initialize metrics exporter: %w", err)
	}
	provider := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(exporter)),
	)
	meter := provider.Meter("github.com/cap-theorem/spectral/internal/node")
	pongsReceived, err := meter.Int64Counter(
		"spectral.pong.recieved",
		metric.WithDescription("Pongs recieved by this node"),
		metric.WithUnit("{pong}"),
	)
	if err != nil {
		return fmt.Errorf("initalize pongsReceived meter: %w", err)
	}

	node := node.NewNode(
		node.NodeConfig{
			ListenAddr: cfg.ListenAddr,
			Seeds:      cfg.Bootstrap,
			Metrics: node.NodeMetrics{
				PongsReceived: pongsReceived,
			},
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
	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.DrainTimeout)
	defer cancel()

	if err := provider.Shutdown(shutdownCtx); err != nil {
		shutdownErr = errors.Join(shutdownErr, fmt.Errorf("metrics shutdown: %w", err))
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
