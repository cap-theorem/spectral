package collector

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
	"uuid"
)

func TestServerIngestAndSummary(t *testing.T) {
	g := NewGraph()
	a := NewAnalyzer(AnalyzerConfig{StaleAfter: time.Minute}, g, slog.New(slog.DiscardHandler))
	srv := httptest.NewServer(NewServer(ServerConfig{}, g, a, Metrics{}, slog.New(slog.DiscardHandler)).Handler())
	t.Cleanup(srv.Close)

	if resp, err := http.Get(srv.URL + "/v1/summary"); err != nil || resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("summary before analysis: %v %v", resp.StatusCode, err)
	}

	post := func(body []byte) int {
		resp, err := http.Post(srv.URL+ReportPath, "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}
	mustJSON := func(v any) []byte {
		b, _ := json.Marshal(v)
		return b
	}

	x, y := uuid.NewV4(), uuid.NewV4()
	for _, r := range []Report{report(x, 1, y), report(y, 1, x)} {
		if code := post(mustJSON(r)); code != http.StatusNoContent {
			t.Fatalf("report: status %d", code)
		}
	}
	if code := post(mustJSON(report(x, 1))); code != http.StatusNoContent {
		t.Fatalf("stale report: status %d, want 204", code)
	}
	for name, body := range map[string][]byte{
		"not json":  []byte("{"),
		"no id":     mustJSON(Report{Seq: 1}),
		"self link": mustJSON(report(x, 9, x)),
	} {
		if code := post(body); code != http.StatusBadRequest {
			t.Fatalf("%s: status %d, want 400", name, code)
		}
	}

	a.step(time.Now())
	resp, err := http.Get(srv.URL + "/v1/summary")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var s Summary
	if err := json.NewDecoder(resp.Body).Decode(&s); err != nil {
		t.Fatal(err)
	}
	if s.Nodes != 2 || s.Edges != 1 || s.Gap != 2 {
		t.Fatalf("summary = %+v, want 2 nodes, 1 edge, gap 2", s)
	}
}
