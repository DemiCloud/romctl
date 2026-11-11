// Package audit implements the check/bios findings report: the
// FAIL/WARN/INFO lines, the Checks table, and the Fixes table.
package audit

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"

	"github.com/jedib0t/go-pretty/v6/table"

	"github.com/demicloud/romctl/internal/cliutil"
)

// Level is a finding's severity.
type Level string

const (
	Fail Level = "FAIL"
	Warn Level = "WARN"
	Info Level = "INFO"
)

// Category groups findings for the checklist "check" prints alongside the
// per-finding lines, so a clean run still says what was actually looked at.
type Category string

const (
	CategoryNaming     Category = "naming"
	CategoryStructure  Category = "structure"
	CategoryContainers Category = "containers"
	CategoryBios       Category = "bios"
	CategoryDuplicates Category = "duplicates"
	CategoryVerify     Category = "verify"
)

var categoryOrder = []Category{CategoryNaming, CategoryStructure, CategoryContainers, CategoryBios, CategoryDuplicates, CategoryVerify}

// knownFixes are the subcommands a finding's fix can name - anything else
// passed as fix is treated as "no command", not a typo, since every
// command-backed finding in commands/check.go uses one of these.
var knownFixes = map[string]bool{
	"rename": true, "unpack": true, "chd": true, "m3u": true,
	"n64": true, "snes": true, "xiso": true, "rvz": true, "ps3": true, "clean": true,
	"saves": true, "relocate": true,
}

// fixLabels is knownFixes' short, Fixes-table-column phrase for each
// subcommand.
var fixLabels = map[string]string{
	"rename":   "Fix naming",
	"unpack":   "Unpack archive",
	"chd":      "Convert to CHD",
	"m3u":      "Group into playlist",
	"n64":      "Normalize byte order",
	"snes":     "Strip/rename SNES",
	"xiso":     "Convert to XISO",
	"rvz":      "Convert to RVZ",
	"ps3":      "Build PS3 ISO",
	"clean":    "Delete pending files",
	"saves":    "Move save/state file",
	"relocate": "Relocate misfiled ROM",
}

// labelDetailCommands maps a handful of command-less (fix=="") finding
// labels - see explanations - to a more specific detail command than
// their own category's generic fallback (detailCommand's check -v for
// anything not BIOS/Duplicates/Verify). Today just "Incomplete disc
// set": commands.Discs is a purpose-built, fast, focused report for
// exactly that finding, instead of pointing at the whole unscoped
// "check -v" (every category, every system) just to see which titles
// are missing a disc.
var labelDetailCommands = map[string]string{
	"Incomplete disc set": "discs",
}

// tally is one Category's running counts: scanned is every item Scan was
// called for (whether or not it produced a finding), fail/warn/info are
// the findings actually recorded. Table's Pass column is scanned minus
// the other three - how much was actually looked at and found fine.
type tally struct{ fail, warn, info, scanned int }

// finding is one buffered Add() call, kept so Fixes() can build its
// suggestions after every category has run.
type finding struct {
	level            Level
	category         Category
	system, path, fx string
	message          string
}

// Report accumulates audit findings. In verbose mode each finding prints
// as it's added; otherwise findings are buffered silently and only
// Table/Fixes print, after every category has finished. Add/Scan are safe
// to call from multiple goroutines (see commands.Check).
type Report struct {
	mu                 sync.Mutex
	w                  io.Writer
	verbose            bool
	format             cliutil.Format
	rootHint           string
	fails, warns, info int
	byCategory         map[Category]*tally
	findings           []finding
	biosSkipped        bool
	collapseBios       bool
}

// New returns a Report that prints to w. verbose prints every finding as
// it's added (Table/Fixes still print at the end either way, except under
// a structured format - see Encode). rootHint, if non-empty
// (an explicitly-passed "-r PATH"), is spliced into every suggested
// command so a copy-pasted fix still targets the right library root.
func New(w io.Writer, verbose bool, format cliutil.Format, rootHint string) *Report {
	return &Report{w: w, verbose: verbose, format: format, rootHint: rootHint, byCategory: make(map[Category]*tally)}
}

// Add records one finding, printing it immediately in verbose mode (with
// the level colored by severity in Fancy format). fix, if a known
// subcommand, is shown inline as "[<prog> [-r root] fix system]" so the
// hint can be copy-pasted; anything else (including "") is treated as no
// command, and Fixes() explains it instead.
func (r *Report) Add(level Level, category Category, system, path, message, fix string) {
	r.record(level, category, system, path, message, fix)
	if !r.verbose || r.format.Structured() {
		return
	}
	r.printFinding(level, system, path, message, fix)
}

// AddQuiet records one finding exactly as Add does (counts, Table/Fixes/
// Encode all see it the same way), but never prints it itself, even in
// verbose mode - for a caller that renders its own live progress/result
// line instead (Verify/Dupes' own cliutil.ParallelLive display), so the
// same failure doesn't get printed twice - or, worse, interleave with a
// live multi-line display's own managed block on the same terminal and
// throw off its cursor-up accounting (see ParallelLive's own doc
// comment).
func (r *Report) AddQuiet(level Level, category Category, system, path, message, fix string) {
	r.record(level, category, system, path, message, fix)
}

// record is Add's bookkeeping half: counts and buffers the finding for
// Table/Fixes/Encode, independent of whether/how it also gets printed.
func (r *Report) record(level Level, category Category, system, path, message, fix string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	switch level {
	case Fail:
		r.fails++
	case Warn:
		r.warns++
	case Info:
		r.info++
	}
	t := r.byCategory[category]
	if t == nil {
		t = &tally{}
		r.byCategory[category] = t
	}
	switch level {
	case Fail:
		t.fail++
	case Warn:
		t.warn++
	case Info:
		t.info++
	}
	r.findings = append(r.findings, finding{level, category, system, path, fix, message})
}

// printFinding renders one finding's verbose line to r.w - Add's other
// half, split out only so it reads as two clear steps (record, then
// print) rather than one long function.
func (r *Report) printFinding(level Level, system, path, message, fix string) {
	colors := cliutil.ColorsFor(r.format)
	levelColor := map[Level]string{Fail: colors.Red, Warn: colors.Yellow, Info: colors.Green}[level]
	coloredLevel := fmt.Sprintf("%s%-4s%s", levelColor, level, colors.Reset)
	if knownFixes[fix] {
		fmt.Fprintf(r.w, "%s  %-12s %s: %s  [%s]\n", coloredLevel, system, path, message, r.commandHint(fix, system))
		return
	}
	fmt.Fprintf(r.w, "%s  %-12s %s: %s\n", coloredLevel, system, path, message)
}

// Scan records n items of category actually examined, whether or not any
// produced a finding - the denominator Table's Pass column is computed
// against, so a clean row still conveys how much ground was actually
// covered rather than just "nothing to report."
func (r *Report) Scan(category Category, n int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	t := r.byCategory[category]
	if t == nil {
		t = &tally{}
		r.byCategory[category] = t
	}
	t.scanned += n
}

// Finding is one recorded Add/AddQuiet call, exposed read-only for a
// caller that needs to react to specific findings itself (commands.Scrap,
// deciding what's a confirmed dead end worth retiring) rather than just
// display them the way Table/Fixes/Encode do - reusing check's own
// structure/containers detection logic (what counts as corrupt, a
// leftover artifact, an unconvertible loose disc, ...) instead of a
// second implementation of the same judgment calls.
type Finding struct {
	Level    Level
	Category Category
	System   string
	Path     string
	Message  string
	Fix      string
}

// Findings returns every finding recorded so far (same underlying data
// Table/Fixes/Encode already read, just not reduced to a display shape).
func (r *Report) Findings() []Finding {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]Finding, len(r.findings))
	for i, f := range r.findings {
		out[i] = Finding{Level: f.level, Category: f.category, System: f.system, Path: f.path, Message: f.message, Fix: f.fx}
	}
	return out
}

// commandHint builds "<prog> [-r root] <fix> <system>" for a per-line hint.
func (r *Report) commandHint(fix, system string) string {
	parts := []string{cliutil.ProgName()}
	if r.rootHint != "" {
		parts = append(parts, r.rootHint)
	}
	parts = append(parts, fix, system)
	return strings.Join(parts, " ")
}

// SkipBios marks the BIOS category as not run (no bios/ directory), so the
// checklist says "skipped" instead of a possibly-misleading "clean".
func (r *Report) SkipBios() {
	r.biosSkipped = true
}

// BiosSkipped reports whether SkipBios was called.
func (r *Report) BiosSkipped() bool {
	return r.biosSkipped
}

// CollapseBios enables collapsing every BIOS finding in Fixes into a
// handful of category-level rows (System "BIOS", pointing at `bios -v`)
// instead of listing each one individually. Check calls this: `bios` is
// a whole dedicated command for "what's wrong with my BIOS files", so
// check's own Fixes table shouldn't try to enumerate them. Bios's own
// Report never calls this - it IS that dedicated command, so it always
// lists every BIOS finding individually, the same as every other
// category does by default (and the same reason Dupes/Verify's own -v
// mode bypasses Fixes entirely in favor of a dedicated live/detailed
// view, rather than needing an equivalent flag for their own category -
// see commands.Dupes/commands.Verify).
func (r *Report) CollapseBios() {
	r.collapseBios = true
}

// Failed reports whether any FAIL-level finding was recorded.
func (r *Report) Failed() bool {
	return r.fails > 0
}

// symbols returns the fail/warn/ok glyphs for format: unicode in Fancy,
// plain ASCII words everywhere else (Plain has no unicode/color at all;
// a structured format never reaches these - see Encode).
func symbols(format cliutil.Format) (fail, warn, ok string) {
	if format.Fancy() {
		return "✗", "!", "✓"
	}
	return "FAIL", "WARN", "OK"
}

// Table renders one row per Category actually engaged this run - a
// category neither Scan nor Add was ever called for (e.g. every category
// but bios, when only Bios() ran instead of the full Check) is left out
// entirely, rather than printed as a misleading "clean" - with a
// pass/warn/fail status glyph and the Pass/Warn/Fail/Info tallies, as a
// single rich table (unicode box-drawing in Fancy format, bare ASCII
// otherwise - see cliutil.NewTable). Bios explicitly skipped (no bios/
// directory) gets a row of dashes instead of real tallies, since nothing
// there is actually knowable either way. Starts with a blank line so
// it's visually separated from the live per-phase status lines printed
// just before it.
func (r *Report) Table(w io.Writer) {
	colors := cliutil.ColorsFor(r.format)
	failSym, warnSym, okSym := symbols(r.format)

	fmt.Fprintln(w)
	var rows []table.Row
	for _, cat := range categoryOrder {
		if cat == CategoryBios && r.biosSkipped {
			rows = append(rows, table.Row{string(cat), "-", "-", "-", "-", "-"})
			continue
		}
		tl := r.byCategory[cat]
		if tl == nil {
			continue
		}
		pass := tl.scanned - tl.fail - tl.warn - tl.info
		if pass < 0 {
			pass = 0
		}
		status := fmt.Sprintf("%s%s%s", colors.Green, okSym, colors.Reset)
		switch {
		case tl.fail > 0:
			status = fmt.Sprintf("%s%s%s", colors.Red, failSym, colors.Reset)
		case tl.warn > 0:
			status = fmt.Sprintf("%s%s%s", colors.Yellow, warnSym, colors.Reset)
		}
		rows = append(rows, table.Row{string(cat), status, pass, tl.warn, tl.fail, tl.info})
	}
	cliutil.RenderTable(w, r.format, []string{"Check", "Status", "Pass", "Warn", "Fail", "Info"}, rows)
}

// explanations maps a message substring to a short Fixes-table label and
// a fix phrase built with the finding's own path - checked in order,
// first match wins. Every entry here corresponds to a fix=="" finding in
// commands/check.go; a future one that isn't listed still gets a row
// (explainLabel falls back to the category name, explain to the raw
// message), just not a prescribed action.
var explanations = []struct {
	substr string
	label  string
	build  func(path string) string
}{
	{"required BIOS missing", "Missing BIOS file", func(p string) string { return fmt.Sprintf("acquire %s and place it in bios/", p) }},
	{"does not match reference", "Bad BIOS hash", func(p string) string {
		return fmt.Sprintf("re-acquire %s; its hash doesn't match the known-good version", p)
	}},
	{"unexpected size", "Bad BIOS size", func(p string) string {
		return fmt.Sprintf("re-acquire bios/%s; wrong file size", p)
	}},
	{"not a valid CHD", "Corrupt CHD", func(p string) string { return fmt.Sprintf("%s is corrupt; re-dump or re-acquire it", p) }},
	{"failed data verify", "Corrupt CHD (data)", func(p string) string {
		return fmt.Sprintf("%s failed chdman's data verify; re-dump or re-acquire it", p)
	}},
	{"not a valid XISO", "Corrupt XISO", func(p string) string { return fmt.Sprintf("%s is corrupt; re-dump or re-acquire it", p) }},
	{"truncated or not ISO9660", "Corrupt ISO", func(p string) string { return fmt.Sprintf("%s is corrupt; re-dump or re-acquire it", p) }},
	{"leftover extraction directory", "Leftover extraction dir", func(p string) string {
		return fmt.Sprintf("delete %s manually; left over from an interrupted run", p)
	}},
	// Checked before the bare "incomplete" entry below: this finding's
	// own message also contains that word ("incomplete disc set"), and
	// explain/explainLabel take the first substring match - without this
	// ordering, every incomplete-disc-set finding would be misreported as
	// a stray leftover file instead.
	{"incomplete disc set", "Incomplete disc set", func(p string) string {
		return fmt.Sprintf("%s is missing disc(s) from its set - acquire the rest, or delete what you have", p)
	}},
	{"incomplete", "Incomplete leftover file", func(p string) string {
		return fmt.Sprintf("delete %s manually; left over from an interrupted run", p)
	}},
	{"no EBOOT.BIN found", "No EBOOT.BIN", func(p string) string {
		return fmt.Sprintf("re-acquire the %s PS3 dump; EBOOT.BIN is missing or the folder is incomplete", p)
	}},
	{"missing playlist", "Missing playlist", func(p string) string {
		return fmt.Sprintf("recreate %s, listing each disc file one per line", p)
	}},
	{"playlist entry not found", "Bad playlist entry", func(p string) string {
		return fmt.Sprintf("restore the disc file referenced in %s, or remove its line from the playlist", p)
	}},
	{"disc file(s) but", "Playlist count mismatch", func(p string) string {
		return fmt.Sprintf("check %s against its actual disc files; counts don't match", p)
	}},
	{"no matching .cue sheet", "Missing cue sheet", func(p string) string {
		return fmt.Sprintf("%s has no .cue sheet; chd can't convert it - re-acquire it with one", p)
	}},
	{"identical to", "Duplicate file", func(p string) string {
		return fmt.Sprintf("remove %s; it's an exact duplicate of another file already kept", p)
	}},
	{"not a system directory", "Unknown system dir", func(p string) string {
		return fmt.Sprintf("check %s for a typo, or remove it if it's an old/unused system", p)
	}},
}

// explain returns the fix phrase for a fix=="" finding's message, falling
// back to the raw message verbatim if nothing in explanations matches -
// so a finding never silently loses its specifics.
func explain(message, path string) string {
	for _, e := range explanations {
		if strings.Contains(message, e.substr) {
			return e.build(path)
		}
	}
	return message
}

// explainLabel returns explanations' short Fixes-table label for a
// fix=="" finding's message, falling back to its category name.
func explainLabel(category Category, message string) string {
	for _, e := range explanations {
		if strings.Contains(message, e.substr) {
			return e.label
		}
	}
	return string(category)
}

// fixRow is one row of Fixes' table: issueType is what's wrong, count is
// how many findings it covers, system is where (a real system name,
// "BIOS"/a category name once rows are collapsed together, or "all"),
// autoFix is whether some romctl command can actually resolve it itself
// (true only for a knownFixes-backed row - see fixRows), and fix is that
// command when autoFix, or, when not, either a plain-English instruction
// or a read-only detail command (bios -v, discs, check -v, ...) that
// shows exactly what's wrong without claiming to fix it. Acquiring a
// replacement BIOS/ROM or the rest of an incomplete disc set is always
// autoFix=false and always will be: sourcing that content is explicitly
// out of scope (see AGENTS.md's Scope boundaries) regardless of how the
// finding is otherwise reported.
type fixRow struct {
	issueType   string
	count       int
	system, fix string
	autoFix     bool
}

// detailCommand returns the command that shows every instance behind a
// collapsed command-less row in full: the category's own dedicated audit
// command (bios -v/dupes -v/verify -v), else check's own -v. In practice
// only reached for a category a caller has marked with Collapse, which
// today is only check calling it for CategoryBios - dupes/duplicates and
// verify's own commands never collapse their own findings (see Collapse's
// doc comment), so these cases exist for correctness if that ever
// changes, not because they're hit today.
func (r *Report) detailCommand(category Category) string {
	parts := []string{cliutil.ProgName()}
	if r.rootHint != "" {
		parts = append(parts, r.rootHint)
	}
	switch category {
	case CategoryBios:
		parts = append(parts, "bios", "-v")
	case CategoryDuplicates:
		parts = append(parts, "dupes", "-v")
	case CategoryVerify:
		parts = append(parts, "verify", "-v")
	default:
		parts = append(parts, "check", "-v")
	}
	return strings.Join(parts, " ")
}

// fixRows groups every actionable (Fail/Warn - an Info finding needs no
// action, so it's left out entirely) finding into Fixes' rows:
//
//   - One row per distinct command-backed fix type, naming every system
//     it actually affects (or "all" once more than one does, since every
//     fix command also accepts "all" - so a copy-pasted command still
//     fixes everything at once rather than needing one run per system).
//
//   - Command-less findings are grouped by (category, label). A library
//     can easily have dozens of bad/missing BIOS files, and check isn't
//     the place to enumerate them one by one - `bios` is a whole
//     dedicated command for exactly that question, so every BIOS finding
//     always collapses to its label, with "BIOS" (not any one console)
//     as System and detailCommand's "bios -v" as the row's Fix. Every
//     other command-less category has no narrower dedicated command, so
//     a label with only one finding keeps its own system and specific
//     instruction (nothing lost by being specific when there's only
//     one); a label with more collapses the same way, pointing at
//     `check -v` instead.
//
// Findings are sorted first so row order (and, for a shared fix type,
// which system(s) land in "all") is deterministic regardless of which
// goroutine's Add() call happened to land first.
func (r *Report) fixRows() []fixRow {
	r.mu.Lock()
	findings := append([]finding(nil), r.findings...)
	r.mu.Unlock()
	sort.Slice(findings, func(i, j int) bool {
		a, b := findings[i], findings[j]
		if a.category != b.category {
			return a.category < b.category
		}
		if a.system != b.system {
			return a.system < b.system
		}
		return a.path < b.path
	})

	var rows []fixRow

	var cmdOrder []string
	cmdCounts := map[string]int{}
	cmdSystems := map[string]map[string]bool{}
	for _, f := range findings {
		if f.level == Info || !knownFixes[f.fx] {
			continue
		}
		if cmdCounts[f.fx] == 0 {
			cmdOrder = append(cmdOrder, f.fx)
			cmdSystems[f.fx] = map[string]bool{}
		}
		cmdCounts[f.fx]++
		cmdSystems[f.fx][f.system] = true
	}
	for _, fx := range cmdOrder {
		sys := "all"
		if systems := cmdSystems[fx]; len(systems) == 1 {
			for s := range systems {
				sys = s
			}
		}
		rows = append(rows, fixRow{issueType: fixLabels[fx], count: cmdCounts[fx], system: sys, fix: r.commandHint(fx, sys), autoFix: true})
	}

	type bucketKey struct {
		category Category
		label    string
	}
	var bucketOrder []bucketKey
	buckets := map[bucketKey][]finding{}
	for _, f := range findings {
		if f.level == Info || knownFixes[f.fx] {
			continue
		}
		k := bucketKey{f.category, explainLabel(f.category, f.message)}
		if buckets[k] == nil {
			bucketOrder = append(bucketOrder, k)
		}
		buckets[k] = append(buckets[k], f)
	}
	for _, k := range bucketOrder {
		fs := buckets[k]
		if k.category == CategoryBios && !r.collapseBios {
			// bios.go's own Report: this IS the dedicated detail view
			// (see CollapseBios), so every BIOS finding is listed
			// individually, same as any other category's lone finding.
			for _, f := range fs {
				rows = append(rows, fixRow{issueType: k.label, count: 1, system: f.system, fix: explain(f.message, f.path)})
			}
			continue
		}
		if k.category != CategoryBios && len(fs) == 1 {
			f := fs[0]
			rows = append(rows, fixRow{issueType: k.label, count: 1, system: f.system, fix: explain(f.message, f.path)})
			continue
		}
		sys := string(k.category)
		if k.category == CategoryBios {
			sys = "BIOS"
		}
		fix := r.detailCommand(k.category)
		// A few command-less labels have a more specific, purpose-built
		// detail command than their own category's generic fallback
		// (check -v/bios -v/...) - see labelDetailCommands - and are
		// worth naming the actually-affected system(s) for, the same way
		// the knownFixes-backed rows above already do, rather than the
		// bare category-name placeholder every other collapsed row uses.
		if cmd, ok := labelDetailCommands[k.label]; ok {
			sys = "all"
			sysSet := map[string]bool{}
			for _, f := range fs {
				sysSet[f.system] = true
			}
			if len(sysSet) == 1 {
				for s := range sysSet {
					sys = s
				}
			}
			fix = r.commandHint(cmd, sys)
		}
		rows = append(rows, fixRow{issueType: k.label, count: len(fs), system: sys, fix: fix})
	}
	return rows
}

// Fixes renders how to resolve what was found, as a single rich table
// (Issue Type/Count/System/Fix - Count its own column rather than baked
// into the label text, standard table UX): omitted entirely once fixRows
// is empty, same as a clean report printing nothing before. Starts with a
// blank line so it's visually separated from the Checks table printed
// just before it.
func (r *Report) Fixes(w io.Writer) {
	rows := r.fixRows()
	if len(rows) == 0 {
		return
	}
	fmt.Fprintln(w)
	tableRows := make([]table.Row, len(rows))
	for i, row := range rows {
		autoFix := "no"
		if row.autoFix {
			autoFix = "yes"
		}
		tableRows[i] = table.Row{row.issueType, row.count, row.system, autoFix, row.fix}
	}
	cliutil.RenderTable(w, r.format, []string{"Issue Type", "Count", "System", "Auto-Fix", "Fix"}, tableRows)
}

// structuredFinding is one finding's shape in Encode's json/yaml/toml/csv
// output. Fix is the bare subcommand name (e.g. "rename"), empty when the
// finding has no command-backed fix - the same knownFixes test Add/Fixes
// already use to tell a real fix from an explanatory message. The `col`
// tags are what CSV's columns/rows come from (see cliutil.RenderList) -
// the same struct json/yaml/toml encode directly, so every format is
// guaranteed to agree on what a finding looks like.
type structuredFinding struct {
	Level    string `json:"level" yaml:"level" toml:"level" col:"level"`
	Category string `json:"category" yaml:"category" toml:"category" col:"category"`
	System   string `json:"system" yaml:"system" toml:"system" col:"system"`
	Path     string `json:"path" yaml:"path" toml:"path" col:"path"`
	Message  string `json:"message" yaml:"message" toml:"message" col:"message"`
	Fix      string `json:"fix,omitempty" yaml:"fix,omitempty" toml:"fix,omitempty" col:"fix"`
}

// structuredSummary is Encode's summary tally shape.
type structuredSummary struct {
	Fail int `json:"fail" yaml:"fail" toml:"fail"`
	Warn int `json:"warn" yaml:"warn" toml:"warn"`
	Info int `json:"info" yaml:"info" toml:"info"`
}

// structuredReport is Encode's top-level json/yaml/toml shape.
type structuredReport struct {
	Findings []structuredFinding `json:"findings" yaml:"findings" toml:"findings"`
	Summary  structuredSummary   `json:"summary" yaml:"summary" toml:"summary"`
}

// Encode writes every finding, plus the summary tally, as json/yaml/toml
// or csv - for scripting against check's results instead of parsing its
// text output. Unlike the text report, this has no Table equivalent
// (which category ran, or how much it scanned, isn't a "finding"); a
// category that found nothing simply contributes no findings. CSV carries
// the findings table alone, with no separate summary row - a consumer can
// get the same tally by counting rows per level.
func (r *Report) Encode(w io.Writer, format cliutil.Format) error {
	r.mu.Lock()
	findings := append([]finding(nil), r.findings...)
	fails, warns, info := r.fails, r.warns, r.info
	r.mu.Unlock()

	sfindings := make([]structuredFinding, 0, len(findings))
	for _, f := range findings {
		sf := structuredFinding{Level: string(f.level), Category: string(f.category), System: f.system, Path: f.path, Message: f.message}
		if knownFixes[f.fx] {
			sf.Fix = f.fx
		}
		sfindings = append(sfindings, sf)
	}

	if format == cliutil.FormatCSV {
		return cliutil.EncodeListCSV(w, sfindings)
	}

	out := structuredReport{Findings: sfindings, Summary: structuredSummary{Fail: fails, Warn: warns, Info: info}}
	return cliutil.Encode(w, format, out)
}
