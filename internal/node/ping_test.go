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

	runCtx, stop := context.WithCancel(t.Context())
	defer stop()
	go a.Run(runCtx)
	go b.Run(runCtx)

	pingCtx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()

	rtt, err := a.Ping(pingCtx, b.self)
	if err != nil {
		t.Fatalf("Ping() error = %v", err)
	}
	if rtt < 0 {
		t.Fatalf("Ping() RTT = %v, want non-negative duration", rtt)
	}

	peer, ok := a.PeerByID(b.self.ID)
	if !ok {
		t.Fatal("A did not learn B from the Pong.")
	}
	if peer != b.self {
		t.Fatalf("A learned peer %#v, want %#v", peer, b.self)
	}
}

func TestPingTimesOutAndRemovesPendingEntry(t *testing.T) {
	network := transport.NewMemNetwork()
	aAddr := netip.MustParseAddrPort("127.0.0.1:7001")
	unreachableAddr := netip.MustParseAddrPort("127.0.0.1:7002")

	a := NewNode(NodeConfig{ListenAddr: aAddr}, network.Join(aAddr), testLogger())

	runCtx, stop := context.WithCancel(t.Context())
	defer stop()
	go a.Run(runCtx)

	peer := wire.Peer{
		ID:      uuid.NewV4(),
		Address: unreachableAddr,
	}

	pingCtx, cancel := context.WithTimeout(t.Context(), 25*time.Millisecond)
	defer cancel()

	_, err := a.Ping(pingCtx, peer)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Ping() error = %v, want context deadline exceeded", err)
	}

	a.pingMu.Lock()
	pendingCount := len(a.pendingPing)
	a.pingMu.Unlock()
	if pendingCount != 0 {
		t.Fatalf("pending Ping entries = %d, want 0", pendingCount)
	}
}

func testLogger() *slog.Logger {
	return slog.New(slog.DiscardHandler)
}
