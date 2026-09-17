package node

import (
	"context"
	"log/slog"
)

type Actor struct {
	inbox  chan Message
	logger *slog.Logger
}

func NewActor(logger *slog.Logger) Actor {
	return Actor{
		inbox:  make(chan Message, 100),
		logger: logger,
	}
}

func (a *Actor) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-a.inbox:
			a.handle(msg)
		}
	}
}

func (a *Actor) handle(msg Message) {
	switch msg := msg.(type) {
	case registerCommand:
		a.logger.Info("endpoint", msg.registration.Endpoint)
	case renewCommand:
		a.logger.Info("token", msg.token)
	case lookupCommand:
		a.logger.Info("service", msg.service)
	case statusCommand:
		a.logger.Info("status command hit")
	}
}

func (a *Actor) Register(ctx context.Context, reg Registeration) {}
func (a *Actor) Lookup(ctx context.Context, service string)      {}
func (a *Actor) Renew(ctx context.Context, token string)         {}
func (a *Actor) Status(ctx context.Context)                      {}
