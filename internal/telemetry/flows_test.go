package telemetry

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
	"uuid"

	"github.com/cap-theorem/spectral/internal/collector"
)

func TestFlowExporterPostsAndDropsOnFailure(t *testing.T) {
	var status atomic.Int32
	status.Store(http.StatusInternalServerError)
	got := make(chan collector.FlowReport, 16)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var rep collector.FlowReport
		if r.Method != http.MethodPost || r.URL.Path != collector.FlowsPath || json.NewDecoder(r.Body).Decode(&rep) != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		got <- rep
		w.WriteHeader(int(status.Load()))
	}))
	t.Cleanup(srv.Close)

	flows := make(chan collector.FlowReport)
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() {
		done <- NewFlowExporter(ExporterConfig{CollectorURL: srv.URL + "/"}, flows, slog.New(slog.DiscardHandler)).Run(ctx)
	}()

	id := uuid.NewV4()
	first := collector.FlowReport{ID: id, Flows: []collector.Flow{{To: uuid.NewV4(), Kind: "ping", Count: 1}}}
	second := collector.FlowReport{ID: id, Flows: []collector.Flow{{To: uuid.NewV4(), Kind: "pong", Count: 1}}}

	flows <- first
	if r := <-got; r.Flows[0].Kind != "ping" {
		t.Fatalf("got %+v", r)
	}
	status.Store(http.StatusNoContent)
	flows <- second
	select {
	case r := <-got:
		if r.Flows[0].Kind != "pong" {
			t.Fatalf("retried a failed batch: %+v", r)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no second post")
	}

	cancel()
	if err := <-done; err != context.Canceled {
		t.Fatalf("Run = %v", err)
	}
}
