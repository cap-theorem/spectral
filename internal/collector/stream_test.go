package collector

import (
	"bufio"
	"context"
	"encoding/json"
	"log/slog"
	"maps"
	"math/rand/v2"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"uuid"

	"github.com/cap-theorem/spectral/internal/wire"
)

// client rebuilds the graph the way a frontend would.
type client struct {
	version uint64
	nodes   map[wire.NodeID]Node
	edges   map[edge]struct{}
}

func newClient(s *Snapshot) *client {
	c := &client{version: s.Version, nodes: map[wire.NodeID]Node{}, edges: map[edge]struct{}{}}
	for _, n := range s.Nodes {
		c.nodes[n.ID] = n
	}
	for _, e := range s.Edges {
		c.edges[edgeOf(s.Nodes[e[0]].ID, s.Nodes[e[1]].ID)] = struct{}{}
	}
	return c
}

func (c *client) apply(t *testing.T, d *Delta) {
	t.Helper()
	if d.Version != c.version+1 {
		t.Fatalf("delta version %d after %d", d.Version, c.version)
	}
	c.version = d.Version
	for _, n := range d.Nodes {
		c.nodes[n.ID] = n
	}
	for _, id := range d.Removed {
		delete(c.nodes, id)
	}
	for _, e := range d.EdgesAdded {
		c.edges[edgeOf(e[0], e[1])] = struct{}{}
	}
	for _, e := range d.EdgesRemoved {
		delete(c.edges, edgeOf(e[0], e[1]))
	}
}

func TestDeltasRebuildSnapshot(t *testing.T) {
	g := NewGraph()
	a := NewAnalyzer(AnalyzerConfig{StaleAfter: time.Hour}, g, slog.New(slog.DiscardHandler))

	rng := rand.New(rand.NewPCG(1, 2))
	now := time.Unix(100, 0)
	reps := overlayReports(60, 3, rng)
	for _, r := range reps {
		g.Apply(r, now)
	}
	a.step(now)
	snap, deltas, cancel := a.hub.subscribe()
	defer cancel()
	var first Snapshot
	if err := json.Unmarshal(snap, &first); err != nil {
		t.Fatal(err)
	}
	c := newClient(&first)

	seq := uint64(1)
	for round := range 30 {
		now = now.Add(time.Second)
		seq++
		switch round % 3 {
		case 0: // a node drops a link and its peer follows
			u := reps[rng.IntN(len(reps))]
			if len(u.Links) == 0 {
				continue
			}
			peer := u.Links[0].Peer
			for i := range reps {
				if reps[i].ID == u.ID || reps[i].ID == peer {
					reps[i].Links = slicesDelete(reps[i].Links, u.ID, peer)
					reps[i].Seq = seq
					g.Apply(reps[i], now)
				}
			}
		case 1: // a new node links to two others
			id := uuid.NewV4()
			x, y := rng.IntN(len(reps)), rng.IntN(len(reps))
			reps = append(reps, Report{ID: id, Seq: seq, Links: []ReportLink{{Peer: reps[x].ID}, {Peer: reps[y].ID}}})
			g.Apply(reps[len(reps)-1], now)
			for _, i := range []int{x, y} {
				reps[i].Links = append(reps[i].Links, ReportLink{Peer: id})
				reps[i].Seq = seq
				g.Apply(reps[i], now)
			}
		case 2: // a node leaves
			i := rng.IntN(len(reps))
			g.Apply(Report{ID: reps[i].ID, Seq: seq + 1000, Left: true}, now)
			reps = append(reps[:i], reps[i+1:]...)
		}
		a.step(now)

		for len(deltas) > 0 {
			var d Delta
			if err := json.Unmarshal(<-deltas, &d); err != nil {
				t.Fatal(err)
			}
			c.apply(t, &d)
		}
		want := newClient(a.prev.snapshot)
		if c.version != want.version || !maps.Equal(c.nodes, want.nodes) || !maps.Equal(c.edges, want.edges) {
			t.Fatalf("round %d: client at version %d has %d nodes, %d edges; snapshot %d has %d, %d",
				round, c.version, len(c.nodes), len(c.edges), want.version, len(want.nodes), len(want.edges))
		}
	}
}

func slicesDelete(links []ReportLink, ids ...wire.NodeID) []ReportLink {
	var out []ReportLink
	for _, l := range links {
		keep := true
		for _, id := range ids {
			keep = keep && l.Peer != id
		}
		if keep {
			out = append(out, l)
		}
	}
	return out
}

func TestViewNodeStates(t *testing.T) {
	g := NewGraph()
	now := time.Unix(100, 0)
	a, b, c, d := uuid.NewV4(), uuid.NewV4(), uuid.NewV4(), uuid.NewV4()
	g.Apply(report(a, 1, b), now)
	g.Apply(report(b, 1, a), now)
	g.Apply(report(c, 1), now)    // isolated
	g.Apply(report(d, 1, a), now) // orphaned

	_, v := analyze(g.Freeze(now))
	want := map[wire.NodeID]Node{
		a: {ID: a, State: StateActive, Component: 0},
		b: {ID: b, State: StateActive, Component: 0},
		c: {ID: c, State: StateIsolated, Component: 1},
		d: {ID: d, State: StateOrphaned, Component: -1},
	}
	if !maps.Equal(v.nodes, want) || len(v.snapshot.Edges) != 1 {
		t.Fatalf("nodes %v, edges %v", v.nodes, v.snapshot.Edges)
	}
}

func TestHubDropsSlowSubscriber(t *testing.T) {
	h := newHub()
	_, ch, cancel := h.subscribe()
	defer cancel()
	for i := range subscriberBuffer + 1 {
		h.publish(&Snapshot{Version: uint64(i)}, &Delta{})
	}
	for range subscriberBuffer {
		<-ch
	}
	if _, ok := <-ch; ok {
		t.Fatal("slow subscriber not dropped")
	}
}

func TestServerGraphAndStream(t *testing.T) {
	g := NewGraph()
	an := NewAnalyzer(AnalyzerConfig{StaleAfter: time.Hour}, g, slog.New(slog.DiscardHandler))
	srv := httptest.NewServer(NewServer(ServerConfig{}, g, an, Metrics{}, slog.New(slog.DiscardHandler)).Handler())
	t.Cleanup(srv.Close)

	x, y, z := uuid.NewV4(), uuid.NewV4(), uuid.NewV4()
	now := time.Now()
	g.Apply(report(x, 1, y), now)
	g.Apply(report(y, 1, x), now)
	an.step(now)

	resp, err := http.Get(srv.URL + "/v1/graph")
	if err != nil {
		t.Fatal(err)
	}
	var snap Snapshot
	err = json.NewDecoder(resp.Body).Decode(&snap)
	resp.Body.Close()
	if err != nil || snap.Version != 1 || len(snap.Nodes) != 2 || len(snap.Edges) != 1 || snap.Gap != 2 {
		t.Fatalf("graph = %+v, err %v", snap, err)
	}
	if resp.Header.Get("Access-Control-Allow-Origin") != "*" {
		t.Fatal("no CORS header")
	}
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/v1/graph", nil)
	req.Header.Set("If-None-Match", resp.Header.Get("ETag"))
	if resp, err := http.DefaultClient.Do(req); err != nil || resp.StatusCode != http.StatusNotModified {
		t.Fatalf("conditional GET: %v %v", resp.StatusCode, err)
	}

	stream, err := http.Get(srv.URL + "/v1/graph/stream")
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Body.Close()
	events := bufio.NewReader(stream.Body)
	readEvent := func() (string, string) {
		t.Helper()
		var event, data string
		for {
			line, err := events.ReadString('\n')
			if err != nil {
				t.Fatal(err)
			}
			line = strings.TrimRight(line, "\n")
			switch {
			case strings.HasPrefix(line, "event: "):
				event = line[len("event: "):]
			case strings.HasPrefix(line, "data: "):
				data = line[len("data: "):]
			case line == "" && event != "":
				return event, data
			}
		}
	}

	if event, data := readEvent(); event != "snapshot" || !strings.Contains(data, `"version":1`) {
		t.Fatalf("first event %q: %s", event, data)
	}

	g.Apply(report(z, 1, x), now)
	g.Apply(report(x, 2, y, z), now)
	an.step(now.Add(time.Second))

	event, data := readEvent()
	var d Delta
	if event != "delta" || json.Unmarshal([]byte(data), &d) != nil {
		t.Fatalf("second event %q: %s", event, data)
	}
	if d.Version != 2 || len(d.Nodes) != 1 || d.Nodes[0].ID != z || len(d.EdgesAdded) != 1 || len(d.EdgesRemoved) != 0 {
		t.Fatalf("delta = %+v", d)
	}
}

func TestShutdownEndsStreams(t *testing.T) {
	g := NewGraph()
	an := NewAnalyzer(AnalyzerConfig{StaleAfter: time.Hour}, g, slog.New(slog.DiscardHandler))
	an.step(time.Now())
	srv := NewServer(ServerConfig{}, g, an, Metrics{}, slog.New(slog.DiscardHandler))
	ts := httptest.NewUnstartedServer(srv.Handler())
	ts.Config = srv.server
	ts.Start()
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/v1/graph/stream")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if _, err := bufio.NewReader(resp.Body).ReadString('\n'); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	start := time.Now()
	if err := srv.Shutdown(ctx); err != nil || time.Since(start) > time.Second {
		t.Fatalf("Shutdown took %v, err %v; want under 1s with a stream open", time.Since(start), err)
	}
}
