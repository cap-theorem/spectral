package node

import (
	"crypto/sha256"
	"encoding/binary"
	"slices"
	"time"

	"github.com/cap-theorem/spectral/internal/wire"
)

const numBuckets = 64

type entry struct {
	wire.Record
	since time.Time
}

type registry struct {
	records map[wire.NodeID]entry
}

func newRegistry() registry {
	return registry{
		records: make(map[wire.NodeID]entry),
	}
}

// `live()` returns both alive and suspect node records
func (r *registry) live() []wire.Record {
	var res []wire.Record
	for _, e := range r.records {
		if e.Status == wire.StatusAlive || e.Status == wire.StatusSuspect {
			res = append(res, e.Record)
		}
	}
	return res
}

func (r *registry) merge(rec wire.Record, since time.Time) bool {
	old, ok := r.records[rec.Peer.ID]
	if ok && !newer(rec.Inc, rec.Status, old.Inc, old.Status) {
		return false
	}

	r.records[rec.Peer.ID] = entry{
		Record: rec,
		since:  since,
	}

	return true
}

func (r *registry) lookup(service string) []wire.Service {
	var res []wire.Service
	for _, e := range r.records {
		if e.Status != wire.StatusAlive {
			continue
		}

		if e.Service.Name == service {
			res = append(res, e.Service)
		}
	}

	return res
}

func (r *registry) digest() []uint64 {
	d := make([]uint64, numBuckets)
	for id, e := range r.records {
		d[bucketOf(id)] ^= headHash(wire.Head{ID: id, Inc: e.Inc, Status: e.Status})
	}
	return d
}

func (r *registry) heads(buckets []int) []wire.Head {
	var res []wire.Head
	for id, e := range r.records {
		if slices.Contains(buckets, bucketOf(id)) {
			res = append(res, wire.Head{ID: id, Inc: e.Inc, Status: e.Status})
		}
	}
	return res
}

func bucketOf(id wire.NodeID) int {
	return int(id[0]) % numBuckets
}

func headHash(h wire.Head) uint64 {
	var b [25]byte
	copy(b[:16], h.ID[:])
	binary.BigEndian.PutUint64(b[16:24], h.Inc)
	b[24] = byte(h.Status)
	sum := sha256.Sum256(b[:])
	return binary.BigEndian.Uint64(sum[:8])
}

func newer(inc uint64, st wire.Status, oldInc uint64, oldSt wire.Status) bool {
	return inc > oldInc || (inc == oldInc && st > oldSt)
}
