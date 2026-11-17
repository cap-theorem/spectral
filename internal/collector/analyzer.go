package collector

import (
	"context"
	"log/slog"
	"math/rand/v2"
	"sync/atomic"
	"time"
)

// Gap is λ₂ of the normalized Laplacian on the largest component.
type Summary struct {
	Version    uint64    `json:"version"`
	ComputedAt time.Time `json:"computed_at"`

	Nodes            int `json:"nodes"`
	Edges            int `json:"edges"`
	OneSidedLinks    int `json:"one_sided_links"`
	Components       int `json:"components"`
	LargestComponent int `json:"largest_component"`
	Isolated         int `json:"isolated"`
	Orphaned         int `json:"orphaned"` // not in Nodes

	DegreeMin  int     `json:"degree_min"`
	DegreeMean float64 `json:"degree_mean"`
	DegreeMax  int     `json:"degree_max"`

	Gap          float64 `json:"gap"`
	GapConverged bool    `json:"gap_converged"`

	AnalysisSeconds float64 `json:"analysis_seconds"`
}

type AnalyzerConfig struct {
	Interval time.Duration
	// Must be several times the node keepalive (30s).
	StaleAfter time.Duration
}

type Analyzer struct {
	graph  *Graph
	cfg    AnalyzerConfig
	logger *slog.Logger
	hub    *hub

	latest  atomic.Pointer[Summary]
	prev    view
	epoch   uint64
	recheck time.Time
	version uint64
	warned  bool
}

func NewAnalyzer(cfg AnalyzerConfig, g *Graph, logger *slog.Logger) *Analyzer {
	return &Analyzer{graph: g, cfg: cfg, logger: logger, hub: newHub()}
}

// Latest is nil before the first analysis.
func (a *Analyzer) Latest() *Summary { return a.latest.Load() }

func (a *Analyzer) Run(ctx context.Context) error {
	ticker := time.NewTicker(a.cfg.Interval)
	defer ticker.Stop()

	a.step(time.Now())
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case now := <-ticker.C:
			a.step(now)
		}
	}
}

func (a *Analyzer) step(now time.Time) {
	if n := a.graph.Expire(now, a.cfg.StaleAfter); n > 0 {
		a.logger.Info("expired silent nodes", "count", n)
	}
	if a.version > 0 && a.epoch == a.graph.Epoch() && (a.recheck.IsZero() || now.Before(a.recheck)) {
		return
	}

	f := a.graph.Freeze(now)
	a.epoch, a.recheck = f.Epoch, f.Recheck
	s, v := analyze(f)

	if !s.GapConverged && !a.warned {
		a.logger.Warn("spectral gap did not converge; the reported gap is an overestimate", "nodes", s.LargestComponent)
	}
	a.warned = !s.GapConverged

	if d := diff(a.prev, v); d != nil || a.version == 0 {
		a.version++
		v.snapshot.Version, v.snapshot.ComputedAt = a.version, now
		if d != nil {
			d.Version, d.ComputedAt = a.version, now
		}
		a.prev = v
		a.hub.publish(v.snapshot, d)
	}
	s.Version, s.ComputedAt = a.version, now
	a.latest.Store(&s)
}

// Fixed RNG seed: the same graph always gives the same gap.
func analyze(f Frozen) (Summary, view) {
	start := time.Now()
	g := f.Adj
	labels, sizes := components(g)

	s := Summary{
		Nodes:         g.n(),
		Edges:         len(g.adj) / 2,
		OneSidedLinks: f.OneSided,
		Components:    len(sizes),
		Orphaned:      len(f.Orphans),
		GapConverged:  true,
	}
	if s.Nodes > 0 {
		s.LargestComponent = sizes[0]
		s.DegreeMin = g.degree(0)
		for v := range s.Nodes {
			d := g.degree(v)
			s.DegreeMin = min(s.DegreeMin, d)
			s.DegreeMax = max(s.DegreeMax, d)
			if d == 0 {
				s.Isolated++
			}
		}
		s.DegreeMean = float64(len(g.adj)) / float64(s.Nodes)
		s.Gap, s.GapConverged = normalizedGap(induce(g, members(labels, 0)), rand.New(rand.NewPCG(1, 2)))
	}

	v := newView(f, labels, s.Gap, s.GapConverged)
	s.AnalysisSeconds = time.Since(start).Seconds()
	return s, v
}
