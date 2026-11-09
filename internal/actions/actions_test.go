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

// TestActPlanDoesNotCountAsPerformed guards the "don't forget to sync"
// reminder's gate: a --dry-run preview must never register as a real
// action, or the reminder would fire after a run that changed nothing.
func TestActPlanDoesNotCountAsPerformed(t *testing.T) {
	before := ActionsPerformed()
	r := &Runner{DryRun: true}
	if ok := r.Act("would do %s", "something"); ok {
		t.Error("Act should report false under DryRun")
	}
	if got := ActionsPerformed() - before; got != 0 {
		t.Errorf("ActionsPerformed increased by %d during a dry run, want 0", got)
	}
}

// TestActCountsPerformedAction is Plan's counterpart: a real (non-dry-run)
// Act call must register, whether or not the format is structured - both
// branches increment actionsPerformed independently.
func TestActCountsPerformedAction(t *testing.T) {
	before := ActionsPerformed()
	r := &Runner{}
	if ok := r.Act("do %s", "something"); !ok {
		t.Error("Act should report true when applying")
	}
	if got := ActionsPerformed() - before; got != 1 {
		t.Errorf("ActionsPerformed increased by %d, want 1", got)
	}
}
