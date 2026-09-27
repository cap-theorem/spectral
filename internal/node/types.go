package node

import "time"

type State string

const (
	StateJoining State = "joining"
	StateActive  State = "active"
	StateLeaving State = "leaving"
	StateLeft    State = "left"
)

type Registration struct {
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

type Provider struct {
	Service    string
	InstanceID string
	Endpoint   string
}

type Status struct {
	Ready bool
	State State
}
