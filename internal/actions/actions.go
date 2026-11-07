// Package actions provides the dry-run/apply framework shared by every
// mutating command, plus the filesystem primitives (retire, case-safe
// rename) and title-cleaning logic the commands build on.
//
// Commands apply by default; passing --dry-run previews instead. This is
// the opposite default from the historical Bash tool (dry-run unless
// --apply), a deliberate choice to cut down on typing --apply for every
// command. Nothing is still deleted outright: Retire moves replaced
// originals to _to_delete/, and only the "clean" command removes them.
package actions

import (
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
)

// Runner announces or performs a mutating step depending on DryRun.
type Runner struct {
	DryRun bool
}

// Act prints a plan:/do: line for desc and reports whether the caller
// should actually perform the action: true when applying, false when
// previewing (the caller should print the line and skip the side effect).
func (r *Runner) Act(format string, args ...any) bool {
	desc := fmt.Sprintf(format, args...)
	if r.DryRun {
		fmt.Printf("plan: %s\n", desc)
		return false
	}
	fmt.Printf("do:   %s\n", desc)
	return true
}

// retired counts successful Retire calls during this process, for the
// pending-cleanup note main prints after a mutating run. One romctl
// invocation runs a single command, but conversion commands now run their
// per-system work concurrently, so this is atomic rather than a plain int.
var retired atomic.Int64

// RetiredCount reports how many paths Retire has moved to _to_delete/
// during this process.
func RetiredCount() int {
	return int(retired.Load())
}

// Retire moves path into its directory's _to_delete/, creating it if
// needed. If a same-named entry already exists there, it is left in place
// (matching "mv -n": no-clobber is not an error).
func Retire(path string) error {
	trash := filepath.Join(filepath.Dir(path), "_to_delete")
	if err := os.MkdirAll(trash, 0o755); err != nil {
		return err
	}
	dst := filepath.Join(trash, filepath.Base(path))
	if _, err := os.Stat(dst); err == nil {
		return nil
	}
	if err := os.Rename(path, dst); err != nil {
		return err
	}
	retired.Add(1)
	return nil
}
