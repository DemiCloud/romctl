package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/demicloud/romctl/internal/audit"
	"github.com/demicloud/romctl/internal/cliutil"
	"github.com/demicloud/romctl/internal/library"
	"github.com/demicloud/romctl/internal/systemdb"
)

// biosEntry is one row of the full BIOS inventory: either a
// systemdb.BiosRequired group (several alternative filenames, any one
// satisfies it) or a single systemdb.BiosOptional file, always tied to
// the system that uses it.
type biosEntry struct {
	system       string
	alternatives []string
	required     bool
}

// biosCatalog returns every known BIOS requirement for the given systems
// (required groups first, then optional files, both in a stable order) -
// the full inventory `bios -v` walks and checkBios audits against. Scoped
// to systems, same as every other check/bios logic, so a library that's
// never had an roms/nds/ directory never sees NDS's optional BIOS entries
// at all - not even as INFO.
func biosCatalog(systems []string) []biosEntry {
	sorted := append([]string(nil), systems...)
	sort.Strings(sorted)

	var out []biosEntry
	for _, sys := range sorted {
		for _, group := range systemdb.BiosRequired[sys] {
			out = append(out, biosEntry{sys, group, true})
		}
		for _, group := range systemdb.BiosOptional[sys] {
			out = append(out, biosEntry{sys, group, false})
		}
	}
	return out
}

// biosLabel returns a short human phrase for entry, from systemdb.BiosLabel
// when listed there, else a generic fallback so a future addition to
// BiosRequired/BiosOptional that hasn't been labeled yet still prints
// something sensible instead of nothing. The per-alternative key
// ("<system>/<file>") is tried first regardless of required/optional -
// every optional entry is already keyed that way, and so is xbox, the one
// required system with more than one independent required file (three
// single-alternative groups, not one group of three alternatives like
// everything else); only a single-group required system (the common
// case) actually needs the bare "<system>" fallback key.
func biosLabel(e biosEntry) string {
	if label, ok := systemdb.BiosLabel[e.system+"/"+e.alternatives[0]]; ok {
		return label
	}
	if label, ok := systemdb.BiosLabel[e.system]; ok {
		return label
	}
	if e.required {
		return fmt.Sprintf("%s BIOS", e.system)
	}
	return fmt.Sprintf("%s: optional BIOS/firmware", e.system)
}

// splitLabel splits a BiosLabel entry at its trailing " (<note>)" into a
// short display name and the technical caveat a beginner doesn't need up
// front - e.g. "DS firmware (per-console MAC address; size-checked, not
// hashed)" becomes ("DS firmware", "per-console MAC address;
// size-checked, not hashed"). A label with no trailing parenthetical
// (most of them) returns unchanged with an empty note.
func splitLabel(label string) (name, note string) {
	if i := strings.Index(label, " ("); i >= 0 && strings.HasSuffix(label, ")") {
		return label[:i], label[i+2 : len(label)-1]
	}
	return label, ""
}

// biosStatus is one biosEntry's computed state against an actual bios/
// directory. issue is message's short form - a couple of words at most,
// e.g. "missing"/"hash mismatch" - for a table/CSV cell a beginner can
// scan at a glance; message keeps the full detail (exact hash values, the
// no-games-yet nuance) for json/yaml/toml, where a script or a curious
// user can still get it.
type biosStatus struct {
	entry   biosEntry
	present string // the alternative actually found under bios/, "" if none
	ok      bool   // present, and correct wherever there's a reference to check
	level   audit.Level
	issue   string
	message string
}

// evaluateBios checks one entry against lib.Bios: whether any alternative
// is present, and if so, whether it matches a known MD5 (systemdb.BiosMD5)
// or size (systemdb.BiosSize) reference - neither of which every file has
// (see their doc comments for why, e.g. PS2's many valid revisions or a
// real console's per-user NDS firmware.bin). Severity follows directly
// from what's actually at stake:
//
//   - missing + required + the system has games right now -> FAIL (broken)
//   - missing + required + no games yet -> WARN (nothing's broken today,
//     but it will be the moment a game is added)
//   - missing + optional -> INFO, regardless of games (nothing needs it)
//   - present but wrong hash/size + required + games -> FAIL
//   - present but wrong hash/size + required + no games, or optional
//     -> WARN (their copy looks suspect either way, but nothing's
//     actively broken without games, and an optional file "mostly
//     working" is the normal case for it)
//   - present and correct, or present with nothing to verify against
//     -> no finding at all, same as before this feature existed.
func evaluateBios(lib *library.Library, entry biosEntry) biosStatus {
	st := biosStatus{entry: entry}
	for _, alt := range entry.alternatives {
		if matches, _ := filepath.Glob(filepath.Join(lib.Bios, alt)); len(matches) > 0 {
			st.present = alt
			break
		}
	}

	hasGames := library.HasGames(filepath.Join(lib.Roms, entry.system))

	if st.present == "" {
		switch {
		case !entry.required:
			st.level, st.issue, st.message = audit.Info, "optional", "optional; not present"
		case hasGames:
			st.level, st.issue, st.message = audit.Fail, "missing", "required BIOS missing"
		default:
			st.level, st.issue, st.message = audit.Warn, "missing", "required BIOS missing (no games present for "+entry.system+" yet)"
		}
		return st
	}

	path := filepath.Join(lib.Bios, st.present)
	badIssue, badMsg := "", ""
	if wantSum, ok := systemdb.BiosMD5[st.present]; ok {
		data, err := os.ReadFile(path)
		if err != nil {
			st.level, st.issue, st.message = audit.Warn, "unreadable", "present but unreadable: "+err.Error()
			return st
		}
		if sum := md5Hex(data); sum != wantSum {
			badIssue, badMsg = "hash mismatch", fmt.Sprintf("MD5 %s does not match reference %s", sum, wantSum)
		}
	} else if wantSize, ok := systemdb.BiosSize[st.present]; ok {
		if info, err := os.Stat(path); err == nil && info.Size() != wantSize {
			badIssue, badMsg = "wrong size", fmt.Sprintf("unexpected size %d (want %d)", info.Size(), wantSize)
		}
	}
	if badMsg == "" {
		st.ok = true
		st.message = "present"
		return st
	}
	st.issue, st.message = badIssue, badMsg
	if entry.required && hasGames {
		st.level = audit.Fail
	} else {
		st.level = audit.Warn
	}
	return st
}

// checkBios audits BIOS files for Check's bios category: one finding per
// biosCatalog entry that isn't simply "present and correct" (see
// evaluateBios), same buffered-findings model every other check category
// uses. A missing bios/ directory skips every system/hash check (nothing
// to check against) but still records every optional entry as its own
// INFO, same as if bios/ existed and was simply empty - a missing
// directory isn't a reason to hide "you don't have an atari7800 BIOS", it
// just means nothing can be a FAIL/WARN either, since presence is
// unknowable either way.
func checkBios(report *audit.Report, lib *library.Library, systems []string) {
	if info, err := os.Stat(lib.Bios); err != nil || !info.IsDir() {
		report.SkipBios()
	}
	catalog := biosCatalog(systems)
	report.Scan(audit.CategoryBios, len(catalog))
	for _, entry := range catalog {
		st := evaluateBios(lib, entry)
		if st.ok {
			continue
		}
		report.Add(st.level, audit.CategoryBios, entry.system, "bios/"+strings.Join(entry.alternatives, "|"), st.message, "")
	}
}

// Bios audits only BIOS files: the same findings-only report as Check's
// bios category by default, or the full inventory - every known entry,
// including the ones that are simply present and correct - when verbose.
func Bios(lib *library.Library, systems []string, verbose bool, format cliutil.Format) (bool, error) {
	if verbose {
		return biosInventory(lib, systems, format)
	}

	report := audit.New(os.Stdout, false, format, "")
	checkBios(report, lib, systems)
	if format.Structured() {
		return !report.Failed(), report.Encode(os.Stdout, format)
	}
	if report.BiosSkipped() {
		fmt.Println("no bios/ directory; skipping BIOS checks")
	}
	report.Table(os.Stdout)
	report.Fixes(os.Stdout)
	return !report.Failed(), nil
}

// biosInventoryRow is one biosStatus's shape for every output form - via
// cliutil.RenderList (see its doc comment). Every field carries a real
// json/yaml/toml tag and round-trips there - the same short text the
// table shows is readable straight out of structured output too, not
// hidden behind it - and `col` tags are the only thing that varies: they
// mark which fields also render in the table/CSV (a plain "flag it out
// of the rich text" rather than a parallel short/full field pair with
// different visibility rules, less special-casing to keep straight).
// Name/Issue carry both: the short text everyone sees - most people
// running this have never heard of an MD5 or a per-console MAC address
// and don't need to - Name being the bios' short name (e.g. "DS
// firmware", not the full "DS firmware (per-console MAC address;
// size-checked, not hashed)") and Issue a couple of words ("missing",
// "hash mismatch"), not the exact values involved. Label/Message carry
// only json/yaml/toml: the same data in full (the complete name+caveat,
// the complete issue with exact hash/size values), for a script or a
// closer look, never cluttering the table/CSV. Status carries the
// severity colored for the format it was built for (see biosRows);
// json/yaml/toml keep the field's own "level" name regardless of the
// table's "Status" header, so a structured consumer's key doesn't change
// along with display wording.
type biosInventoryRow struct {
	System       string   `json:"system" yaml:"system" toml:"system" col:"System"`
	Alternatives []string `json:"alternatives" yaml:"alternatives" toml:"alternatives" col:"Alternatives"`
	Required     bool     `json:"required" yaml:"required" toml:"required" col:"Required"`
	Status       string   `json:"level" yaml:"level" toml:"level" col:"Status"`
	Present      string   `json:"present,omitempty" yaml:"present,omitempty" toml:"present,omitempty" col:"Present"`
	Issue        string   `json:"issue" yaml:"issue" toml:"issue" col:"Issue"`
	Name         string   `json:"name" yaml:"name" toml:"name" col:"Name"`
	Message      string   `json:"message" yaml:"message" toml:"message"`
	Label        string   `json:"label" yaml:"label" toml:"label"`
}

// biosRows builds the full BIOS inventory - present-and-correct entries
// included - for `romctl bios -v`. failed reports whether any entry is
// FAIL, for Bios' own exit code; Status is colored by severity under
// format (blank outside Fancy - see cliutil.ColorsFor), computed here
// rather than left to the renderer, since RenderTable/RenderList have no
// per-row severity concept of their own to color by.
func biosRows(lib *library.Library, systems []string, format cliutil.Format) (rows []biosInventoryRow, failed bool) {
	colors := cliutil.ColorsFor(format)
	levelColor := map[string]string{
		string(audit.Fail): colors.Red, string(audit.Warn): colors.Yellow,
		string(audit.Info): colors.Green, "OK": colors.Green,
	}
	catalog := biosCatalog(systems)
	rows = make([]biosInventoryRow, 0, len(catalog))
	for _, entry := range catalog {
		st := evaluateBios(lib, entry)
		level := "OK"
		issue := st.issue
		if !st.ok {
			level = string(st.level)
		} else {
			issue = "-"
		}
		if level == string(audit.Fail) {
			failed = true
		}
		label := biosLabel(entry)
		name, _ := splitLabel(label)
		rows = append(rows, biosInventoryRow{
			System: entry.system, Alternatives: entry.alternatives, Required: entry.required,
			Status: levelColor[level] + level + colors.Reset, Present: st.present,
			Issue: issue, Name: name, Message: st.message, Label: label,
		})
	}
	return rows, failed
}

// biosInventory renders the full BIOS inventory for `romctl bios -v`
// (every format RenderList handles - fancy/plain/json/yaml/toml/csv).
func biosInventory(lib *library.Library, systems []string, format cliutil.Format) (bool, error) {
	rows, failed := biosRows(lib, systems, format)
	return !failed, cliutil.RenderList(os.Stdout, format, rows)
}
