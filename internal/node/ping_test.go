package node

import (
	"context"
	"errors"
	"log/slog"
	"net/netip"
	"testing"
	"time"
	"uuid"

	"github.com/cap-theorem/spectral/internal/transport"
	"github.com/cap-theorem/spectral/internal/wire"
)

func TestPingRoundTrip(t *testing.T) {
	network := transport.NewMemNetwork()
	aAddr := netip.MustParseAddrPort("127.0.0.1:7001")
	bAddr := netip.MustParseAddrPort("127.0.0.1:7002")

	a := NewNode(NodeConfig{ListenAddr: aAddr}, network.Join(aAddr), testLogger())
	b := NewNode(NodeConfig{ListenAddr: bAddr}, network.Join(bAddr), testLogger())
	a.reg.merge(wire.Record{Peer: b.self, Status: wire.StatusAlive}, time.Now())

	runCtx, stop := context.WithCancel(t.Context())
	defer stop()
	go a.Run(runCtx)
	go b.Run(runCtx)

	pingCtx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()

	rtt, err := a.Ping(pingCtx, b.self.ID)
	if err != nil {
		t.Fatalf("Ping() error = %v", err)
	}
	if rtt < 0 {
		t.Fatalf("Ping() RTT = %v, want non-negative duration", rtt)
	}
}

func TestPingUnknownPeer(t *testing.T) {
	a := testNode()

	runCtx, stop := context.WithCancel(t.Context())
	defer stop()
	go a.Run(runCtx)

	_, err := a.Ping(t.Context(), uuid.NewV4())
	if !errors.Is(err, ErrUnknownPeer) {
		t.Fatalf("Ping() error = %v, want ErrUnknownPeer", err)
	}
}

func TestPingTimesOutAndRemovesPendingEntry(t *testing.T) {
	network := transport.NewMemNetwork()
	aAddr := netip.MustParseAddrPort("127.0.0.1:7001")
	unreachableAddr := netip.MustParseAddrPort("127.0.0.1:7002")

	a := NewNode(NodeConfig{ListenAddr: aAddr}, network.Join(aAddr), testLogger())

	peer := wire.Peer{
		ID:      uuid.NewV4(),
		Address: unreachableAddr,
	}
	a.reg.merge(wire.Record{Peer: peer, Status: wire.StatusAlive}, time.Now())

	runCtx, stop := context.WithCancel(t.Context())
	stopped := make(chan struct{})
	go func() {
		a.Run(runCtx)
		close(stopped)
	}()

	pingCtx, cancel := context.WithTimeout(t.Context(), 25*time.Millisecond)
	defer cancel()

	_, err := a.Ping(pingCtx, peer.ID)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Ping() error = %v, want context deadline exceeded", err)
	}

	// The loop drops expired entries on its next tick; stop it and run that tick here.
	stop()
	<-stopped
	a.tick(time.Now())

	if pendingCount := len(a.pendingPing); pendingCount != 0 {
		t.Fatalf("pending Ping entries = %d, want 0", pendingCount)
	}
}

func testLogger() *slog.Logger {
	return slog.New(slog.DiscardHandler)
}
