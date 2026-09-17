package node

import "time"

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
