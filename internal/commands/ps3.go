package commands

import (
	"path/filepath"

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
	log := cliutil.NewLogger("ps3")
	failed := false
	for _, sys := range systems {
		spec, ok := discimage.SpecFor(systemdb.ClassOf(sys))
		if !ok {
			continue
		}
		sysDir := filepath.Join(lib.Roms, sys)
		if err := discimage.UnpackArchives(sysDir, sys, spec, runner, log, scheme); err != nil {
			failed = true
		}
		if err := discimage.BuildISOs(sysDir, sys, spec, runner, log); err != nil {
			failed = true
		}
	}
	return !failed, nil
}
