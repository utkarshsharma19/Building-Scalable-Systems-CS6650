package main

import (
	"fmt"
	"runtime"
	"time"
)

const roundTrips = 1_000_000

func pingPong() time.Duration {
	ping, pong := make(chan struct{}), make(chan struct{})
	go func() {
		for range roundTrips {
			<-ping
			pong <- struct{}{}
		}
	}()

	start := time.Now()
	for range roundTrips {
		ping <- struct{}{}
		<-pong
	}
	return time.Since(start)
}

func measure(procs int) time.Duration {
	runtime.GOMAXPROCS(procs)
	var total time.Duration
	for r := 1; r <= 3; r++ {
		d := pingPong()
		avg := d / (2 * roundTrips)
		total += avg
		fmt.Printf("  GOMAXPROCS=%d run %d: total=%v  avg switch=%v\n", procs, r, d, avg)
	}
	return total / 3
}

func main() {
	cpus := runtime.NumCPU()
	single, multi := measure(1), measure(cpus)
	fmt.Printf("\nMEAN avg switch: GOMAXPROCS=1 -> %v   GOMAXPROCS=%d -> %v\n", single, cpus, multi)
}
