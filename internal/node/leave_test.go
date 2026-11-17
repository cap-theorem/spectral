package node

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/cap-theorem/spectral/internal/transport"
	"github.com/cap-theorem/spectral/internal/wire"
)

func testRegistration() Registration {
	return Registration{
		Service:    "payments-api",
		InstanceID: "i-1",
		Endpoint:   "http://127.0.0.1:9000",
		TTL:        time.Minute,
	}
}

func TestLeaveAnnouncesAndUnlinks(t *testing.T) {
	t0 := time.Now()
	network := transport.NewMemNetwork()
	a := memNode(network, 7400)
	peers := []*Node{memNode(network, 7401), memNode(network, 7402)}
	for _, p := range peers {
		linkTo(a, p.self, t0)
	}
	a.lease = &Lease{Token: "token", TTL: time.Minute, ExpiresAt: t0.Add(time.Minute)}
	inc := a.reg.records[a.self.ID].Inc

	a.leave(t0)

	own := a.reg.records[a.self.ID]
	if own.Status != wire.StatusLeft || own.Inc != inc+1 {
		t.Fatalf("own record = %v inc %d, want Left inc %d", own.Status, own.Inc, inc+1)
	}
	if a.state != StateLeft {
		t.Fatalf("state = %v, want %v", a.state, StateLeft)
	}
	if a.lease != nil {
		t.Fatal("lease kept after leave")
	}
	if len(a.ov.links) != 0 {
		t.Fatalf("links = %d, want 0", len(a.ov.links))
	}
	if len(a.mem.outbox) != 0 {
		t.Fatalf("outbox = %d, want 0", len(a.mem.outbox))
	}

	for _, p := range peers {
		env := next(t, p)
		upd, ok := env.Msg.(wire.Update)
		if !ok || len(upd.Records) != 1 || upd.Records[0].Peer.ID != a.self.ID || upd.Records[0].Status != wire.StatusLeft {
			t.Fatalf("peer got %#v, want Update with own Left record", env.Msg)
		}
		if env := next(t, p); env.Msg != (wire.Unlink{}) {
			t.Fatalf("peer got %#v, want Unlink", env.Msg)
		}
		quiet(t, p)
	}
}

func TestLeftNodeStaysLeft(t *testing.T) {
	t0 := time.Now()
	network := transport.NewMemNetwork()
	a := memNode(network, 7410)
	p := memNode(network, 7411)
	linkTo(a, p.self, t0)

	a.leave(t0)
	next(t, p)
	next(t, p)
	own := a.reg.records[a.self.ID].Record

	a.leave(t0)
	a.apply(own, p.self.ID, t0)
	a.handleEnvelope(wire.Envelope{From: p.self, To: a.self.ID, Msg: wire.Update{Records: []wire.Record{own}}})
	a.handleEnvelope(wire.Envelope{From: p.self, To: a.self.ID, Msg: wire.Link{}})
	a.handleEnvelope(wire.Envelope{From: p.self, To: a.self.ID, Msg: wire.Ping{Nonce: 1}})
	a.tick(t0.Add(time.Minute))
	a.flush()

	got := a.reg.records[a.self.ID]
	if got.Status != wire.StatusLeft || got.Inc != own.Inc {
		t.Fatalf("own record = %v inc %d, want Left inc %d", got.Status, got.Inc, own.Inc)
	}
	if len(a.ov.links) != 0 {
		t.Fatalf("links = %d, want 0", len(a.ov.links))
	}
	quiet(t, p)
}

func TestPeerDropsLeftNode(t *testing.T) {
	network := transport.NewMemNetwork()
	a := memNode(network, 7420)
	b := memNode(network, 7421)
	t0 := time.Now()
	linkTo(a, b.self, t0)
	linkTo(b, a.self, t0)

	if _, err := a.registerService(testRegistration()); err != nil {
		t.Fatalf("registerService() error = %v", err)
	}
	a.flush()
	b.handleEnvelope(next(t, b))
	if got := b.reg.lookup("payments-api"); len(got) != 1 {
		t.Fatalf("lookup before leave = %d providers, want 1", len(got))
	}

	a.leave(time.Now())
	b.handleEnvelope(next(t, b))
	b.handleEnvelope(next(t, b))
	quiet(t, b)

	if got := b.reg.records[a.self.ID].Status; got != wire.StatusLeft {
		t.Fatalf("peer sees %v, want Left", got)
	}
	if _, ok := b.ov.links[a.self.ID]; ok {
		t.Fatal("peer kept link to left node")
	}
	if got := b.reg.lookup("payments-api"); len(got) != 0 {
		t.Fatalf("lookup after leave = %d providers, want 0", len(got))
	}

	b.overlayTick(time.Now().Add(linkTimeout + time.Millisecond))
	if len(b.mem.probes) != 0 {
		t.Fatalf("probes = %d, want 0", len(b.mem.probes))
	}
	if got := b.reg.records[a.self.ID].Status; got != wire.StatusLeft {
		t.Fatalf("peer sees %v after tick, want Left", got)
	}
}

func TestLeaveThroughRun(t *testing.T) {
	a := testNode()

	runCtx, stop := context.WithCancel(t.Context())
	defer stop()
	go a.Run(runCtx)

	st, err := a.Status(t.Context())
	if err != nil || !st.Ready || st.State != StateActive {
		t.Fatalf("Status() = %+v, %v; want ready and active", st, err)
	}

	if err := a.Leave(t.Context()); err != nil {
		t.Fatalf("Leave() error = %v", err)
	}

	st, err = a.Status(t.Context())
	if err != nil || st.Ready || st.State != StateLeft {
		t.Fatalf("Status() = %+v, %v; want not ready and left", st, err)
	}

	if _, err := a.Register(t.Context(), testRegistration()); !errors.Is(err, ErrNodeLeft) {
		t.Fatalf("Register() error = %v, want ErrNodeLeft", err)
	}
}
