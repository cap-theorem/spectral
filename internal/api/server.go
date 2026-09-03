package api

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/httplog/v3"
	"github.com/golang-cz/devslog"
)

type Server struct {
	http     *http.Server
	listener net.Listener
	done     chan error
}

func NewServer() (Server, error) {
	logFormat := httplog.SchemaECS.Concise(true)
	r := chi.NewRouter()

	logger := slog.New(devslog.NewHandler(os.Stdout, &devslog.Options{
		SortKeys:           true,
		MaxErrorStackTrace: 5,
		HandlerOptions: &slog.HandlerOptions{
			Level:       slog.LevelDebug,
			ReplaceAttr: logFormat.ReplaceAttr,
		},
	})).With(
		slog.String("app", "spectral"),
		slog.String("version", "v0.1.0-prealpha"),
		slog.String("environment", "local"),
	)

	r.Use(httplog.RequestLogger(logger, &httplog.Options{
		Level:         slog.LevelInfo,
		Schema:        logFormat,
		RecoverPanics: true,
	}))

	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	server := &http.Server{
		Handler: r,

		ReadHeaderTimeout: 2 * time.Second,
		ReadTimeout:       5 * time.Second,
		WriteTimeout:      5 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    16 << 10,
	}

	listener, err := net.Listen("tcp", "127.0.0.1:3000")
	if err != nil {
		return Server{}, err
	}

	return Server{
		http:     server,
		listener: listener,
		done:     make(chan error, 1),
	}, nil
}

func (s *Server) Start() {
	go func() {
		err := s.http.Serve(s.listener)

		// Shutdown makes Serve return ErrServerClosed. That is expected.
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}

		s.done <- err
		close(s.done)
	}()
}

func (s *Server) Addr() string {
	return s.listener.Addr().String()
}

func (s *Server) Done() <-chan error {
	return s.done
}

func (s *Server) Shutdown(ctx context.Context) error {
	shutdownErr := s.http.Shutdown(ctx)
	if shutdownErr != nil {
		// Graceful shutdown timed out. Force remaining connections closed.
		_ = s.http.Close()
	}

	serveErr := <-s.done
	return errors.Join(shutdownErr, serveErr)
}
