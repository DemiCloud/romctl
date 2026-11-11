package commands

import (
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/demicloud/romctl/internal/cliutil"
	"github.com/demicloud/romctl/internal/library"
	"github.com/demicloud/romctl/internal/systemdb"
)

// discSetRow is one confirmed-incomplete disc set's shape for every
// output form, same cliutil.RenderList convention as collectionRow:
// System is the human-readable display name, Directory the actual
// roms/<Directory>/ name.
type discSetRow struct {
	System    string `json:"system" yaml:"system" toml:"system" col:"System"`
	Directory string `json:"directory" yaml:"directory" toml:"directory" col:"Directory"`
	Title     string `json:"title" yaml:"title" toml:"title" col:"Title"`
	Have      string `json:"have" yaml:"have" toml:"have" col:"Have"`
	Missing   string `json:"missing" yaml:"missing" toml:"missing" col:"Missing"`
	File      string `json:"file" yaml:"file" toml:"file" col:"File"`
}

// Discs reports every confirmed-incomplete multi-disc set across the
// given systems - the detail view check's own Fixes table points an
// "Incomplete disc set" bucket at (see audit.labelDetailCommands)
// instead of the generic, unscoped "check -v", which would re-run and
// print every category for every system just to find these few lines.
// This runs only the same cheap, gated check/checkIncompleteDiscSet
// already does as part of check's own structure category - no naming/
// containers/bios pass, no re-hashing anything check -v wouldn't also
// have hashed - so it's exactly as fast as that one slice of check
// already is, just without everything else's noise. Read-only, same as
// every other report-named command (identify/collection/check/...): it
// never renames or regroups anything itself, see `m3u`/`rename` for
// that.
func Discs(lib *library.Library, systems []string, format cliutil.Format) (bool, error) {
	esdeNames, _ := lib.ESDESystems()
	var rows []discSetRow
	for _, sys := range systems {
		class := systemdb.ClassOf(sys)
		if class == systemdb.ClassArcade {
			continue
		}
		sysDir := filepath.Join(lib.Roms, sys)
		entries, err := os.ReadDir(sysDir)
		if err != nil {
			continue
		}
		display := systemdb.DisplayNameWith(sys, esdeNames)
		for _, row := range discSetRowsFor(sys, display, sysDir, entries) {
			rows = append(rows, row)
		}
	}

	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].Directory != rows[j].Directory {
			return rows[i].Directory < rows[j].Directory
		}
		return rows[i].Title < rows[j].Title
	})

	if format.Structured() || len(rows) > 0 {
		if err := cliutil.RenderList(os.Stdout, format, rows); err != nil {
			return false, err
		}
	}
	return true, nil
}

// discSetRowsFor scans one system's already-read directory listing for
// confirmed-incomplete disc sets - the exact same buildDiscGroups +
// checkIncompleteDiscSet logic checkStructureFile uses, just collecting
// rows instead of audit.Report findings.
func discSetRowsFor(sys, display, sysDir string, entries []os.DirEntry) []discSetRow {
	groups := buildDiscGroups(entries)
	var rows []discSetRow
	for _, group := range groups {
		nums := map[int]bool{}
		for _, f := range group {
			nums[f.num] = true
		}
		if len(nums) != 1 {
			continue
		}
		status := checkIncompleteDiscSet(sys, sysDir, group[0].num, group)
		if !status.incomplete() {
			continue
		}
		parts := make([]string, len(status.missing))
		for i, n := range status.missing {
			parts[i] = strconv.Itoa(n)
		}
		rows = append(rows, discSetRow{
			System:    display,
			Directory: sys,
			Title:     status.title,
			Have:      strconv.Itoa(status.present) + " of " + strconv.Itoa(status.totalDiscs),
			Missing:   strings.Join(parts, ", "),
			File:      group[0].base,
		})
	}
	return rows
}
