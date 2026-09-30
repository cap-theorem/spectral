package node

import (
	"errors"
)

var (
	ErrRegistrationAlreadyExists = errors.New("this registration already exists")
	ErrInvalidRegistration       = errors.New("invalid registration")
	ErrTokenGenerationFail       = errors.New("failed to generate token")
)

// request is a message submitted to the node's inbox by one of its exported methods.
type request interface {
	isRequest()
}

type result[T any] struct {
	Value T
	Err   error
}

type registerRequest struct {
	Registration Registration
	Reply        chan result[Lease]
}

type renewRequest struct {
	Token string
}

type lookupRequest struct {
	Service string
	Reply   chan result[Provider]
}

type statusRequest struct {
	Reply chan result[Status]
}

func (registerRequest) isRequest() {}
func (renewRequest) isRequest()    {}
func (lookupRequest) isRequest()   {}
func (statusRequest) isRequest()   {}
