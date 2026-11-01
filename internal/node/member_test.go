package node

import (
	"log/slog"
	"net/netip"
	"testing"
	"time"
	"uuid"

	"github.com/cap-theorem/spectral/internal/transport"
	"github.com/cap-theorem/spectral/internal/wire"
)

func testNode() *Node {
	addr := netip.MustParseAddrPort("127.0.0.1:7000")
	return NewNode(NodeConfig{ListenAddr: addr}, transport.NewMemNetwork().Join(addr), slog.New(slog.DiscardHandler))
}

func TestSuspect(t *testing.T) {
	t0 := time.Unix(100, 0)

	tests := []struct {
		name  string
		have  *wire.Status
		want  wire.Status
		flood bool
	}{
		{"alive", new(wire.StatusAlive), wire.StatusSuspect, true},
		{"already suspect", new(wire.StatusSuspect), wire.StatusSuspect, false},
		{"dead", new(wire.StatusDead), wire.StatusDead, false},
		{"unknown", nil, 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := testNode()
			x := wire.Peer{ID: uuid.NewV4()}
			if tt.have != nil {
				n.reg.merge(wire.Record{Peer: x, Status: *tt.have}, t0)
			}

			n.suspect(x.ID, t0)

			e, ok := n.reg.records[x.ID]
			if ok != (tt.have != nil) || (ok && e.Status != tt.want) {
				t.Fatalf("got %v (known %v), want %v", e.Status, ok, tt.want)
			}
			if got := len(n.mem.outbox) == 1; got != tt.flood {
				t.Fatalf("flooded = %v, want %v", got, tt.flood)
			}
		})
	}
}

func TestSilentLinkSuspectsThenDies(t *testing.T) {
	t0 := time.Unix(100, 0)
	n := testNode()
	x := wire.Peer{ID: uuid.NewV4(), Address: netip.MustParseAddrPort("127.0.0.1:7001")}
	n.apply(wire.Record{Peer: x, Status: wire.StatusAlive}, uuid.NewV4(), t0)
	n.ov.links[x.ID] = link{peer: x, since: t0, lastRecv: t0, lastSent: t0}

	silent := t0.Add(linkTimeout + time.Millisecond)
	n.overlayTick(silent)
	if got := n.reg.records[x.ID].Status; got != wire.StatusSuspect {
		t.Fatalf("after silent link: %v, want Suspect", got)
	}
	if _, ok := n.ov.links[x.ID]; ok {
		t.Fatal("silent link kept")
	}

	n.expireSuspects(silent.Add(suspectTimeout))
	if got := n.reg.records[x.ID].Status; got != wire.StatusSuspect {
		t.Fatalf("at suspect timeout: %v, want Suspect", got)
	}
	n.expireSuspects(silent.Add(suspectTimeout + time.Millisecond))
	if got := n.reg.records[x.ID].Status; got != wire.StatusDead {
		t.Fatalf("after suspect timeout: %v, want Dead", got)
	}
}
