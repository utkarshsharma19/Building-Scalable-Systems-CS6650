package main

import (
	"flag"
	"fmt"
	"sync"
	"time"
)

type store interface {
	Set(k, v int)
	Get(k int)
	Len() int
}

type plainMap struct{ m map[int]int }

func (s *plainMap) Set(k, v int) { s.m[k] = v }
func (s *plainMap) Get(k int)    { _ = s.m[k] }
func (s *plainMap) Len() int     { return len(s.m) }

type mutexMap struct {
	mu sync.Mutex
	m  map[int]int
}

func (s *mutexMap) Set(k, v int) { s.mu.Lock(); s.m[k] = v; s.mu.Unlock() }
func (s *mutexMap) Get(k int)    { s.mu.Lock(); _ = s.m[k]; s.mu.Unlock() }
func (s *mutexMap) Len() int     { s.mu.Lock(); defer s.mu.Unlock(); return len(s.m) }

type rwMutexMap struct {
	mu sync.RWMutex
	m  map[int]int
}

func (s *rwMutexMap) Set(k, v int) { s.mu.Lock(); s.m[k] = v; s.mu.Unlock() }
func (s *rwMutexMap) Get(k int)    { s.mu.RLock(); _ = s.m[k]; s.mu.RUnlock() }
func (s *rwMutexMap) Len() int     { s.mu.RLock(); defer s.mu.RUnlock(); return len(s.m) }

type syncMap struct{ m sync.Map }

func (s *syncMap) Set(k, v int) { s.m.Store(k, v) }
func (s *syncMap) Get(k int)    { s.m.Load(k) }
func (s *syncMap) Len() int {
	n := 0
	s.m.Range(func(_, _ any) bool { n++; return true })
	return n
}

func newStore(mode string) store {
	switch mode {
	case "mutex":
		return &mutexMap{m: map[int]int{}}
	case "rwmutex":
		return &rwMutexMap{m: map[int]int{}}
	case "syncmap":
		return &syncMap{}
	}
	return &plainMap{m: map[int]int{}}
}

func runOnce(mode string, readPct int) (int, time.Duration) {
	s := newStore(mode)
	if readPct > 0 {
		for k := range 50000 {
			s.Set(k, k)
		}
	}

	var wg sync.WaitGroup
	start := time.Now()
	for g := range 50 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range 1000 {
				if i%100 < readPct {
					s.Get(g*1000 + i)
				} else {
					s.Set(g*1000+i, i)
				}
			}
		}()
	}
	wg.Wait()
	return s.Len(), time.Since(start)
}

func main() {
	mode := flag.String("mode", "all", "plain | mutex | rwmutex | syncmap | all")
	reads := flag.Int("reads", 0, "percentage of operations that are reads")
	flag.Parse()

	modes := []string{*mode}
	if *mode == "all" {
		modes = []string{"mutex", "rwmutex", "syncmap"}
	}

	fmt.Printf("50 goroutines x 1000 ops, %d%% reads, 3 runs each\n\n", *reads)
	for _, m := range modes {
		var total time.Duration
		var n int
		for r := 1; r <= 3; r++ {
			length, d := runOnce(m, *reads)
			n, total = length, total+d
			fmt.Printf("%-8s run %d: len=%d  time=%v\n", m, r, length, d)
		}
		fmt.Printf("%-8s MEAN:  len=%d  time=%v\n\n", m, n, total/3)
	}
}
