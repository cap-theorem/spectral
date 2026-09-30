package node

import (
	"errors"
	"testing"
	"time"
)

func TestRegistrationValidate(t *testing.T) {
	valid := Registration{
		Service:    "payments",
		InstanceID: "payments-1",
		Endpoint:   "http://10.0.0.1:8080",
		TTL:        10 * time.Second,
	}

	tests := []struct {
		name    string
		edit    func(r *Registration)
		wantErr bool
	}{
		{"valid", func(r *Registration) {}, false},
		{"https with path", func(r *Registration) { r.Endpoint = "https://pay.svc:443/v1" }, false},
		{"ipv6 host", func(r *Registration) { r.Endpoint = "http://[::1]:8080" }, false},
		{"blank service", func(r *Registration) { r.Service = " " }, true},
		{"blank instance", func(r *Registration) { r.InstanceID = "" }, true},
		{"zero ttl", func(r *Registration) { r.TTL = 0 }, true},
		{"negative ttl", func(r *Registration) { r.TTL = -time.Second }, true},
		{"no scheme", func(r *Registration) { r.Endpoint = "10.0.0.1:80" }, true},
		{"wrong scheme", func(r *Registration) { r.Endpoint = "tcp://x:80" }, true},
		{"no host", func(r *Registration) { r.Endpoint = "http://:80" }, true},
		{"port 0", func(r *Registration) { r.Endpoint = "http://x:0" }, true},
		{"port too big", func(r *Registration) { r.Endpoint = "http://x:70000" }, true},
		{"trailing colon", func(r *Registration) { r.Endpoint = "http://x:" }, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := valid
			tt.edit(&reg)

			err := reg.Validate()
			if (err != nil) != tt.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil && !errors.Is(err, ErrInvalidRegistration) {
				t.Fatalf("Validate() error = %v, want ErrInvalidRegistration", err)
			}
		})
	}
}
