package api

import (
	"time"
)

type RegistrationRequest struct {
	Service  string `json:"service"`
	Endpoint string `json:"endpoint"`
	TTL      string `json:"ttl"`
}

type RenewRequest struct {
	Token string `json:"token"`
}

type LeaseResponse struct {
	Token     string    `json:"token"`
	TTL       string    `json:"ttl"`
	ExpiresAt time.Time `json:"expires_at"`
}

type ProviderResponse struct {
	Service  string `json:"service"`
	Endpoint string `json:"endpoint"`
}

type StatusResponse struct {
	Ready bool   `json:"ready"`
	State string `json:"state"`
}
