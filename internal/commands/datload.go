package commands

import (
	"errors"
	"fmt"
	"io/fs"
	"os"

	"github.com/demicloud/romctl/internal/cliutil"
	"github.com/demicloud/romctl/internal/datdb"
)

// datLoadRow is one system's DAT-load problem: systemdb.Dat maps it to a
// provider/pack, but that pack isn't usable right now - either nothing's
// been staged yet (the common case until `make dats` has run - see
// internal/datdb's doc comment) or the staged file failed to parse.
// Collected into one small table (printDatLoadIssues) instead of a
// stderr line per system - collection/identify both used to print these
// as they were found, which buried the run's actual output below a wall
// of near-identical "no DAT for X" lines on a build with nothing staged.
type datLoadRow struct {
	System string `json:"system" yaml:"system" toml:"system" col:"System"`
	Issue  string `json:"issue" yaml:"issue" toml:"issue" col:"Issue"`
}

// loadDat wraps datdb.Load for collection/identify: ok is false only when
// sys has no DatSource mapping at all (nothing to report, same as
// datdb.Load's own ok). A true ok with a nil df means row is non-nil and
// already describes why - the caller should record it and move on to the
// next system, same as it would have done with datdb.Load's own error.
func loadDat(sys, datDir string) (df *datdb.Datafile, ok bool, row *datLoadRow) {
	df, ok, err := datdb.Load(sys, datDir)
	if !ok || err == nil {
		return df, ok, nil
	}
	issue := "no DAT staged"
	if !errors.Is(err, fs.ErrNotExist) {
		issue = err.Error()
	}
	return nil, true, &datLoadRow{System: sys, Issue: issue}
}

// printDatLoadIssues renders issues (if any) as a small table to stderr,
// always in Plain - a side-channel diagnostic, not part of the command's
// own --format=json/yaml/csv output contract, so it stays readable
// (and parseable the same way) no matter what format the actual result
// is requested in.
func printDatLoadIssues(cmd string, issues []datLoadRow) {
	if len(issues) == 0 {
		return
	}
	fmt.Fprintf(os.Stderr, "%s: DAT not available for %d system(s):\n", cmd, len(issues))
	cliutil.RenderList(os.Stderr, cliutil.FormatPlain, issues)
}

// printNoDatSystems renders systems (if any) with no DatSource mapping
// at all - distinct from printDatLoadIssues' "mapped but not staged"
// case, and never a run failure: collection still lists whatever files
// exist for one of these (see listOnlyRows), just with no have/missing
// verdict to give them.
func printNoDatSystems(cmd string, systems []datLoadRow) {
	if len(systems) == 0 {
		return
	}
	fmt.Fprintf(os.Stderr, "%s: no DAT coverage for %d system(s), listing files only:\n", cmd, len(systems))
	cliutil.RenderList(os.Stderr, cliutil.FormatPlain, systems)
}
