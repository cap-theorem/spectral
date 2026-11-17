package node

import (
	"net/netip"
	"testing"
	"time"
	"uuid"

	"github.com/cap-theorem/spectral/internal/transport"
	"github.com/cap-theorem/spectral/internal/wire"
)

func memNode(network *transport.MemNetwork, port uint16) *Node {
	addr := netip.AddrPortFrom(netip.MustParseAddr("127.0.0.1"), port)
	return NewNode(NodeConfig{ListenAddr: addr}, network.Join(addr), testLogger())
}

func linkTo(n *Node, p wire.Peer, now time.Time) {
	n.reg.merge(wire.Record{Peer: p, Status: wire.StatusAlive}, now)
	n.ov.links[p.ID] = link{peer: p, since: now, lastRecv: now, lastSent: now}
}

func next(t *testing.T, n *Node) wire.Envelope {
	t.Helper()
	select {
	case env := <-n.transport.Recv():
		return env
	default:
		t.Fatalf("node %v has no message", n.self.ID)
		return wire.Envelope{}
	}
}

func quiet(t *testing.T, n *Node) {
	t.Helper()
	select {
	case env := <-n.transport.Recv():
		t.Fatalf("node %v got unexpected %#v", n.self.ID, env.Msg)
	default:
	}
}

func probeNonce(t *testing.T, n *Node) uint64 {
	t.Helper()
	if len(n.mem.probes) != 1 {
		t.Fatalf("probes = %d, want 1", len(n.mem.probes))
	}
	for nonce := range n.mem.probes {
		return nonce
	}
	return 0
}

func TestStartProbeSendsPingAndPingReqs(t *testing.T) {
	t0 := time.Now()
	network := transport.NewMemNetwork()
	a := memNode(network, 7300)
	target := memNode(network, 7301)
	linkTo(a, target.self, t0)

	helpers := make([]*Node, 5)
	for i := range helpers {
		helpers[i] = memNode(network, uint16(7302+i))
		linkTo(a, helpers[i].self, t0)
	}

	a.startProbe(target.self, t0)
	nonce := probeNonce(t, a)

	env := next(t, target)
	ping, ok := env.Msg.(wire.Ping)
	if !ok || ping.Nonce != nonce || env.From.ID != a.self.ID {
		t.Fatalf("target got %#v from %v, want Ping{Nonce: %d} from %v", env.Msg, env.From.ID, nonce, a.self.ID)
	}
	quiet(t, target)

	asked := 0
	for _, h := range helpers {
		select {
		case env := <-h.transport.Recv():
			req, ok := env.Msg.(wire.PingReq)
			if !ok || req.Nonce != nonce || req.Target != target.self {
				t.Fatalf("helper got %#v, want PingReq{Nonce: %d, Target: %v}", env.Msg, nonce, target.self)
			}
			asked++
			quiet(t, h)
		default:
		}
	}
	if asked != probeFanout {
		t.Fatalf("helpers asked = %d, want %d", asked, probeFanout)
	}
}

func TestStartProbeSkips(t *testing.T) {
	t0 := time.Unix(100, 0)

	tests := []struct {
		name   string
		status *wire.Status
		probes int
	}{
		{"unknown", nil, 0},
		{"suspect", new(wire.StatusSuspect), 0},
		{"dead", new(wire.StatusDead), 0},
		{"already probing", new(wire.StatusAlive), 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := testNode()
			x := wire.Peer{ID: uuid.NewV4(), Address: netip.MustParseAddrPort("127.0.0.1:7001")}
			if tt.status != nil {
				n.reg.merge(wire.Record{Peer: x, Status: *tt.status}, t0)
			}

			n.startProbe(x, t0)
			n.startProbe(x, t0)

			if len(n.mem.probes) != tt.probes {
				t.Fatalf("probes = %d, want %d", len(n.mem.probes), tt.probes)
			}
		})
	}
}

func TestIndirectProbeKeepsPeerAlive(t *testing.T) {
	t0 := time.Now()
	network := transport.NewMemNetwork()
	a := memNode(network, 7310)
	b := memNode(network, 7311)
	c := memNode(network, 7312)
	a.reg.merge(wire.Record{Peer: c.self, Status: wire.StatusAlive}, t0)
	linkTo(a, b.self, t0)

	a.startProbe(c.self, t0)

	if env := next(t, c); env.From.ID != a.self.ID {
		t.Fatalf("target got %#v from %v, want direct Ping from %v", env.Msg, env.From.ID, a.self.ID)
	}

	env := next(t, b)
	if _, ok := env.Msg.(wire.PingReq); !ok {
		t.Fatalf("helper got %#v, want PingReq", env.Msg)
	}
	b.handleEnvelope(env)

	env = next(t, c)
	if _, ok := env.Msg.(wire.Ping); !ok || env.From.ID != b.self.ID {
		t.Fatalf("target got %#v from %v, want Ping from helper", env.Msg, env.From.ID)
	}
	c.handleEnvelope(env)

	env = next(t, b)
	if _, ok := env.Msg.(wire.Pong); !ok {
		t.Fatalf("helper got %#v, want Pong", env.Msg)
	}
	b.handleEnvelope(env)
	if len(b.mem.relays) != 0 {
		t.Fatalf("relays = %d, want 0", len(b.mem.relays))
	}

	env = next(t, a)
	if ack, ok := env.Msg.(wire.PingReqAck); !ok || ack.Target != c.self.ID {
		t.Fatalf("prober got %#v, want PingReqAck for %v", env.Msg, c.self.ID)
	}
	a.handleEnvelope(env)
	if len(a.mem.probes) != 0 {
		t.Fatalf("probes = %d, want 0", len(a.mem.probes))
	}

	a.expireProbes(t0.Add(probeTimeout + time.Millisecond))
	if got := a.reg.records[c.self.ID].Status; got != wire.StatusAlive {
		t.Fatalf("after acked probe: %v, want Alive", got)
	}
}

func TestUnansweredProbeSuspects(t *testing.T) {
	t0 := time.Now()
	network := transport.NewMemNetwork()
	a := memNode(network, 7320)
	b := memNode(network, 7321)
	c := wire.Peer{ID: uuid.NewV4(), Address: netip.MustParseAddrPort("127.0.0.1:7322")}
	a.reg.merge(wire.Record{Peer: c, Status: wire.StatusAlive}, t0)
	linkTo(a, b.self, t0)

	a.startProbe(c, t0)
	b.handleEnvelope(next(t, b))
	if len(b.mem.relays) != 1 {
		t.Fatalf("relays = %d, want 1", len(b.mem.relays))
	}

	b.expireProbes(time.Now().Add(probeTimeout + time.Millisecond))
	if len(b.mem.relays) != 0 {
		t.Fatalf("relays after timeout = %d, want 0", len(b.mem.relays))
	}
	quiet(t, a)

	a.expireProbes(t0.Add(probeTimeout + time.Millisecond))
	if got := a.reg.records[c.ID].Status; got != wire.StatusSuspect {
		t.Fatalf("after unanswered probe: %v, want Suspect", got)
	}
	if len(a.mem.probes) != 0 {
		t.Fatalf("probes = %d, want 0", len(a.mem.probes))
	}
}

func TestMessageFromTargetClearsProbe(t *testing.T) {
	t0 := time.Now()
	n := testNode()
	x := wire.Peer{ID: uuid.NewV4(), Address: netip.MustParseAddrPort("127.0.0.1:7001")}
	n.reg.merge(wire.Record{Peer: x, Status: wire.StatusAlive}, t0)

	n.startProbe(x, t0)
	n.handleEnvelope(wire.Envelope{From: x, To: n.self.ID, Msg: wire.Heartbeat{}})
	if len(n.mem.probes) != 0 {
		t.Fatalf("probes = %d, want 0", len(n.mem.probes))
	}

	n.expireProbes(t0.Add(probeTimeout + time.Millisecond))
	if got := n.reg.records[x.ID].Status; got != wire.StatusAlive {
		t.Fatalf("after reply: %v, want Alive", got)
	}
}

func TestPingReqAckMustMatchProbe(t *testing.T) {
	t0 := time.Now()
	helper := wire.Peer{ID: uuid.NewV4(), Address: netip.MustParseAddrPort("127.0.0.1:7002")}

	tests := []struct {
		name        string
		wrongNonce  bool
		wrongTarget bool
	}{
		{"unknown nonce", true, false},
		{"wrong target", false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := testNode()
			x := wire.Peer{ID: uuid.NewV4(), Address: netip.MustParseAddrPort("127.0.0.1:7001")}
			n.reg.merge(wire.Record{Peer: x, Status: wire.StatusAlive}, t0)
			n.startProbe(x, t0)

			ack := wire.PingReqAck{Nonce: probeNonce(t, n), Target: x.ID}
			if tt.wrongNonce {
				ack.Nonce++
			}
			if tt.wrongTarget {
				ack.Target = uuid.NewV4()
			}

			n.handleEnvelope(wire.Envelope{From: helper, To: n.self.ID, Msg: ack})

			if len(n.mem.probes) != 1 {
				t.Fatalf("probes = %d, want 1", len(n.mem.probes))
			}
		})
	}
}

func TestRelayPongMustComeFromTarget(t *testing.T) {
	network := transport.NewMemNetwork()
	a := memNode(network, 7330)
	b := memNode(network, 7331)
	c := wire.Peer{ID: uuid.NewV4(), Address: netip.MustParseAddrPort("127.0.0.1:7332")}

	b.handleEnvelope(wire.Envelope{From: a.self, To: b.self.ID, Msg: wire.PingReq{Nonce: 1, Target: c}})
	if len(b.mem.relays) != 1 {
		t.Fatalf("relays = %d, want 1", len(b.mem.relays))
	}
	var nonce uint64
	for k := range b.mem.relays {
		nonce = k
	}

	other := wire.Peer{ID: uuid.NewV4(), Address: netip.MustParseAddrPort("127.0.0.1:7333")}
	b.handleEnvelope(wire.Envelope{From: other, To: b.self.ID, Msg: wire.Pong{Nonce: nonce}})
	if len(b.mem.relays) != 1 {
		t.Fatalf("relays after foreign pong = %d, want 1", len(b.mem.relays))
	}
	quiet(t, a)
}

func TestPingReqForSelfIgnored(t *testing.T) {
	n := testNode()
	from := wire.Peer{ID: uuid.NewV4(), Address: netip.MustParseAddrPort("127.0.0.1:7001")}

	n.handleEnvelope(wire.Envelope{From: from, To: n.self.ID, Msg: wire.PingReq{Nonce: 1, Target: n.self}})

	if len(n.mem.relays) != 0 {
		t.Fatalf("relays = %d, want 0", len(n.mem.relays))
	}
}
