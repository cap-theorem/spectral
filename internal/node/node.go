package node

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"log/slog"
	"net/netip"
	"sync"
	"time"
	"uuid"

	"github.com/cap-theorem/spectral/internal/transport"
	"github.com/cap-theorem/spectral/internal/wire"
)

type Node struct {
	self wire.Peer

	registrations map[RegistrationKey]RegistrationEntry

	inbox     chan request
	transport transport.Transport
	logger    *slog.Logger

	peersMu sync.RWMutex
	peers   map[wire.NodeID]wire.Peer

	pingMu      sync.Mutex
	pendingPing map[uint64]pendingPing
}

type pendingPing struct {
	peerID  wire.NodeID
	started time.Time
	reply   chan time.Duration
}

type NodeConfig struct {
	ListenAddr netip.AddrPort
}

func NewNode(cfg NodeConfig, tr transport.Transport, logger *slog.Logger) *Node {
	return &Node{
		self: wire.Peer{
			ID:      uuid.NewV4(),
			Address: cfg.ListenAddr,
		},
		registrations: make(map[RegistrationKey]RegistrationEntry),
		inbox:         make(chan request, 100),
		transport:     tr,
		logger:        logger,
		peers:         make(map[wire.NodeID]wire.Peer),
		pendingPing:   make(map[uint64]pendingPing),
	}
}

func (n *Node) Register(ctx context.Context, reg Registration) (Lease, error) {
	req := registerRequest{Registration: reg, Reply: make(chan result[Lease], 1)}

	select {
	case n.inbox <- req:
	case <-ctx.Done():
		return Lease{}, ctx.Err()
	}

	select {
	case res := <-req.Reply:
		return res.Value, res.Err
	case <-ctx.Done():
		return Lease{}, ctx.Err()
	}
}

func (n *Node) Renew(ctx context.Context, token string) error {
	select {
	case n.inbox <- renewRequest{Token: token}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (n *Node) Lookup(ctx context.Context, service string) (Provider, error) {
	req := lookupRequest{Service: service, Reply: make(chan result[Provider], 1)}

	select {
	case n.inbox <- req:
	case <-ctx.Done():
		return Provider{}, ctx.Err()
	}

	select {
	case res := <-req.Reply:
		return res.Value, res.Err
	case <-ctx.Done():
		return Provider{}, ctx.Err()
	}
}

func (n *Node) Status(ctx context.Context) (Status, error) {
	req := statusRequest{Reply: make(chan result[Status], 1)}

	select {
	case n.inbox <- req:
	case <-ctx.Done():
		return Status{}, ctx.Err()
	}

	select {
	case res := <-req.Reply:
		return res.Value, res.Err
	case <-ctx.Done():
		return Status{}, ctx.Err()
	}
}

func (n *Node) Run(ctx context.Context) {
	n.logger.Info("Node started", "id", n.self.ID, "address", n.self.Address)

	for {
		select {
		case <-ctx.Done():
			return
		case req := <-n.inbox:
			n.handleRequest(req)
		case env := <-n.transport.Recv():
			n.handleEnvelope(env)
		}
	}
}

func (n *Node) handleRequest(req request) {
	switch req := req.(type) {
	case registerRequest:
		n.logger.Info("registration received", "endpoint", req.Registration.Endpoint)
		lease, err := n.registerService(req.Registration)
		req.Reply <- result[Lease]{Value: lease, Err: err}
	case renewRequest:
		n.logger.Info("renew request received", "token", req.Token)
	case lookupRequest:
		n.logger.Info("lookup request received", "service", req.Service)
		req.Reply <- result[Provider]{Value: Provider{Service: req.Service}}
	case statusRequest:
		n.logger.Info("status request received")
		req.Reply <- result[Status]{Value: Status{Ready: true, State: StateActive}}
	}
}

func (n *Node) handleEnvelope(env wire.Envelope) {
	n.rememberPeer(env.From)

	switch msg := env.Msg.(type) {
	case wire.Ping:
		n.logger.Debug("ping received", "from", env.From.ID)
		n.send(env.From, wire.Pong{Nonce: msg.Nonce})
	case wire.Pong:
		n.logger.Info("pong received", "from", env.From.ID, "address", env.From.Address)
		n.pingMu.Lock()
		pending, ok := n.pendingPing[msg.Nonce]
		if ok && pending.peerID == env.From.ID && env.To == n.self.ID {
			delete(n.pendingPing, msg.Nonce)
		} else {
			ok = false
		}
		n.pingMu.Unlock()

		if ok {
			pending.reply <- time.Since(pending.started)
		}
	}
}

func (n *Node) send(to wire.Peer, msg wire.Message) {
	n.transport.Send(to.Address, wire.Envelope{
		From: n.self,
		To:   to.ID,
		Msg:  msg,
	})
}

func (n *Node) registerService(reg Registration) (Lease, error) {
	if err := reg.Validate(); err != nil {
		return Lease{}, err
	}

	key := RegistrationKey{
		Service:    reg.Service,
		InstanceID: reg.InstanceID,
	}
	now := time.Now()
	if existing, ok := n.registrations[key]; ok && now.Before(existing.Lease.ExpiresAt) {
		return Lease{}, ErrRegistrationAlreadyExists
	}

	token, err := generateHexToken(32)
	if err != nil {
		return Lease{}, ErrTokenGenerationFail
	}

	lease := Lease{
		Token:     token,
		ExpiresAt: now.Add(reg.TTL),
		TTL:       reg.TTL,
	}
	n.registrations[key] = RegistrationEntry{
		Lease: lease,
		Provider: Provider{
			Service:    reg.Service,
			InstanceID: reg.InstanceID,
			Endpoint:   reg.Endpoint,
		},
	}
	return lease, nil
}

func generateHexToken(byteLength int) (string, error) {
	b := make([]byte, byteLength)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func (n *Node) PeerByID(id wire.NodeID) (wire.Peer, bool) {
	n.peersMu.RLock()
	defer n.peersMu.RUnlock()

	peer, ok := n.peers[id]
	return peer, ok
}

func (n *Node) rememberPeer(peer wire.Peer) {
	if !peer.Address.IsValid() {
		return
	}

	n.peersMu.Lock()
	n.peers[peer.ID] = peer
	n.peersMu.Unlock()
}

func (n *Node) Ping(ctx context.Context, to wire.Peer) (time.Duration, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return 0, err
	}
	nonce := binary.LittleEndian.Uint64(b[:])

	reply := make(chan time.Duration, 1)
	pending := pendingPing{
		peerID:  to.ID,
		started: time.Now(),
		reply:   reply,
	}

	n.pingMu.Lock()
	n.pendingPing[nonce] = pending
	n.pingMu.Unlock()

	defer func() {
		n.pingMu.Lock()
		delete(n.pendingPing, nonce)
		n.pingMu.Unlock()
	}()

	n.send(to, wire.Ping{Nonce: nonce})

	select {
	case rtt := <-reply:
		return rtt, nil
	case <-ctx.Done():
		return 0, ctx.Err()
	}
}
