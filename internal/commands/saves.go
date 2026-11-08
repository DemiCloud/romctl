package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/demicloud/romctl/internal/actions"
	"github.com/demicloud/romctl/internal/cliutil"
	"github.com/demicloud/romctl/internal/library"
)

// saveExtSubdir routes a stray save-data extension (the same set
// checkStructureFile flags as "does not belong in roms/", see check.go)
// to its RetroDECK-standard destination subdirectory name: save states go
// in states/<system>/, everything else (sav/srm/sh) in saves/<system>/ -
// RetroDECK's own per-system layout since v0.5.0b, so files end up
// exactly where a RetroDECK install (or RetroArch pointed at the same
// layout) already expects to find them.
var saveExtSubdir = map[string]string{
	"sav": "saves", "srm": "saves", "sh": "saves",
	"state": "states",
}

// Saves relocates stray save/save-state files out of roms/<system>/ into
// saves/<system>/ or states/<system>/, under the library root by default
// or under dest if set (e.g. to keep save data somewhere other than the
// RetroDECK-managed tree entirely). Unlike every format-conversion
// command, nothing here is ever retired or deleted - these are the
// user's actual save data, just not part of a ROM dump - so a file whose
// destination already exists is left in both places and reported as a
// conflict instead, rather than silently dropped or overwritten. There
// is no scraped-metadata update to make (gamelist.Rename): a stray save
// file was never itself a scraped entry, the same reasoning that exempts
// internal/discimage's PS3 path (see AGENTS.md).
func Saves(lib *library.Library, runner *actions.Runner, systems []string, dest string) (bool, error) {
	start := time.Now()
	log := cliutil.NewLogger("saves", runner.Format)
	destRoot := lib.Root
	if dest != "" {
		destRoot = dest
	}
	var moved, failed atomic.Int64
	var mu sync.Mutex
	var conflicts []fileConflict

	cliutil.Parallel(systems, func(sys string) {
		sysDir := filepath.Join(lib.Roms, sys)
		entries, err := os.ReadDir(sysDir)
		if err != nil {
			return
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			base := e.Name()
			ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(base), "."))
			subdir, ok := saveExtSubdir[ext]
			if !ok {
				continue
			}
			src := filepath.Join(sysDir, base)
			destDir := filepath.Join(destRoot, subdir, sys)
			dst := filepath.Join(destDir, base)
			if exists(dst) {
				c := compareFileConflict(sys, base, filepath.Join("roms", sys), filepath.Join(subdir, sys), src, dst)
				mu.Lock()
				conflicts = append(conflicts, c)
				mu.Unlock()
				continue
			}
			if !runner.Act("move %s/%s -> %s", sys, base, dst) {
				continue
			}
			if err := os.MkdirAll(destDir, 0o755); err != nil {
				log.Errf("mkdir failed: %s: %v", destDir, err)
				failed.Add(1)
				continue
			}
			if err := os.Rename(src, dst); err != nil {
				log.Errf("move failed: %s: %v", src, err)
				failed.Add(1)
				continue
			}
			moved.Add(1)
		}
	})

	if runner.Format.Structured() {
		runner.SetCount("moved", moved.Load())
		runner.SetCount("conflicts", int64(len(conflicts)))
		runner.SetCount("failed", failed.Load())
		return failed.Load() == 0, nil
	}
	color, reset := cliutil.StatusColor(runner.Format, failed.Load() > 0)
	fmt.Printf("%ssaves: %d moved, %d conflicts, %d failed in %s%s\n", color, moved.Load(), len(conflicts), failed.Load(), time.Since(start).Round(time.Second), reset)
	printFileConflicts(os.Stdout, conflicts)
	return failed.Load() == 0, nil
}
