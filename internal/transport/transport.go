package transport

import (
	"net/netip"

	"github.com/cap-theorem/spectral/internal/wire"
)

type Transport interface {
	Send(to netip.AddrPort, env wire.Envelope)
	Recv() <-chan wire.Envelope
}
