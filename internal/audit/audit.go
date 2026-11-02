// Package audit implements the check/bios findings report: the
// FAIL/WARN/INFO lines, the per-category checklist, and the colorized
// summary.
package audit

import (
	"fmt"
	"io"

	"github.com/demicloud/romctl/internal/cliutil"
)

// Level is a finding's severity.
type Level string

const (
	Fail Level = "FAIL"
	Warn Level = "WARN"
	Info Level = "INFO"
)

// Category groups findings for the checklist "check" prints alongside the
// per-finding lines, so a clean run still says what was actually looked at.
type Category string

const (
	CategoryNaming     Category = "naming"
	CategoryStructure  Category = "structure"
	CategoryContainers Category = "containers"
	CategoryBios       Category = "bios"
	CategoryDuplicates Category = "duplicates"
)

var categoryOrder = []Category{CategoryNaming, CategoryStructure, CategoryContainers, CategoryBios, CategoryDuplicates}

var categoryLabels = map[Category]string{
	CategoryNaming:     "naming (tags, articles)",
	CategoryStructure:  "structure (leftovers, playlists)",
	CategoryContainers: "containers (formats, conversions)",
	CategoryBios:       "bios (presence, hashes)",
	CategoryDuplicates: "duplicates (exact content matches)",
}

type tally struct{ fail, warn, info int }

// Report accumulates and prints audit findings.
type Report struct {
	w                  io.Writer
	fails, warns, info int
	byCategory         map[Category]*tally
	biosSkipped        bool
	dupesEnabled       bool
}

// New returns a Report that prints findings to w as they're added.
func New(w io.Writer) *Report {
	return &Report{w: w, byCategory: make(map[Category]*tally)}
}

// Add records and prints one finding. fix, if non-empty, is a command
// suffix shown as "[<prog> fix system]" so the hint can be copy-pasted.
func (r *Report) Add(level Level, category Category, system, path, message, fix string) {
	switch level {
	case Fail:
		r.fails++
	case Warn:
		r.warns++
	case Info:
		r.info++
	}
	t := r.byCategory[category]
	if t == nil {
		t = &tally{}
		r.byCategory[category] = t
	}
	switch level {
	case Fail:
		t.fail++
	case Warn:
		t.warn++
	case Info:
		t.info++
	}

	if fix != "" {
		fmt.Fprintf(r.w, "%-4s  %-12s %s: %s  [%s %s %s]\n",
			level, system, path, message, cliutil.ProgName(), fix, system)
		return
	}
	fmt.Fprintf(r.w, "%-4s  %-12s %s: %s\n", level, system, path, message)
}

// SkipBios marks the BIOS category as not run (no bios/ directory), so the
// checklist says "skipped" instead of a possibly-misleading "clean".
func (r *Report) SkipBios() {
	r.biosSkipped = true
}

// BiosSkipped reports whether SkipBios was called.
func (r *Report) BiosSkipped() bool {
	return r.biosSkipped
}

// EnableDuplicates marks the duplicates category as run (--dupes was
// passed), so the checklist shows its real tally instead of "skipped".
// Unlike every other check, finding exact duplicates means reading every
// file's full content, which is a meaningfully different cost on a
// library with large CHD/ISO files - so it only runs when asked for.
func (r *Report) EnableDuplicates() {
	r.dupesEnabled = true
}

// Failed reports whether any FAIL-level finding was recorded.
func (r *Report) Failed() bool {
	return r.fails > 0
}

// Checklist prints one line per Category, independent of whether it found
// anything, so a run that ends "Clean" still shows what was actually
// checked rather than just the absence of findings.
func (r *Report) Checklist(w io.Writer, colors cliutil.Colors) {
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Checks:")
	for _, cat := range categoryOrder {
		label := categoryLabels[cat]
		if cat == CategoryBios && r.biosSkipped {
			fmt.Fprintf(w, "  %-34s - skipped (no bios/ directory)\n", label)
			continue
		}
		if cat == CategoryDuplicates && !r.dupesEnabled {
			fmt.Fprintf(w, "  %-34s - skipped (use --dupes)\n", label)
			continue
		}
		t := r.byCategory[cat]
		if t == nil {
			t = &tally{}
		}
		switch {
		case t.fail > 0:
			fmt.Fprintf(w, "  %-34s %s✗ %d fail%s, %d warn\n", label, colors.Red, t.fail, colors.Reset, t.warn)
		case t.warn > 0:
			fmt.Fprintf(w, "  %-34s %s! %d warn%s\n", label, colors.Yellow, t.warn, colors.Reset)
		default:
			fmt.Fprintf(w, "  %-34s %s✓ clean%s\n", label, colors.Green, colors.Reset)
		}
	}
}

// Summary prints the colorized one-line tally, honoring NO_COLOR.
func (r *Report) Summary(w io.Writer, colors cliutil.Colors) {
	fmt.Fprintln(w)
	switch {
	case r.fails > 0:
		fmt.Fprintf(w, "%s✗ %d fail%s, %d warn, %d info\n", colors.Red, r.fails, colors.Reset, r.warns, r.info)
	case r.warns > 0:
		fmt.Fprintf(w, "%s! %d warn%s, %d info\n", colors.Yellow, r.warns, colors.Reset, r.info)
	case r.info > 0:
		fmt.Fprintf(w, "%s✓ Clean%s (%d info)\n", colors.Green, colors.Reset, r.info)
	default:
		fmt.Fprintf(w, "%s✓ Clean%s\n", colors.Green, colors.Reset)
	}
}
