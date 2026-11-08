package commands

import (
	"fmt"
	"time"

	"github.com/demicloud/romctl/internal/actions"
	"github.com/demicloud/romctl/internal/cliutil"
	"github.com/demicloud/romctl/internal/library"
)

// fixStep is one command Fix chains, by name (for its own error/log
// prefix) and a closure over the shared lib/runner/systems/scheme/
// savesDest arguments. The wrapping normalizes every step to (bool,
// error): n64/snes/m3u/rename return only error today (see dispatch in
// cmd/romctl/main.go), so those are wrapped as always-ok unless they
// return an error, matching main.go's own existing treatment of them.
type fixStep struct {
	name string
	run  func() (bool, error)
}

// fixSteps lists, in order, every local normalization command Fix chains:
// unpack first so n64/snes/chd see already-extracted files, convert disc
// images to their emulator-native containers, group any loose multi-disc
// CHDs that convert didn't already group itself, apply the naming scheme
// last (once every file has settled into its final extension), then
// relocate stray saves. scrape and sync are deliberately excluded - both
// need credentials/network/an explicit device and are their own command,
// not an implicit side effect of "fix everything local".
func fixSteps(lib *library.Library, runner *actions.Runner, systems []string, scheme actions.Scheme, savesDest string) []fixStep {
	return []fixStep{
		{"unpack", func() (bool, error) { return Unpack(lib, runner, systems) }},
		{"chd", func() (bool, error) { return CHD(lib, runner, systems, scheme) }},
		{"dvd", func() (bool, error) { return DVD(lib, runner, systems) }},
		{"rvz", func() (bool, error) { return RVZ(lib, runner, systems) }},
		{"xiso", func() (bool, error) { return Xiso(lib, runner, systems) }},
		{"n64", func() (bool, error) { return true, N64(lib, runner, systems) }},
		{"snes", func() (bool, error) { return true, SNES(lib, runner, systems) }},
		{"ps3", func() (bool, error) { return PS3(lib, runner, systems, scheme) }},
		{"m3u", func() (bool, error) { return true, M3U(lib, runner, systems) }},
		{"rename", func() (bool, error) { return true, Rename(lib, runner, systems, scheme) }},
		{"saves", func() (bool, error) { return Saves(lib, runner, systems, savesDest) }},
	}
}

// Fix runs every local normalization step (see fixSteps) against the
// given systems in order, so a fresh or stale library can be brought
// fully into convention with one command instead of several. A step's
// own failures don't stop the run - same "report everything, don't abort
// midway" behavior every step already has internally - they just mark
// the overall result failed and move on to the next step.
func Fix(lib *library.Library, runner *actions.Runner, systems []string, scheme actions.Scheme, savesDest string) (bool, error) {
	start := time.Now()
	log := cliutil.NewLogger("fix")
	ok := true
	for _, step := range fixSteps(lib, runner, systems, scheme, savesDest) {
		stepOK, err := step.run()
		if err != nil {
			log.Errf("%s: %v", step.name, err)
			ok = false
			continue
		}
		if !stepOK {
			ok = false
		}
	}
	fmt.Printf("fix: done in %s\n", time.Since(start).Round(time.Second))
	return ok, nil
}
