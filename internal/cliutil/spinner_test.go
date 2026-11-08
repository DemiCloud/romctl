package cliutil

import (
	"sync"
	"sync/atomic"
	"testing"
)

// TestMaxLiveSlotsFallsBackWithoutATerminal guards maxLiveSlots' fallback:
// go test's stderr is never a real terminal, so term.GetSize always fails
// here, and the conservative fixed fallback (8) must be returned instead
// of 0 or a negative number that would break ParallelLive's worker math.
func TestMaxLiveSlotsFallsBackWithoutATerminal(t *testing.T) {
	if got := maxLiveSlots(); got != 8 {
		t.Errorf("maxLiveSlots() = %d, want the 8-slot fallback (no real terminal under go test)", got)
	}
}

// TestParallelLiveProcessesEveryItem guards the worker-pool/event dispatch
// in ParallelLive's non-animated fallback path - the only one exercisable
// under go test, which never runs with a real terminal attached, so
// CanAnimate is always false here regardless of format. Every item must
// still reach fn exactly once, with its result correctly reflected.
func TestParallelLiveProcessesEveryItem(t *testing.T) {
	items := make([]int, 50)
	for i := range items {
		items[i] = i
	}

	var mu sync.Mutex
	seen := map[int]int{}
	var passes, fails atomic.Int64

	ParallelLive(FormatPlain, items, func(i int) string { return "" }, func(i int) bool {
		mu.Lock()
		seen[i]++
		mu.Unlock()
		if i%7 == 0 {
			fails.Add(1)
			return false
		}
		passes.Add(1)
		return true
	})

	if len(seen) != len(items) {
		t.Fatalf("got %d distinct items processed, want %d", len(seen), len(items))
	}
	for i, n := range seen {
		if n != 1 {
			t.Errorf("item %d processed %d times, want 1", i, n)
		}
	}
	var wantFails int64
	for _, i := range items {
		if i%7 == 0 {
			wantFails++
		}
	}
	if fails.Load() != wantFails {
		t.Errorf("fails = %d, want %d", fails.Load(), wantFails)
	}
	if passes.Load() != int64(len(items))-wantFails {
		t.Errorf("passes = %d, want %d", passes.Load(), int64(len(items))-wantFails)
	}
}

// TestParallelLiveEmptyItems guards the len(items)==0 short-circuit ahead
// of the animated branch's worker-count math - it must just return
// immediately, calling fn zero times.
func TestParallelLiveEmptyItems(t *testing.T) {
	called := false
	ParallelLive(FormatPlain, []int{}, func(i int) string { return "" }, func(i int) bool {
		called = true
		return true
	})
	if called {
		t.Error("fn should never be called for an empty item slice")
	}
}
