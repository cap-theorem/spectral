package collector

import (
	"log/slog"
	"math/rand/v2"
	"strings"
	"testing"
	"time"
	"uuid"

	"github.com/cap-theorem/spectral/internal/wire"
)

func report(id wire.NodeID, seq uint64, peers ...wire.NodeID) Report {
	r := Report{ID: id, Seq: seq}
	for _, p := range peers {
		r.Links = append(r.Links, ReportLink{Peer: p})
	}
	return r
}

func TestGraphCountsOnlyMutualLinks(t *testing.T) {
	g := NewGraph()
	now := time.Now()
	a, b, c, d := uuid.NewV4(), uuid.NewV4(), uuid.NewV4(), uuid.NewV4()

	g.Apply(report(a, 1, b, c), now)
	g.Apply(report(b, 1, a), now)
	g.Apply(report(c, 1), now)    // c reports nothing: isolated, and a-c is one-sided
	g.Apply(report(d, 1, a), now) // nobody reciprocates d: orphaned

	f := g.Freeze(now)
	if f.Adj.n() != 3 || len(f.Adj.adj) != 2 || f.OneSided != 1 || isolated(f) != 1 || len(f.Orphans) != 1 {
		t.Fatalf("got %d nodes, %d adjacency entries, %d one-sided, %d isolated, %d orphaned; want 3, 2, 1, 1, 1",
			f.Adj.n(), len(f.Adj.adj), f.OneSided, isolated(f), len(f.Orphans))
	}
}

func TestGraphApplyOrderingAndEpoch(t *testing.T) {
	g := NewGraph()
	now := time.Now()
	a, b := uuid.NewV4(), uuid.NewV4()

	if !g.Apply(report(a, 2, b), now) {
		t.Fatal("first report rejected")
	}
	e := g.Epoch()
	if g.Apply(report(a, 1), now) || g.Apply(report(a, 2), now) {
		t.Fatal("stale report applied")
	}
	if !g.Apply(report(a, 3, b), now) || g.Epoch() != e {
		t.Fatal("keepalive with the same links should apply without changing the epoch")
	}
	if !g.Apply(report(a, 4), now) || g.Epoch() == e {
		t.Fatal("changed links should change the epoch")
	}
	if !g.Apply(Report{ID: a, Seq: 5, Left: true}, now) || g.Freeze(now).Adj.n() != 0 {
		t.Fatal("left node still present")
	}
}

func TestGraphExpire(t *testing.T) {
	g := NewGraph()
	now := time.Now()
	a, b := uuid.NewV4(), uuid.NewV4()
	g.Apply(report(a, 1, b), now)
	g.Apply(report(b, 1, a), now.Add(time.Minute))

	if n := g.Expire(now.Add(90*time.Second), time.Minute); n != 1 {
		t.Fatalf("expired %d, want 1", n)
	}
	if f := g.Freeze(now); f.Adj.n() != 0 || len(f.Orphans) != 1 {
		t.Fatalf("after expiry got %d nodes, %d orphaned; want 0, 1", f.Adj.n(), len(f.Orphans))
	}
}

// Crashed nodes linked to each other must not stay as a phantom component.
func TestFreezeDropsCrashedPair(t *testing.T) {
	g := NewGraph()
	t0 := time.Now()
	a, b, c, d := uuid.NewV4(), uuid.NewV4(), uuid.NewV4(), uuid.NewV4()
	g.Apply(report(a, 1, b, c), t0)
	g.Apply(report(b, 1, a, d), t0)
	g.Apply(report(c, 1, a, d), t0)
	g.Apply(report(d, 1, b, c), t0)

	// c and d crash at t0+1; a and b drop them at t0+3.
	g.Apply(report(a, 2, b), t0.Add(3*time.Second))
	g.Apply(report(b, 2, a), t0.Add(3*time.Second))

	f := g.Freeze(t0.Add(4 * time.Second))
	if f.Adj.n() != 4 || len(f.Orphans) != 0 || f.Recheck != t0.Add(3*time.Second+dropGrace) {
		t.Fatalf("within grace: %d nodes, %d orphaned, recheck %v; want 4, 0, t0+8s", f.Adj.n(), len(f.Orphans), f.Recheck.Sub(t0))
	}
	if _, sizes := components(f.Adj); len(sizes) != 2 {
		t.Fatalf("within grace: %d components, want 2 (the phantom pair still counts)", len(sizes))
	}

	f = g.Freeze(t0.Add(9 * time.Second))
	if f.Adj.n() != 2 || len(f.Orphans) != 2 || len(f.Adj.adj) != 2 || f.OneSided != 0 || !f.Recheck.IsZero() {
		t.Fatalf("after grace: %d nodes, %d orphaned, %d adjacency, %d one-sided, recheck %v; want 2, 2, 2, 0, zero",
			f.Adj.n(), len(f.Orphans), len(f.Adj.adj), f.OneSided, f.Recheck)
	}
}

// A node that reacts within dropGrace is kept.
func TestFreezeKeepsNodeThatReacts(t *testing.T) {
	g := NewGraph()
	t0 := time.Now()
	a, b, c := uuid.NewV4(), uuid.NewV4(), uuid.NewV4()
	g.Apply(report(a, 1, b, c), t0)
	g.Apply(report(b, 1, a, c), t0)
	g.Apply(report(c, 1, a, b), t0)

	g.Apply(report(a, 2, b), t0.Add(time.Second))   // a unlinks c
	g.Apply(report(c, 2, b), t0.Add(4*time.Second)) // c notices late, within grace

	if f := g.Freeze(t0.Add(time.Minute)); f.Adj.n() != 3 || len(f.Orphans) != 0 || f.OneSided != 0 {
		t.Fatalf("%d nodes, %d orphaned, %d one-sided; want 3, 0, 0", f.Adj.n(), len(f.Orphans), f.OneSided)
	}
}

func TestAnalyzerRechecksAfterGrace(t *testing.T) {
	g := NewGraph()
	t0 := time.Now()
	a, b, c, d := uuid.NewV4(), uuid.NewV4(), uuid.NewV4(), uuid.NewV4()
	g.Apply(report(a, 1, b, c), t0)
	g.Apply(report(b, 1, a, d), t0)
	g.Apply(report(c, 1, a, d), t0)
	g.Apply(report(d, 1, b, c), t0)
	g.Apply(report(a, 2, b), t0.Add(time.Second)) // c and d crashed
	g.Apply(report(b, 2, a), t0.Add(time.Second))

	an := NewAnalyzer(AnalyzerConfig{StaleAfter: time.Hour}, g, slog.New(slog.DiscardHandler))
	an.step(t0.Add(2 * time.Second))
	if s := an.Latest(); s.Orphaned != 0 || s.Components != 2 {
		t.Fatalf("before grace: %d orphaned, %d components; want 0, 2", s.Orphaned, s.Components)
	}
	// No new reports; only Recheck triggers this.
	an.step(t0.Add(time.Second + dropGrace + time.Millisecond))
	if s := an.Latest(); s.Orphaned != 2 || s.Nodes != 2 || s.Components != 1 {
		t.Fatalf("after grace: %d orphaned, %d nodes, %d components; want 2, 2, 1", s.Orphaned, s.Nodes, s.Components)
	}
}

// An excluded node that reports again with the same links must come back.
func TestAnalyzerSeesReadmittedNode(t *testing.T) {
	g := NewGraph()
	t0 := time.Unix(100, 0)
	a, b, c, d := uuid.NewV4(), uuid.NewV4(), uuid.NewV4(), uuid.NewV4()
	g.Apply(report(a, 1, b, c), t0)
	g.Apply(report(b, 1, a), t0)
	g.Apply(report(c, 1, a, d), t0)
	g.Apply(report(d, 1, c), t0)
	g.Apply(report(a, 2, b), t0.Add(time.Second)) // a drops c; c stays silent

	an := NewAnalyzer(AnalyzerConfig{StaleAfter: time.Hour}, g, slog.New(slog.DiscardHandler))
	an.step(t0.Add(time.Second + dropGrace + time.Millisecond))
	if s := an.Latest(); s.Orphaned != 2 {
		t.Fatalf("c and d not excluded: %d orphaned", s.Orphaned)
	}

	g.Apply(report(c, 2, a, d), t0.Add(10*time.Second)) // keepalive, same links
	an.step(t0.Add(11 * time.Second))
	if s := an.Latest(); s.Orphaned != 0 || s.Components != 2 {
		t.Fatalf("after c reports: %d orphaned, %d components; want 0, 2", s.Orphaned, s.Components)
	}
}

// Random overlay, each node linked to at least k others.
func overlayReports(n, k int, rng *rand.Rand) []Report {
	ids := make([]wire.NodeID, n)
	for i := range ids {
		ids[i] = uuid.NewV4()
	}
	links := make([]map[int]bool, n)
	for i := range links {
		links[i] = map[int]bool{}
	}
	for u := range n {
		for len(links[u]) < k {
			v := rng.IntN(n)
			if v != u {
				links[u][v] = true
				links[v][u] = true
			}
		}
	}
	reps := make([]Report, n)
	for u := range n {
		reps[u] = Report{ID: ids[u], Seq: 1}
		for v := range links[u] {
			reps[u].Links = append(reps[u].Links, ReportLink{Peer: ids[v]})
		}
	}
	return reps
}

func TestAnalyzerOnOverlay(t *testing.T) {
	g := NewGraph()
	now := time.Now()
	for _, r := range overlayReports(1000, 6, rand.New(rand.NewPCG(1, 1))) {
		g.Apply(r, now)
	}
	a := NewAnalyzer(AnalyzerConfig{StaleAfter: time.Minute}, g, slog.New(slog.DiscardHandler))
	a.step(now)
	s := a.Latest()

	if s.Nodes != 1000 || s.Components != 1 || s.LargestComponent != 1000 || s.Isolated != 0 || s.Orphaned != 0 || s.OneSidedLinks != 0 {
		t.Fatalf("summary = %+v", s)
	}
	if !s.GapConverged || s.Gap <= 0.2 {
		t.Fatalf("gap = %v (converged %v)", s.Gap, s.GapConverged)
	}

	// Unchanged graph: no new analysis.
	a.step(now.Add(time.Second))
	if a.Latest() != s {
		t.Fatal("re-analyzed an unchanged graph")
	}
}

func BenchmarkAnalyze(b *testing.B) {
	for _, n := range []int{1000, 5000} {
		g := NewGraph()
		now := time.Now()
		for _, r := range overlayReports(n, 6, rand.New(rand.NewPCG(1, 1))) {
			g.Apply(r, now)
		}
		f := g.Freeze(now)
		b.Run("analyze/n="+itoa(n), func(b *testing.B) {
			for b.Loop() {
				analyze(f)
			}
		})
		b.Run("freeze/n="+itoa(n), func(b *testing.B) {
			for b.Loop() {
				g.Freeze(now)
			}
		})
	}
}

func itoa(n int) string {
	return string(rune('0'+n/1000)) + "k"
}

func isolated(f Frozen) int {
	n := 0
	for v := range f.Adj.n() {
		if f.Adj.degree(v) == 0 {
			n++
		}
	}
	return n
}

func TestGraphCarriesLabels(t *testing.T) {
	g := NewGraph()
	now := time.Unix(100, 0)
	x, y, z := uuid.NewV4(), uuid.NewV4(), uuid.NewV4()

	rx := report(x, 1, y)
	rx.Service, rx.Instance = "orders", "ord-9869"
	g.Apply(rx, now)
	g.Apply(report(y, 1, x), now)
	rz := report(z, 1, y)
	rz.Service = "payments"
	g.Apply(rz, now)

	epoch := g.Epoch()
	rx.Seq, rx.Instance = 2, "ord-0001"
	g.Apply(rx, now)
	if g.Epoch() == epoch {
		t.Fatal("relabel did not bump the epoch")
	}

	f := g.Freeze(now)
	got := map[wire.NodeID]Label{}
	for i, id := range f.IDs {
		got[id] = f.Labels[i]
	}
	if got[x] != (Label{Service: "orders", Instance: "ord-0001"}) || got[y] != (Label{}) {
		t.Fatalf("labels = %+v", got)
	}

	v := newView(f, make([]int32, len(f.IDs)), 0, true)
	for _, n := range v.snapshot.Nodes {
		if n.ID == x && (n.Service != "orders" || n.Instance != "ord-0001") {
			t.Fatalf("snapshot node = %+v", n)
		}
	}
}

func TestReportRejectsLongLabels(t *testing.T) {
	r := report(uuid.NewV4(), 1)
	r.Service = strings.Repeat("s", maxLabelBytes+1)
	if r.Validate() == nil {
		t.Fatal("accepted an oversized service name")
	}
}
