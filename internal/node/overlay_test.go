package node

import (
	"maps"
	"slices"
	"testing"
	"time"
	"uuid"

	"github.com/cap-theorem/spectral/internal/wire"
)

func liveRecords(n int) []wire.Record {
	recs := make([]wire.Record, n)
	for i := range recs {
		recs[i] = wire.Record{Peer: wire.Peer{ID: uuid.NewV4()}, Status: wire.StatusAlive}
	}
	return recs
}

func best(self wire.NodeID, live []wire.Record) int {
	i := slices.IndexFunc(live, func(r wire.Record) bool { return r.Peer.ID != self })
	for j, rec := range live {
		if rec.Peer.ID != self && rank(self, rec.Peer.ID) > rank(self, live[i].Peer.ID) {
			i = j
		}
	}
	return i
}

func TestOverlayRecompute(t *testing.T) {
	now := time.Unix(100, 0)

	tests := []struct {
		name string
		edit func(o *overlay, live []wire.Record, b int) ([]wire.Record, []wire.NodeID)
	}{
		{"all alive", func(o *overlay, live []wire.Record, b int) ([]wire.Record, []wire.NodeID) {
			return live, nil
		}},
		{"fewer than K", func(o *overlay, live []wire.Record, b int) ([]wire.Record, []wire.NodeID) {
			return live[:K-2], nil
		}},
		{"avoided", func(o *overlay, live []wire.Record, b int) ([]wire.Record, []wire.NodeID) {
			o.avoid[live[b].Peer.ID] = now.Add(time.Second)
			return live, []wire.NodeID{live[b].Peer.ID}
		}},
		{"avoid expired", func(o *overlay, live []wire.Record, b int) ([]wire.Record, []wire.NodeID) {
			o.avoid[live[b].Peer.ID] = now
			return live, nil
		}},
		{"unlinked suspect", func(o *overlay, live []wire.Record, b int) ([]wire.Record, []wire.NodeID) {
			live[b].Status = wire.StatusSuspect
			return live, []wire.NodeID{live[b].Peer.ID}
		}},
		{"linked suspect", func(o *overlay, live []wire.Record, b int) ([]wire.Record, []wire.NodeID) {
			live[b].Status = wire.StatusSuspect
			o.links[live[b].Peer.ID] = link{}
			return live, nil
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			live := liveRecords(20)
			o := newOverlay(live[0].Peer.ID)
			in, ineligible := tt.edit(&o, live, best(o.self, live))

			o.recompute(in, now)

			var eligible []wire.NodeID
			for _, rec := range in {
				if rec.Peer.ID != o.self && !slices.Contains(ineligible, rec.Peer.ID) {
					eligible = append(eligible, rec.Peer.ID)
				}
			}

			if got, want := len(o.want), min(K, len(eligible)); got != want {
				t.Fatalf("len(want) = %d, want %d", got, want)
			}
			for picked := range o.want {
				if !slices.Contains(eligible, picked) {
					t.Fatalf("picked ineligible %v", picked)
				}
				for _, id := range eligible {
					if _, ok := o.want[id]; !ok && rank(o.self, id) > rank(o.self, picked) {
						t.Fatalf("skipped %v that outranks picked %v", id, picked)
					}
				}
			}
		})
	}
}

func TestOverlayRecomputeStable(t *testing.T) {
	live := liveRecords(50)
	a := newOverlay(live[0].Peer.ID)
	a.recompute(live, time.Unix(100, 0))

	reversed := slices.Clone(live)
	slices.Reverse(reversed)
	again := newOverlay(live[0].Peer.ID)
	again.recompute(reversed, time.Unix(100, 0))
	if !maps.Equal(a.want, again.want) {
		t.Fatalf("got %v for reversed input, want %v", again.want, a.want)
	}

	b := newOverlay(live[1].Peer.ID)
	b.recompute(live, time.Unix(100, 0))
	if maps.Equal(a.want, b.want) {
		t.Fatalf("nodes %v and %v picked the same peers", a.self, b.self)
	}
}
