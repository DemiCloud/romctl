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
	"github.com/demicloud/romctl/internal/gamelist"
	"github.com/demicloud/romctl/internal/library"
	"github.com/demicloud/romctl/internal/systemdb"
	"github.com/demicloud/romctl/internal/tools"
)

// rvzJob is one disc image to convert, flattened out of its system's
// directory listing up front - see CHD's doc comment for why every
// system's files run through one shared worker pool instead of one per
// system.
type rvzJob struct {
	sys, base  string
	idx, total int
}

// RVZ converts GameCube/Wii images to Dolphin's native RVZ format.
func RVZ(lib *library.Library, runner *actions.Runner, systems []string) (bool, error) {
	start := time.Now()
	log := cliutil.NewLogger("rvz", runner.Format)
	var converted, failed atomic.Int64

	// Resolve dolphin-tool once, up front, before any mutating action -
	// same invariant CHD now follows (see resolveCHDManIfNeeded's own
	// comment): one clear error before the run starts, not a "dolphin-tool
	// not found" failure repeated once per file as convertToRVZ's own
	// inner resolve call hits it over and over. Skipped for --dry-run:
	// convertToRVZ is only ever reached after runner.Act (which returns
	// false there, before any real work), so a dry-run preview still
	// works without dolphin-tool installed at all.
	if !runner.DryRun {
		needed := false
		for _, sys := range systems {
			if systemdb.ClassOf(sys) == systemdb.ClassDolphin {
				needed = true
				break
			}
		}
		if needed {
			if _, err := tools.Resolve("dolphin-tool"); err != nil {
				return false, err
			}
		}
	}

	var jobs []rvzJob
	for _, sys := range systems {
		if systemdb.ClassOf(sys) != systemdb.ClassDolphin {
			continue
		}
		sysDir := filepath.Join(lib.Roms, sys)
		entries, err := os.ReadDir(sysDir)
		if err != nil {
			continue
		}
		var candidates []string
		for _, n := range entryNames(entries) {
			switch strings.ToLower(filepath.Ext(n)) {
			case ".iso", ".gcm", ".wbfs", ".ciso", ".gcz":
				candidates = append(candidates, n)
			}
		}
		for i, base := range candidates {
			jobs = append(jobs, rvzJob{sys, base, i, len(candidates)})
		}
	}

	cliutil.Progress(runner.Format, runner.Verbose || runner.Quiet, runner.DryRun, "rvz", len(jobs), func(tick func()) {
		cliutil.Parallel(jobs, func(j rvzJob) {
			defer tick()
			path := filepath.Join(lib.Roms, j.sys, j.base)
			out := rvzOutputPath(path)
			if exists(out) {
				return
			}
			if msg := signatureMismatch(j.sys, path); msg != "" {
				log.Errf("%s", msg)
				failed.Add(1)
				return
			}
			if !runner.Act("[%d/%d] convert %s/%s -> %s", j.idx+1, j.total, j.sys, j.base, filepath.Base(out)) {
				return
			}
			if err := convertToRVZ(path, out); err != nil {
				log.Errf("failed, source kept: %s: %v", path, err)
				failed.Add(1)
				return
			}
			_ = actions.Retire(path)
			if err := gamelist.Rename(lib.Root, j.sys, j.base, filepath.Base(out)); err != nil {
				log.Errf("gamelist update failed: %s/%s: %v", j.sys, j.base, err)
			}
			converted.Add(1)
		})
	})

	if runner.Format.Structured() {
		runner.SetCount("converted", converted.Load())
		runner.SetCount("failed", failed.Load())
		return failed.Load() == 0, nil
	}
	color, reset := cliutil.StatusColor(runner.Format, failed.Load() > 0)
	line := fmt.Sprintf("%srvz: %d converted, %d failed in %s%s\n", color, converted.Load(), failed.Load(), time.Since(start).Round(time.Second), reset)
	if runner.Quiet {
		runner.Summarize(line)
	} else {
		fmt.Print(line)
	}
	return failed.Load() == 0, nil
}

// rvzOutputPath strips path's image extension and, if present, a trailing
// ".nkit" infix (an NKit-shrunk source, e.g. "Title.nkit.iso") before
// appending ".rvz" - otherwise the naive TrimSuffix(path, Ext(path)) only
// strips the real extension, leaving "Title.nkit.rvz": a name m3u's own
// discFileRE (and any other "(Disc N).rvz" matcher) doesn't recognize.
// dolphin-tool's own RVZ container still carries the source's NKit-shrunk
// data either way (confirmed via its own "output will still be NKit"
// conversion warning) - this only fixes the filename, not that property.
func rvzOutputPath(path string) string {
	base := strings.TrimSuffix(path, filepath.Ext(path))
	if ext := filepath.Ext(base); strings.EqualFold(ext, ".nkit") {
		base = strings.TrimSuffix(base, ext)
	}
	return base + ".rvz"
}

func convertToRVZ(in, out string) error {
	path, err := tools.Resolve("dolphin-tool")
	if err != nil {
		return err
	}
	tmp := out + ".tmp"
	cmdOut, err := executil.Run(path, "convert", "-i", in, "-o", tmp, "-f", "rvz", "-c", "zstd", "-l", "5", "-b", "131072")
	if err != nil {
		os.Remove(tmp)
		return fmt.Errorf("dolphin-tool convert: %w: %s", err, cmdOut)
	}
	return os.Rename(tmp, out)
}
