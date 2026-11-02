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
	found := false
	for _, sys := range systems {
		dir := filepath.Join(lib.Roms, sys, "_to_delete")
		info, err := os.Stat(dir)
		if err != nil || !info.IsDir() {
			continue
		}
		found = true
		size := humanSize(dirSize(dir))
		if !runner.Act("delete %s/_to_delete (%s)", sys, size) {
			continue
		}
		if !assumeYes && !cliutil.Confirm(fmt.Sprintf("Permanently delete %s? This cannot be undone.", dir)) {
			log.Errf("skipped (not confirmed): %s", dir)
			continue
		}
		if err := os.RemoveAll(dir); err != nil {
			log.Errf("delete failed: %s: %v", dir, err)
		}
	}
	if !found {
		colors := cliutil.NewColors(os.Stdout)
		fmt.Printf("%s✓ nothing to clean%s\n", colors.Green, colors.Reset)
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
