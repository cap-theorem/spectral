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
	rejoinInterval      = 15 * time.Second
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
	nextRejoin   time.Time

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
	if (rec.Status == wire.StatusDead || rec.Status == wire.StatusLeft) && !known {
		return false
	}
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
	if n.state == StateLeft || rec.Status == wire.StatusAlive || rec.Inc < own.Inc {
		return
	}

	own.Inc = rec.Inc + 1
	own.Status = wire.StatusAlive
	n.apply(own, n.self.ID, now)
}

func (n *Node) leave(now time.Time) {
	if n.state == StateLeft {
		return
	}
	n.state = StateLeft
	n.lease = nil
	n.instance = ""

	own := n.reg.records[n.self.ID].Record
	own.Inc++
	own.Status = wire.StatusLeft
	n.apply(own, n.self.ID, now)
	n.flush()

	for id, l := range n.ov.links {
		n.send(l.peer, wire.Unlink{})
		delete(n.ov.links, id)
	}

	n.logger.Info("left cluster", "inc", own.Inc)
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

func (n *Node) startProbe(target wire.Peer, now time.Time) {
	e, ok := n.reg.records[target.ID]
	if !ok || e.Status != wire.StatusAlive {
		return
	}
	for _, p := range n.mem.probes {
		if p.target.ID == target.ID {
			return
		}
	}

	nonce := rand.Uint64()
	n.mem.probes[nonce] = probe{target: target, deadline: now.Add(probeTimeout)}
	n.send(target, wire.Ping{Nonce: nonce})

	var helpers []wire.Peer
	for id, l := range n.ov.links {
		if id != target.ID {
			helpers = append(helpers, l.peer)
		}
	}
	rand.Shuffle(len(helpers), func(i, j int) {
		helpers[i], helpers[j] = helpers[j], helpers[i]
	})
	helpers = helpers[:min(probeFanout, len(helpers))]
	for _, h := range helpers {
		n.send(h, wire.PingReq{Nonce: nonce, Target: target})
	}

	n.logger.Debug("probe started", "peer", target.ID, "helpers", len(helpers))
}

func (n *Node) handlePingReq(from wire.Peer, msg wire.PingReq, now time.Time) {
	if msg.Target.ID == n.self.ID || msg.Target.ID == (wire.NodeID{}) {
		return
	}

	nonce := rand.Uint64()
	n.mem.relays[nonce] = relay{
		requester: from,
		target:    msg.Target.ID,
		nonce:     msg.Nonce,
		deadline:  now.Add(probeTimeout),
	}
	n.send(msg.Target, wire.Ping{Nonce: nonce})
}

func (n *Node) handleRelayPong(from wire.Peer, msg wire.Pong) {
	r, ok := n.mem.relays[msg.Nonce]
	if !ok || r.target != from.ID {
		return
	}

	delete(n.mem.relays, msg.Nonce)
	n.send(r.requester, wire.PingReqAck{Nonce: r.nonce, Target: r.target})
}

func (n *Node) handlePingReqAck(from wire.Peer, msg wire.PingReqAck) {
	p, ok := n.mem.probes[msg.Nonce]
	if !ok || p.target.ID != msg.Target {
		return
	}

	delete(n.mem.probes, msg.Nonce)
	n.logger.Debug("probe acked by helper", "peer", msg.Target, "helper", from.ID)
}

func (n *Node) clearProbes(id wire.NodeID) {
	for nonce, p := range n.mem.probes {
		if p.target.ID == id {
			delete(n.mem.probes, nonce)
		}
	}
}

func (n *Node) expireProbes(now time.Time) {
	for nonce, p := range n.mem.probes {
		if now.After(p.deadline) {
			delete(n.mem.probes, nonce)
			n.logger.Debug("probe failed", "peer", p.target.ID)
			n.suspect(p.target.ID, now)
		}
	}

	for nonce, r := range n.mem.relays {
		if now.After(r.deadline) {
			delete(n.mem.relays, nonce)
		}
	}
}

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

	if !now.Before(n.mem.nextRejoin) {
		n.mem.nextRejoin = now.Add(rejoinInterval)
		if len(n.ov.links) == 0 || len(n.reg.live()) <= 1 {
			n.mem.seedSynced = false
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

	n.expireProbes(now)
	n.expireSuspects(now)
	n.expireTombstones(now)
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

func (n *Node) expireTombstones(now time.Time) {
	for id, e := range n.reg.records {
		if (e.Status == wire.StatusDead || e.Status == wire.StatusLeft) && now.Sub(e.since) > tombstoneTTL {
			delete(n.reg.records, id)
		}
	}
}
