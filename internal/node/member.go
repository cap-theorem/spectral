package node

import (
	"context"
	"math/rand/v2"
	"net"
	"net/netip"
	"slices"
	"strconv"
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
	maxAEHeads          = 200
	seedResolveTimeout  = 200 * time.Millisecond
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
	seeds        []string
	seedAddrs    []netip.AddrPort
	seedSynced   bool
	nextSeedPull time.Time
	nextAE       time.Time

	outbox []queued
	probes map[uint64]probe
	relays map[uint64]relay
}

func newMembership(seeds []string) membership {
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
	if slices.Contains(n.mem.seedAddrs, from.Address) {
		n.mem.seedSynced = true
	}

	for _, rec := range msg.Records {
		n.apply(rec, from.ID, now)
	}
}

func (n *Node) sendDigest(to wire.Peer) {
	n.send(to, wire.AEDigest{Digest: n.reg.digest()})
}

func (n *Node) handleAEDigest(from wire.Peer, msg wire.AEDigest) {
	if len(msg.Digest) != numBuckets {
		return
	}

	mine := n.reg.digest()
	var buckets []int
	var heads []wire.Head
	for b := range numBuckets {
		if mine[b] == msg.Digest[b] {
			continue
		}

		bh := n.reg.heads([]int{b})
		if len(heads) > 0 && len(heads)+len(bh) > maxAEHeads {
			n.send(from, wire.AEHeads{Buckets: buckets, Heads: heads})
			buckets, heads = nil, nil
		}
		buckets = append(buckets, b)
		heads = append(heads, bh...)
	}

	if len(buckets) > 0 {
		n.send(from, wire.AEHeads{Buckets: buckets, Heads: heads})
	}
}

func (n *Node) handleAEHeads(from wire.Peer, msg wire.AEHeads, now time.Time) {
	theirs := make(map[wire.NodeID]wire.Head, len(msg.Heads))
	var ids []wire.NodeID
	for _, h := range msg.Heads {
		theirs[h.ID] = h
		e, ok := n.reg.records[h.ID]
		if !ok || newer(h.Inc, h.Status, e.Inc, e.Status) {
			ids = append(ids, h.ID)
		}
	}

	var push []wire.Record
	for _, h := range n.reg.heads(msg.Buckets) {
		t, ok := theirs[h.ID]
		if !ok || newer(h.Inc, h.Status, t.Inc, t.Status) {
			push = append(push, n.reg.records[h.ID].Record)
		}
	}

	for len(ids) > 0 {
		k := min(maxAEHeads, len(ids))
		n.send(from, wire.AEWant{IDs: ids[:k]})
		ids = ids[k:]
	}
	n.sendRecords(from, push)
}

func (n *Node) handleAEWant(from wire.Peer, msg wire.AEWant, now time.Time) {
	for _, rec := range msg.Records {
		n.apply(rec, from.ID, now)
	}

	var recs []wire.Record
	for _, id := range msg.IDs {
		if e, ok := n.reg.records[id]; ok {
			recs = append(recs, e.Record)
		}
	}
	n.sendRecords(from, recs)
}

func (n *Node) sendRecords(to wire.Peer, recs []wire.Record) {
	for len(recs) > 0 {
		k := min(maxUpdateRecords, len(recs))
		n.send(to, wire.Update{Records: recs[:k]})
		recs = recs[k:]
	}
}

func (n *Node) startProbe(target wire.Peer, now time.Time) {}

func (n *Node) handlePingReq(from wire.Peer, msg wire.PingReq, now time.Time) {}

func (n *Node) handlePingReqAck(from wire.Peer, msg wire.PingReqAck) {}

func (n *Node) suspect(id wire.NodeID, now time.Time) {
	e, ok := n.reg.records[id]
	if !ok || e.Status != wire.StatusAlive {
		return
	}

	rec := e.Record
	rec.Status = wire.StatusSuspect
	n.apply(rec, n.self.ID, now)
}

func (n *Node) memberTick(now time.Time) {
	if !now.Before(n.mem.nextAE) {
		n.mem.nextAE = now.Add(antiEntropyInterval)
		if peer, ok := n.randomLink(); ok {
			n.sendDigest(peer)
		}
	}

	if !n.mem.seedSynced && len(n.mem.seeds) > 0 && !now.Before(n.mem.nextSeedPull) {
		n.mem.nextSeedPull = now.Add(seedPullInterval)
		for _, i := range rand.Perm(len(n.mem.seeds)) {
			addrs := n.resolveSeed(n.mem.seeds[i])
			if len(addrs) == 0 {
				continue
			}

			for _, addr := range addrs {
				if !slices.Contains(n.mem.seedAddrs, addr) {
					n.mem.seedAddrs = append(n.mem.seedAddrs, addr)
				}
			}
			n.sendDigest(wire.Peer{Address: addrs[rand.IntN(len(addrs))]})
			break
		}
	}

	n.expireSuspects(now)
}

func (n *Node) expireSuspects(now time.Time) {
	var dead []wire.Record
	for _, e := range n.reg.records {
		if e.Status == wire.StatusSuspect && now.Sub(e.since) > suspectTimeout {
			rec := e.Record
			rec.Status = wire.StatusDead
			dead = append(dead, rec)
		}
	}

	for _, rec := range dead {
		n.apply(rec, n.self.ID, now)
	}
}

func (n *Node) resolveSeed(seed string) []netip.AddrPort {
	host, portStr, err := net.SplitHostPort(seed)
	if err != nil {
		n.logger.Warn("invalid seed", "seed", seed, "err", err)
		return nil
	}
	port, err := strconv.ParseUint(portStr, 10, 16)
	if err != nil {
		n.logger.Warn("invalid seed port", "seed", seed, "err", err)
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), seedResolveTimeout)
	defer cancel()
	ips, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
	if err != nil {
		n.logger.Debug("seed lookup failed", "seed", seed, "err", err)
		return nil
	}

	var addrs []netip.AddrPort
	for _, ip := range ips {
		ip = ip.Unmap()
		if ip.Is4() != n.self.Address.Addr().Is4() {
			continue
		}
		addr := netip.AddrPortFrom(ip, uint16(port))
		if addr != n.self.Address {
			addrs = append(addrs, addr)
		}
	}
	return addrs
}

func (n *Node) randomLink() (wire.Peer, bool) {
	if len(n.ov.links) == 0 {
		return wire.Peer{}, false
	}

	i := rand.IntN(len(n.ov.links))
	for _, l := range n.ov.links {
		if i == 0 {
			return l.peer, true
		}
		i--
	}
	return wire.Peer{}, false
}
