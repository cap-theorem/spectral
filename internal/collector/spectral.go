package collector

import (
	"math"
	"math/rand/v2"
	"slices"
)

// Neighbours of i are adj[off[i]:off[i+1]]. Each edge is in both rows.
type csr struct {
	off []int32
	adj []int32
}

func (g csr) n() int { return len(g.off) - 1 }

func (g csr) degree(i int) int { return int(g.off[i+1] - g.off[i]) }

func (g csr) neighbours(i int) []int32 { return g.adj[g.off[i]:g.off[i+1]] }

// components labels each vertex with its component; 0 is the largest.
func components(g csr) (labels []int32, sizes []int) {
	n := g.n()
	comp := make([]int32, n)
	for i := range comp {
		comp[i] = -1
	}

	var (
		count int32
		found []int
		queue []int32
	)
	for s := range n {
		if comp[s] >= 0 {
			continue
		}
		comp[s] = count
		queue = append(queue[:0], int32(s))
		size := 0
		for len(queue) > 0 {
			v := queue[len(queue)-1]
			queue = queue[:len(queue)-1]
			size++
			for _, w := range g.neighbours(int(v)) {
				if comp[w] < 0 {
					comp[w] = count
					queue = append(queue, w)
				}
			}
		}
		found = append(found, size)
		count++
	}

	order := make([]int32, count)
	for i := range order {
		order[i] = int32(i)
	}
	slices.SortStableFunc(order, func(a, b int32) int { return found[b] - found[a] })
	rank := make([]int32, count)
	sizes = make([]int, count)
	for r, c := range order {
		rank[c] = int32(r)
		sizes[r] = found[c]
	}
	for v := range comp {
		comp[v] = rank[comp[v]]
	}
	return comp, sizes
}

func members(labels []int32, label int32) []int32 {
	var vs []int32
	for v, l := range labels {
		if l == label {
			vs = append(vs, int32(v))
		}
	}
	return vs
}

// keep must be whole components.
func induce(g csr, keep []int32) csr {
	if len(keep) == g.n() {
		return g
	}
	index := make(map[int32]int32, len(keep))
	for i, v := range keep {
		index[v] = int32(i)
	}
	off := make([]int32, len(keep)+1)
	adj := make([]int32, 0, len(keep)*8)
	for i, v := range keep {
		for _, w := range g.neighbours(int(v)) {
			adj = append(adj, index[w])
		}
		off[i+1] = int32(len(adj))
	}
	return csr{off: off, adj: adj}
}

const (
	maxLanczosSteps = 300
	lanczosTol      = 1e-9
)

// normalizedGap returns 1 - μ₂ of D^-1/2 A D^-1/2 (λ₂ of the normalized
// Laplacian). g must be connected. Lanczos runs orthogonal to the known top
// eigenvector √d. converged is false if it hits maxLanczosSteps (clustered
// spectra, e.g. long cycles); the gap is then too high.
func normalizedGap(g csr, rng *rand.Rand) (gap float64, converged bool) {
	n := g.n()
	if n < 2 {
		return 0, true
	}

	invSqrt := make([]float64, n)
	top := make([]float64, n)
	for i := range n {
		d := float64(g.degree(i))
		invSqrt[i] = 1 / math.Sqrt(d)
		top[i] = math.Sqrt(d)
	}
	scale(top, 1/norm(top))

	matvec := func(x, y []float64) {
		for i := range n {
			s := 0.0
			for _, j := range g.neighbours(i) {
				s += invSqrt[j] * x[j]
			}
			y[i] = invSqrt[i] * s
		}
	}

	steps := min(n-1, maxLanczosSteps)
	basis := make([][]float64, 0, steps)
	alpha := make([]float64, 0, steps)
	beta := make([]float64, 0, steps)

	q := make([]float64, n)
	for i := range q {
		q[i] = rng.NormFloat64()
	}
	orthogonalize(q, top)
	scale(q, 1/norm(q))

	prev := math.Inf(-1)
	for k := range steps {
		basis = append(basis, q)
		w := make([]float64, n)
		matvec(q, w)
		alpha = append(alpha, dot(w, q))

		// Twice for numerical stability.
		for range 2 {
			orthogonalize(w, top)
			for _, b := range basis {
				orthogonalize(w, b)
			}
		}

		mu := largestTridiagonal(alpha, beta)
		b := norm(w)
		if b < 1e-12 || k == n-2 || (k >= 10 && math.Abs(mu-prev) < lanczosTol) {
			return 1 - mu, true
		}
		prev = mu
		beta = append(beta, b)
		scale(w, 1/b)
		q = w
	}
	return 1 - largestTridiagonal(alpha, beta[:len(alpha)-1]), false
}

// Sturm-sequence bisection on the tridiagonal matrix (diagonal a, off-diagonal b).
func largestTridiagonal(a, b []float64) float64 {
	k := len(a)
	lo, hi := math.Inf(1), math.Inf(-1)
	for i := range k {
		r := 0.0
		if i > 0 {
			r += math.Abs(b[i-1])
		}
		if i < k-1 {
			r += math.Abs(b[i])
		}
		lo = min(lo, a[i]-r)
		hi = max(hi, a[i]+r)
	}

	// Number of eigenvalues < x.
	below := func(x float64) int {
		count := 0
		d := 1.0
		for i := range k {
			off := 0.0
			if i > 0 {
				off = b[i-1] * b[i-1] / d
			}
			d = a[i] - x - off
			if d == 0 {
				d = -1e-300
			}
			if d < 0 {
				count++
			}
		}
		return count
	}

	for hi-lo > 1e-13*max(1, math.Abs(hi)) {
		mid := (lo + hi) / 2
		if below(mid) == k {
			hi = mid
		} else {
			lo = mid
		}
	}
	return hi
}

func dot(x, y []float64) float64 {
	s := 0.0
	for i := range x {
		s += x[i] * y[i]
	}
	return s
}

func norm(x []float64) float64 { return math.Sqrt(dot(x, x)) }

func scale(x []float64, c float64) {
	for i := range x {
		x[i] *= c
	}
}

// u must be a unit vector.
func orthogonalize(x, u []float64) {
	c := dot(x, u)
	for i := range x {
		x[i] -= c * u[i]
	}
}
