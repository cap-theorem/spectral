package collector

import (
	"net/netip"
	"slices"
	"time"

	"github.com/cap-theorem/spectral/internal/wire"
)

type NodeState string

const (
	StateActive   NodeState = "active"
	StateIsolated NodeState = "isolated" // reports no links
	StateOrphaned NodeState = "orphaned" // presumed dead; kept until its reports expire
)

type Label struct {
	Service  string
	Instance string
}

type Node struct {
	ID        wire.NodeID    `json:"id"`
	Address   netip.AddrPort `json:"address"`
	Service   string         `json:"service,omitempty"`
	Instance  string         `json:"instance,omitempty"`
	State     NodeState      `json:"state"`
	Component int            `json:"component"` // 0 is the largest; -1 if orphaned
}

type Snapshot struct {
	Version      uint64     `json:"version"`
	ComputedAt   time.Time  `json:"computed_at"`
	Gap          float64    `json:"gap"`
	GapConverged bool       `json:"gap_converged"`
	Nodes        []Node     `json:"nodes"`
	Edges        [][2]int32 `json:"edges"` // indexes into Nodes
}

// Delta turns the snapshot of Version-1 into the snapshot of Version.
type Delta struct {
	Version      uint64           `json:"version"`
	ComputedAt   time.Time        `json:"computed_at"`
	Gap          float64          `json:"gap"`
	GapConverged bool             `json:"gap_converged"`
	Nodes        []Node           `json:"nodes,omitempty"` // added or changed
	Removed      []wire.NodeID    `json:"removed,omitempty"`
	EdgesAdded   [][2]wire.NodeID `json:"edges_added,omitempty"`
	EdgesRemoved [][2]wire.NodeID `json:"edges_removed,omitempty"`
}

type edge [2]wire.NodeID

func edgeOf(a, b wire.NodeID) edge {
	if a.Compare(b) > 0 {
		a, b = b, a
	}
	return edge{a, b}
}

type view struct {
	snapshot *Snapshot
	nodes    map[wire.NodeID]Node
	edges    map[edge]struct{}
}

func newView(f Frozen, labels []int32, gap float64, converged bool) view {
	g := f.Adj
	s := &Snapshot{Gap: gap, GapConverged: converged, Nodes: make([]Node, 0, len(f.IDs)+len(f.Orphans))}
	v := view{snapshot: s, nodes: make(map[wire.NodeID]Node, cap(s.Nodes)), edges: make(map[edge]struct{}, len(g.adj)/2)}

	for i, id := range f.IDs {
		state := StateActive
		if g.degree(i) == 0 {
			state = StateIsolated
		}
		s.Nodes = append(s.Nodes, Node{ID: id, Address: f.Addrs[i], Service: f.Labels[i].Service, Instance: f.Labels[i].Instance, State: state, Component: int(labels[i])})
		for _, j := range g.neighbours(i) {
			if int(j) > i {
				s.Edges = append(s.Edges, [2]int32{int32(i), j})
				v.edges[edgeOf(id, f.IDs[j])] = struct{}{}
			}
		}
	}
	s.Nodes = append(s.Nodes, f.Orphans...)
	for _, n := range s.Nodes {
		v.nodes[n.ID] = n
	}
	return v
}

// diff returns nil if nothing a viewer sees has changed.
func diff(prev, cur view) *Delta {
	d := &Delta{Gap: cur.snapshot.Gap, GapConverged: cur.snapshot.GapConverged}
	for _, n := range cur.snapshot.Nodes {
		if old, ok := prev.nodes[n.ID]; !ok || old != n {
			d.Nodes = append(d.Nodes, n)
		}
	}
	for id := range prev.nodes {
		if _, ok := cur.nodes[id]; !ok {
			d.Removed = append(d.Removed, id)
		}
	}
	for e := range cur.edges {
		if _, ok := prev.edges[e]; !ok {
			d.EdgesAdded = append(d.EdgesAdded, e)
		}
	}
	for e := range prev.edges {
		if _, ok := cur.edges[e]; !ok {
			d.EdgesRemoved = append(d.EdgesRemoved, e)
		}
	}

	if prev.snapshot != nil && d.Gap == prev.snapshot.Gap && d.Nodes == nil && d.Removed == nil && d.EdgesAdded == nil && d.EdgesRemoved == nil {
		return nil
	}
	slices.SortFunc(d.Removed, func(a, b wire.NodeID) int { return a.Compare(b) })
	return d
}
