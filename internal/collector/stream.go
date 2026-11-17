package collector

import (
	"encoding/json"
	"sync"
)

// A subscriber that falls this many deltas behind is dropped and must
// reconnect for a fresh snapshot.
const subscriberBuffer = 64

type hub struct {
	mu      sync.Mutex
	version uint64
	snap    []byte
	subs    map[chan []byte]struct{}
}

func newHub() *hub {
	return &hub{subs: make(map[chan []byte]struct{})}
}

func (h *hub) publish(snap *Snapshot, d *Delta) {
	sb, _ := json.Marshal(snap)
	var db []byte
	if d != nil {
		db, _ = json.Marshal(d)
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	h.version, h.snap = snap.Version, sb
	if db == nil {
		return
	}
	for ch := range h.subs {
		select {
		case ch <- db:
		default:
			delete(h.subs, ch)
			close(ch)
		}
	}
}

func (h *hub) snapshot() (uint64, []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.version, h.snap
}

// subscribe returns the current snapshot and every delta after it, in order.
// The channel is closed if the subscriber falls behind.
func (h *hub) subscribe() ([]byte, <-chan []byte, func()) {
	h.mu.Lock()
	defer h.mu.Unlock()
	ch := make(chan []byte, subscriberBuffer)
	h.subs[ch] = struct{}{}
	cancel := func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		if _, ok := h.subs[ch]; ok {
			delete(h.subs, ch)
			close(ch)
		}
	}
	return h.snap, ch, cancel
}
