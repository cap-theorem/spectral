package clock

import (
	"context"
	"time"
)

const TickIntervalMs = time.Millisecond * 250

type Timer struct {
	ticks chan struct{}
}

func NewTimer() *Timer {
	return &Timer{
		ticks: make(chan struct{}),
	}
}

func (t *Timer) C() <-chan struct{} {
	return t.ticks
}

func (t *Timer) Run(ctx context.Context) {
	ticker := time.NewTicker(TickIntervalMs)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			select {
			case t.ticks <- struct{}{}:
				// Delivered a tick
			default:
				// Channel full, consumer still processing
			}
		}
	}
}
