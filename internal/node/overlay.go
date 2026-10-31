package node

import (
	"cmp"
	"crypto/sha256"
	"encoding/binary"
	"slices"
	"time"

	"github.com/cap-theorem/spectral/internal/wire"
)

const K = 6

type link struct {
	peer     wire.Peer
	lastRevc time.Time
}

type overlay struct {
	self wire.NodeID

	want  map[wire.NodeID]wire.Peer
	links map[wire.NodeID]link
	avoid map[wire.NodeID]time.Time
}

func newOverlay(self wire.NodeID) overlay {
	return overlay{
		self:  self,
		want:  make(map[wire.NodeID]wire.Peer),
		links: make(map[wire.NodeID]link),
		avoid: make(map[wire.NodeID]time.Time),
	}
}

type candidate struct {
	rank uint64
	peer wire.Peer
}

func (o *overlay) recompute(live []wire.Record, now time.Time) {
	var cands []candidate
	for _, rec := range live {
		id := rec.Peer.ID
		if id == o.self {
			continue
		}
		if until, ok := o.avoid[id]; ok && now.Before(until) {
			continue
		}
		if _, linked := o.links[id]; rec.Status == wire.StatusSuspect && !linked {
			continue
		}

		cands = append(cands, candidate{rank: rank(o.self, id), peer: rec.Peer})
	}

	slices.SortFunc(cands, func(a, b candidate) int {
		return cmp.Compare(b.rank, a.rank)
	})

	o.want = make(map[wire.NodeID]wire.Peer, K)
	for _, c := range cands[:min(K, len(cands))] {
		o.want[c.peer.ID] = c.peer
	}
}

func rank(u, v wire.NodeID) uint64 {
	sum := sha256.Sum256(append(u[:], v[:]...))
	return binary.BigEndian.Uint64(sum[:8])
}
