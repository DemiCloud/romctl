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
	"github.com/demicloud/romctl/internal/gamelist"
	"github.com/demicloud/romctl/internal/library"
	"github.com/demicloud/romctl/internal/systemdb"
	"github.com/demicloud/romctl/internal/tools"
)

// RVZ converts GameCube/Wii images to Dolphin's native RVZ format.
func RVZ(lib *library.Library, runner *actions.Runner, systems []string) (bool, error) {
	log := cliutil.NewLogger("rvz")
	var converted, failed atomic.Int64

	cliutil.Parallel(systems, func(sys string) {
		if systemdb.ClassOf(sys) != systemdb.ClassDolphin {
			return
		}
		sysDir := filepath.Join(lib.Roms, sys)
		entries, err := os.ReadDir(sysDir)
		if err != nil {
			return
		}
		names := entryNames(entries)
		var candidates []string
		for _, n := range names {
			switch strings.ToLower(filepath.Ext(n)) {
			case ".iso", ".gcm", ".wbfs", ".ciso", ".gcz":
				candidates = append(candidates, n)
			}
		}

		for i, base := range candidates {
			path := filepath.Join(sysDir, base)
			out := strings.TrimSuffix(path, filepath.Ext(path)) + ".rvz"
			if exists(out) {
				continue
			}
			if !runner.Act("[%d/%d] convert %s/%s -> %s", i+1, len(candidates), sys, base, filepath.Base(out)) {
				continue
			}
			if err := convertToRVZ(path, out); err != nil {
				log.Errf("failed, source kept: %s: %v", path, err)
				failed.Add(1)
				continue
			}
			_ = actions.Retire(path)
			if err := gamelist.Rename(lib.Root, sys, base, filepath.Base(out)); err != nil {
				log.Errf("gamelist update failed: %s/%s: %v", sys, base, err)
			}
			converted.Add(1)
		}
	})

	fmt.Printf("rvz: %d converted, %d failed\n", converted.Load(), failed.Load())
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
