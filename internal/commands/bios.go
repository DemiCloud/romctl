package commands

import (
	"fmt"
	"io/fs"
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
// something sensible instead of nothing.
func biosLabel(e biosEntry) string {
	key := e.system
	if !e.required {
		key = e.system + "/" + e.alternatives[0]
	}
	if label, ok := systemdb.BiosLabel[key]; ok {
		return label
	}
	if e.required {
		return fmt.Sprintf("%s BIOS", e.system)
	}
	return fmt.Sprintf("%s: optional BIOS/firmware", e.system)
}

// biosStatus is one biosEntry's computed state against an actual bios/
// directory.
type biosStatus struct {
	entry   biosEntry
	present string // the alternative actually found under bios/, "" if none
	ok      bool   // present, and correct wherever there's a reference to check
	level   audit.Level
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
			st.level, st.message = audit.Info, "optional; not present"
		case hasGames:
			st.level, st.message = audit.Fail, "required BIOS missing"
		default:
			st.level, st.message = audit.Warn, "required BIOS missing (no games present for "+entry.system+" yet)"
		}
		return st
	}

	path := filepath.Join(lib.Bios, st.present)
	badMsg := ""
	if wantSum, ok := systemdb.BiosMD5[st.present]; ok {
		data, err := os.ReadFile(path)
		if err != nil {
			st.level, st.message = audit.Warn, "present but unreadable: "+err.Error()
			return st
		}
		if sum := md5Hex(data); sum != wantSum {
			badMsg = fmt.Sprintf("MD5 %s does not match reference %s", sum, wantSum)
		}
	} else if wantSize, ok := systemdb.BiosSize[st.present]; ok {
		if info, err := os.Stat(path); err == nil && info.Size() != wantSize {
			badMsg = fmt.Sprintf("unexpected size %d (want %d)", info.Size(), wantSize)
		}
	}
	if badMsg == "" {
		st.ok = true
		st.message = "present"
		return st
	}
	if entry.required && hasGames {
		st.level, st.message = audit.Fail, badMsg
	} else {
		st.level, st.message = audit.Warn, badMsg
	}
	return st
}

// biosKnownAlternatives returns every filename/glob pattern used by any
// system's BiosRequired group or BiosOptional entry, across every system
// romctl knows about - bios/ is one shared directory, not scoped per
// system, so a file sitting there could belong to any system, whether or
// not that system is in the current systems list.
func biosKnownAlternatives() map[string]bool {
	known := map[string]bool{}
	for _, groups := range systemdb.BiosRequired {
		for _, group := range groups {
			for _, alt := range group {
				known[alt] = true
			}
		}
	}
	for _, groups := range systemdb.BiosOptional {
		for _, group := range groups {
			for _, alt := range group {
				known[alt] = true
			}
		}
	}
	return known
}

// unrecognizedBiosFiles walks lib.Bios and returns the forward-slash
// relative path of every regular file that doesn't match any known
// alternative (a glob pattern like "ps2-*.bin" is matched with
// filepath.Match, not exact string equality) - the inverse of the usual
// "what's missing" check: what's present that romctl doesn't recognize
// at all. Always INFO wherever it's used (see checkBios/biosRows): an
// unrecognized file is never romctl's business to call wrong, just worth
// a heads-up (a typo'd filename, leftover from a system no longer used,
// ordinary OS/editor clutter). Anything under rpcs3/ is skipped wholesale
// - that's an installed, multi-file PS3 firmware tree (see
// systemdb.BiosRequired's ps3 entry, which only names one file inside it
// as a presence proxy), not a single file to catalog, so every one of
// its thousands of files would otherwise be flagged. Dotfiles (editor/OS
// metadata) are skipped too.
func unrecognizedBiosFiles(lib *library.Library) []string {
	known := biosKnownAlternatives()
	var out []string
	filepath.WalkDir(lib.Bios, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(lib.Bios, path)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if strings.HasPrefix(filepath.Base(rel), ".") || strings.HasPrefix(rel, "rpcs3/") {
			return nil
		}
		for alt := range known {
			if matched, _ := filepath.Match(alt, rel); matched {
				return nil
			}
		}
		out = append(out, rel)
		return nil
	})
	sort.Strings(out)
	return out
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
	for _, entry := range biosCatalog(systems) {
		st := evaluateBios(lib, entry)
		if st.ok {
			continue
		}
		report.Add(st.level, audit.CategoryBios, entry.system, "bios/"+strings.Join(entry.alternatives, "|"), st.message, "")
	}
	for _, rel := range unrecognizedBiosFiles(lib) {
		report.Add(audit.Info, audit.CategoryBios, "bios", "bios/"+rel, "not a romctl-tracked BIOS/firmware file", "")
	}
}

// Bios audits only BIOS files: the same findings-only report as Check's
// bios category by default, or the full inventory - every known entry,
// including the ones that are simply present and correct - when verbose.
func Bios(lib *library.Library, systems []string, verbose bool, format cliutil.Format) (bool, error) {
	if verbose && !format.Structured() {
		return biosInventory(lib, systems, format)
	}

	report := audit.New(os.Stdout, false, format, "")
	checkBios(report, lib, systems)
	if format.Structured() {
		if verbose {
			return !report.Failed(), biosInventoryEncode(lib, systems, format)
		}
		return !report.Failed(), report.Encode(os.Stdout, format)
	}
	if report.BiosSkipped() {
		fmt.Println("no bios/ directory; skipping BIOS checks")
	}
	report.Summary(os.Stdout)
	return !report.Failed(), nil
}

// biosInventoryRow is one biosStatus rendered for text or structured
// output - Label is the one addition beyond biosStatus's own fields,
// since it's derived (systemdb.BiosLabel/biosLabel), not stored.
type biosInventoryRow struct {
	System       string   `json:"system" yaml:"system" toml:"system"`
	Alternatives []string `json:"alternatives" yaml:"alternatives" toml:"alternatives"`
	Required     bool     `json:"required" yaml:"required" toml:"required"`
	Level        string   `json:"level" yaml:"level" toml:"level"`
	Present      string   `json:"present,omitempty" yaml:"present,omitempty" toml:"present,omitempty"`
	Message      string   `json:"message" yaml:"message" toml:"message"`
	Label        string   `json:"label" yaml:"label" toml:"label"`
}

func biosRows(lib *library.Library, systems []string) []biosInventoryRow {
	catalog := biosCatalog(systems)
	rows := make([]biosInventoryRow, 0, len(catalog))
	for _, entry := range catalog {
		st := evaluateBios(lib, entry)
		level := "OK"
		if !st.ok {
			level = string(st.level)
		}
		rows = append(rows, biosInventoryRow{
			System: entry.system, Alternatives: entry.alternatives, Required: entry.required,
			Level: level, Present: st.present, Message: st.message, Label: biosLabel(entry),
		})
	}
	for _, rel := range unrecognizedBiosFiles(lib) {
		rows = append(rows, biosInventoryRow{
			System: "bios", Alternatives: []string{rel}, Required: false,
			Level: string(audit.Info), Present: rel, Message: "not a romctl-tracked BIOS/firmware file",
		})
	}
	return rows
}

// biosInventory prints every known BIOS entry - present and correct ones
// included - as a text table, for `romctl bios -v`. Each row's level is
// colored by severity in Fancy format.
func biosInventory(lib *library.Library, systems []string, format cliutil.Format) (bool, error) {
	rows := biosRows(lib, systems)
	colors := cliutil.ColorsFor(format)
	levelColor := map[string]string{
		string(audit.Fail): colors.Red, string(audit.Warn): colors.Yellow,
		string(audit.Info): colors.Green, "OK": colors.Green,
	}
	failed := false
	fmt.Println("BIOS inventory:")
	for _, r := range rows {
		if r.Level == string(audit.Fail) {
			failed = true
		}
		detail := r.Message
		if r.Present != "" && len(r.Alternatives) > 1 {
			// Only worth naming which one, out of several alternatives,
			// was actually found - a single-alternative row already
			// shows that same name in the Alternatives column.
			detail = r.Present + ": " + detail
		}
		if r.Label != "" {
			detail += " (" + r.Label + ")"
		}
		fmt.Printf("  %s%-4s%s  %-12s %-50s %s\n", levelColor[r.Level], r.Level, colors.Reset, r.System, strings.Join(r.Alternatives, "|"), detail)
	}
	return !failed, nil
}

// biosInventoryEncode encodes the same full inventory biosInventory prints,
// for `romctl bios -v --format=json` (etc.) - `-v`'s structured payload is
// the complete row set rather than Check/Bios' usual findings-only one,
// since that's the entire point of asking for the inventory specifically.
func biosInventoryEncode(lib *library.Library, systems []string, format cliutil.Format) error {
	rows := biosRows(lib, systems)
	if format == cliutil.FormatCSV {
		header := []string{"system", "alternatives", "required", "level", "present", "message", "label"}
		csvRows := make([][]string, 0, len(rows))
		for _, r := range rows {
			csvRows = append(csvRows, []string{
				r.System, strings.Join(r.Alternatives, "|"), fmt.Sprintf("%t", r.Required),
				r.Level, r.Present, r.Message, r.Label,
			})
		}
		return cliutil.EncodeCSV(os.Stdout, header, csvRows)
	}
	return cliutil.Encode(os.Stdout, format, rows)
}
