package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"

	"github.com/demicloud/romctl/internal/actions"
	"github.com/demicloud/romctl/internal/cliutil"
	"github.com/demicloud/romctl/internal/gamelist"
	"github.com/demicloud/romctl/internal/library"
	"github.com/demicloud/romctl/internal/systemdb"
)

// Rename applies scheme (`[naming] scheme` in config) to every loose file
// in the given systems (arcade systems are skipped: their filenames are
// emulator-resolved set names). See internal/actions.ApplyScheme.
func Rename(lib *library.Library, runner *actions.Runner, systems []string, scheme actions.Scheme) error {
	log := cliutil.NewLogger("rename")
	var renamed, failed atomic.Int64
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
			if base == "systeminfo.txt" || strings.HasPrefix(base, ".") {
				continue
			}
			ext := filepath.Ext(base)
			name := strings.TrimSuffix(base, ext)
			clean := actions.ApplyScheme(name, scheme)
			if clean == name {
				continue
			}
			src := filepath.Join(sysDir, base)
			dst := filepath.Join(sysDir, clean+ext)

			if same, _ := sameExists(dst, src); exists(dst) && !same {
				log.Errf("collision, skipped: %s/%s -> %s%s", sys, base, clean, ext)
				continue
			}
			if !runner.Act("rename %s/%s -> %s%s", sys, base, clean, ext) {
				continue
			}
			if err := actions.RenamePath(src, dst); err != nil {
				log.Errf("rename failed: %s/%s: %v", sys, base, err)
				failed.Add(1)
				continue
			}
			if err := gamelist.Rename(lib.Root, sys, base, clean+ext); err != nil {
				log.Errf("gamelist update failed: %s/%s: %v", sys, base, err)
			}
			renamed.Add(1)
		}
	})
	fmt.Printf("rename: %d renamed, %d failed\n", renamed.Load(), failed.Load())
	return nil
}

func sameExists(dst, src string) (bool, error) {
	di, err := os.Stat(dst)
	if err != nil {
		return false, err
	}
	si, err := os.Stat(src)
	if err != nil {
		return false, err
	}
	return os.SameFile(di, si), nil
}
