// Package audit implements the check/bios findings report: the
// FAIL/WARN/INFO lines, the per-category checklist, the Fixes section, and
// the colorized summary.
package audit

import (
	"fmt"
	"io"
	"strings"
	"sync"

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
)

var categoryOrder = []Category{CategoryNaming, CategoryStructure, CategoryContainers, CategoryBios, CategoryDuplicates}

var categoryLabels = map[Category]string{
	CategoryNaming:     "naming (tags, articles)",
	CategoryStructure:  "structure (leftovers, playlists)",
	CategoryContainers: "containers (formats, conversions)",
	CategoryBios:       "bios (presence, hashes)",
	CategoryDuplicates: "duplicates (exact content matches)",
}

// knownFixes are the subcommands a finding's fix can name - anything else
// passed as fix is treated as "no command", not a typo, since every
// command-backed finding in commands/check.go uses one of these.
var knownFixes = map[string]bool{
	"rename": true, "unpack": true, "chd": true, "m3u": true, "dvd": true,
	"n64": true, "snes": true, "xiso": true, "rvz": true, "ps3": true, "clean": true,
	"saves": true, "relocate": true,
}

type tally struct{ fail, warn, info int }

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
// Checklist/Fixes/Summary print, after every category has finished.
// Add is safe to call from multiple goroutines (see commands.Check).
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
	dupesEnabled       bool
}

// New returns a Report that prints to w. verbose prints every finding as
// it's added (Fixes/Checklist/Summary still print at the end either way,
// except under a structured format - see Encode). rootHint, if non-empty
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

	if !r.verbose || r.format.Structured() {
		return
	}
	colors := cliutil.ColorsFor(r.format)
	levelColor := map[Level]string{Fail: colors.Red, Warn: colors.Yellow, Info: colors.Green}[level]
	coloredLevel := fmt.Sprintf("%s%-4s%s", levelColor, level, colors.Reset)
	if knownFixes[fix] {
		fmt.Fprintf(r.w, "%s  %-12s %s: %s  [%s]\n", coloredLevel, system, path, message, r.commandHint(fix, system))
		return
	}
	fmt.Fprintf(r.w, "%s  %-12s %s: %s\n", coloredLevel, system, path, message)
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

// EnableDuplicates marks the duplicates category as run (--dupes was
// passed), so the checklist shows its real tally instead of "skipped".
// Unlike every other check, finding exact duplicates means reading every
// file's full content, which is a meaningfully different cost on a
// library with large CHD/ISO files - so it only runs when asked for.
func (r *Report) EnableDuplicates() {
	r.dupesEnabled = true
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

// Checklist prints one line per Category, independent of whether it found
// anything, so a run that ends "Clean" still shows what was actually
// checked rather than just the absence of findings.
func (r *Report) Checklist(w io.Writer) {
	colors := cliutil.ColorsFor(r.format)
	failSym, warnSym, okSym := symbols(r.format)
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Checks:")
	for _, cat := range categoryOrder {
		label := categoryLabels[cat]
		if cat == CategoryBios && r.biosSkipped {
			fmt.Fprintf(w, "  %-34s - skipped (no bios/ directory)\n", label)
			continue
		}
		if cat == CategoryDuplicates && !r.dupesEnabled {
			fmt.Fprintf(w, "  %-34s - skipped (use --dupes)\n", label)
			continue
		}
		t := r.byCategory[cat]
		if t == nil {
			t = &tally{}
		}
		switch {
		case t.fail > 0:
			fmt.Fprintf(w, "  %-34s %s%s %d fail%s, %d warn\n", label, colors.Red, failSym, t.fail, colors.Reset, t.warn)
		case t.warn > 0:
			fmt.Fprintf(w, "  %-34s %s%s %d warn%s\n", label, colors.Yellow, warnSym, t.warn, colors.Reset)
		default:
			fmt.Fprintf(w, "  %-34s %s%s clean%s\n", label, colors.Green, okSym, colors.Reset)
		}
	}
}

// explanations maps a message substring to a fix phrase, built with the
// finding's own path - checked in order, first match wins. Every entry
// here corresponds to a fix=="" finding in commands/check.go; a future
// one that isn't listed still gets a line (Fixes falls back to the raw
// message), just not a prescribed action.
var explanations = []struct {
	substr string
	build  func(path string) string
}{
	{"required BIOS missing", func(p string) string { return fmt.Sprintf("acquire %s and place it in bios/", p) }},
	{"does not match reference", func(p string) string {
		return fmt.Sprintf("re-acquire %s; its hash doesn't match the known-good version", p)
	}},
	{"unexpected size for DS firmware", func(p string) string {
		return fmt.Sprintf("re-acquire bios/%s; wrong file size", p)
	}},
	{"not a valid CHD", func(p string) string { return fmt.Sprintf("%s is corrupt; re-dump or re-acquire it", p) }},
	{"not a valid XISO", func(p string) string { return fmt.Sprintf("%s is corrupt; re-dump or re-acquire it", p) }},
	{"truncated or not ISO9660", func(p string) string { return fmt.Sprintf("%s is corrupt; re-dump or re-acquire it", p) }},
	{"leftover extraction directory", func(p string) string {
		return fmt.Sprintf("delete %s manually; left over from an interrupted run", p)
	}},
	{"incomplete", func(p string) string {
		return fmt.Sprintf("delete %s manually; left over from an interrupted run", p)
	}},
	{"no EBOOT.BIN found", func(p string) string {
		return fmt.Sprintf("re-acquire the %s PS3 dump; EBOOT.BIN is missing or the folder is incomplete", p)
	}},
	{"missing playlist", func(p string) string {
		return fmt.Sprintf("recreate %s, listing each disc file one per line", p)
	}},
	{"playlist entry not found", func(p string) string {
		return fmt.Sprintf("restore the disc file referenced in %s, or remove its line from the playlist", p)
	}},
	{"disc file(s) but", func(p string) string {
		return fmt.Sprintf("check %s against its actual disc files; counts don't match", p)
	}},
	{"identical to", func(p string) string {
		return fmt.Sprintf("remove %s; it's an exact duplicate of another file already kept", p)
	}},
	{"not a system directory", func(p string) string {
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

// Fixes prints how to resolve what was found: one aggregate line per
// command-backed fix type (running it fixes every instance, so a count is
// enough), then one line per command-less finding, each naming its actual
// file (a count alone wouldn't say what to go acquire).
func (r *Report) Fixes(w io.Writer) {
	if len(r.findings) == 0 {
		return
	}
	var commandOrder []string
	commandCounts := map[string]int{}
	var explained []finding
	for _, f := range r.findings {
		if knownFixes[f.fx] {
			if commandCounts[f.fx] == 0 {
				commandOrder = append(commandOrder, f.fx)
			}
			commandCounts[f.fx]++
			continue
		}
		explained = append(explained, f)
	}
	if len(commandOrder) == 0 && len(explained) == 0 {
		return
	}

	fmt.Fprintln(w)
	fmt.Fprintln(w, "Fixes:")
	for _, fix := range commandOrder {
		fmt.Fprintf(w, "  %d finding(s) -> [%s]\n", commandCounts[fix], r.commandHint(fix, "all"))
	}
	for _, f := range explained {
		fmt.Fprintf(w, "  %-12s %s: %s\n", f.system, f.path, explain(f.message, f.path))
	}
}

// structuredFinding is one finding's shape in Encode's json/yaml/toml/csv
// output. Fix is the bare subcommand name (e.g. "rename"), empty when the
// finding has no command-backed fix - the same knownFixes test Add/Fixes
// already use to tell a real fix from an explanatory message.
type structuredFinding struct {
	Level    string `json:"level" yaml:"level" toml:"level"`
	Category string `json:"category" yaml:"category" toml:"category"`
	System   string `json:"system" yaml:"system" toml:"system"`
	Path     string `json:"path" yaml:"path" toml:"path"`
	Message  string `json:"message" yaml:"message" toml:"message"`
	Fix      string `json:"fix,omitempty" yaml:"fix,omitempty" toml:"fix,omitempty"`
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
// text output. Unlike the text report, this has no Checklist equivalent
// (which category ran isn't a "finding"); a category that found nothing
// simply contributes no findings. CSV carries the findings table alone,
// with no separate summary row - a consumer can get the same tally by
// counting rows per level.
func (r *Report) Encode(w io.Writer, format cliutil.Format) error {
	r.mu.Lock()
	findings := append([]finding(nil), r.findings...)
	fails, warns, info := r.fails, r.warns, r.info
	r.mu.Unlock()

	if format == cliutil.FormatCSV {
		header := []string{"level", "category", "system", "path", "message", "fix"}
		rows := make([][]string, 0, len(findings))
		for _, f := range findings {
			fix := ""
			if knownFixes[f.fx] {
				fix = f.fx
			}
			rows = append(rows, []string{string(f.level), string(f.category), f.system, f.path, f.message, fix})
		}
		return cliutil.EncodeCSV(w, header, rows)
	}

	out := structuredReport{Findings: make([]structuredFinding, 0, len(findings))}
	for _, f := range findings {
		sf := structuredFinding{Level: string(f.level), Category: string(f.category), System: f.system, Path: f.path, Message: f.message}
		if knownFixes[f.fx] {
			sf.Fix = f.fx
		}
		out.Findings = append(out.Findings, sf)
	}
	out.Summary = structuredSummary{Fail: fails, Warn: warns, Info: info}
	return cliutil.Encode(w, format, out)
}

// Summary prints the one-line tally, colorized (and with unicode symbols)
// in Fancy format, honoring NO_COLOR.
func (r *Report) Summary(w io.Writer) {
	colors := cliutil.ColorsFor(r.format)
	failSym, warnSym, okSym := symbols(r.format)
	fmt.Fprintln(w)
	switch {
	case r.fails > 0:
		fmt.Fprintf(w, "%s%s %d fail%s, %d warn, %d info\n", colors.Red, failSym, r.fails, colors.Reset, r.warns, r.info)
	case r.warns > 0:
		fmt.Fprintf(w, "%s%s %d warn%s, %d info\n", colors.Yellow, warnSym, r.warns, colors.Reset, r.info)
	case r.info > 0:
		fmt.Fprintf(w, "%s%s Clean%s (%d info)\n", colors.Green, okSym, colors.Reset, r.info)
	default:
		fmt.Fprintf(w, "%s%s Clean%s\n", colors.Green, okSym, colors.Reset)
	}
}
