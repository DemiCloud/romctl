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
	log := cliutil.NewLogger("rvz")
	var converted, failed atomic.Int64

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

	cliutil.Parallel(jobs, func(j rvzJob) {
		path := filepath.Join(lib.Roms, j.sys, j.base)
		out := strings.TrimSuffix(path, filepath.Ext(path)) + ".rvz"
		if exists(out) {
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

	fmt.Printf("rvz: %d converted, %d failed in %s\n", converted.Load(), failed.Load(), time.Since(start).Round(time.Second))
	return failed.Load() == 0, nil
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
