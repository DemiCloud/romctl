package commands

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/demicloud/romctl/internal/actions"
	"github.com/demicloud/romctl/internal/cliutil"
	"github.com/demicloud/romctl/internal/library"
)

// Clean permanently deletes every system's _to_delete/ directory. Unlike
// every other command, this step is not reversible, so it asks for
// confirmation before actually deleting unless assumeYes is set
// (non-interactive runs, e.g. CI, never delete without it: Confirm
// defaults to "no" when stdin isn't a terminal).
func Clean(lib *library.Library, runner *actions.Runner, systems []string, assumeYes bool) error {
	log := cliutil.NewLogger("clean")

	type target struct {
		sys, dir string
		size     int64
	}
	var targets []*target
	for _, sys := range systems {
		dir := filepath.Join(lib.Roms, sys, "_to_delete")
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			targets = append(targets, &target{sys: sys, dir: dir})
		}
	}
	if len(targets) == 0 {
		colors := cliutil.NewColors(os.Stdout)
		fmt.Printf("%s✓ nothing to clean%s\n", colors.Green, colors.Reset)
		return nil
	}

	// Measuring each _to_delete/ (a full directory walk) is the only slow
	// part here - do it for every system up front, in parallel, before the
	// loop below, which must stay sequential since it prompts
	// interactively per system.
	cliutil.Parallel(targets, func(t *target) { t.size = dirSize(t.dir) })

	for _, t := range targets {
		if !runner.Act("delete %s/_to_delete (%s)", t.sys, humanSize(t.size)) {
			continue
		}
		if !assumeYes && !cliutil.Confirm(fmt.Sprintf("Permanently delete %s? This cannot be undone.", t.dir)) {
			log.Errf("skipped (not confirmed): %s", t.dir)
			continue
		}
		if err := os.RemoveAll(t.dir); err != nil {
			log.Errf("delete failed: %s: %v", t.dir, err)
		}
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
