// In-memory transport implementation used for unit tests.
// It runs on a single process and delivers everything quickly and in-order, so don't use this to simulate network conditions.
// Used to verify message response and effects on nodes.

package transport

import (
	"net/netip"
	"sync"

	"github.com/cap-theorem/spectral/internal/wire"
)

var _ Transport = (*Mem)(nil)

type MemNetwork struct {
	mu    sync.Mutex
	nodes map[netip.AddrPort]*Mem
}

func NewMemNetwork() *MemNetwork {
	return &MemNetwork{
		nodes: make(map[netip.AddrPort]*Mem),
	}
}

func (n *MemNetwork) Join(addr netip.AddrPort) *Mem {
	m := &Mem{
		network: n,
		recv:    make(chan wire.Envelope, 100),
	}

	n.mu.Lock()
	n.nodes[addr] = m
	n.mu.Unlock()

	return m
}

type Mem struct {
	network *MemNetwork
	recv    chan wire.Envelope
}

func (m *Mem) Send(to netip.AddrPort, env wire.Envelope) {
	m.network.mu.Lock()
	dst, ok := m.network.nodes[to]
	m.network.mu.Unlock()
	if !ok {
		return
	}

	b, err := wire.Encode(env)
	if err != nil {
		panic("mem transport: " + err.Error())
	}
	env, err = wire.Decode(b)
	if err != nil {
		panic("mem transport: " + err.Error())
	}

	select {
	case dst.recv <- env:
	default:
	}
}

func (m *Mem) Recv() <-chan wire.Envelope {
	return m.recv
}
