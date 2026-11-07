package actions

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// TestRetireConcurrent guards against a regression of the race fixed when
// conversion commands started running their per-system work in parallel:
// retired used to be a plain unguarded int, which would lose increments
// under concurrent Retire calls.
func TestRetireConcurrent(t *testing.T) {
	dir := t.TempDir()
	const n = 50
	before := RetiredCount()

	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			path := filepath.Join(dir, fmt.Sprintf("file%d.txt", i))
			if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
				t.Error(err)
				return
			}
			if err := Retire(path); err != nil {
				t.Error(err)
			}
		}(i)
	}
	wg.Wait()

	if got := RetiredCount() - before; got != n {
		t.Errorf("RetiredCount increased by %d, want %d", got, n)
	}
}
