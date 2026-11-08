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
	"github.com/demicloud/romctl/internal/formats/snes"
	"github.com/demicloud/romctl/internal/gamelist"
	"github.com/demicloud/romctl/internal/library"
	"github.com/demicloud/romctl/internal/systemdb"
)

// snesJob is one ROM to examine/normalize, flattened out of its system's
// directory listing up front - see CHD's doc comment for why every
// system's files run through one shared worker pool instead of one per
// system.
type snesJob struct {
	sys, base string
}

// SNES strips 512-byte copier headers and renames .smc to .sfc.
func SNES(lib *library.Library, runner *actions.Runner, systems []string) error {
	start := time.Now()
	log := cliutil.NewLogger("snes", runner.Format)
	var processed, failed atomic.Int64

	var jobs []snesJob
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
			ext := strings.ToLower(filepath.Ext(e.Name()))
			if ext != ".smc" && ext != ".sfc" {
				continue
			}
			jobs = append(jobs, snesJob{sys, e.Name()})
		}
	}

	cliutil.Progress(runner.Format, runner.Verbose, runner.DryRun, "snes", len(jobs), func(tick func()) {
		cliutil.Parallel(jobs, func(j snesJob) {
			defer tick()
			sys, base := j.sys, j.base
			src := filepath.Join(lib.Roms, sys, base)
			out := strings.TrimSuffix(src, filepath.Ext(src)) + ".sfc"
			outBase := filepath.Base(out)

			info, err := os.Stat(src)
			if err != nil {
				return
			}
			if msg := signatureMismatch(sys, src); msg != "" {
				log.Errf("%s", msg)
				failed.Add(1)
				return
			}
			switch {
			case snes.HasHeader(info.Size()):
				if !runner.Act("strip header %s/%s -> %s", sys, base, outBase) {
					return
				}
				if err := stripSNESHeader(src, out); err != nil {
					log.Errf("strip header failed: %s/%s: %v", sys, base, err)
					failed.Add(1)
					return
				}
				if err := gamelist.Rename(lib.Root, sys, base, outBase); err != nil {
					log.Errf("gamelist update failed: %s/%s: %v", sys, base, err)
				}
				processed.Add(1)
			case src != out:
				if exists(out) {
					log.Errf("exists, skipped: %s", out)
					return
				}
				if runner.Act("rename %s/%s -> %s", sys, base, outBase) {
					if err := os.Rename(src, out); err != nil {
						log.Errf("rename failed: %s/%s: %v", sys, base, err)
						failed.Add(1)
						return
					}
					if err := gamelist.Rename(lib.Root, sys, base, outBase); err != nil {
						log.Errf("gamelist update failed: %s/%s: %v", sys, base, err)
					}
					processed.Add(1)
				}
			}
		})
	})
	if runner.Format.Structured() {
		runner.SetCount("processed", processed.Load())
		runner.SetCount("failed", failed.Load())
		return nil
	}
	color, reset := cliutil.StatusColor(runner.Format, failed.Load() > 0)
	fmt.Printf("%ssnes: %d processed, %d failed in %s%s\n", color, processed.Load(), failed.Load(), time.Since(start).Round(time.Second), reset)
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
