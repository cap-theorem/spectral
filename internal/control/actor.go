package control

import "context"

type Actor struct {
	inbox <-chan struct{}
}

func NewActor() *Actor {
	return &Actor{
		inbox: make(<-chan struct{}),
	}
}

func (a *Actor) Inbox() <-chan struct{} {
	return a.inbox
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

func (a *Actor) handle(x struct{}) {
	// Do something here
}
