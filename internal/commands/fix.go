package commands

import (
	"fmt"
	"time"

	"github.com/demicloud/romctl/internal/actions"
	"github.com/demicloud/romctl/internal/cliutil"
	"github.com/demicloud/romctl/internal/library"
	"github.com/demicloud/romctl/internal/systemdb"
)

// fixStep is one command Fix chains, by name (for its own error/log
// prefix, and to look itself up in stepClasses) and a closure over the
// shared lib/runner/systems/scheme/savesDest arguments. The wrapping
// normalizes every step to (bool, error): n64/snes/m3u/rename return only
// error today (see dispatch in cmd/romctl/main.go), so those are wrapped
// as always-ok unless they return an error, matching main.go's own
// existing treatment of them.
type fixStep struct {
	name string
	run  func() (bool, error)
}

// stepClasses limits a step to the systemdb classes it can actually do
// anything for - the same classes that step's own code already gates on
// internally (see e.g. unpack.go's class switch), just surfaced here so
// Fix can skip the step's phase entirely - no 0-count output, no "done"
// line, nothing - when none of the given systems belong to any of them.
// Without this, "fix snes" ran (and printed a result for) all eleven
// steps even though only unpack/snes/relocate/rename/saves could ever do
// anything for a SNES-only selection; chd/rvz/xiso/n64/ps3 would report
// nothing found every single time, which looked like - and was
// frequently mistaken for - "fix" touching unrelated systems.
//
// A step with no entry here (relocate, m3u, rename, saves) is generic:
// gated on file content or a filename pattern rather than system class,
// so it's always included regardless of which systems were given.
var stepClasses = map[string][]systemdb.Class{
	"unpack": {systemdb.ClassCart, systemdb.ClassN64, systemdb.ClassSNES, systemdb.ClassDolphin, systemdb.ClassXbox},
	"chd":    {systemdb.ClassCD, systemdb.ClassDVD, systemdb.ClassUMD},
	"rvz":    {systemdb.ClassDolphin},
	"xiso":   {systemdb.ClassXbox},
	"n64":    {systemdb.ClassN64},
	"snes":   {systemdb.ClassSNES},
	"ps3":    {systemdb.ClassPS3},
}

// relevantSteps filters steps down to the ones that could possibly do
// something for systems, per stepClasses.
func relevantSteps(steps []fixStep, systems []string) []fixStep {
	present := map[systemdb.Class]bool{}
	for _, sys := range systems {
		present[systemdb.ClassOf(sys)] = true
	}
	out := make([]fixStep, 0, len(steps))
	for _, step := range steps {
		want, limited := stepClasses[step.name]
		if !limited {
			out = append(out, step)
			continue
		}
		for _, c := range want {
			if present[c] {
				out = append(out, step)
				break
			}
		}
	}
	return out
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
		{"rename", func() (bool, error) { return true, Rename(lib, runner, systems, scheme, false, "", false) }},
		{"saves", func() (bool, error) { return Saves(lib, runner, systems, savesDest) }},
	}
}

// Fix runs every local normalization step relevant to the given systems
// (see fixSteps, relevantSteps) in order, so a fresh or stale library can
// be brought fully into convention with one command instead of several. A
// step's own failures don't stop the run - same "report everything, don't
// abort midway" behavior every step already has internally - they just
// mark the overall result failed and move on to the next step.
//
// Every step that does run covers every given system, not a different
// directory per step - but a step whose class(es) (see stepClasses) none
// of the given systems belong to is skipped outright rather than run and
// left to report zero: "fix snes" runs relocate/unpack/snes/rename/saves,
// not all eleven steps padded out with chd/rvz/xiso/n64/ps3/m3u no-ops.
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

	steps := relevantSteps(fixSteps(lib, runner, systems, scheme, savesDest), systems)
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
