// Package metrics implements OpenTelemetry pipelines, so that we can we can observe the state of our system inside of dashboards.
// We will support the collection and aggregation of logs, metrics, and traces.
// A "Pipeline" as depicted here may be an unfamiliar concept. If this is true to you, then please read about OpenTelemetry's architecture: https://opentelemetry.io/docs/collector/architecture/
package metrics

import (
	"context"
	"fmt"
	"log/slog"
	"time"

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

	reader := sdkmetric.NewPeriodicReader(
		exporter,
		sdkmetric.WithInterval(5*time.Second), // TODO: turn into envar
	)

	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))

	return &Pipeline{provider: provider}, nil
}

func (p *Pipeline) Meter(scope string) otelmetric.Meter {
	return p.provider.Meter(scope)
}

func (p *Pipeline) Shutdown(ctx context.Context) error {
	return p.provider.Shutdown(ctx)
}
