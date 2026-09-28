package node

import (
	"context"
	"errors"
)

var (
	ErrRegistrationAlreadyExists = errors.New("this registration already exists")
	ErrInvalidRegistration       = errors.New("invalid registration")
	ErrTokenGenerationFail       = errors.New("failed to generate token")
)

// Request is an API request submitted to the node actor from the local API.
type Request interface {
	isRequest()
}

type Result[T any] struct {
	Value T
	Err   error
}

type RegisterRequest struct {
	Ctx          context.Context
	Registration Registration
	Reply        chan Result[Lease]
}

type RenewRequest struct {
	Ctx   context.Context
	Token string
	Reply chan Result[Lease]
}

type LookupRequest struct {
	Ctx     context.Context
	Service string
	Reply   chan Result[Provider]
}

type StatusRequest struct {
	Ctx   context.Context
	Reply chan Result[Status]
}

func (RegisterRequest) isRequest() {}
func (RenewRequest) isRequest()    {}
func (LookupRequest) isRequest()   {}
func (StatusRequest) isRequest()   {}
