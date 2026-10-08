package transport

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"io"
	"log/slog"
	"math/big"
	"net"
	"net/netip"
	"sync"
	"time"

	"github.com/cap-theorem/spectral/internal/wire"
	"github.com/quic-go/quic-go"
)

var _ Transport = (*Quic)(nil)

type QuicConfig struct {
	ListenAddr netip.AddrPort
}

type Quic struct {
	// conns is the connections cache, it does **not** represent a node's neighbors
	conns map[netip.AddrPort]*quic.Conn
	mu    sync.Mutex

	udp  *net.UDPConn
	ln   *quic.Listener
	tr   quic.Transport
	tls  tls.Config
	qcfg quic.Config

	// Some context stuff
	ctx    context.Context
	cancel context.CancelFunc

	recv chan wire.Envelope

	logger *slog.Logger
}

func NewQuic(cfg QuicConfig, logger *slog.Logger) (*Quic, error) {
	cert, err := selfSignedCert()
	if err != nil {
		return nil, err
	}

	udp, err := net.ListenUDP("udp", net.UDPAddrFromAddrPort(cfg.ListenAddr))
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())
	quic := &Quic{
		conns: make(map[netip.AddrPort]*quic.Conn),

		udp: udp,
		tr:  quic.Transport{Conn: udp},
		tls: tls.Config{
			Certificates:       []tls.Certificate{cert},
			NextProtos:         []string{"spectral/1"},
			InsecureSkipVerify: true,
		},
		qcfg: quic.Config{
			MaxIdleTimeout: time.Minute * 1,
		},
		ctx:    ctx,
		cancel: cancel,
		recv:   make(chan wire.Envelope, 500),

		logger: logger,
	}

	quic.ln, err = quic.tr.Listen(&quic.tls, &quic.qcfg)
	if err != nil {
		cancel()
		udp.Close()
		return nil, err
	}

	go quic.acceptConns()

	return quic, nil
}

func (q *Quic) Send(to netip.AddrPort, env wire.Envelope) {
	b, err := wire.Encode(env)
	if err != nil {
		q.logger.Warn("encode failed", "to", to, "env", env)
		return
	}

	ctx, cancel := context.WithTimeout(q.ctx, time.Second*10)
	defer cancel()

	conn, err := q.getConn(ctx, to)
	if err != nil {
		q.logger.Debug("getConn failed", "err", err)
		return
	}

	s, err := conn.OpenUniStreamSync(ctx)
	if err != nil {
		q.logger.Debug("open stream failed", "err", err)
		return
	}

	s.SetWriteDeadline(time.Now().Add(time.Second * 5))
	if _, err := s.Write(b); err != nil {
		s.CancelWrite(0)
		q.logger.Debug("failed to write", "err", err)
		return
	}

	s.Close()
}

func (q *Quic) Recv() <-chan wire.Envelope {
	return q.recv
}

func (q *Quic) getConn(ctx context.Context, to netip.AddrPort) (*quic.Conn, error) {
	q.mu.Lock()
	conn, ok := q.conns[to]
	q.mu.Unlock()

	if ok && conn.Context().Err() == nil {
		return conn, nil
	}

	conn, err := q.tr.Dial(ctx, net.UDPAddrFromAddrPort(to), &q.tls, &q.qcfg)
	if err != nil {
		q.logger.Warn("failed to dial quic connection", "to", to)
		return nil, err
	}

	q.mu.Lock()
	q.conns[to] = conn
	q.mu.Unlock()

	return conn, nil
}

func (q *Quic) registerConn(addr netip.AddrPort, conn *quic.Conn) {
	q.mu.Lock()
	q.conns[addr] = conn
	q.mu.Unlock()

	go q.startConn(addr, conn)
}

func (q *Quic) unregisterConn(addr netip.AddrPort, conn *quic.Conn) {
	q.mu.Lock()
	defer q.mu.Unlock()

	currConn, ok := q.conns[addr]
	if !ok {
		return
	}

	if currConn == conn {
		delete(q.conns, addr)
	}

}

func (q *Quic) startConn(addr netip.AddrPort, conn *quic.Conn) {
	// This runs when the stream timesout exiting the loop below
	defer q.unregisterConn(addr, conn)

	for {
		s, err := conn.AcceptUniStream(q.ctx)
		if err != nil {
			return
		}
		go q.readStream(s)
	}
}

func (q *Quic) readStream(s *quic.ReceiveStream) {
	s.SetReadDeadline(time.Now().Add(time.Second * 10))

	b, err := io.ReadAll(io.LimitReader(s, wire.MaxFrame+1))
	if err != nil {
		s.CancelRead(0)
		return
	}

	env, err := wire.Decode(b)
	if err != nil {
		q.logger.Debug("decode failed", "err", err)
	}

	select {
	case q.recv <- env:
	default:
		q.logger.Debug("recv channel full, dropping", "env", env)
	}
}

func (q *Quic) acceptConns() {
	for {
		conn, err := q.ln.Accept(q.ctx)
		if err != nil {
			return
		}

		addr := conn.RemoteAddr().(*net.UDPAddr).AddrPort()
		// Unmap normalizes IPv4-mapped IPv6 addrs so cache keys are consistent
		q.registerConn(netip.AddrPortFrom(addr.Addr().Unmap(), addr.Port()), conn)
	}
}

func selfSignedCert() (tls.Certificate, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return tls.Certificate{}, err
	}

	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour * 24 * 365),
	}

	cert, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, pub, priv)
	if err != nil {
		return tls.Certificate{}, err
	}

	return tls.Certificate{
		Certificate: [][]byte{cert},
		PrivateKey:  priv,
	}, nil
}
