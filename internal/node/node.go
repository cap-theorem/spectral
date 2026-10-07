package node

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/netip"
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
	n.logger.Info("node started", "id", n.self.ID, "address", n.self.Address)

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
	switch msg := env.Msg.(type) {
	case wire.Ping:
		n.logger.Debug("ping received", "from", env.From.ID)
		n.send(env.From, wire.Pong{Nonce: msg.Nonce})
	case wire.Pong:
		n.logger.Info("pong received", "from", env.From.ID, "address", env.From.Address)
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
