package node

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"time"

	"github.com/cap-theorem/spectral/internal/wire"
)

type Node struct {
	self wire.Peer

	registrations map[RegistrationKey]RegistrationEntry

	requests <-chan Request
	logger   *slog.Logger
}

func NewNode(logger *slog.Logger, requests <-chan Request) *Node {
	return &Node{
		registrations: make(map[RegistrationKey]RegistrationEntry),
		requests:      requests,
		logger:        logger,
	}
}

func (a *Node) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case req := <-a.requests:
			a.handleRequest(req)
		}
	}
}

func (a *Node) handleRequest(req Request) {
	switch req := req.(type) {
	case RegisterRequest:
		a.logger.Info("registration received", "endpoint", req.Registration.Endpoint)
		lease, err := a.registerService(req.Registration)
		req.Reply <- Result[Lease]{Value: lease, Err: err}
	case RenewRequest:
		a.logger.Info("renew request received", "token", req.Token)
	case LookupRequest:
		a.logger.Info("lookup request received", "service", req.Service)
		if req.Reply != nil {
			provider := Provider{Service: req.Service}
			req.Reply <- Result[Provider]{
				Value: provider,
			}
		}
	case StatusRequest:
		a.logger.Info("status request received")
		if req.Reply != nil {
			status := Status{Ready: true, State: StateActive}
			req.Reply <- Result[Status]{
				Value: status,
			}
		}
	}
}

func (a *Node) registerService(reg Registration) (Lease, error) {
	if err := reg.Validate(); err != nil {
		return Lease{}, err
	}

	key := RegistrationKey{
		Service:    reg.Service,
		InstanceID: reg.InstanceID,
	}
	now := time.Now()
	if existing, ok := a.registrations[key]; ok && now.Before(existing.Lease.ExpiresAt) {
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
	a.registrations[key] = RegistrationEntry{
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
