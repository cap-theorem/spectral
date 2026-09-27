package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/cap-theorem/spectral/internal/node"
)

func (a *API) handleRegister(w http.ResponseWriter, r *http.Request) {
	if err := a.enqueueRequest(r.Context(), node.RegisterRequest{
		Ctx: r.Context(),
		Registration: node.Registration{
			Service:    "payments-api",
			InstanceID: "payments-api-1",
			Endpoint:   "http://localhost:8081",
			TTL:        30 * time.Second,
		},
	}); err != nil {
		writeError(w, http.StatusGatewayTimeout, err)
		return
	}

	w.WriteHeader(http.StatusAccepted)
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
