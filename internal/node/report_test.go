package node

import (
	"testing"
	"time"
	"uuid"

	"github.com/cap-theorem/spectral/internal/collector"
	"github.com/cap-theorem/spectral/internal/wire"
)

func TestReportTick(t *testing.T) {
	n := testNode()
	n.telemetry = make(chan collector.Report, 1)
	t0 := time.Unix(100, 0)

	a, b := uuid.NewV4(), uuid.NewV4()
	n.ov.addLink(wire.Peer{ID: a}, true, t0)
	n.ov.addLink(wire.Peer{ID: b}, false, t0)
	n.ov.want[b] = wire.Peer{ID: b}

	n.reportTick(t0)
	r := <-n.telemetry
	if r.ID != n.self.ID || r.Address != n.self.Address || len(r.Links) != 2 {
		t.Fatalf("report = %+v", r)
	}
	if r.Links[0].Peer.Compare(r.Links[1].Peer) >= 0 {
		t.Fatalf("links not sorted: %v", r.Links)
	}
	for _, l := range r.Links {
		if l.Out != (l.Peer == b) {
			t.Fatalf("link %v Out = %v", l.Peer, l.Out)
		}
	}

	n.reportTick(t0.Add(reportInterval))
	if len(n.telemetry) != 0 {
		t.Fatal("unchanged links reported before keepalive")
	}

	delete(n.ov.links, a)
	n.reportTick(t0.Add(reportInterval / 2 * 3))
	if len(n.telemetry) != 0 {
		t.Fatal("reported before reportInterval passed")
	}
	n.reportTick(t0.Add(2 * reportInterval))
	if r := <-n.telemetry; len(r.Links) != 1 || r.Links[0].Peer != b {
		t.Fatalf("after unlink: %+v", r)
	}

	n.reportTick(t0.Add(2*reportInterval + reportKeepalive*11/10))
	if len(n.telemetry) != 1 {
		t.Fatal("no keepalive")
	}
}

func TestReportTickReplacesWaitingReport(t *testing.T) {
	n := testNode()
	n.telemetry = make(chan collector.Report, 1)
	t0 := time.Unix(100, 0)

	n.reportTick(t0)
	n.ov.addLink(wire.Peer{ID: uuid.NewV4()}, true, t0)
	n.reportTick(t0.Add(reportInterval))

	if r := <-n.telemetry; len(r.Links) != 1 {
		t.Fatalf("waiting report not replaced: %+v", r)
	}
}

func TestReportTickOffWithoutChannel(t *testing.T) {
	n := testNode()
	n.reportTick(time.Unix(100, 0))
	if n.lastReport != nil {
		t.Fatal("reported with telemetry off")
	}
}

func TestReportCarriesServiceLabel(t *testing.T) {
	n := testNode()
	n.telemetry = make(chan collector.Report, 1)
	t0 := time.Unix(100, 0)

	n.reportTick(t0)
	if r := <-n.telemetry; r.Service != "" || r.Instance != "" {
		t.Fatalf("labelled before registering: %+v", r)
	}

	reg := testRegistration()
	reg.Instance = "ord-9869"
	if _, err := n.registerService(reg); err != nil {
		t.Fatal(err)
	}
	n.reportTick(t0.Add(reportInterval))
	r := <-n.telemetry
	if r.Service != reg.Service || r.Instance != "ord-9869" {
		t.Fatalf("after register: %+v", r)
	}

	n.reportTick(t0.Add(2 * reportInterval))
	if len(n.telemetry) != 0 {
		t.Fatal("unchanged label reported again before keepalive")
	}

	n.expireLease(n.lease.ExpiresAt)
	n.reportTick(t0.Add(3 * reportInterval))
	if r := <-n.telemetry; r.Service != "" || r.Instance != "" {
		t.Fatalf("after lease expired: %+v", r)
	}
}
