package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/cap-theorem/spectral/internal/node"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/httplog/v3"
)

type API struct {
	requests chan node.Request
	server   *http.Server
	logger   *slog.Logger
}

type APIConfig struct {
	Port int
}

func NewAPI(cfg APIConfig, logger *slog.Logger) *API {
	a := &API{
		requests: make(chan node.Request, 100),
		logger:   logger,
	}

	r := chi.NewRouter()
	r.Use(httplog.RequestLogger(logger, &httplog.Options{
		Level:  slog.LevelInfo,
		Schema: httplog.SchemaECS.Concise(true),
	}))
	r.Use(middleware.Timeout(5 * time.Second))

	r.Post("/register", a.handleRegister)
	r.Post("/renew", a.handleRenew)
	r.Get("/lookup", a.handleLookup)
	r.Get("/status", a.handleStatus)

	server := &http.Server{
		Addr:              ":" + strconv.Itoa(cfg.Port),
		Handler:           r,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	a.server = server
	return a
}

func (a *API) Requests() <-chan node.Request {
	return a.requests
}

func (a *API) Serve() error {
	a.logger.Info("API server starting", "address", "http://localhost"+a.server.Addr)
	err := a.server.ListenAndServe()

	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}

	return err
}

func (a *API) enqueueRequest(ctx context.Context, request node.Request) error {
	select {
	case a.requests <- request:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (a *API) Shutdown(ctx context.Context) error {
	return a.server.Shutdown(ctx)
}

func (a *API) Close() error {
	return a.server.Close()
}
