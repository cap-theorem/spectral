package node

import (
	"time"

	"github.com/cap-theorem/spectral/internal/wire"
)

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

func newer(inc uint64, st wire.Status, oldInc uint64, oldSt wire.Status) bool {
	return inc > oldInc || (inc == oldInc && st > oldSt)
}
