package main

import (
	"fmt"
	"sync"
	"sync/atomic"
)

func main() {
	for run := 1; run <= 10; run++ {
		var ops atomic.Uint64
		var plain uint64
		var wg sync.WaitGroup

		for range 50 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for range 1000 {
					ops.Add(1)
					plain++
				}
			}()
		}
		wg.Wait()
		fmt.Printf("run %2d: atomic=%d  plain=%d  (lost %d)\n", run, ops.Load(), plain, 50000-plain)
	}
}
