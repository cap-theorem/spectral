package node

import (
	"context"
	"log/slog"

	"github.com/cap-theorem/spectral/internal/wire"
)

type Node struct {
	self wire.Peer

	requests <-chan Request
	logger   *slog.Logger
}

func NewNode(logger *slog.Logger, requests <-chan Request) *Node {
	return &Node{
		requests: requests,
		logger:   logger,
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
