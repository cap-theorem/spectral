package collector

import (
	"bufio"
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"uuid"
)

func TestFlowReportValidate(t *testing.T) {
	x, y := uuid.NewV4(), uuid.NewV4()
	for name, r := range map[string]FlowReport{
		"no id":      {Flows: []Flow{{To: y, Kind: "ping", Count: 1}}},
		"self":       {ID: x, Flows: []Flow{{To: x, Kind: "ping", Count: 1}}},
		"no target":  {ID: x, Flows: []Flow{{Kind: "ping", Count: 1}}},
		"no kind":    {ID: x, Flows: []Flow{{To: y, Count: 1}}},
		"zero count": {ID: x, Flows: []Flow{{To: y, Kind: "ping"}}},
	} {
		if r.Validate() == nil {
			t.Errorf("%s: valid", name)
		}
	}
	if err := (FlowReport{ID: x, Flows: []Flow{{To: y, Kind: "ping", Count: 3}}}).Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestFlowHubSkipsWithoutSubscribers(t *testing.T) {
	h := newFlowHub()
	h.add(FlowReport{ID: uuid.NewV4(), Flows: []Flow{{To: uuid.NewV4(), Kind: "ping", Count: 1}}})
	if len(h.pending) != 0 {
		t.Fatal("buffered flows with nobody watching")
	}
}

func TestFlowHubSlowSubscriberMissesBatches(t *testing.T) {
	h := newFlowHub()
	ch, cancel := h.subscribe()
	defer cancel()
	r := FlowReport{ID: uuid.NewV4(), Flows: []Flow{{To: uuid.NewV4(), Kind: "ping", Count: 1}}}
	for range flowSubBuffer + 5 {
		h.add(r)
		h.flush(time.Now())
	}
	if len(ch) != flowSubBuffer {
		t.Fatalf("buffered %d batches, want %d", len(ch), flowSubBuffer)
	}
	if len(h.subs) != 1 {
		t.Fatal("slow subscriber was dropped")
	}
}

func TestServerFlowsStream(t *testing.T) {
	g := NewGraph()
	a := NewAnalyzer(AnalyzerConfig{StaleAfter: time.Minute}, g, slog.New(slog.DiscardHandler))
	s := NewServer(ServerConfig{}, g, a, Metrics{}, slog.New(slog.DiscardHandler))
	srv := httptest.NewServer(s.Handler())
	t.Cleanup(srv.Close)

	resp, err := http.Get(srv.URL + "/v1/flows/stream")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.Header.Get("Access-Control-Allow-Origin") != "*" {
		t.Fatal("flow stream not readable cross-origin")
	}
	lines := bufio.NewScanner(resp.Body)
	if !lines.Scan() || lines.Text() != ": flows" {
		t.Fatalf("first line %q", lines.Text())
	}

	x, y := uuid.NewV4(), uuid.NewV4()
	post := func(r FlowReport) int {
		b, _ := json.Marshal(r)
		resp, err := http.Post(srv.URL+FlowsPath, "application/json", bytes.NewReader(b))
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}
	if code := post(FlowReport{ID: x, Flows: []Flow{{To: x, Kind: "ping", Count: 1}}}); code != http.StatusBadRequest {
		t.Fatalf("self flow: status %d", code)
	}
	if code := post(FlowReport{ID: x, Flows: []Flow{{To: y, Kind: "heartbeat", Count: 2}}}); code != http.StatusNoContent {
		t.Fatalf("flow: status %d", code)
	}
	s.flows.flush(time.Now())

	var event, data string
	for lines.Scan() {
		line := lines.Text()
		if v, ok := strings.CutPrefix(line, "event: "); ok {
			event = v
		}
		if v, ok := strings.CutPrefix(line, "data: "); ok {
			data = v
			break
		}
	}
	if event != "flows" {
		t.Fatalf("event %q", event)
	}
	var b FlowBatch
	if err := json.Unmarshal([]byte(data), &b); err != nil {
		t.Fatal(err)
	}
	want := FlowEvent{From: x, To: y, Kind: "heartbeat", Count: 2}
	if len(b.Flows) != 1 || b.Flows[0] != want {
		t.Fatalf("batch = %+v", b)
	}
}
