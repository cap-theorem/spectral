package collector

import (
	"maps"
	"net/netip"
	"slices"
	"sync"
	"time"

	"github.com/cap-theorem/spectral/internal/wire"
)

// A live node reports a lost link within ~3s (2.5s link timeout + 0.5s poll).
const dropGrace = 5 * time.Second

type member struct {
	address  netip.AddrPort
	service  string
	instance string
	seq      uint64
	links    map[wire.NodeID]bool // peer -> Out
	seen     time.Time
	dropped  map[wire.NodeID]time.Time // peer -> when reports stopped listing it
}

type Graph struct {
	mu      sync.Mutex
	members map[wire.NodeID]*member
	epoch   uint64
}

func NewGraph() *Graph {
	return &Graph{members: make(map[wire.NodeID]*member)}
}

// Apply returns false for a stale report.
func (g *Graph) Apply(r Report, now time.Time) bool {
	g.mu.Lock()
	defer g.mu.Unlock()

	m, known := g.members[r.ID]
	if known && r.Seq <= m.seq {
		return false
	}

	if r.Left {
		if known {
			delete(g.members, r.ID)
			g.epoch++
		}
		return true
	}

	links := make(map[wire.NodeID]bool, len(r.Links))
	for _, l := range r.Links {
		links[l.Peer] = l.Out
	}

	if !known {
		m = &member{dropped: make(map[wire.NodeID]time.Time)}
		g.members[r.ID] = m
	}
	relabeled := m.service != r.Service || m.instance != r.Instance
	if !known || m.address != r.Address || relabeled || !maps.Equal(m.links, links) || g.readmits(r.ID, m.seen, links) {
		g.epoch++
	}
	for peer := range m.links {
		if _, kept := links[peer]; !kept {
			m.dropped[peer] = now
		}
	}
	for peer := range links {
		delete(m.dropped, peer)
	}
	m.address = r.Address
	m.service = r.Service
	m.instance = r.Instance
	m.seq = r.Seq
	m.links = links
	m.seen = now
	return true
}

func (g *Graph) Expire(now time.Time, after time.Duration) int {
	g.mu.Lock()
	defer g.mu.Unlock()

	n := 0
	for id, m := range g.members {
		if now.Sub(m.seen) > after {
			delete(g.members, id)
			n++
			continue
		}
		for peer, at := range m.dropped {
			if now.Sub(at) > after {
				delete(m.dropped, peer)
			}
		}
	}
	if n > 0 {
		g.epoch++
	}
	return n
}

// readmits reports whether a peer dropped the node after its last report,
// which Freeze may have excluded it for.
func (g *Graph) readmits(id wire.NodeID, seen time.Time, links map[wire.NodeID]bool) bool {
	for peer := range links {
		if pm, ok := g.members[peer]; ok {
			if at, dropped := pm.dropped[id]; dropped && seen.Before(at) {
				return true
			}
		}
	}
	return false
}

func (g *Graph) Epoch() uint64 {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.epoch
}

// Frozen holds an edge only if both ends report it.
type Frozen struct {
	Epoch    uint64
	IDs      []wire.NodeID
	Addrs    []netip.AddrPort
	Labels   []Label
	Adj      csr
	Orphans  []Node    // presumed dead, left out of Adj
	OneSided int       // links only one end reports
	Recheck  time.Time // when a pending dropGrace decision is due; zero if none
}

// Freeze leaves out nodes presumed dead, so crashed nodes linked to each
// other do not form a phantom component until they expire:
//   - a peer dropped the node over dropGrace ago and it has not reported since
//   - repeatedly: the node reports links but none are reciprocated
func (g *Graph) Freeze(now time.Time) Frozen {
	g.mu.Lock()
	defer g.mu.Unlock()

	f := Frozen{Epoch: g.epoch}
	out := make(map[wire.NodeID]bool)

	for id, m := range g.members {
		for peer := range m.links {
			pm, ok := g.members[peer]
			if !ok {
				continue
			}
			at, dropped := pm.dropped[id]
			if !dropped || !m.seen.Before(at) {
				continue
			}
			if due := at.Add(dropGrace); now.Before(due) {
				if f.Recheck.IsZero() || due.Before(f.Recheck) {
					f.Recheck = due
				}
				continue
			}
			out[id] = true
			break
		}
	}

	mutual := func(id, peer wire.NodeID) bool {
		pm, ok := g.members[peer]
		if !ok || out[peer] {
			return false
		}
		_, back := pm.links[id]
		return back
	}
	for changed := true; changed; {
		changed = false
		for id, m := range g.members {
			if out[id] || len(m.links) == 0 {
				continue
			}
			reciprocated := false
			for peer := range m.links {
				if mutual(id, peer) {
					reciprocated = true
					break
				}
			}
			if !reciprocated {
				out[id] = true
				changed = true
			}
		}
	}
	ids := make([]wire.NodeID, 0, len(g.members)-len(out))
	for id, m := range g.members {
		if out[id] {
			f.Orphans = append(f.Orphans, Node{ID: id, Address: m.address, Service: m.service, Instance: m.instance, State: StateOrphaned, Component: -1})
		} else {
			ids = append(ids, id)
		}
	}
	slices.SortFunc(ids, func(a, b wire.NodeID) int { return a.Compare(b) })
	slices.SortFunc(f.Orphans, func(a, b Node) int { return a.ID.Compare(b.ID) })
	index := make(map[wire.NodeID]int32, len(ids))
	for i, id := range ids {
		index[id] = int32(i)
	}

	f.Addrs = make([]netip.AddrPort, len(ids))
	f.Labels = make([]Label, len(ids))
	off := make([]int32, len(ids)+1)
	adj := make([]int32, 0, len(ids)*8)
	for i, id := range ids {
		f.Addrs[i] = g.members[id].address
		f.Labels[i] = Label{Service: g.members[id].service, Instance: g.members[id].instance}
		start := len(adj)
		links := g.members[id].links
		for peer := range links {
			if mutual(id, peer) {
				adj = append(adj, index[peer])
			}
		}
		f.OneSided += len(links) - (len(adj) - start)
		slices.Sort(adj[start:])
		off[i+1] = int32(len(adj))
	}
	f.IDs = ids
	f.Adj = csr{off: off, adj: adj}
	return f
}
