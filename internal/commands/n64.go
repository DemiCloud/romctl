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
	"github.com/demicloud/romctl/internal/formats/n64"
	"github.com/demicloud/romctl/internal/gamelist"
	"github.com/demicloud/romctl/internal/library"
	"github.com/demicloud/romctl/internal/systemdb"
)

// n64Job is one ROM to examine/normalize, flattened out of its system's
// directory listing up front - see CHD's doc comment for why every
// system's files run through one shared worker pool instead of one per
// system.
type n64Job struct {
	sys, base string
}

// N64 normalizes N64 ROMs to big-endian .z64.
func N64(lib *library.Library, runner *actions.Runner, systems []string) error {
	start := time.Now()
	log := cliutil.NewLogger("n64", runner.Format)
	var normalized, failed atomic.Int64

	var jobs []n64Job
	for _, sys := range systems {
		if systemdb.ClassOf(sys) != systemdb.ClassN64 {
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
			if ext != ".z64" && ext != ".v64" && ext != ".n64" {
				continue
			}
			jobs = append(jobs, n64Job{sys, e.Name()})
		}
	}

	cliutil.Progress(runner.Format, runner.Verbose, runner.DryRun, "n64", len(jobs), func(tick func()) {
		cliutil.Parallel(jobs, func(j n64Job) {
			defer tick()
			sys, base := j.sys, j.base
			src := filepath.Join(lib.Roms, sys, base)
			order, err := n64.Detect(src)
			if err != nil || order == n64.OrderUnknown {
				log.Errf("unknown N64 header: %s/%s", sys, base)
				return
			}
			out := strings.TrimSuffix(src, filepath.Ext(src)) + ".z64"
			if order == n64.OrderZ64 && src == out {
				return
			}
			if exists(out) && src != out {
				log.Errf("exists, skipped: %s", out)
				return
			}
			outBase := filepath.Base(out)
			if !runner.Act("normalize %s/%s -> %s", sys, base, outBase) {
				return
			}
			if err := normalizeN64(src, out, order); err != nil {
				log.Errf("conversion failed: %s/%s: %v", sys, base, err)
				failed.Add(1)
				return
			}
			if err := gamelist.Rename(lib.Root, sys, base, outBase); err != nil {
				log.Errf("gamelist update failed: %s/%s: %v", sys, base, err)
			}
			normalized.Add(1)
		})
	})
	if runner.Format.Structured() {
		runner.SetCount("normalized", normalized.Load())
		runner.SetCount("failed", failed.Load())
		return nil
	}
	color, reset := cliutil.StatusColor(runner.Format, failed.Load() > 0)
	fmt.Printf("%sn64: %d normalized, %d failed in %s%s\n", color, normalized.Load(), failed.Load(), time.Since(start).Round(time.Second), reset)
	return nil
}

func normalizeN64(src, out string, order n64.Order) error {
	if order == n64.OrderZ64 {
		return os.Rename(src, out)
	}
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	converted, err := n64.ToBigEndian(data, order)
	if err != nil {
		return err
	}
	tmp := out + ".tmp"
	if err := os.WriteFile(tmp, converted, 0o644); err != nil {
		os.Remove(tmp)
		return err
	}
	if src == out {
		if err := actions.Retire(src); err != nil {
			os.Remove(tmp)
			return err
		}
		return os.Rename(tmp, out)
	}
	if err := os.Rename(tmp, out); err != nil {
		os.Remove(tmp)
		return err
	}
	return actions.Retire(src)
}
