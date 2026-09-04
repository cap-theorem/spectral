package main

import (
	"context"
	"fmt"

	"github.com/cap-theorem/spectral/internal/clock"
)

func main() {
	timer := clock.NewTimer()
	tick := 0

	go timer.Run(context.Background())

	for range timer.C() {
		tick++
		fmt.Printf("Tick: %d\n", tick)
	}
}
