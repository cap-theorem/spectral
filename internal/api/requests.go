package api

import (
	"time"
)

type RegistrationRequest struct {
	Service    string        `json:"service"`
	InstanceID string        `json:"instance_id"`
	Endpoint   string        `json:"endpoint"`
	TTL        time.Duration `json:"ttl"`
}

type LeaseResponse struct {
	Token     string        `json:"token"`
	TTL       time.Duration `json:"ttl"`
	ExpiresAt time.Time     `json:"expires_at"`
}

type ProviderResponse struct {
	Service    string `json:"service"`
	InstanceID string `json:"instance_id"`
	Endpoint   string `json:"endpoint"`
}

type StatusResponse struct {
	Ready bool   `json:"ready"`
	State string `json:"state"`
}
