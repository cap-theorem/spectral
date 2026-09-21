package node

import (
	"context"
	"log/slog"
	"time"
)

type State string

const (
	StateJoining State = "joining"
	StateActive  State = "active"
	StateLeaving State = "leaving"
	StateLeft    State = "left"
)

type Registeration struct {
	Service    string
	InstanceID string
	Endpoint   string
	TTL        time.Duration
}

type Lease struct {
	Token     string
	TTL       time.Duration
	ExpiresAt time.Time
}

// TODO: Move this out to another file
type Provider struct {
	Service    string
	InstanceID string
	Endpoint   string
}

// TODO: Move this out to another file
type Status struct {
	// Ready is a mix of state = active, transport ready, and live neighbors/peers > 0
	Ready bool
	State State
}

type Actor struct {
	inbox  chan Message
	logger *slog.Logger
}

func NewActor(logger *slog.Logger) *Actor {
	return &Actor{
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
		a.logger.Info("registration recieved", "endpoint", msg.registration.Endpoint)
	case renewCommand:
		a.logger.Info("", "token", msg.token)
	case lookupCommand:
		a.logger.Info("", "service", msg.service)
	case statusCommand:
		a.logger.Info("status command hit")
	}
}

func (a *Actor) Register(ctx context.Context, reg Registeration) {}
func (a *Actor) Lookup(ctx context.Context, service string) (Provider, error) {
	replyChan := make(chan result[Provider], 1)

	a.inbox <- lookupCommand{
		ctx:     ctx,
		service: service,
		reply:   replyChan,
	}

	// Block until we get response
	result := <-replyChan
	if result.err != nil {
		return Provider{}, result.err
	}

	return result.value, nil
}
func (a *Actor) Renew(ctx context.Context, token string) {}
func (a *Actor) Status(ctx context.Context)              {}
