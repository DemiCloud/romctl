package commands

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/demicloud/romctl/internal/actions"
	"github.com/demicloud/romctl/internal/cliutil"
	"github.com/demicloud/romctl/internal/gamelist"
	"github.com/demicloud/romctl/internal/library"
	"github.com/demicloud/romctl/internal/systemdb"
)

// Detag strips region/dump tags and moves a leading article to the end of
// the title for every loose file in the given systems (arcade systems are
// skipped: their filenames are emulator-resolved set names).
func Detag(lib *library.Library, runner *actions.Runner, systems []string) error {
	log := cliutil.NewLogger("detag")
	for _, sys := range systems {
		if systemdb.ClassOf(sys) == systemdb.ClassArcade {
			continue
		}
		sysDir := filepath.Join(lib.Roms, sys)
		entries, err := os.ReadDir(sysDir)
		if err != nil {
			continue
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
			clean := actions.CleanName(name)
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
				continue
			}
			if err := gamelist.Rename(lib.Root, sys, base, clean+ext); err != nil {
				log.Errf("gamelist update failed: %s/%s: %v", sys, base, err)
			}
		}
	}
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
