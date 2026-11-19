package collector

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"go.opentelemetry.io/otel/metric"
)

const maxReportBytes = 1 << 20

type ServerConfig struct {
	ListenAddr string
}

type Server struct {
	graph    *Graph
	analyzer *Analyzer
	metrics  Metrics
	logger   *slog.Logger
	server   *http.Server
	streams  context.Context
	flows    *flowHub
}

func NewServer(cfg ServerConfig, g *Graph, a *Analyzer, m Metrics, logger *slog.Logger) *Server {
	s := &Server{graph: g, analyzer: a, metrics: m, logger: logger, flows: newFlowHub()}

	// No request logging: ingest is most of the traffic.
	r := chi.NewRouter()
	r.Use(middleware.Recoverer)

	r.Group(func(r chi.Router) {
		r.Use(middleware.Timeout(5 * time.Second))
		r.Post(ReportPath, s.handleReport)
		r.Post(FlowsPath, s.handleFlows)
		r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	})
	// Read-only, so any origin may read it (e.g. a frontend dev server).
	r.Group(func(r chi.Router) {
		r.Use(allowAnyOrigin)
		r.With(middleware.Timeout(5*time.Second)).Get("/v1/summary", s.handleSummary)
		r.With(middleware.Timeout(5*time.Second), middleware.Compress(5, "application/json")).Get("/v1/graph", s.handleGraph)
		r.Get("/v1/graph/stream", s.handleStream)
		r.Get("/v1/flows/stream", s.handleFlowStream)
	})

	s.server = &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           r,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	// Shutdown waits for open requests, so streams must end when it starts.
	streams, stop := context.WithCancel(context.Background())
	s.streams = streams
	s.server.RegisterOnShutdown(stop)
	return s
}

func (s *Server) Handler() http.Handler { return s.server.Handler }

func (s *Server) Serve() error {
	s.logger.Info("collector listening", "address", s.server.Addr)
	err := s.server.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func (s *Server) RunFlows(ctx context.Context) error { return s.flows.run(ctx, FlowFlushInterval) }

func (s *Server) Shutdown(ctx context.Context) error { return s.server.Shutdown(ctx) }

func (s *Server) Close() error { return s.server.Close() }

func (s *Server) handleReport(w http.ResponseWriter, r *http.Request) {
	var rep Report
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxReportBytes)).Decode(&rep); err != nil {
		s.count(r.Context(), s.metrics.ReportsRejected)
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := rep.Validate(); err != nil {
		s.count(r.Context(), s.metrics.ReportsRejected)
		writeError(w, http.StatusBadRequest, err)
		return
	}

	if s.graph.Apply(rep, time.Now()) {
		s.count(r.Context(), s.metrics.ReportsAccepted)
	} else {
		// Stale is harmless; still 204 so the node does not retry.
		s.count(r.Context(), s.metrics.ReportsStale)
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleFlows(w http.ResponseWriter, r *http.Request) {
	var rep FlowReport
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxReportBytes)).Decode(&rep); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := rep.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	s.flows.add(rep)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleFlowStream(w http.ResponseWriter, r *http.Request) {
	batches, cancel := s.flows.subscribe()
	defer cancel()

	rc := http.NewResponseController(w)
	_ = rc.SetWriteDeadline(time.Time{})
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	if _, err := fmt.Fprint(w, ": flows\n\n"); err != nil || rc.Flush() != nil {
		return
	}

	ping := time.NewTicker(15 * time.Second)
	defer ping.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-s.streams.Done():
			return
		case b := <-batches:
			if _, err := fmt.Fprintf(w, "event: flows\ndata: %s\n\n", b); err != nil || rc.Flush() != nil {
				return
			}
		case <-ping.C:
			if _, err := fmt.Fprint(w, ": ping\n\n"); err != nil || rc.Flush() != nil {
				return
			}
		}
	}
}

func (s *Server) handleSummary(w http.ResponseWriter, _ *http.Request) {
	sum := s.analyzer.Latest()
	if sum == nil {
		writeError(w, http.StatusServiceUnavailable, errors.New("no analysis yet"))
		return
	}
	writeJSON(w, http.StatusOK, sum)
}

func (s *Server) handleGraph(w http.ResponseWriter, r *http.Request) {
	version, snap := s.analyzer.hub.snapshot()
	if snap == nil {
		writeError(w, http.StatusServiceUnavailable, errors.New("no analysis yet"))
		return
	}
	etag := `"` + strconv.FormatUint(version, 10) + `"`
	w.Header().Set("ETag", etag)
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(snap)
}

// handleStream sends a "snapshot" event, then a "delta" event per change.
// The stream ends if the client falls behind; it should reconnect.
func (s *Server) handleStream(w http.ResponseWriter, r *http.Request) {
	snap, deltas, cancel := s.analyzer.hub.subscribe()
	defer cancel()
	if snap == nil {
		writeError(w, http.StatusServiceUnavailable, errors.New("no analysis yet"))
		return
	}

	rc := http.NewResponseController(w)
	_ = rc.SetWriteDeadline(time.Time{})
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)

	send := func(event string, data []byte) bool {
		_, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, data)
		return err == nil && rc.Flush() == nil
	}
	if !send("snapshot", snap) {
		return
	}

	// Comments keep idle proxies from closing the stream.
	ping := time.NewTicker(15 * time.Second)
	defer ping.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-s.streams.Done():
			return
		case d, ok := <-deltas:
			if !ok || !send("delta", d) {
				return
			}
		case <-ping.C:
			if _, err := fmt.Fprint(w, ": ping\n\n"); err != nil || rc.Flush() != nil {
				return
			}
		}
	}
}

func allowAnyOrigin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Expose-Headers", "ETag")
		next.ServeHTTP(w, r)
	})
}

func (s *Server) count(ctx context.Context, c metric.Int64Counter) {
	if c != nil {
		c.Add(ctx, 1)
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}
