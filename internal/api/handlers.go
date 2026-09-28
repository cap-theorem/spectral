package api

import (
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

	reply := make(chan node.Result[node.Lease], 1)
	if err := a.enqueueRequest(r.Context(), node.RegisterRequest{
		Ctx: r.Context(),
		Registration: node.Registration{
			Service:    reg.Service,
			InstanceID: reg.InstanceID,
			Endpoint:   reg.Endpoint,
			TTL:        ttl,
		},
		Reply: reply,
	}); err != nil {
		writeError(w, http.StatusGatewayTimeout, err)
		return
	}

	var res node.Result[node.Lease]
	select {
	case res = <-reply:
	case <-r.Context().Done():
		writeError(w, http.StatusGatewayTimeout, r.Context().Err())
		return
	}
	if res.Err != nil {
		status := http.StatusInternalServerError
		switch {
		case errors.Is(res.Err, node.ErrInvalidRegistration):
			status = http.StatusBadRequest
		case errors.Is(res.Err, node.ErrRegistrationAlreadyExists):
			status = http.StatusConflict
		}
		writeError(w, status, res.Err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(LeaseResponse{
		Token:     res.Value.Token,
		TTL:       res.Value.TTL.String(),
		ExpiresAt: res.Value.ExpiresAt,
	}); err != nil {
		a.logger.Error("write registration response", "error", err)
	}
}

func (a *API) handleRenew(w http.ResponseWriter, r *http.Request) {
	if err := a.enqueueRequest(r.Context(), node.RenewRequest{
		Ctx:   r.Context(),
		Token: "mock-lease-token",
	}); err != nil {
		writeError(w, http.StatusGatewayTimeout, err)
		return
	}

	w.WriteHeader(http.StatusAccepted)
}

func (a *API) handleLookup(w http.ResponseWriter, r *http.Request) {
	reply := make(chan node.Result[node.Provider], 1)
	if err := a.enqueueRequest(r.Context(), node.LookupRequest{
		Ctx:     r.Context(),
		Service: "payments-api",
		Reply:   reply,
	}); err != nil {
		writeError(w, http.StatusGatewayTimeout, err)
		return
	}

	select {
	case result := <-reply:
		if result.Err != nil {
			writeError(w, http.StatusInternalServerError, result.Err)
			return
		}
		writeJSON(w, ProviderResponse{
			Service:    result.Value.Service,
			InstanceID: result.Value.InstanceID,
			Endpoint:   result.Value.Endpoint,
		})
	case <-r.Context().Done():
		writeError(w, http.StatusGatewayTimeout, r.Context().Err())
	}
}

func (a *API) handleStatus(w http.ResponseWriter, r *http.Request) {
	reply := make(chan node.Result[node.Status], 1)
	if err := a.enqueueRequest(r.Context(), node.StatusRequest{
		Ctx:   r.Context(),
		Reply: reply,
	}); err != nil {
		writeError(w, http.StatusGatewayTimeout, err)
		return
	}

	select {
	case result := <-reply:
		if result.Err != nil {
			writeError(w, http.StatusInternalServerError, result.Err)
			return
		}
		writeJSON(w, StatusResponse{
			Ready: result.Value.Ready,
			State: string(result.Value.State),
		})
	case <-r.Context().Done():
		writeError(w, http.StatusGatewayTimeout, r.Context().Err())
	}
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
