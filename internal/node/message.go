package node

import "context"

type Message interface {
	isMessage()
}

type result[T any] struct {
	value T
	err   error
}

type registerCommand struct {
	ctx          context.Context
	registration Registeration
	reply        chan result[Lease]
}

type renewCommand struct {
	ctx   context.Context
	token string
	reply chan result[Lease]
}

type lookupCommand struct {
	ctx     context.Context
	service string
	reply   chan result[Provider]
}

type statusCommand struct {
	ctx   context.Context
	reply chan result[Status]
}

func (registerCommand) isMessage() {}
func (renewCommand) isMessage()    {}
func (lookupCommand) isMessage()   {}
func (statusCommand) isMessage()   {}
