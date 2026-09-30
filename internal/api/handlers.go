package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/cap-theorem/spectral/internal/node"
)

func (a *API) handleRegister(w http.ResponseWriter, r *http.Request) {
	var reg RegistrationRequest
	if err := json.NewDecoder(r.Body).Decode(&reg); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	ttl, err := time.ParseDuration(reg.TTL)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	lease, err := a.node.Register(r.Context(), node.Registration{
		Service:    reg.Service,
		InstanceID: reg.InstanceID,
		Endpoint:   reg.Endpoint,
		TTL:        ttl,
	})
	if err != nil {
		status := http.StatusInternalServerError
		switch {
		case errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
			status = http.StatusGatewayTimeout
		case errors.Is(err, node.ErrInvalidRegistration):
			status = http.StatusBadRequest
		case errors.Is(err, node.ErrRegistrationAlreadyExists):
			status = http.StatusConflict
		}
		writeError(w, status, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(LeaseResponse{
		Token:     lease.Token,
		TTL:       lease.TTL.String(),
		ExpiresAt: lease.ExpiresAt,
	}); err != nil {
		a.logger.Error("write registration response", "error", err)
	}
}

func (a *API) handleRenew(w http.ResponseWriter, r *http.Request) {
	if err := a.node.Renew(r.Context(), "mock-lease-token"); err != nil {
		writeError(w, http.StatusGatewayTimeout, err)
		return
	}

	w.WriteHeader(http.StatusAccepted)
}

func (a *API) handleLookup(w http.ResponseWriter, r *http.Request) {
	provider, err := a.node.Lookup(r.Context(), "payments-api")
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			status = http.StatusGatewayTimeout
		}
		writeError(w, status, err)
		return
	}

	writeJSON(w, ProviderResponse{
		Service:    provider.Service,
		InstanceID: provider.InstanceID,
		Endpoint:   provider.Endpoint,
	})
}

func (a *API) handleStatus(w http.ResponseWriter, r *http.Request) {
	status, err := a.node.Status(r.Context())
	if err != nil {
		code := http.StatusInternalServerError
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			code = http.StatusGatewayTimeout
		}
		writeError(w, code, err)
		return
	}

	writeJSON(w, StatusResponse{
		Ready: status.Ready,
		State: string(status.State),
	})
}

func writeError(w http.ResponseWriter, status int, err error) {
	http.Error(w, err.Error(), status)
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		writeError(w, http.StatusInternalServerError, err)
	}
}
