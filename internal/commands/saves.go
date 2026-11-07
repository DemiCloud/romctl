package commands

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
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

// saveConflict records a stray file left in place because something
// already occupies its destination - printed as its own section so the
// user can tell at a glance whether it's safe to just delete one copy
// (identical) or needs a manual look (different content, or either side
// unreadable).
type saveConflict struct {
	sys, name        string
	srcDir, dstDir   string
	srcSize, dstSize int64
	srcMod, dstMod   time.Time
	identical        bool
	compareErr       bool
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
	log := cliutil.NewLogger("saves")
	destRoot := lib.Root
	if dest != "" {
		destRoot = dest
	}
	var moved, failed atomic.Int64
	var mu sync.Mutex
	var conflicts []saveConflict

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
				c := compareSaveConflict(sys, base, filepath.Join("roms", sys), filepath.Join(subdir, sys), src, dst)
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

	fmt.Printf("saves: %d moved, %d conflicts, %d failed\n", moved.Load(), len(conflicts), failed.Load())
	printSaveConflicts(os.Stdout, conflicts)
	return failed.Load() == 0, nil
}

// compareSaveConflict stats and, size permitting, content-compares src
// and dst (already known to both exist) so printSaveConflicts can tell
// the user whether the stray copy in roms/ is safe to discard or needs a
// manual look.
func compareSaveConflict(sys, name, srcLabel, dstLabel, src, dst string) saveConflict {
	c := saveConflict{sys: sys, name: name, srcDir: srcLabel, dstDir: dstLabel}
	srcInfo, srcErr := os.Stat(src)
	dstInfo, dstErr := os.Stat(dst)
	if srcErr != nil || dstErr != nil {
		c.compareErr = true
		return c
	}
	c.srcSize, c.srcMod = srcInfo.Size(), srcInfo.ModTime()
	c.dstSize, c.dstMod = dstInfo.Size(), dstInfo.ModTime()
	if c.srcSize != c.dstSize {
		return c
	}
	// Save/state files are small (KB, rarely multi-MB), so a CRC32 of
	// each - same check checkDuplicates uses for exact-duplicate
	// detection elsewhere - is cheap here even run per conflict.
	srcSum, err := crc32File(src)
	if err != nil {
		c.compareErr = true
		return c
	}
	dstSum, err := crc32File(dst)
	if err != nil {
		c.compareErr = true
		return c
	}
	c.identical = srcSum == dstSum
	return c
}

// printSaveConflicts lists every stray file left in roms/ because its
// destination was already occupied, each with both copies' size and
// modification time so the user can judge which one to keep.
func printSaveConflicts(w io.Writer, conflicts []saveConflict) {
	if len(conflicts) == 0 {
		return
	}
	sort.Slice(conflicts, func(i, j int) bool {
		if conflicts[i].sys != conflicts[j].sys {
			return conflicts[i].sys < conflicts[j].sys
		}
		return conflicts[i].name < conflicts[j].name
	})
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Conflicts (destination already exists; left in roms/ untouched):")
	const timeFmt = "2006-01-02 15:04"
	for _, c := range conflicts {
		if c.compareErr {
			fmt.Fprintf(w, "  %-10s %-32s %s vs %s: could not compare\n", c.sys, c.name, c.srcDir, c.dstDir)
			continue
		}
		status := "DIFFERENT - resolve manually"
		if c.identical {
			status = "identical, safe to delete either copy"
		}
		fmt.Fprintf(w, "  %-10s %-32s %s: %s, %s   vs   %s: %s, %s   [%s]\n",
			c.sys, c.name,
			c.srcDir, humanSize(c.srcSize), c.srcMod.Format(timeFmt),
			c.dstDir, humanSize(c.dstSize), c.dstMod.Format(timeFmt),
			status)
	}
}
