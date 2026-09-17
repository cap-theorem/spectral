package node

import "context"

type message struct{}

type Actor struct {
	inbox chan message
}

func NewActor() Actor {
	return Actor{
		inbox: make(chan message, 100),
	}
}

func (a *Actor) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-a.inbox:
			// Do something
			println(msg)
		}
	}
}
