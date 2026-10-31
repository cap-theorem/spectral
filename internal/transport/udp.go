package transport

import (
	"errors"
	"log/slog"
	"net"
	"net/netip"

	"github.com/cap-theorem/spectral/internal/wire"
)

const (
	udpRecvBufferSize = 1024
)

var _ Transport = (*UDP)(nil)

type UDPConfig struct {
	ListenAddr netip.AddrPort
}

type UDP struct {
	udp *net.UDPConn

	recv chan wire.Envelope

	logger *slog.Logger
}

func NewUDP(cfg UDPConfig, logger *slog.Logger) (*UDP, error) {
	udp, err := net.ListenUDP("udp", net.UDPAddrFromAddrPort(cfg.ListenAddr))
	if err != nil {
		return nil, err
	}

	u := &UDP{
		udp:  udp,
		recv: make(chan wire.Envelope, udpRecvBufferSize),

		logger: logger,
	}

	go u.readPackets()

	return u, nil
}

func (u *UDP) Send(to netip.AddrPort, env wire.Envelope) {
	b, err := wire.Encode(env)
	if err != nil {
		u.logger.Warn("encode failed", "to", to, "env", env)
		return
	}

	if _, err := u.udp.WriteToUDPAddrPort(b, to); err != nil {
		u.logger.Debug("failed to write", "to", to, "err", err)
	}
}

func (u *UDP) Recv() <-chan wire.Envelope {
	return u.recv
}

func (u *UDP) Close() error {
	return u.udp.Close()
}

func (u *UDP) readPackets() {
	buf := make([]byte, wire.MaxFrame)

	for {
		n, _, err := u.udp.ReadFromUDPAddrPort(buf)
		if errors.Is(err, net.ErrClosed) {
			return
		}
		if err != nil {
			u.logger.Debug("read failed", "err", err)
			continue
		}

		env, err := wire.Decode(buf[:n])
		if err != nil {
			u.logger.Debug("decode failed", "err", err)
			continue
		}

		select {
		case u.recv <- env:
		default:
			u.logger.Debug("recv channel full, dropping", "env", env)
		}
	}
}
