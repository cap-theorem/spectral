package collector

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/cap-theorem/spectral/internal/wire"
)

const FlowsPath = "/v1/flows"

const (
	maxFlowsPerReport = 4096
	maxPendingFlows   = 50_000
	flowSubBuffer     = 32
	FlowFlushInterval = 100 * time.Millisecond
)

type FlowReport struct {
	ID    wire.NodeID `json:"id"`
	Flows []Flow      `json:"flows"`
}

type Flow struct {
	To    wire.NodeID `json:"to"`
	Kind  string      `json:"kind"`
	Count uint32      `json:"count"`
}

func (r FlowReport) Validate() error {
	if r.ID == (wire.NodeID{}) {
		return fmt.Errorf("%w: id is required", ErrInvalidReport)
	}
	if len(r.Flows) > maxFlowsPerReport {
		return fmt.Errorf("%w: %d flows exceeds limit %d", ErrInvalidReport, len(r.Flows), maxFlowsPerReport)
	}
	for _, f := range r.Flows {
		if f.To == (wire.NodeID{}) || f.To == r.ID {
			return fmt.Errorf("%w: flow target must be another node's id", ErrInvalidReport)
		}
		if f.Kind == "" || f.Count == 0 {
			return fmt.Errorf("%w: flow needs a kind and a count", ErrInvalidReport)
		}
	}
	return nil
}

type FlowEvent struct {
	From  wire.NodeID `json:"from"`
	To    wire.NodeID `json:"to"`
	Kind  string      `json:"kind"`
	Count uint32      `json:"count"`
}

type FlowBatch struct {
	At    time.Time   `json:"at"`
	Flows []FlowEvent `json:"flows"`
}

type flowHub struct {
	mu      sync.Mutex
	pending []FlowEvent
	dropped int
	subs    map[chan []byte]struct{}
}

func newFlowHub() *flowHub {
	return &flowHub{subs: make(map[chan []byte]struct{})}
}

func (h *flowHub) add(r FlowReport) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.subs) == 0 {
		return
	}
	for _, f := range r.Flows {
		if len(h.pending) >= maxPendingFlows {
			h.dropped++
			continue
		}
		h.pending = append(h.pending, FlowEvent{From: r.ID, To: f.To, Kind: f.Kind, Count: f.Count})
	}
}

func (h *flowHub) flush(now time.Time) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.pending) == 0 {
		return
	}
	b, _ := json.Marshal(FlowBatch{At: now, Flows: h.pending})
	h.pending = nil
	for ch := range h.subs {
		select {
		case ch <- b:
		default:
		}
	}
}

func (h *flowHub) subscribe() (<-chan []byte, func()) {
	h.mu.Lock()
	defer h.mu.Unlock()
	ch := make(chan []byte, flowSubBuffer)
	h.subs[ch] = struct{}{}
	cancel := func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		delete(h.subs, ch)
	}
	return ch, cancel
}

func (h *flowHub) run(ctx context.Context, every time.Duration) error {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case now := <-t.C:
			h.flush(now)
		}
	}
}
