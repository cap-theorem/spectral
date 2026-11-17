package node

import (
	"fmt"
	"log/slog"
	"maps"
	"net/netip"
	"slices"
	"testing"
	"time"
	"uuid"

	"github.com/cap-theorem/spectral/internal/transport"
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
			o := newOverlay(live[0].Peer.ID, OverlayHRW)
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
	a := newOverlay(live[0].Peer.ID, OverlayHRW)
	a.recompute(live, time.Unix(100, 0))

	reversed := slices.Clone(live)
	slices.Reverse(reversed)
	again := newOverlay(live[0].Peer.ID, OverlayHRW)
	again.recompute(reversed, time.Unix(100, 0))
	if !maps.Equal(a.want, again.want) {
		t.Fatalf("got %v for reversed input, want %v", again.want, a.want)
	}

	b := newOverlay(live[1].Peer.ID, OverlayHRW)
	b.recompute(live, time.Unix(100, 0))
	if maps.Equal(a.want, b.want) {
		t.Fatalf("nodes %v and %v picked the same peers", a.self, b.self)
	}
}

func TestOverlayLinksFormOverMem(t *testing.T) {
	network := transport.NewMemNetwork()
	nodes := make([]*Node, 10)
	for i := range nodes {
		addr := netip.MustParseAddrPort(fmt.Sprintf("127.0.0.1:%d", 7100+i))
		nodes[i] = NewNode(NodeConfig{ListenAddr: addr}, network.Join(addr), slog.New(slog.DiscardHandler))
	}
	for _, n := range nodes {
		for _, m := range nodes {
			n.reg.merge(wire.Record{Peer: m.self, Status: wire.StatusAlive}, time.Now())
		}
		n.ov.dirty = true
	}

	for range 5 {
		for _, n := range nodes {
			for drained := false; !drained; {
				select {
				case env := <-n.transport.Recv():
					n.handleEnvelope(env)
				default:
					drained = true
				}
			}
			n.tick(time.Now())
		}
	}

	for _, n := range nodes {
		if len(n.ov.links) < K {
			t.Fatalf("node %v has %d links, want at least %d", n.self.ID, len(n.ov.links), K)
		}
		for _, m := range nodes {
			_, nm := n.ov.links[m.self.ID]
			_, mn := m.ov.links[n.self.ID]
			if nm != mn {
				t.Fatalf("link %v-%v is one-sided", n.self.ID, m.self.ID)
			}
		}
	}
}

func TestOverlayCoin(t *testing.T) {
	live := liveRecords(300)
	now := time.Unix(100, 0)

	overlays := make([]overlay, len(live))
	total := 0
	for i, rec := range live {
		overlays[i] = newOverlay(rec.Peer.ID, OverlayCoin)
		overlays[i].recompute(live, now)
		total += len(overlays[i].want)

		if _, ok := overlays[i].want[rec.Peer.ID]; ok {
			t.Fatalf("node %v picked itself", rec.Peer.ID)
		}
	}

	for i, a := range overlays {
		for j, b := range overlays {
			_, ab := a.want[live[j].Peer.ID]
			_, ba := b.want[live[i].Peer.ID]
			if ab != ba {
				t.Fatalf("coin between %v and %v is one-sided", a.self, b.self)
			}
		}
	}

	if mean := float64(total) / float64(len(live)); mean < 2*K-1.5 || mean > 2*K+1.5 {
		t.Fatalf("mean degree = %.2f, want about %d", mean, 2*K)
	}
}

func TestParseOverlay(t *testing.T) {
	tests := []struct {
		in      string
		want    OverlayKind
		wantErr bool
	}{
		{"hrw", OverlayHRW, false},
		{"coin", OverlayCoin, false},
		{"nope", 0, true},
		{"", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := ParseOverlay(tt.in)
			if (err != nil) != tt.wantErr || got != tt.want {
				t.Fatalf("ParseOverlay(%q) = %v, %v; want %v, wantErr %v", tt.in, got, err, tt.want, tt.wantErr)
			}
		})
	}
}
