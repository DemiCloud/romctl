package audit

import (
	"bytes"
	"strings"
	"testing"
)

func TestAddVerboseGating(t *testing.T) {
	var buf bytes.Buffer
	r := New(&buf, false, "")
	r.Add(Warn, CategoryNaming, "psx", "Game.chd", "naming -> \"Game\"", "rename")
	if buf.Len() != 0 {
		t.Errorf("non-verbose Add() printed a line, want nothing buffered: %q", buf.String())
	}

	buf.Reset()
	rv := New(&buf, true, "")
	rv.Add(Warn, CategoryNaming, "psx", "Game.chd", "naming -> \"Game\"", "rename")
	if buf.Len() == 0 {
		t.Error("verbose Add() printed nothing, want a line")
	}
}

func TestAddCommandHintIncludesRootHint(t *testing.T) {
	var buf bytes.Buffer
	r := New(&buf, true, "-r /scratch")
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
	r := New(&buf, true, "")
	r.Add(Fail, CategoryBios, "bios", "scph5501.bin", "required BIOS missing", "not-a-real-command")
	if strings.Contains(buf.String(), "[") {
		t.Errorf("an unrecognized fix should not be rendered as a command hint, got: %q", buf.String())
	}
}

func TestFixesCollapsesCommandBackedFindings(t *testing.T) {
	var buf bytes.Buffer
	r := New(&buf, false, "")
	r.Add(Warn, CategoryNaming, "psx", "A.chd", "naming -> \"A\"", "rename")
	r.Add(Warn, CategoryNaming, "snes", "B.sfc", "naming -> \"B\"", "rename")
	r.Add(Warn, CategoryContainers, "psx", "C.cue", "raw/archived disc image; convert to CHD", "chd")

	var out bytes.Buffer
	r.Fixes(&out)
	got := out.String()

	if !strings.Contains(got, "2 finding(s)") || !strings.Contains(got, "rename all") {
		t.Errorf("expected one collapsed line for the 2 rename findings, got:\n%s", got)
	}
	if !strings.Contains(got, "1 finding(s)") || !strings.Contains(got, "chd all") {
		t.Errorf("expected one collapsed line for the 1 chd finding, got:\n%s", got)
	}
	if strings.Contains(got, "A.chd") || strings.Contains(got, "C.cue") {
		t.Errorf("command-backed findings must not be listed individually, got:\n%s", got)
	}
}

func TestFixesListsNoCommandFindingsIndividuallyWithTheRealFile(t *testing.T) {
	var buf bytes.Buffer
	r := New(&buf, false, "")
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

func TestFixesFallsBackToRawMessageWhenNoExplanationMatches(t *testing.T) {
	var buf bytes.Buffer
	r := New(&buf, false, "")
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
	r := New(&buf, false, "")
	var out bytes.Buffer
	r.Fixes(&out)
	if out.Len() != 0 {
		t.Errorf("expected no Fixes output for a clean report, got: %q", out.String())
	}
}

func TestChecklistAndSummaryStillWork(t *testing.T) {
	var buf bytes.Buffer
	r := New(&buf, false, "")
	r.Add(Fail, CategoryContainers, "psx", "Game.chd", "not a valid CHD", "")
	if !r.Failed() {
		t.Error("Failed() should be true after a Fail-level Add")
	}
}
