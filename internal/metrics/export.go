package metrics

import (
	"context"
	"fmt"
	"log/slog"

	otelmetric "go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"

	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
)

type Pipeline struct {
	provider *sdkmetric.MeterProvider
}

func NewPipeline(ctx context.Context, logger *slog.Logger) (*Pipeline, error) {
	exporter, err := otlpmetricgrpc.New(ctx)
	if err != nil {
		return nil, fmt.Errorf("create OTLP metric exporter: %w", err)
	}

	reader := sdkmetric.NewPeriodicReader(exporter)

	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))

	return &Pipeline{provider: provider}, nil
}

func (p *Pipeline) Meter(scope string) otelmetric.Meter {
	return p.provider.Meter(scope)
}

func (p *Pipeline) Shutdown(ctx context.Context) error {
	return p.provider.Shutdown(ctx)
}
