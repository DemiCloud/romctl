package commands

import (
	"strings"
	"testing"
)

// TestLoadDatReportsUnstagedAsRow guards the common case (no DAT snapshot
// committed yet - see internal/datdb's doc comment): a system with a real
// systemdb.Dat mapping but nothing actually staged must come back as a
// row to collect, not an immediate print, so callers can batch every
// system's issue into one small table instead of a line each. Uses an
// empty --dat-dir override (t.TempDir()) rather than "" so this doesn't
// depend on whether this build happens to have psx's real DAT embedded.
func TestLoadDatReportsUnstagedAsRow(t *testing.T) {
	df, ok, row := loadDat("psx", t.TempDir())
	if !ok {
		t.Fatal("psx has a systemdb.Dat mapping, loadDat should report ok")
	}
	if df != nil {
		t.Error("an unstaged DAT must not return a Datafile")
	}
	if row == nil {
		t.Fatal("expected a datLoadRow describing why psx's DAT isn't usable")
	}
	if row.System != "psx" {
		t.Errorf("row.System = %q, want psx", row.System)
	}
	if row.Issue != "no DAT staged" {
		t.Errorf("row.Issue = %q, want the plain not-staged message, not the raw open() error", row.Issue)
	}
}

// TestLoadDatSkipsUnmappedSystem guards systems with no DatSource entry
// at all (arcade, a few niches): nothing to report, same as datdb.Load's
// own ok=false.
func TestLoadDatSkipsUnmappedSystem(t *testing.T) {
	_, ok, row := loadDat("arcade", "")
	if ok {
		t.Error("arcade has no systemdb.Dat mapping, loadDat should report ok=false")
	}
	if row != nil {
		t.Error("an unmapped system should never produce a row")
	}
}

// TestPrintDatLoadIssuesTable guards the table shape itself: one row per
// system, plus a header count, both readable without ANSI color codes
// (printDatLoadIssues always renders Plain to stderr, regardless of the
// command's own --format).
func TestPrintDatLoadIssuesTable(t *testing.T) {
	out := captureStderr(t, func() {
		printDatLoadIssues("identify", []datLoadRow{
			{System: "psx", Issue: "no DAT staged"},
			{System: "ps2", Issue: "no DAT staged"},
		})
	})
	if !strings.Contains(out, "2 system(s)") {
		t.Errorf("expected a count of affected systems, got:\n%s", out)
	}
	if !strings.Contains(out, "psx") || !strings.Contains(out, "ps2") {
		t.Errorf("expected both systems listed, got:\n%s", out)
	}
}

func TestPrintDatLoadIssuesNoop(t *testing.T) {
	out := captureStderr(t, func() {
		printDatLoadIssues("identify", nil)
	})
	if out != "" {
		t.Errorf("expected no output for an empty issue list, got:\n%s", out)
	}
}
