package commands

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/demicloud/romctl/internal/actions"
	"github.com/demicloud/romctl/internal/cliutil"
	"github.com/demicloud/romctl/internal/library"
)

// Clean permanently deletes every system's _to_delete/ directory, plus
// any _to_delete/ nested one level deeper inside a Title.m3u/ multi-disc
// directory (chd's DVD re-encode pass retires a replaced CD-type CHD
// right next to it, inside the playlist directory, same as any other
// conversion retires a replaced original beside it - see reencodeAsDVD).
// Unlike every other command, this step is not reversible, so it asks
// for confirmation before actually deleting unless assumeYes is set
// (non-interactive runs, e.g. CI, never delete without it: Confirm
// defaults to "no" when stdin isn't a terminal).
func Clean(lib *library.Library, runner *actions.Runner, systems []string, assumeYes bool) error {
	log := cliutil.NewLogger("clean", runner.Format)

	type target struct {
		sys, dir, label string
		size            int64
	}
	var targets []*target
	for _, sys := range systems {
		sysDir := filepath.Join(lib.Roms, sys)
		if dir := filepath.Join(sysDir, "_to_delete"); exists(dir) {
			targets = append(targets, &target{sys: sys, dir: dir, label: filepath.Join(sys, "_to_delete")})
		}
		nested, _ := filepath.Glob(filepath.Join(sysDir, "*.m3u", "_to_delete"))
		for _, dir := range nested {
			if rel, err := filepath.Rel(lib.Roms, dir); err == nil {
				targets = append(targets, &target{sys: sys, dir: dir, label: rel})
			}
		}
	}
	if len(targets) == 0 {
		if runner.Format.Structured() {
			runner.SetCount("deleted", 0)
			return nil
		}
		colors := cliutil.ColorsFor(runner.Format)
		sym := "OK"
		if runner.Format.Fancy() {
			sym = "✓"
		}
		fmt.Printf("%s%s nothing to clean%s\n", colors.Green, sym, colors.Reset)
		return nil
	}

	// Measuring each _to_delete/ (a full directory walk) is the only slow
	// part here - do it for every system up front, in parallel.
	cliutil.Parallel(targets, func(t *target) { t.size = dirSize(t.dir) })

	var toDelete []*target
	var total int64
	for _, t := range targets {
		if !runner.Act("delete %s (%s)", t.label, humanSize(t.size)) {
			continue
		}
		toDelete = append(toDelete, t)
		total += t.size
	}
	if len(toDelete) == 0 {
		if runner.Format.Structured() {
			runner.SetCount("deleted", 0)
		}
		return nil
	}

	// One confirmation for the whole batch instead of one per system: a
	// multi-system `clean all` would otherwise mean answering the same
	// question a dozen times in a row for no added safety. This - like
	// Confirm's own prompt - always goes to stderr, not stdout: stdout
	// stays reserved for the structured payload when one is requested, and
	// it's confirmation-aid text either way, not clean's actual result.
	if !assumeYes {
		fmt.Fprintln(os.Stderr, "This will permanently delete:")
		for _, t := range toDelete {
			fmt.Fprintf(os.Stderr, "  %s (%s)\n", t.dir, humanSize(t.size))
		}
		if !cliutil.Confirm(fmt.Sprintf("Permanently delete %d item(s) totaling %s? This cannot be undone.", len(toDelete), humanSize(total))) {
			log.Errf("skipped (not confirmed)")
			if runner.Format.Structured() {
				runner.SetCount("deleted", 0)
			}
			return nil
		}
	}
	var deleted, failed int64
	for _, t := range toDelete {
		if err := os.RemoveAll(t.dir); err != nil {
			log.Errf("delete failed: %s: %v", t.dir, err)
			failed++
			continue
		}
		deleted++
	}
	if runner.Format.Structured() {
		runner.SetCount("deleted", deleted)
		runner.SetCount("failed", failed)
	}
	return nil
}

func dirSize(dir string) int64 {
	var total int64
	filepath.Walk(dir, func(_ string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			total += info.Size()
		}
		return nil
	})
	return total
}

func humanSize(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%dB", n)
	}
	div, exp := int64(unit), 0
	for v := n / unit; v >= unit; v /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f%ciB", float64(n)/float64(div), "KMGTPE"[exp])
}
