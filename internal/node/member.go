package node

import (
	"net/netip"
	"time"

	"github.com/cap-theorem/spectral/internal/wire"
)

const (
	antiEntropyInterval = 2 * time.Second
	seedPullInterval    = 2 * time.Second
	probeTimeout        = 500 * time.Millisecond
	probeFanout         = 3
	suspectTimeout      = 4 * time.Second
	tombstoneTTL        = 60 * time.Second
	maxUpdateRecords    = 30
)

type queued struct {
	rec  wire.Record
	from wire.NodeID
}

type probe struct {
	target   wire.Peer
	deadline time.Time
}

type relay struct {
	requester wire.Peer
	target    wire.NodeID
	nonce     uint64
	deadline  time.Time
}

type membership struct {
	seeds        []netip.AddrPort
	seedSynced   bool
	nextSeedPull time.Time
	nextAE       time.Time

	outbox []queued
	probes map[uint64]probe
	relays map[uint64]relay
}

func newMembership(seeds []netip.AddrPort) membership {
	return membership{
		seeds:  seeds,
		probes: make(map[uint64]probe),
		relays: make(map[uint64]relay),
	}
}

func (n *Node) apply(rec wire.Record, from wire.NodeID, now time.Time) bool {
	if rec.Peer.ID == n.self.ID && from != n.self.ID {
		n.refute(rec, now)
		return false
	}

	old, known := n.reg.records[rec.Peer.ID]
	if from != n.self.ID && rec.Status == wire.StatusDead && known && isLive(old.Status) {
		rec.Status = wire.StatusSuspect
	}

	if !n.reg.merge(rec, now) {
		return false
	}

	if !known || old.Status != rec.Status {
		n.ov.dirty = true
	}
	n.mem.outbox = append(n.mem.outbox, queued{rec: rec, from: from})

	return true
}

func (n *Node) refute(rec wire.Record, now time.Time) {
	own := n.reg.records[n.self.ID].Record
	if rec.Status == wire.StatusAlive || rec.Inc < own.Inc {
		return
	}

	own.Inc = rec.Inc + 1
	own.Status = wire.StatusAlive
	n.apply(own, n.self.ID, now)
}

func isLive(st wire.Status) bool {
	return st == wire.StatusAlive || st == wire.StatusSuspect
}

func (n *Node) flush() {
	if len(n.mem.outbox) == 0 {
		return
	}

	for id, l := range n.ov.links {
		var batch []wire.Record
		for _, q := range n.mem.outbox {
			if q.from == id {
				continue
			}
			batch = append(batch, q.rec)
			if len(batch) == maxUpdateRecords {
				n.send(l.peer, wire.Update{Records: batch})
				batch = nil
			}
		}
		if len(batch) > 0 {
			n.send(l.peer, wire.Update{Records: batch})
		}
	}

	n.mem.outbox = n.mem.outbox[:0]
}

func (n *Node) handleUpdate(from wire.Peer, msg wire.Update, now time.Time) {
	for _, rec := range msg.Records {
		n.apply(rec, from.ID, now)
	}
}

func (n *Node) sendDigest(to wire.Peer) {}

func (n *Node) handleAEDigest(from wire.Peer, msg wire.AEDigest) {}

func (n *Node) handleAEHeads(from wire.Peer, msg wire.AEHeads, now time.Time) {}

func (n *Node) handleAEWant(from wire.Peer, msg wire.AEWant, now time.Time) {}

func (n *Node) startProbe(target wire.Peer, now time.Time) {}

func (n *Node) handlePingReq(from wire.Peer, msg wire.PingReq, now time.Time) {}

func (n *Node) handlePingReqAck(from wire.Peer, msg wire.PingReqAck) {}

func (n *Node) suspect(id wire.NodeID, now time.Time) {}

func (n *Node) memberTick(now time.Time) {}
