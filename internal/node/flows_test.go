package node

import (
	"testing"
	"time"
	"uuid"

	"github.com/cap-theorem/spectral/internal/collector"
	"github.com/cap-theorem/spectral/internal/wire"
)

func TestFlowTickBatchesSends(t *testing.T) {
	n := testNode()
	n.flowOut = make(chan collector.FlowReport, 1)
	t0 := time.Unix(100, 0)

	a, b := wire.Peer{ID: uuid.NewV4()}, wire.Peer{ID: uuid.NewV4()}
	n.send(a, wire.Heartbeat{})
	n.send(a, wire.Heartbeat{})
	n.send(a, wire.Ping{})
	n.send(b, wire.Update{})
	n.send(wire.Peer{}, wire.Ping{})
	n.send(n.self, wire.Ping{})

	n.flowTick(t0)
	r := <-n.flowOut
	if r.ID != n.self.ID || len(r.Flows) != 3 {
		t.Fatalf("report = %+v", r)
	}
	want := map[[2]string]uint32{
		{a.ID.String(), "heartbeat"}: 2,
		{a.ID.String(), "ping"}:      1,
		{b.ID.String(), "update"}:    1,
	}
	for _, f := range r.Flows {
		if want[[2]string{f.To.String(), f.Kind}] != f.Count {
			t.Fatalf("unexpected flow %+v", f)
		}
	}
	for i := 1; i < len(r.Flows); i++ {
		if c := r.Flows[i-1].To.Compare(r.Flows[i].To); c > 0 || (c == 0 && r.Flows[i-1].Kind > r.Flows[i].Kind) {
			t.Fatalf("flows not sorted: %+v", r.Flows)
		}
	}

	n.send(a, wire.Pong{})
	n.flowTick(t0.Add(flowInterval / 2))
	if len(n.flowOut) != 0 {
		t.Fatal("flushed before flowInterval")
	}
	n.flowTick(t0.Add(flowInterval))
	if r := <-n.flowOut; len(r.Flows) != 1 || r.Flows[0].Kind != "pong" {
		t.Fatalf("second batch = %+v", r)
	}

	n.flowTick(t0.Add(2 * flowInterval))
	if len(n.flowOut) != 0 {
		t.Fatal("sent an empty batch")
	}
}

func TestFlowTickDropsWhenExporterBusy(t *testing.T) {
	n := testNode()
	n.flowOut = make(chan collector.FlowReport, 1)
	t0 := time.Unix(100, 0)
	p := wire.Peer{ID: uuid.NewV4()}

	n.send(p, wire.Ping{})
	n.flowTick(t0)
	n.send(p, wire.Pong{})
	n.flowTick(t0.Add(flowInterval))

	if r := <-n.flowOut; r.Flows[0].Kind != "ping" {
		t.Fatalf("kept %+v, want the first batch", r)
	}
	if len(n.flows) != 0 {
		t.Fatal("dropped batch still counted")
	}
}

func TestNoFlowsWithoutCollector(t *testing.T) {
	n := testNode()
	n.send(wire.Peer{ID: uuid.NewV4()}, wire.Ping{})
	if len(n.flows) != 0 {
		t.Fatal("counted flows with no collector")
	}
}
