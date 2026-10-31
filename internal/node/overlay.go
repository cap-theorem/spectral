package node

import (
	"cmp"
	"crypto/sha256"
	"encoding/binary"
	"slices"
	"time"

	"github.com/cap-theorem/spectral/internal/wire"
)

const (
	K = 6

	heartbeatInterval  = 500 * time.Millisecond
	linkTimeout        = 2500 * time.Millisecond
	linkRequestTimeout = time.Second
	avoidDuration      = 30 * time.Second
)

type link struct {
	peer     wire.Peer
	since    time.Time
	lastRecv time.Time
	lastSent time.Time
	theirs   bool
}

type linkRequest struct {
	peer     wire.Peer
	deadline time.Time
	retried  bool
}

type overlay struct {
	self wire.NodeID

	want    map[wire.NodeID]wire.Peer
	links   map[wire.NodeID]link
	pending map[wire.NodeID]linkRequest
	avoid   map[wire.NodeID]time.Time

	dirty bool
}

func newOverlay(self wire.NodeID) overlay {
	return overlay{
		self:    self,
		want:    make(map[wire.NodeID]wire.Peer),
		links:   make(map[wire.NodeID]link),
		pending: make(map[wire.NodeID]linkRequest),
		avoid:   make(map[wire.NodeID]time.Time),
	}
}

func (o *overlay) addLink(peer wire.Peer, theirs bool, now time.Time) {
	l, ok := o.links[peer.ID]
	if !ok {
		l = link{peer: peer, since: now}
	}
	l.lastRecv = now
	l.theirs = l.theirs || theirs
	o.links[peer.ID] = l
	delete(o.pending, peer.ID)
}

func (o *overlay) heard(id wire.NodeID, now time.Time) {
	if l, ok := o.links[id]; ok {
		l.lastRecv = now
		o.links[id] = l
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

func (o *overlay) expireAvoids(now time.Time) bool {
	expired := false
	for id, until := range o.avoid {
		if !now.Before(until) {
			delete(o.avoid, id)
			expired = true
		}
	}
	return expired
}

func (n *Node) avoidPeer(id wire.NodeID, now time.Time) {
	if _, wanted := n.ov.want[id]; !wanted {
		return
	}
	n.ov.avoid[id] = now.Add(avoidDuration)
	n.ov.recompute(n.reg.live(), now)
}

func (n *Node) overlayTick(now time.Time) {
	if n.ov.expireAvoids(now) || n.ov.dirty {
		n.ov.recompute(n.reg.live(), now)
		n.ov.dirty = false
	}

	for id, l := range n.ov.links {
		if now.Sub(l.lastRecv) > linkTimeout {
			delete(n.ov.links, id)
			n.avoidPeer(id, now)
			continue
		}

		_, wanted := n.ov.want[id]
		if !wanted && !l.theirs && now.Sub(l.since) > 2*heartbeatInterval {
			delete(n.ov.links, id)
			n.send(l.peer, wire.Unlink{})
			continue
		}

		if now.Sub(l.lastSent) >= heartbeatInterval {
			n.send(l.peer, wire.Heartbeat{Want: wanted})
			l.lastSent = now
			n.ov.links[id] = l
		}
	}

	for id, req := range n.ov.pending {
		if now.Before(req.deadline) {
			continue
		}

		if _, wanted := n.ov.want[id]; wanted && !req.retried {
			req.retried = true
			req.deadline = now.Add(linkRequestTimeout)
			n.ov.pending[id] = req
			n.send(req.peer, wire.Link{})
			continue
		}

		delete(n.ov.pending, id)
		n.avoidPeer(id, now)
	}

	for id, peer := range n.ov.want {
		_, linked := n.ov.links[id]
		_, asked := n.ov.pending[id]
		if !linked && !asked {
			n.ov.pending[id] = linkRequest{peer: peer, deadline: now.Add(linkRequestTimeout)}
			n.send(peer, wire.Link{})
		}
	}
}

func rank(u, v wire.NodeID) uint64 {
	sum := sha256.Sum256(append(u[:], v[:]...))
	return binary.BigEndian.Uint64(sum[:8])
}
