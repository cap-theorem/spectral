package node

import (
	"cmp"
	"slices"
	"time"

	"github.com/cap-theorem/spectral/internal/collector"
	"github.com/cap-theorem/spectral/internal/wire"
)

const flowInterval = 250 * time.Millisecond

type flowKey struct {
	to   wire.NodeID
	kind wire.Kind
}

func (n *Node) countFlow(to wire.NodeID, kind wire.Kind) {
	if n.flowOut == nil || to == (wire.NodeID{}) || to == n.self.ID {
		return
	}
	n.flows[flowKey{to: to, kind: kind}]++
}

func (n *Node) flowTick(now time.Time) {
	if n.flowOut == nil || now.Before(n.nextFlows) {
		return
	}
	n.nextFlows = now.Add(flowInterval)
	if len(n.flows) == 0 {
		return
	}

	flows := make([]collector.Flow, 0, len(n.flows))
	for k, c := range n.flows {
		flows = append(flows, collector.Flow{To: k.to, Kind: k.kind.String(), Count: c})
	}
	clear(n.flows)
	slices.SortFunc(flows, func(a, b collector.Flow) int {
		if c := a.To.Compare(b.To); c != 0 {
			return c
		}
		return cmp.Compare(a.Kind, b.Kind)
	})

	select {
	case n.flowOut <- collector.FlowReport{ID: n.self.ID, Flows: flows}:
	default:
	}
}
