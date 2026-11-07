package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"

	"github.com/demicloud/romctl/internal/actions"
	"github.com/demicloud/romctl/internal/cliutil"
	"github.com/demicloud/romctl/internal/executil"
	"github.com/demicloud/romctl/internal/formats/xiso"
	"github.com/demicloud/romctl/internal/gamelist"
	"github.com/demicloud/romctl/internal/library"
	"github.com/demicloud/romctl/internal/systemdb"
	"github.com/demicloud/romctl/internal/tools"
)

// Xiso converts Xbox disc images to XISO. A file that's already a valid
// XISO (just misnamed .iso) is renamed directly; a full/redump dump (which
// has a video partition ahead of the actual XDVDFS game data, at one of
// three fixed offsets depending on XGD1/2/3 disc layout) is rebuilt via
// embedded/PATH extract-xiso -r, which finds the real game partition and
// rewrites it as a trimmed, valid XISO.
func Xiso(lib *library.Library, runner *actions.Runner, systems []string) (bool, error) {
	log := cliutil.NewLogger("xiso")
	var converted, renamed, failed atomic.Int64

	cliutil.Parallel(systems, func(sys string) {
		if systemdb.ClassOf(sys) != systemdb.ClassXbox {
			return
		}
		sysDir := filepath.Join(lib.Roms, sys)
		entries, err := os.ReadDir(sysDir)
		if err != nil {
			return
		}
		for _, e := range entries {
			if e.IsDir() || strings.ToLower(filepath.Ext(e.Name())) != ".iso" {
				continue
			}
			base := e.Name()
			src := filepath.Join(sysDir, base)
			out := strings.TrimSuffix(src, filepath.Ext(src)) + ".xiso"
			if exists(out) {
				continue
			}

			ok, err := xiso.IsXISO(src)
			if err != nil {
				continue
			}

			if ok {
				if runner.Act("rename %s/%s -> %s", sys, base, filepath.Base(out)) {
					if err := os.Rename(src, out); err != nil {
						log.Errf("rename failed: %s: %v", src, err)
						failed.Add(1)
						continue
					}
					if err := gamelist.Rename(lib.Root, sys, base, filepath.Base(out)); err != nil {
						log.Errf("gamelist update failed: %s/%s: %v", sys, base, err)
					}
					renamed.Add(1)
				}
				continue
			}

			if !runner.Act("convert %s/%s -> %s", sys, base, filepath.Base(out)) {
				continue
			}
			if err := convertToXISO(src, out); err != nil {
				log.Errf("not an XISO, skipped: %s: %v", src, err)
				failed.Add(1)
				continue
			}
			if err := gamelist.Rename(lib.Root, sys, base, filepath.Base(out)); err != nil {
				log.Errf("gamelist update failed: %s/%s: %v", sys, base, err)
			}
			converted.Add(1)
		}
	})

	if c, r, f := converted.Load(), renamed.Load(), failed.Load(); c+r+f > 0 {
		fmt.Printf("xiso: %d converted, %d renamed, %d failed\n", c, r, f)
	}
	return failed.Load() == 0, nil
}

// convertToXISO rebuilds a full/redump Xbox disc image in place via
// extract-xiso -r, which renames the untouched original to src+".old";
// that backup is retired through the usual _to_delete/ safety net instead
// of using the tool's own -D (delete) flag. extract-xiso writes its
// rebuilt output relative to the current directory rather than src's own
// directory, so it's run with cwd set to src's directory and given just
// its base name.
func convertToXISO(src, out string) error {
	path, err := tools.Resolve("extract-xiso")
	if err != nil {
		return err
	}
	dir, base := filepath.Dir(src), filepath.Base(src)
	cmdOut, err := executil.RunIn(dir, path, "-r", base)
	if err != nil {
		return fmt.Errorf("extract-xiso -r: %w: %s", err, cmdOut)
	}
	backup := src + ".old"
	if !exists(backup) {
		return fmt.Errorf("extract-xiso -r produced no output")
	}
	if err := os.Rename(src, out); err != nil {
		return err
	}
	return actions.Retire(backup)
}
