package commands

import (
	"fmt"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/demicloud/romctl/internal/actions"
	"github.com/demicloud/romctl/internal/cliutil"
	"github.com/demicloud/romctl/internal/discimage"
	"github.com/demicloud/romctl/internal/library"
	"github.com/demicloud/romctl/internal/systemdb"
)

// PS3 unpacks archived PS3 dumps into disc folders, then builds an ISO
// from each one. PS3 is the only class that goes through plain
// ISO9660 (internal/discimage) instead of CHD, because RPCS3 doesn't read
// CHD at all; a future system needing the same treatment is a new entry
// in internal/discimage's Spec table, not a new command. scheme
// (`[naming] scheme`) controls how region/dump tags end up in the
// resulting ISO's name; see internal/actions.ApplyScheme.
func PS3(lib *library.Library, runner *actions.Runner, systems []string, scheme actions.Scheme) (bool, error) {
	start := time.Now()
	log := cliutil.NewLogger("ps3", runner.Format)
	var unpacked, built, failed atomic.Int64
	cliutil.Parallel(systems, func(sys string) {
		spec, ok := discimage.SpecFor(systemdb.ClassOf(sys))
		if !ok {
			return
		}
		sysDir := filepath.Join(lib.Roms, sys)
		n, err := discimage.UnpackArchives(sysDir, sys, spec, runner, log, scheme)
		unpacked.Add(int64(n))
		if err != nil {
			failed.Add(1)
		}
		n, err = discimage.BuildISOs(sysDir, sys, spec, runner, log)
		built.Add(int64(n))
		if err != nil {
			failed.Add(1)
		}
	})
	if runner.Format.Structured() {
		runner.SetCount("unpacked", unpacked.Load())
		runner.SetCount("built", built.Load())
		runner.SetCount("failed", failed.Load())
		return failed.Load() == 0, nil
	}
	color, reset := cliutil.StatusColor(runner.Format, failed.Load() > 0)
	line := fmt.Sprintf("%sps3: %d unpacked, %d built, %d failed in %s%s\n", color, unpacked.Load(), built.Load(), failed.Load(), time.Since(start).Round(time.Second), reset)
	if runner.Quiet {
		runner.Summarize(line)
	} else {
		fmt.Print(line)
	}
	return failed.Load() == 0, nil
}
