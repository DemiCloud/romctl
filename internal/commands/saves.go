package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"

	"github.com/demicloud/romctl/internal/actions"
	"github.com/demicloud/romctl/internal/cliutil"
	"github.com/demicloud/romctl/internal/library"
)

// saveExtDirs routes a stray save-data extension (the same set
// checkStructureFile flags as "does not belong in roms/", see check.go)
// to its RetroDECK-standard destination directory, a sibling of roms/ and
// bios/ at the library root: save states go in states/<system>/,
// everything else (sav/srm/sh) in saves/<system>/ - RetroDECK's own
// per-system layout since v0.5.0b, so files end up exactly where a
// RetroDECK install (or RetroArch pointed at the same layout) already
// expects to find them.
var saveExtDirs = map[string]func(l *library.Library) string{
	"sav":   func(l *library.Library) string { return l.Saves },
	"srm":   func(l *library.Library) string { return l.Saves },
	"sh":    func(l *library.Library) string { return l.Saves },
	"state": func(l *library.Library) string { return l.States },
}

// Saves relocates stray save/save-state files out of roms/<system>/ into
// RetroDECK's own saves/<system>/ or states/<system>/ at the library
// root. Unlike every format-conversion command, nothing here is ever
// retired or deleted - these are the user's actual save data, just not
// part of a ROM dump - so a file whose destination already exists is left
// in place rather than overwritten. There is no scraped-metadata update
// to make (gamelist.Rename): a stray save file was never itself a
// scraped entry, the same reasoning that exempts internal/discimage's PS3
// path (see AGENTS.md).
func Saves(lib *library.Library, runner *actions.Runner, systems []string) (bool, error) {
	log := cliutil.NewLogger("saves")
	var moved, failed atomic.Int64

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
			destRoot, ok := saveExtDirs[ext]
			if !ok {
				continue
			}
			src := filepath.Join(sysDir, base)
			destDir := filepath.Join(destRoot(lib), sys)
			dst := filepath.Join(destDir, base)
			if exists(dst) {
				log.Errf("exists, skipped: %s", dst)
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

	fmt.Printf("saves: %d moved, %d failed\n", moved.Load(), failed.Load())
	return failed.Load() == 0, nil
}
