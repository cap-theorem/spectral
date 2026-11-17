package telemetry

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"sync/atomic"
	"testing"
	"time"
	"uuid"

	"github.com/cap-theorem/spectral/internal/collector"
)

func collectorStub(t *testing.T, status *atomic.Int32) (string, <-chan collector.Report) {
	t.Helper()
	got := make(chan collector.Report, 16)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var rep collector.Report
		if r.Method != http.MethodPost || r.URL.Path != collector.ReportPath || json.NewDecoder(r.Body).Decode(&rep) != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		code := int(status.Load())
		got <- rep
		w.WriteHeader(code)
	}))
	t.Cleanup(srv.Close)
	return srv.URL, got
}

func next(t *testing.T, got <-chan collector.Report) collector.Report {
	t.Helper()
	select {
	case r := <-got:
		return r
	case <-time.After(2 * time.Second):
		t.Fatal("no report")
		return collector.Report{}
	}
}

func TestExporterPostsThenLeaves(t *testing.T) {
	var status atomic.Int32
	status.Store(http.StatusNoContent)
	url, got := collectorStub(t, &status)

	reports := make(chan collector.Report, 1)
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() {
		done <- NewExporter(ExporterConfig{CollectorURL: url + "/"}, reports, slog.New(slog.DiscardHandler)).Run(ctx)
	}()

	id := uuid.NewV4()
	addr := netip.MustParseAddrPort("127.0.0.1:7000")
	reports <- collector.Report{ID: id, Address: addr, Links: []collector.ReportLink{{Peer: uuid.NewV4()}}}
	if r := next(t, got); r.ID != id || r.Seq != 1 || len(r.Links) != 1 {
		t.Fatalf("first = %+v", r)
	}
	reports <- collector.Report{ID: id, Address: addr}
	if r := next(t, got); r.Seq != 2 || len(r.Links) != 0 {
		t.Fatalf("second = %+v", r)
	}

	cancel()
	if r := next(t, got); !r.Left || r.ID != id || r.Address != addr || r.Seq != 3 {
		t.Fatalf("leave = %+v", r)
	}
	<-done
}

func TestExporterRetriesNewestWithBackoff(t *testing.T) {
	var status atomic.Int32
	status.Store(http.StatusServiceUnavailable)
	url, got := collectorStub(t, &status)

	reports := make(chan collector.Report, 1)
	go NewExporter(ExporterConfig{CollectorURL: url}, reports, slog.New(slog.DiscardHandler)).Run(t.Context())

	id := uuid.NewV4()
	reports <- collector.Report{ID: id}
	start := time.Now()
	next(t, got)
	reports <- collector.Report{ID: id, Links: []collector.ReportLink{{Peer: uuid.NewV4()}}}

	// The newer report waits for the backoff, then replaces the failed one.
	r := next(t, got)
	if waited := time.Since(start); waited < minBackoff || len(r.Links) != 1 {
		t.Fatalf("retry after %v with %d links; want >= %v and the newer report", waited, len(r.Links), minBackoff)
	}

	status.Store(http.StatusNoContent)
	next(t, got) // second retry, after 2*minBackoff, succeeds
	select {
	case r := <-got:
		t.Fatalf("posted again after success: %+v", r)
	case <-time.After(4 * minBackoff):
	}
}
