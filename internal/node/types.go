package node

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type State string

const (
	StateJoining State = "joining"
	StateActive  State = "active"
	StateLeaving State = "leaving"
	StateLeft    State = "left"
)

type RegistrationKey struct {
	Service    string
	InstanceID string
}

type RegistrationEntry struct {
	Provider Provider
	Lease    Lease
}

type Registration struct {
	Service    string
	InstanceID string
	Endpoint   string
	TTL        time.Duration
}

func (r Registration) Validate() error {
	if strings.TrimSpace(r.Service) == "" || strings.TrimSpace(r.InstanceID) == "" {
		return fmt.Errorf("%w: service and instance_id are required", ErrInvalidRegistration)
	}
	if r.TTL <= 0 {
		return fmt.Errorf("%w: ttl must be positive", ErrInvalidRegistration)
	}
	endpoint, err := url.Parse(r.Endpoint)
	if err != nil || (endpoint.Scheme != "http" && endpoint.Scheme != "https") || endpoint.Hostname() == "" {
		return fmt.Errorf("%w: endpoint must be an HTTP(S) URL with a host", ErrInvalidRegistration)
	}
	if strings.HasSuffix(endpoint.Host, ":") {
		return fmt.Errorf("%w: endpoint port must be between 1 and 65535", ErrInvalidRegistration)
	}
	if port := endpoint.Port(); port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			return fmt.Errorf("%w: endpoint port must be between 1 and 65535", ErrInvalidRegistration)
		}
	}
	return nil
}

type Lease struct {
	Token     string        `json:"token"`
	TTL       time.Duration `json:"ttl"`
	ExpiresAt time.Time     `json:"expires_at"`
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
