package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/cap-theorem/spectral/internal/node"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/httplog/v3"
)

type API struct {
	node   *node.Node
	server *http.Server
	logger *slog.Logger
}

func NewAPI(node *node.Node, logger *slog.Logger) *API {
	r := chi.NewRouter()
	r.Use(httplog.RequestLogger(logger, &httplog.Options{
		Level:  slog.LevelInfo,
		Schema: httplog.SchemaECS.Concise(true),
	}))
	r.Use(middleware.Timeout(5 * time.Second))

	r.Get("/", HandleHello)

	server := &http.Server{
		Addr:              ":8080",
		Handler:           r,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	return &API{
		node,
		server,
		logger,
	}
}

func (a *API) Serve() error {
	a.logger.Info("API server starting", "address", "http://localhost"+a.server.Addr)
	err := a.server.ListenAndServe()

	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}

	return err
}

func (a *API) HandleLookup(w http.ResponseWriter, r *http.Request) {
	a.node.Lookup(r.Context(), "payments-api")
}

func (a *API) Shutdown(ctx context.Context) error {
	return a.server.Shutdown(ctx)
}

func (a *API) Close() error {
	return a.server.Close()
}
