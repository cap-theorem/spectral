package node

import "context"

type Actor struct {
	inbox chan Message
}

func NewActor() Actor {
	return Actor{
		inbox: make(chan Message, 100),
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
		println(msg.registration)
	case renewCommand:
		println(msg.token)
	case lookupCommand:
		println(msg.service)
	case statusCommand:
		println("status command hit")
	}
}

func (a *Actor) Register(ctx context.Context, reg Registeration) {}
func (a *Actor) Lookup(ctx context.Context, service string)      {}
func (a *Actor) Renew(ctx context.Context, token string)         {}
func (a *Actor) Status(ctx context.Context)                      {}
