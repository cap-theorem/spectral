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

func TestNodeRepliesToPing(t *testing.T) {
	network := transport.NewMemNetwork()
	nodeAddr := netip.MustParseAddrPort("127.0.0.1:7001")
	peerAddr := netip.MustParseAddrPort("127.0.0.1:7002")

	n := NewNode(NodeConfig{ListenAddr: nodeAddr}, network.Join(nodeAddr), slog.New(slog.DiscardHandler))
	peer := network.Join(peerAddr)
	go n.Run(t.Context())

	from := wire.Peer{ID: uuid.NewV4(), Address: peerAddr}
	peer.Send(nodeAddr, wire.Envelope{From: from, Msg: wire.Ping{Nonce: 42}})

	select {
	case env := <-peer.Recv():
		pong, ok := env.Msg.(wire.Pong)
		if !ok || pong.Nonce != 42 {
			t.Fatalf("got %#v, want Pong{Nonce: 42}", env.Msg)
		}
		if env.To != from.ID || env.From.Address != nodeAddr {
			t.Fatalf("got to=%v from=%v, want to=%v from=%v", env.To, env.From.Address, from.ID, nodeAddr)
		}
	case <-time.After(time.Second):
		t.Fatal("no pong")
	}
}
