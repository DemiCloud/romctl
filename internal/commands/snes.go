package commands

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/demicloud/romctl/internal/actions"
	"github.com/demicloud/romctl/internal/cliutil"
	"github.com/demicloud/romctl/internal/formats/snes"
	"github.com/demicloud/romctl/internal/gamelist"
	"github.com/demicloud/romctl/internal/library"
	"github.com/demicloud/romctl/internal/systemdb"
)

// SNES strips 512-byte copier headers and renames .smc to .sfc.
func SNES(lib *library.Library, runner *actions.Runner, systems []string) error {
	log := cliutil.NewLogger("snes")
	for _, sys := range systems {
		if systemdb.ClassOf(sys) != systemdb.ClassSNES {
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
			ext := strings.ToLower(filepath.Ext(base))
			if ext != ".smc" && ext != ".sfc" {
				continue
			}
			src := filepath.Join(sysDir, base)
			out := strings.TrimSuffix(src, filepath.Ext(src)) + ".sfc"
			outBase := filepath.Base(out)

			info, err := os.Stat(src)
			if err != nil {
				continue
			}
			switch {
			case snes.HasHeader(info.Size()):
				if !runner.Act("strip header %s/%s -> %s", sys, base, outBase) {
					continue
				}
				if err := stripSNESHeader(src, out); err != nil {
					log.Errf("strip header failed: %s/%s: %v", sys, base, err)
					continue
				}
				if err := gamelist.Rename(lib.Root, sys, base, outBase); err != nil {
					log.Errf("gamelist update failed: %s/%s: %v", sys, base, err)
				}
			case src != out:
				if exists(out) {
					log.Errf("exists, skipped: %s", out)
					continue
				}
				if runner.Act("rename %s/%s -> %s", sys, base, outBase) {
					if err := os.Rename(src, out); err != nil {
						log.Errf("rename failed: %s/%s: %v", sys, base, err)
						continue
					}
					if err := gamelist.Rename(lib.Root, sys, base, outBase); err != nil {
						log.Errf("gamelist update failed: %s/%s: %v", sys, base, err)
					}
				}
			}
		}
	}
	return nil
}

func stripSNESHeader(src, out string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if len(data) < snes.HeaderSize {
		return os.ErrInvalid
	}
	tmp := out + ".tmp"
	if err := os.WriteFile(tmp, data[snes.HeaderSize:], 0o644); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := actions.Retire(src); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, out)
}
