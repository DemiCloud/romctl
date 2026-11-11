package commands

import (
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"

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
	var jobs []discsJob
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
		jobs = append(jobs, discsJobsFor(sys, display, class, sysDir, entries)...)
	}

	// Confirming a candidate is a real chdman/dolphin-tool extraction for
	// a .chd/.rvz, not a filename check - running every candidate across
	// every system through one shared worker pool, rather than one
	// system's candidates before moving to the next, is what keeps this
	// command fast even when a system turns up more than one (see
	// check's own structure phase, which collects+resolves jobs the same
	// way for the exact same reason).
	discTools := &discToolResolver{}
	var mu sync.Mutex
	var rows []discSetRow
	cliutil.Parallel(jobs, func(j discsJob) {
		status := checkIncompleteDiscSet(j.sys, j.class, j.sysDir, j.num, j.group, discTools)
		row, ok := discSetRowFrom(j.display, j.sys, status, j.file)
		if !ok {
			return
		}
		mu.Lock()
		rows = append(rows, row)
		mu.Unlock()
	})

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

// discsJob is one ambiguous disc discsJobsFor found, deferred for
// confirmation the same way check.go's discCompletenessJob is (see its
// own doc comment) - a separate, Discs-local type rather than reusing
// that one directly, since Discs needs the fuller "Title.m3u/Disc.ext"
// File column value for an already-grouped disc, where check's own
// reportPath deliberately stays just the directory name (its finding's
// path convention, shared with checkM3uDir's other findings).
type discsJob struct {
	sys, display string
	class        systemdb.Class
	sysDir       string
	file         string // discSetRow's own File column value
	num          int
	group        []discFile
}

// discsJobsFor scans one system's already-read directory listing for
// ambiguous discs worth confirming - the exact same buildDiscGroups/
// m3uLoneDiscGroup logic checkStructureFile/checkM3uDiscSet use, just
// collecting jobs instead of enqueueing them on a shared
// discJobCollector. Covers both a loose disc and one already grouped
// inside a Title.m3u/ directory, same scope check's own structure
// category has.
func discsJobsFor(sys, display string, class systemdb.Class, sysDir string, entries []os.DirEntry) []discsJob {
	var jobs []discsJob
	for _, group := range buildDiscGroups(entries) {
		nums := map[int]bool{}
		for _, f := range group {
			nums[f.num] = true
		}
		if len(nums) != 1 {
			continue
		}
		jobs = append(jobs, discsJob{
			sys: sys, display: display, class: class, sysDir: sysDir,
			file: group[0].base, num: group[0].num, group: group,
		})
	}
	for _, e := range entries {
		if !e.IsDir() || !strings.HasSuffix(e.Name(), ".m3u") {
			continue
		}
		dirPath := filepath.Join(sysDir, e.Name())
		m3uEntries, err := os.ReadDir(dirPath)
		if err != nil {
			continue
		}
		group, ok := m3uLoneDiscGroup(e.Name(), m3uEntries)
		if !ok {
			continue
		}
		jobs = append(jobs, discsJob{
			sys: sys, display: display, class: class, sysDir: dirPath,
			file: filepath.Join(e.Name(), group[0].base), num: group[0].num, group: group,
		})
	}
	return jobs
}

// discSetRowFrom builds a discSetRow from a confirmed-incomplete status;
// ok is false (nothing to add) for anything else - unconfirmed, or
// confirmed but actually complete.
func discSetRowFrom(display, sys string, status discSetStatus, file string) (discSetRow, bool) {
	if !status.incomplete() {
		return discSetRow{}, false
	}
	return discSetRow{
		System:    display,
		Directory: sys,
		Title:     status.title,
		Have:      strconv.Itoa(status.present) + " of " + strconv.Itoa(status.totalDiscs),
		Missing:   joinInts(status.missing),
		File:      file,
	}, true
}
