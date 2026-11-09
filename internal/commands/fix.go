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
// relocate misfiled cartridge ROMs first (so a Genesis ROM sitting in
// roms/segacd/ gets out of the way before chd ever has to fail on it),
// then unpack so n64/snes/chd see already-extracted files, convert disc
// images to their emulator-native containers (chd also re-encodes any
// existing CD-type CHD a DVD-class system needs as DVD-type - see CHD's
// own doc comment, not a separate step), group any loose multi-disc CHDs
// that convert didn't already group itself, apply the naming scheme last
// (once every file has settled into its final extension), then relocate
// stray saves. scrape and sync are deliberately excluded - both need
// credentials/network/an explicit device and are their own command, not
// an implicit side effect of "fix everything local".
func fixSteps(lib *library.Library, runner *actions.Runner, systems []string, scheme actions.Scheme, savesDest string) []fixStep {
	return []fixStep{
		{"relocate", func() (bool, error) { return Relocate(lib, runner, systems) }},
		{"unpack", func() (bool, error) { return Unpack(lib, runner, systems) }},
		{"chd", func() (bool, error) { return CHD(lib, runner, systems, scheme) }},
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
//
// Every step runs across the same systems, not a different directory per
// step - a step simply has nothing to do, and reports zero, wherever none
// of the given systems belong to the class it handles (e.g. "fix snes"
// runs all eleven steps, but chd/rvz/xiso/ps3 are no-ops for a system
// whose class isn't theirs).
//
// On a real (non-dry-run, non-verbose) run to a Fancy terminal, this
// drives the same live per-phase display check/bios/dupes/verify already
// use (cliutil.RunPhases) instead of each step printing its own counts
// the moment it finishes: runner.Quiet tells every step to hand its
// one-line summary to runner.Summarize instead, and every summary prints
// together, in step order, once every phase is done - one coherent
// report instead of eleven lines each sandwiched between that step's own
// "N/N done" progress line and the next step's label.
func Fix(lib *library.Library, runner *actions.Runner, systems []string, scheme actions.Scheme, savesDest string) (bool, error) {
	start := time.Now()
	log := cliutil.NewLogger("fix", runner.Format)
	runner.Quiet = cliutil.CanAnimate(runner.Format) && !runner.Verbose && !runner.DryRun

	steps := fixSteps(lib, runner, systems, scheme, savesDest)
	ok := true
	phases := make([]cliutil.Phase, len(steps))
	for i, step := range steps {
		step := step
		phases[i] = cliutil.Phase{Label: step.name, Fn: func() bool {
			stepOK, err := step.run()
			if err != nil {
				log.Errf("%s: %v", step.name, err)
				ok = false
				return false
			}
			if !stepOK {
				ok = false
			}
			return stepOK
		}}
	}
	cliutil.RunPhases(runner.Format, runner.Verbose, runner.DryRun, phases)

	if runner.Format.Structured() {
		// Every step above already reported its own counts to this same
		// shared runner (see actions.Result's doc comment on same-named
		// keys merging across steps); nothing left to add here.
		return ok, nil
	}
	for _, line := range runner.Summaries() {
		fmt.Print(line)
	}
	color, reset := cliutil.StatusColor(runner.Format, !ok)
	fmt.Printf("%sfix: done in %s%s\n", color, time.Since(start).Round(time.Second), reset)
	return ok, nil
}
