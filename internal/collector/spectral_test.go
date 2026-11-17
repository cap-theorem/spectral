package collector

import (
	"math"
	"math/rand/v2"
	"slices"
	"testing"
)

func fromEdges(n int, edges [][2]int32) csr {
	rows := make([][]int32, n)
	for _, e := range edges {
		rows[e[0]] = append(rows[e[0]], e[1])
		rows[e[1]] = append(rows[e[1]], e[0])
	}
	off := make([]int32, n+1)
	var adj []int32
	for v, r := range rows {
		slices.Sort(r)
		adj = append(adj, r...)
		off[v+1] = int32(len(adj))
	}
	return csr{off: off, adj: adj}
}

func cycle(n int) csr {
	var e [][2]int32
	for i := range n {
		e = append(e, [2]int32{int32(i), int32((i + 1) % n)})
	}
	return fromEdges(n, e)
}

func complete(n int) csr {
	var e [][2]int32
	for i := range n {
		for j := i + 1; j < n; j++ {
			e = append(e, [2]int32{int32(i), int32(j)})
		}
	}
	return fromEdges(n, e)
}

func hypercube(d int) csr {
	n := 1 << d
	var e [][2]int32
	for v := range n {
		for b := range d {
			if w := v ^ (1 << b); w > v {
				e = append(e, [2]int32{int32(v), int32(w)})
			}
		}
	}
	return fromEdges(n, e)
}

func petersen() csr {
	var e [][2]int32
	for i := range 5 {
		e = append(e,
			[2]int32{int32(i), int32((i + 1) % 5)},     // outer cycle
			[2]int32{int32(i), int32(i + 5)},           // spokes
			[2]int32{int32(i + 5), int32((i+2)%5 + 5)}, // inner pentagram
		)
	}
	return fromEdges(10, e)
}

// Dense Jacobi μ₂, to check Lanczos.
func denseSecond(g csr) float64 {
	n := g.n()
	m := make([][]float64, n)
	for i := range m {
		m[i] = make([]float64, n)
	}
	for i := range n {
		for _, j := range g.neighbours(i) {
			m[i][j] = 1 / math.Sqrt(float64(g.degree(i))*float64(g.degree(int(j))))
		}
	}
	for sweep := 0; sweep < 100; sweep++ {
		off := 0.0
		for p := range n {
			for q := p + 1; q < n; q++ {
				off += m[p][q] * m[p][q]
			}
		}
		if off < 1e-22 {
			break
		}
		for p := range n {
			for q := p + 1; q < n; q++ {
				if math.Abs(m[p][q]) < 1e-300 {
					continue
				}
				theta := (m[q][q] - m[p][p]) / (2 * m[p][q])
				t := math.Copysign(1, theta) / (math.Abs(theta) + math.Sqrt(theta*theta+1))
				c := 1 / math.Sqrt(t*t+1)
				s := t * c
				for k := range n {
					mkp, mkq := m[k][p], m[k][q]
					m[k][p], m[k][q] = c*mkp-s*mkq, s*mkp+c*mkq
				}
				for k := range n {
					mpk, mqk := m[p][k], m[q][k]
					m[p][k], m[q][k] = c*mpk-s*mqk, s*mpk+c*mqk
				}
			}
		}
	}
	eig := make([]float64, n)
	for i := range n {
		eig[i] = m[i][i]
	}
	slices.Sort(eig)
	return eig[n-2]
}

func TestNormalizedGapKnownSpectra(t *testing.T) {
	tests := []struct {
		name string
		g    csr
		mu2  float64
	}{
		{"single edge", complete(2), -1},
		{"triangle", complete(3), -0.5},
		{"K20", complete(20), -1.0 / 19},
		{"C50", cycle(50), math.Cos(2 * math.Pi / 50)},
		{"C400", cycle(400), math.Cos(2 * math.Pi / 400)},
		{"Q6", hypercube(6), 4.0 / 6},
		{"Q10", hypercube(10), 8.0 / 10},
		{"Petersen", petersen(), 1.0 / 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, converged := normalizedGap(tt.g, rand.New(rand.NewPCG(1, 2)))
			if !converged {
				t.Fatal("did not converge")
			}
			if want := 1 - tt.mu2; math.Abs(got-want) > 1e-6 {
				t.Fatalf("gap = %.9f, want %.9f", got, want)
			}
		})
	}
}

func TestNormalizedGapMatchesDense(t *testing.T) {
	rng := rand.New(rand.NewPCG(3, 4))
	for trial := range 20 {
		n := 20 + rng.IntN(100)
		deg := make([]int, n)
		sum := 0
		for i := range deg {
			deg[i] = 2 + rng.IntN(8)
			sum += deg[i]
		}
		if sum%2 == 1 {
			deg[0]++
		}
		g := randomGraph(deg, rng)
		labels, _ := components(g)
		g = induce(g, members(labels, 0))

		got, _ := normalizedGap(g, rng)
		want := 1 - denseSecond(g)
		if math.Abs(got-want) > 1e-6 {
			t.Fatalf("trial %d (n=%d): gap = %.9f, dense = %.9f", trial, g.n(), got, want)
		}
	}
}

func TestNormalizedGapBottleneck(t *testing.T) {
	// Two random 6-regular halves joined by two edges.
	rng := rand.New(rand.NewPCG(9, 10))
	const half = 60
	deg := make([]int, half)
	for i := range deg {
		deg[i] = 6
	}
	a := randomGraph(deg, rng)
	var edges [][2]int32
	for v := range half {
		for _, w := range a.neighbours(v) {
			if int(w) > v {
				edges = append(edges, [2]int32{int32(v), w}, [2]int32{int32(v + half), w + half})
			}
		}
	}
	edges = append(edges, [2]int32{0, half}, [2]int32{1, half + 1})
	g := fromEdges(2*half, edges)

	got, converged := normalizedGap(g, rng)
	want := 1 - denseSecond(g)
	if !converged || math.Abs(got-want) > 1e-6 || got > 0.05 {
		t.Fatalf("gap = %.9f (converged %v), dense = %.9f; want them equal and small", got, converged, want)
	}
}

func TestNormalizedGapReportsNonConvergence(t *testing.T) {
	if _, converged := normalizedGap(cycle(2000), rand.New(rand.NewPCG(1, 2))); converged {
		t.Fatal("C2000 reported convergence within the step limit")
	}
}

func TestComponents(t *testing.T) {
	// A triangle, a path of 4 and an isolated vertex.
	g := fromEdges(8, [][2]int32{{0, 1}, {1, 2}, {2, 0}, {3, 4}, {4, 5}, {5, 6}})
	labels, sizes := components(g)
	largest := members(labels, 0)
	if !slices.Equal(sizes, []int{4, 3, 1}) || !slices.Equal(largest, []int32{3, 4, 5, 6}) || labels[0] != 1 || labels[7] != 2 {
		t.Fatalf("labels %v, sizes %v; want the path as 0, the triangle as 1, the isolated vertex as 2", labels, sizes)
	}
	sub := induce(g, largest)
	if sub.n() != 4 || len(sub.adj) != 6 {
		t.Fatalf("induced path has %d vertices, %d adjacency entries; want 4, 6", sub.n(), len(sub.adj))
	}
}

func TestNormalizedGapRandomRegular(t *testing.T) {
	// Random d-regular graphs are near-Ramanujan: μ₂ ≈ 2√(d-1)/d.
	const n, d = 2000, 8
	deg := make([]int, n)
	for i := range deg {
		deg[i] = d
	}
	rng := rand.New(rand.NewPCG(7, 8))
	g := randomGraph(deg, rng)
	labels, _ := components(g)
	got, _ := normalizedGap(induce(g, members(labels, 0)), rng)
	if want := 1 - 2*math.Sqrt(d-1)/d; math.Abs(got-want) > 0.03 {
		t.Fatalf("gap = %.4f, want about %.4f", got, want)
	}
}

// randomGraph matches stubs at random and drops loops and multi-edges.
func randomGraph(deg []int, rng *rand.Rand) csr {
	var stubs []int32
	for v, d := range deg {
		for range d {
			stubs = append(stubs, int32(v))
		}
	}
	rng.Shuffle(len(stubs), func(i, j int) { stubs[i], stubs[j] = stubs[j], stubs[i] })
	seen := map[[2]int32]bool{}
	var edges [][2]int32
	for i := 0; i+1 < len(stubs); i += 2 {
		a, b := min(stubs[i], stubs[i+1]), max(stubs[i], stubs[i+1])
		if a != b && !seen[[2]int32{a, b}] {
			seen[[2]int32{a, b}] = true
			edges = append(edges, [2]int32{a, b})
		}
	}
	return fromEdges(len(deg), edges)
}
