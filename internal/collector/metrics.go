package collector

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel/metric"
)

type Metrics struct {
	ReportsAccepted metric.Int64Counter
	ReportsStale    metric.Int64Counter
	ReportsRejected metric.Int64Counter
}

func NewMetrics(meter metric.Meter) (Metrics, error) {
	var m Metrics
	var err error
	if m.ReportsAccepted, err = meter.Int64Counter("spectral.collector.reports.accepted",
		metric.WithDescription("Topology reports applied"), metric.WithUnit("{report}")); err != nil {
		return m, err
	}
	if m.ReportsStale, err = meter.Int64Counter("spectral.collector.reports.stale",
		metric.WithDescription("Topology reports older than one already applied"), metric.WithUnit("{report}")); err != nil {
		return m, err
	}
	if m.ReportsRejected, err = meter.Int64Counter("spectral.collector.reports.rejected",
		metric.WithDescription("Topology reports that failed to decode or validate"), metric.WithUnit("{report}")); err != nil {
		return m, err
	}
	return m, nil
}

func RegisterSummaryGauges(meter metric.Meter, a *Analyzer) error {
	ints := []struct {
		name, desc, unit string
		get              func(*Summary) int
	}{
		{"spectral.overlay.nodes", "Nodes reporting to the collector", "{node}", func(s *Summary) int { return s.Nodes }},
		{"spectral.overlay.edges", "Links both ends report", "{link}", func(s *Summary) int { return s.Edges }},
		{"spectral.overlay.links.one_sided", "Links only one end reports", "{link}", func(s *Summary) int { return s.OneSidedLinks }},
		{"spectral.overlay.components", "Connected components", "{component}", func(s *Summary) int { return s.Components }},
		{"spectral.overlay.largest_component", "Nodes in the largest component", "{node}", func(s *Summary) int { return s.LargestComponent }},
		{"spectral.overlay.isolated", "Nodes reporting no links", "{node}", func(s *Summary) int { return s.Isolated }},
		{"spectral.overlay.orphaned", "Nodes left out of the graph as presumed dead", "{node}", func(s *Summary) int { return s.Orphaned }},
		{"spectral.overlay.degree.min", "Smallest node degree", "{link}", func(s *Summary) int { return s.DegreeMin }},
		{"spectral.overlay.degree.max", "Largest node degree", "{link}", func(s *Summary) int { return s.DegreeMax }},
	}
	floats := []struct {
		name, desc, unit string
		get              func(*Summary) float64
	}{
		{"spectral.overlay.degree.mean", "Mean node degree", "{link}", func(s *Summary) float64 { return s.DegreeMean }},
		{"spectral.overlay.gap", "Normalized Laplacian λ₂ of the largest component", "1", func(s *Summary) float64 { return s.Gap }},
		{"spectral.collector.analysis.duration", "Time the last analysis took", "s", func(s *Summary) float64 { return s.AnalysisSeconds }},
	}

	var observables []metric.Observable
	intGauges := make([]metric.Int64ObservableGauge, len(ints))
	for i, g := range ints {
		gauge, err := meter.Int64ObservableGauge(g.name, metric.WithDescription(g.desc), metric.WithUnit(g.unit))
		if err != nil {
			return fmt.Errorf("gauge %s: %w", g.name, err)
		}
		intGauges[i] = gauge
		observables = append(observables, gauge)
	}
	floatGauges := make([]metric.Float64ObservableGauge, len(floats))
	for i, g := range floats {
		gauge, err := meter.Float64ObservableGauge(g.name, metric.WithDescription(g.desc), metric.WithUnit(g.unit))
		if err != nil {
			return fmt.Errorf("gauge %s: %w", g.name, err)
		}
		floatGauges[i] = gauge
		observables = append(observables, gauge)
	}

	_, err := meter.RegisterCallback(func(_ context.Context, o metric.Observer) error {
		s := a.Latest()
		if s == nil {
			return nil
		}
		for i, g := range ints {
			o.ObserveInt64(intGauges[i], int64(g.get(s)))
		}
		for i, g := range floats {
			o.ObserveFloat64(floatGauges[i], g.get(s))
		}
		return nil
	}, observables...)
	return err
}
