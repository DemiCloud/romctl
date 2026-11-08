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
	"sync"
	"sync/atomic"

	"github.com/demicloud/romctl/internal/cliutil"
)

// Runner announces or performs a mutating step depending on DryRun, and
// collects what it did for Format's structured encodings.
type Runner struct {
	DryRun bool
	// Verbose prints a "do:" line for every action as it's applied; off
	// by default, every mutating command still prints its own one-line
	// summary once finished. DryRun always previews in full regardless -
	// that's the whole point of --dry-run - so Verbose has no effect
	// there.
	Verbose bool
	// Format drives Act's rendering: colored plan:/do: lines in Fancy,
	// unstyled in Plain, and recorded into Log instead of printed for
	// every structured format (json/yaml/toml/csv) - main.go builds one
	// Result from Snapshot/Log/DryRun|&c. after the command returns,
	// rather than the command printing its own text piecemeal.
	Format cliutil.Format

	mu     sync.Mutex
	counts map[string]int64
	log    []string
}

// Act prints a plan:/do: line for desc (recording it instead, under a
// structured Format - see Log) and reports whether the caller should
// actually perform the action: true when applying, false when previewing
// (the caller should print the line and skip the side effect).
func (r *Runner) Act(format string, args ...any) bool {
	desc := fmt.Sprintf(format, args...)
	if r.Format.Structured() {
		r.mu.Lock()
		r.log = append(r.log, desc)
		r.mu.Unlock()
		return !r.DryRun
	}
	colors := cliutil.ColorsFor(r.Format)
	if r.DryRun {
		fmt.Printf("%splan:%s %s\n", colors.Yellow, colors.Reset, desc)
		return false
	}
	if r.Verbose {
		fmt.Printf("%sdo:%s   %s\n", colors.Green, colors.Reset, desc)
	}
	return true
}

// Count increments key in this run's structured result (see Result);
// SetCount is the cheaper alternative for a command that already tracks
// its own tally locally (e.g. an atomic.Int64 per outcome inside a
// parallel loop) and just wants to report the final number once.
func (r *Runner) Count(key string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.counts == nil {
		r.counts = map[string]int64{}
	}
	r.counts[key]++
}

// SetCount overwrites key's count directly; see Count.
func (r *Runner) SetCount(key string, value int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.counts == nil {
		r.counts = map[string]int64{}
	}
	r.counts[key] = value
}

// Snapshot returns a copy of every count reported so far.
func (r *Runner) Snapshot() map[string]int64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make(map[string]int64, len(r.counts))
	for k, v := range r.counts {
		out[k] = v
	}
	return out
}

// Log returns every action Act has recorded so far (see Act's Format.Structured() branch).
func (r *Runner) Log() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.log...)
}

// Result is a mutating command's structured payload: main.go builds one
// from the shared Runner after dispatch returns and encodes it via
// cliutil.Encode/EncodeCSV, instead of each command hand-rolling its own
// JSON/YAML/TOML/CSV. Counts' keys are whatever the command itself
// chose to report (e.g. chd's "converted"/"skipped"/"failed") - running
// `fix`, which shares one Runner across every step it chains, merges
// same-named keys across steps (several steps report "converted" or
// "failed"), trading per-step precision for one combined total; run a
// step on its own for a per-step breakdown.
type Result struct {
	Command string           `json:"command" yaml:"command" toml:"command"`
	Systems []string         `json:"systems,omitempty" yaml:"systems,omitempty" toml:"systems,omitempty"`
	Counts  map[string]int64 `json:"counts" yaml:"counts" toml:"counts"`
	Actions []string         `json:"actions,omitempty" yaml:"actions,omitempty" toml:"actions,omitempty"`
	Failed  bool             `json:"failed" yaml:"failed" toml:"failed"`
	Seconds float64          `json:"seconds" yaml:"seconds" toml:"seconds"`
}

// retired counts successful Retire calls during this process, for the
// pending-cleanup note main prints after a mutating run. One romctl
// invocation runs a single command, but conversion commands run every
// matching file across every system concurrently, so this is atomic
// rather than a plain int.
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
