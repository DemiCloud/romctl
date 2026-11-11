package audit

import (
	"bytes"
	"strings"
	"testing"

	"github.com/demicloud/romctl/internal/cliutil"
)

// rowContains reports whether some line of a rendered table contains
// every one of want, in order - used instead of a single Contains so a
// test can assert two values (e.g. a label and its Count) landed in the
// same row, without depending on exact column widths/padding.
func rowContains(text string, want ...string) bool {
	for _, line := range strings.Split(text, "\n") {
		ok := true
		pos := 0
		for _, w := range want {
			i := strings.Index(line[pos:], w)
			if i < 0 {
				ok = false
				break
			}
			pos += i + len(w)
		}
		if ok {
			return true
		}
	}
	return false
}

func TestAddVerboseGating(t *testing.T) {
	var buf bytes.Buffer
	r := New(&buf, false, cliutil.FormatPlain, "")
	r.Add(Warn, CategoryNaming, "psx", "Game.chd", "naming -> \"Game\"", "rename")
	if buf.Len() != 0 {
		t.Errorf("non-verbose Add() printed a line, want nothing buffered: %q", buf.String())
	}

	buf.Reset()
	rv := New(&buf, true, cliutil.FormatPlain, "")
	rv.Add(Warn, CategoryNaming, "psx", "Game.chd", "naming -> \"Game\"", "rename")
	if buf.Len() == 0 {
		t.Error("verbose Add() printed nothing, want a line")
	}
}

func TestAddCommandHintIncludesRootHint(t *testing.T) {
	var buf bytes.Buffer
	r := New(&buf, true, cliutil.FormatPlain, "-r /scratch")
	r.Add(Warn, CategoryNaming, "psx", "Game.chd", "naming -> \"Game\"", "rename")
	out := buf.String()
	if !strings.Contains(out, "-r /scratch") {
		t.Errorf("expected the per-line hint to include -r /scratch, got: %q", out)
	}
	if !strings.Contains(out, "rename psx") {
		t.Errorf("expected the per-line hint to end with the fix command and system, got: %q", out)
	}
}

func TestAddUnknownFixTreatedAsNoCommand(t *testing.T) {
	var buf bytes.Buffer
	r := New(&buf, true, cliutil.FormatPlain, "")
	r.Add(Fail, CategoryBios, "bios", "scph5501.bin", "required BIOS missing", "not-a-real-command")
	if strings.Contains(buf.String(), "[") {
		t.Errorf("an unrecognized fix should not be rendered as a command hint, got: %q", buf.String())
	}
}

// TestFixesAutoFixColumnReflectsWhetherRomctlCanActuallyFixIt guards the
// Auto-Fix column: a knownFixes-backed finding (romctl has a real
// command for it) reports "yes", while a command-less one (missing
// BIOS/ROM content romctl can never acquire on its own - see AGENTS.md's
// Scope boundaries) reports "no", regardless of what instructional text
// its Fix column shows.
func TestFixesAutoFixColumnReflectsWhetherRomctlCanActuallyFixIt(t *testing.T) {
	var buf bytes.Buffer
	r := New(&buf, false, cliutil.FormatPlain, "")
	r.Add(Warn, CategoryContainers, "psx", "C.cue", "raw/archived disc image; convert to CHD", "chd")
	r.Add(Fail, CategoryBios, "bios", "scph5501.bin", "required BIOS missing", "")

	var out bytes.Buffer
	r.Fixes(&out)
	got := out.String()

	if !rowContains(got, "Convert to CHD", "yes") {
		t.Errorf("expected the command-backed chd row to report Auto-Fix yes, got:\n%s", got)
	}
	if !rowContains(got, "Missing BIOS file", "no") {
		t.Errorf("expected the command-less BIOS row to report Auto-Fix no, got:\n%s", got)
	}
}

// TestFixesPointsIncompleteDiscSetAtDiscsCommandScopedToItsSystem guards
// labelDetailCommands: an "incomplete disc set" finding must point at
// commands.Discs (fast, focused) rather than the generic, unscoped
// "check -v" (every category, every system) detailCommand would
// otherwise fall back to - and name the one affected system rather than
// "all" when there's only one.
func TestFixesPointsIncompleteDiscSetAtDiscsCommandScopedToItsSystem(t *testing.T) {
	var buf bytes.Buffer
	r := New(&buf, false, cliutil.FormatPlain, "")
	r.Add(Warn, CategoryStructure, "psx", "Game A (Disc 1).bin",
		"incomplete disc set (Game A): have disc 1 of 2, missing 2", "")
	r.Add(Warn, CategoryStructure, "psx", "Game B (Disc 1).bin",
		"incomplete disc set (Game B): have disc 1 of 2, missing 2", "")

	var out bytes.Buffer
	r.Fixes(&out)
	got := out.String()

	if !rowContains(got, "Incomplete disc set", "psx") || !strings.Contains(got, "discs psx") {
		t.Errorf("expected the collapsed row to name psx and point at discs psx, got:\n%s", got)
	}
	if strings.Contains(got, "check -v") {
		t.Errorf("expected discs, not the generic check -v, got:\n%s", got)
	}
}

func TestFixesCollapsesCommandBackedFindings(t *testing.T) {
	var buf bytes.Buffer
	r := New(&buf, false, cliutil.FormatPlain, "")
	r.Add(Warn, CategoryNaming, "psx", "A.chd", "naming -> \"A\"", "rename")
	r.Add(Warn, CategoryNaming, "snes", "B.sfc", "naming -> \"B\"", "rename")
	r.Add(Warn, CategoryContainers, "psx", "C.cue", "raw/archived disc image; convert to CHD", "chd")

	var out bytes.Buffer
	r.Fixes(&out)
	got := out.String()

	if !rowContains(got, "Fix naming", "2") || !strings.Contains(got, "rename all") {
		t.Errorf("expected one collapsed row for the 2 rename findings (spanning 2 systems), with Count 2 in its own column, got:\n%s", got)
	}
	if !strings.Contains(got, "chd psx") {
		t.Errorf("expected one row for the 1 chd finding naming its single system, got:\n%s", got)
	}
	if strings.Contains(got, "A.chd") || strings.Contains(got, "C.cue") {
		t.Errorf("command-backed findings must not be listed individually, got:\n%s", got)
	}
}

// TestFixesCollapseBiosGroupsUnderTheCategory guards CollapseBios (what
// Check's own Report enables): once on, BIOS findings always collapse
// under "BIOS" (never a specific console) pointing at `bios -v`, since
// that dedicated command - not check's own Fixes table - is the right
// place to enumerate BIOS files one by one, however many there are.
func TestFixesCollapseBiosGroupsUnderTheCategory(t *testing.T) {
	var buf bytes.Buffer
	r := New(&buf, false, cliutil.FormatPlain, "")
	r.CollapseBios()
	r.Add(Fail, CategoryBios, "psx", "bios/scph5501.bin|scph5502.bin", "required BIOS missing", "")
	r.Add(Fail, CategoryBios, "bios", "dc_boot.bin", "MD5 ab12 does not match reference cd34", "")

	var out bytes.Buffer
	r.Fixes(&out)
	got := out.String()

	if strings.Contains(got, "scph5501") || strings.Contains(got, "dc_boot.bin") {
		t.Errorf("BIOS findings should collapse to the category, not name individual files, got:\n%s", got)
	}
	if !strings.Contains(got, "Missing BIOS file") || !strings.Contains(got, "Bad BIOS hash") {
		t.Errorf("expected one row per distinct BIOS issue label, got:\n%s", got)
	}
	if !strings.Contains(got, "BIOS") {
		t.Errorf("expected the System column to read BIOS, got:\n%s", got)
	}
	if !strings.Contains(got, "bios -v") {
		t.Errorf("expected the Fix column to point at the dedicated bios -v command, got:\n%s", got)
	}
}

// TestFixesListsBiosFindingsIndividuallyWithoutCollapseBios guards the
// default (what Bios's own Report uses, by never calling Collapse): it IS
// the dedicated BIOS detail view, so every finding is still listed with
// its own specific file and instruction, the same as before.
func TestFixesListsBiosFindingsIndividuallyWithoutCollapseBios(t *testing.T) {
	var buf bytes.Buffer
	r := New(&buf, false, cliutil.FormatPlain, "")
	r.Add(Fail, CategoryBios, "psx", "bios/scph5501.bin|scph5502.bin", "required BIOS missing", "")
	r.Add(Fail, CategoryBios, "bios", "dc_boot.bin", "MD5 ab12 does not match reference cd34", "")

	var out bytes.Buffer
	r.Fixes(&out)
	got := out.String()

	if !strings.Contains(got, "bios/scph5501.bin|scph5502.bin") {
		t.Errorf("expected the specific missing BIOS path to appear, got:\n%s", got)
	}
	if !strings.Contains(got, "acquire bios/scph5501.bin|scph5502.bin") {
		t.Errorf("expected an actionable acquire phrase naming the real path, got:\n%s", got)
	}
	if !strings.Contains(got, "dc_boot.bin") || !strings.Contains(got, "re-acquire dc_boot.bin") {
		t.Errorf("expected the MD5 mismatch line to name dc_boot.bin specifically, got:\n%s", got)
	}
}

// TestFixesKeepsSpecificsForALoneNonBiosFinding guards the non-BIOS case:
// with only one finding under a label, and no dedicated command to
// redirect to, the row should still name the real system and the
// specific instruction rather than a generic pointer.
func TestFixesKeepsSpecificsForALoneNonBiosFinding(t *testing.T) {
	var buf bytes.Buffer
	r := New(&buf, false, cliutil.FormatPlain, "")
	r.Add(Fail, CategoryContainers, "ps3", "SomeGame", "no EBOOT.BIN found", "")

	var out bytes.Buffer
	r.Fixes(&out)
	got := out.String()
	if !strings.Contains(got, "ps3") {
		t.Errorf("expected the real system to appear, got:\n%s", got)
	}
	if !strings.Contains(got, "SomeGame") {
		t.Errorf("expected the specific instruction naming SomeGame, got:\n%s", got)
	}
}

// TestFixesCollapsesManyNonBiosFindingsUnderOneLabel guards the other
// half of the same rule: once a non-BIOS label has more than one
// finding, it collapses too (pointing at check -v for the specifics),
// the same way BIOS always does.
func TestFixesCollapsesManyNonBiosFindingsUnderOneLabel(t *testing.T) {
	var buf bytes.Buffer
	r := New(&buf, false, cliutil.FormatPlain, "")
	r.Add(Fail, CategoryContainers, "ps3", "GameA", "no EBOOT.BIN found", "")
	r.Add(Fail, CategoryContainers, "ps3", "GameB", "no EBOOT.BIN found", "")

	var out bytes.Buffer
	r.Fixes(&out)
	got := out.String()
	if strings.Contains(got, "GameA") || strings.Contains(got, "GameB") {
		t.Errorf("collapsed row should not name individual files, got:\n%s", got)
	}
	if !rowContains(got, "No EBOOT.BIN", "2") {
		t.Errorf("expected the collapsed label with Count 2 in its own column, got:\n%s", got)
	}
	if !strings.Contains(got, "check -v") {
		t.Errorf("expected the Fix column to point at check -v, got:\n%s", got)
	}
}

// TestFixesDoesNotMisreportIncompleteDiscSetAsLeftoverFile guards an
// explanations-ordering bug: an "incomplete disc set" finding's own
// message contains the word "incomplete", the same substring the
// generic "Incomplete leftover file" entry matches on. Without the more
// specific entry checked first, every incomplete-disc-set finding would
// get mislabeled as a stray leftover file, with the wrong fix text.
func TestFixesDoesNotMisreportIncompleteDiscSetAsLeftoverFile(t *testing.T) {
	var buf bytes.Buffer
	r := New(&buf, false, cliutil.FormatPlain, "")
	r.Add(Warn, CategoryStructure, "psx", "Game (Disc 1).bin",
		"incomplete disc set (Game): have disc 1 of 2, missing 2", "")

	var out bytes.Buffer
	r.Fixes(&out)
	got := out.String()
	if !rowContains(got, "Incomplete disc set") {
		t.Errorf("expected the specific \"Incomplete disc set\" label, got:\n%s", got)
	}
	if strings.Contains(got, "Incomplete leftover file") || strings.Contains(got, "left over from an interrupted run") {
		t.Errorf("incomplete disc set finding was misreported as a leftover file, got:\n%s", got)
	}
}

func TestFixesFallsBackToRawMessageWhenNoExplanationMatches(t *testing.T) {
	var buf bytes.Buffer
	r := New(&buf, false, cliutil.FormatPlain, "")
	r.Add(Warn, CategoryContainers, "ps3", "SomeGame", "a completely novel finding text", "")

	var out bytes.Buffer
	r.Fixes(&out)
	got := out.String()
	if !strings.Contains(got, "a completely novel finding text") {
		t.Errorf("expected the raw message as a fallback, got:\n%s", got)
	}
}

func TestFixesEmptyWhenNothingFound(t *testing.T) {
	var buf bytes.Buffer
	r := New(&buf, false, cliutil.FormatPlain, "")
	var out bytes.Buffer
	r.Fixes(&out)
	if out.Len() != 0 {
		t.Errorf("expected no Fixes output for a clean report, got: %q", out.String())
	}
}

func TestFailedAfterFailLevelAdd(t *testing.T) {
	var buf bytes.Buffer
	r := New(&buf, false, cliutil.FormatPlain, "")
	r.Add(Fail, CategoryContainers, "psx", "Game.chd", "not a valid CHD", "")
	if !r.Failed() {
		t.Error("Failed() should be true after a Fail-level Add")
	}
}
