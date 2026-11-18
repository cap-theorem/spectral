package node

import (
	"testing"
	"time"

	"github.com/cap-theorem/spectral/internal/transport"
	"github.com/cap-theorem/spectral/internal/wire"
)

func pump(nodes ...*Node) {
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
			n.flush()
		}
	}
}

func seededNode(network *transport.MemNetwork, port uint16, seed *Node) *Node {
	n := memNode(network, port)
	n.mem.seeds = []string{seed.self.Address.String()}
	n.mem.seedSynced = true
	return n
}

func TestRejoinCheck(t *testing.T) {
	tests := []struct {
		name       string
		linked     bool
		wantRejoin bool
	}{
		{"cut off", false, true},
		{"linked to known members", true, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			network := transport.NewMemNetwork()
			seed := memNode(network, 7701)
			n := seededNode(network, 7702, seed)
			now := time.Now()
			if tt.linked {
				linkTo(n, seed.self, now)
			}

			n.memberTick(now)

			if got := !n.mem.seedSynced; got != tt.wantRejoin {
				t.Fatalf("rejoin = %v, want %v", got, tt.wantRejoin)
			}
		})
	}
}

func TestRejoinInterval(t *testing.T) {
	network := transport.NewMemNetwork()
	seed := memNode(network, 7711)
	n := seededNode(network, 7712, seed)
	t0 := time.Now()

	n.memberTick(t0)
	n.mem.seedSynced = true

	n.memberTick(t0.Add(rejoinInterval - time.Millisecond))
	if !n.mem.seedSynced {
		t.Fatal("rejoined before the interval passed")
	}

	n.memberTick(t0.Add(rejoinInterval))
	if n.mem.seedSynced {
		t.Fatal("no rejoin after the interval")
	}
}

func TestCutOffNodeRejoinsAndRefutes(t *testing.T) {
	network := transport.NewMemNetwork()
	seed := memNode(network, 7721)
	n := seededNode(network, 7722, seed)
	now := time.Now()

	seed.reg.merge(wire.Record{Peer: n.self, Status: wire.StatusDead}, now)
	inc := n.reg.records[n.self.ID].Inc

	n.memberTick(now)
	pump(seed, n)

	if e, ok := n.reg.records[seed.self.ID]; !ok || e.Status != wire.StatusAlive {
		t.Fatalf("cut-off node did not relearn the seed: %+v, %v", e.Record, ok)
	}
	if own := n.reg.records[n.self.ID]; own.Status != wire.StatusAlive || own.Inc <= inc {
		t.Fatalf("own record after rejoin = %+v, want Alive with Inc > %d", own.Record, inc)
	}
	if !n.mem.seedSynced {
		t.Fatal("seed sync not completed")
	}
}
