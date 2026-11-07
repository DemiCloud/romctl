package cliutil

import (
	"runtime"
	"sync"
)

// Parallel runs fn for every item in items across a worker pool sized to
// GOMAXPROCS, blocking until every call returns. Intended for I/O-bound
// work (file reads, hashing) where wall-clock time, not CPU, is the
// bottleneck - fn must not touch shared state that isn't itself
// concurrency-safe.
func Parallel[T any](items []T, fn func(T)) {
	if len(items) == 0 {
		return
	}
	workers := runtime.GOMAXPROCS(0)
	if workers > len(items) {
		workers = len(items)
	}
	sem := make(chan struct{}, workers)
	var wg sync.WaitGroup
	for _, item := range items {
		wg.Add(1)
		sem <- struct{}{}
		go func(item T) {
			defer wg.Done()
			defer func() { <-sem }()
			fn(item)
		}(item)
	}
	wg.Wait()
}
