// In-memory transport for testing nodes in one process, without a real network.
// Messages use the real codec but arrive instantly and in order, so it does not test real network conditions.

package transport

import (
	"net/netip"
	"sync"

	"github.com/cap-theorem/spectral/internal/wire"
)

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
