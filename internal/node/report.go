package node

import (
	"math/rand/v2"
	"slices"
	"time"

	"github.com/cap-theorem/spectral/internal/collector"
)

const (
	reportInterval = 500 * time.Millisecond
	// Collector --stale-after must be several times this.
	reportKeepalive = 30 * time.Second
)

func (n *Node) reportTick(now time.Time) {
	if n.telemetry == nil || now.Before(n.nextReport) {
		return
	}
	n.nextReport = now.Add(reportInterval)

	links := make([]collector.ReportLink, 0, len(n.ov.links))
	for id := range n.ov.links {
		_, wanted := n.ov.want[id]
		links = append(links, collector.ReportLink{Peer: id, Out: wanted})
	}
	slices.SortFunc(links, func(a, b collector.ReportLink) int { return a.Peer.Compare(b.Peer) })

	if n.lastReport != nil && slices.Equal(links, n.lastReport) && now.Before(n.keepalive) {
		return
	}
	n.lastReport = links
	// ±10%, so nodes started together do not send in sync.
	n.keepalive = now.Add(time.Duration(float64(reportKeepalive) * (0.9 + 0.2*rand.Float64())))

	report := collector.Report{ID: n.self.ID, Address: n.self.Address, Links: links}
	select {
	case n.telemetry <- report:
	default:
		// Exporter busy: replace the waiting report.
		select {
		case <-n.telemetry:
		default:
		}
		n.telemetry <- report
	}
}
