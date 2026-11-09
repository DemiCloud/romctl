package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/demicloud/romctl/internal/actions"
	"github.com/demicloud/romctl/internal/cliutil"
	"github.com/demicloud/romctl/internal/executil"
	"github.com/demicloud/romctl/internal/formats/xiso"
	"github.com/demicloud/romctl/internal/gamelist"
	"github.com/demicloud/romctl/internal/library"
	"github.com/demicloud/romctl/internal/systemdb"
	"github.com/demicloud/romctl/internal/tools"
)

// xisoJob is one .iso to examine/convert, flattened out of its system's
// directory listing up front - see CHD's doc comment for why every
// system's files run through one shared worker pool instead of one per
// system.
type xisoJob struct {
	sys, base string
}

// Xiso converts Xbox disc images to XISO. A file that's already a valid
// XISO (just misnamed .iso) is renamed directly; a full/redump dump (which
// has a video partition ahead of the actual XDVDFS game data, at one of
// three fixed offsets depending on XGD1/2/3 disc layout) is rebuilt via
// embedded/PATH extract-xiso -r, which finds the real game partition and
// rewrites it as a trimmed, valid XISO.
func Xiso(lib *library.Library, runner *actions.Runner, systems []string) (bool, error) {
	start := time.Now()
	log := cliutil.NewLogger("xiso", runner.Format)
	var converted, renamed, failed atomic.Int64

	var jobs []xisoJob
	for _, sys := range systems {
		if systemdb.ClassOf(sys) != systemdb.ClassXbox {
			continue
		}
		sysDir := filepath.Join(lib.Roms, sys)
		entries, err := os.ReadDir(sysDir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() || strings.ToLower(filepath.Ext(e.Name())) != ".iso" {
				continue
			}
			jobs = append(jobs, xisoJob{sys, e.Name()})
		}
	}

	cliutil.Progress(runner.Format, runner.Verbose || runner.Quiet, runner.DryRun, "xiso", len(jobs), func(tick func()) {
		cliutil.Parallel(jobs, func(j xisoJob) {
			defer tick()
			sys, base := j.sys, j.base
			src := filepath.Join(lib.Roms, sys, base)
			out := strings.TrimSuffix(src, filepath.Ext(src)) + ".xiso"
			if exists(out) {
				return
			}

			ok, err := xiso.IsXISO(src)
			if err != nil {
				return
			}

			if ok {
				if runner.Act("rename %s/%s -> %s", sys, base, filepath.Base(out)) {
					if err := os.Rename(src, out); err != nil {
						log.Errf("rename failed: %s: %v", src, err)
						failed.Add(1)
						return
					}
					if err := gamelist.Rename(lib.Root, sys, base, filepath.Base(out)); err != nil {
						log.Errf("gamelist update failed: %s/%s: %v", sys, base, err)
					}
					renamed.Add(1)
				}
				return
			}

			if msg := signatureMismatch(sys, src); msg != "" {
				log.Errf("%s", msg)
				failed.Add(1)
				return
			}
			if !runner.Act("convert %s/%s -> %s", sys, base, filepath.Base(out)) {
				return
			}
			if err := convertToXISO(src, out); err != nil {
				log.Errf("not an XISO, skipped: %s: %v", src, err)
				failed.Add(1)
				return
			}
			if err := gamelist.Rename(lib.Root, sys, base, filepath.Base(out)); err != nil {
				log.Errf("gamelist update failed: %s/%s: %v", sys, base, err)
			}
			converted.Add(1)
		})
	})

	if runner.Format.Structured() {
		runner.SetCount("converted", converted.Load())
		runner.SetCount("renamed", renamed.Load())
		runner.SetCount("failed", failed.Load())
		return failed.Load() == 0, nil
	}
	color, reset := cliutil.StatusColor(runner.Format, failed.Load() > 0)
	line := fmt.Sprintf("%sxiso: %d converted, %d renamed, %d failed in %s%s\n", color, converted.Load(), renamed.Load(), failed.Load(), time.Since(start).Round(time.Second), reset)
	if runner.Quiet {
		runner.Summarize(line)
	} else {
		fmt.Print(line)
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
