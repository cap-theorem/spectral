package transport

import (
	"log/slog"
	"net"
	"net/netip"
	"testing"
	"time"

	"github.com/cap-theorem/spectral/internal/wire"
)

func TestQuicReplyOverDialedConn(t *testing.T) {
	a, aAddr := newTestQuic(t)
	b, bAddr := newTestQuic(t)

	a.Send(bAddr, wire.Envelope{Msg: wire.Ping{Nonce: 1}})
	env := recvWithin(t, b, 2*time.Second)
	if env.Msg != (wire.Ping{Nonce: 1}) {
		t.Fatalf("b got %#v, want Ping{Nonce: 1}", env.Msg)
	}

	b.Send(aAddr, wire.Envelope{Msg: wire.Pong{Nonce: 1}})
	env = recvWithin(t, a, 2*time.Second)
	if env.Msg != (wire.Pong{Nonce: 1}) {
		t.Fatalf("a got %#v, want Pong{Nonce: 1}", env.Msg)
	}
}

func newTestQuic(t *testing.T) (*Quic, netip.AddrPort) {
	t.Helper()

	q, err := NewQuic(
		QuicConfig{ListenAddr: netip.MustParseAddrPort("127.0.0.1:0")},
		slog.New(slog.DiscardHandler),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { q.Close() })

	return q, q.udp.LocalAddr().(*net.UDPAddr).AddrPort()
}

func recvWithin(t *testing.T, q *Quic, d time.Duration) wire.Envelope {
	t.Helper()

	select {
	case env := <-q.Recv():
		return env
	case <-time.After(d):
		t.Fatal("timed out waiting for envelope")
		return wire.Envelope{}
	}
}
