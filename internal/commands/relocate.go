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
	"github.com/demicloud/romctl/internal/formats/signature"
	"github.com/demicloud/romctl/internal/library"
	"github.com/demicloud/romctl/internal/systemdb"
)

// Relocate moves a cartridge/handheld ROM that's been misfiled into the
// wrong system's directory - identified by its own header, see
// internal/formats/signature, the same check check's "identifies as a ...
// ROM" finding uses - into its correct system directory (the first, most
// common system signature.Systems lists for its detected family; several
// systems can share one format, e.g. genesis/megadrive/megadrivejp, and
// content alone can't say which region-specific folder a given dump was
// meant for). Only bare files are checked, same cost tradeoff as check's
// own containers category: archive contents aren't inspected here, so a
// misfiled ROM still inside an archive is instead caught by chd/unpack's
// own conversion errors. Like Saves, nothing here is ever overwritten: a
// destination that already exists is left in both places and reported as
// a conflict instead of guessed at.
func Relocate(lib *library.Library, runner *actions.Runner, systems []string) (bool, error) {
	start := time.Now()
	log := cliutil.NewLogger("relocate", runner.Format)
	var moved, failed atomic.Int64
	var mu sync.Mutex
	var conflicts []fileConflict

	cliutil.Parallel(systems, func(sys string) {
		if systemdb.ClassOf(sys) == systemdb.ClassArcade {
			return
		}
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
			path := filepath.Join(sysDir, base)
			fam, ok := signature.Identify(path)
			if !ok || signature.ValidFor(fam, sys) {
				continue
			}
			destSys := signature.Systems[fam][0]
			destDir := filepath.Join(lib.Roms, destSys)
			dst := filepath.Join(destDir, base)
			if exists(dst) {
				c := compareFileConflict(sys, base, filepath.Join("roms", sys), filepath.Join("roms", destSys), path, dst)
				mu.Lock()
				conflicts = append(conflicts, c)
				mu.Unlock()
				continue
			}
			if !runner.Act("%s is a %s ROM, move %s/%s -> %s/%s", base, signature.Name[fam], sys, base, destSys, base) {
				continue
			}
			if err := os.MkdirAll(destDir, 0o755); err != nil {
				log.Errf("mkdir failed: %s: %v", destDir, err)
				failed.Add(1)
				continue
			}
			if err := os.Rename(path, dst); err != nil {
				log.Errf("move failed: %s: %v", path, err)
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
	line := fmt.Sprintf("%srelocate: %d moved, %d conflicts, %d failed in %s%s\n", color, moved.Load(), len(conflicts), failed.Load(), time.Since(start).Round(time.Second), reset)
	if runner.Quiet {
		var buf strings.Builder
		printFileConflicts(&buf, conflicts)
		runner.Summarize(line + buf.String())
	} else {
		fmt.Print(line)
		printFileConflicts(os.Stdout, conflicts)
	}
	return failed.Load() == 0, nil
}
