package commands

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/demicloud/romctl/internal/actions"
	"github.com/demicloud/romctl/internal/library"
)

// captureStdout runs fn with os.Stdout redirected to a pipe and returns
// everything written to it - Check/Bios/List write straight to os.Stdout,
// so there's no writer to pass in instead.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	orig := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = orig }()

	fn()

	w.Close()
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

// scratchLibrary builds a minimal on-disk library covering a finding in
// each category the naming/structure/containers split (see check.go) now
// handles separately, plus a missing-BIOS finding: psx (class cd) gets a
// tagged filename (naming), a leftover extraction dir (structure), and a
// raw .cue disc image (containers) with no bios/ directory present at all
// (bios, via SkipBios rather than a missing-file Fail - simplest to set up
// and still exercises the bios phase).
func scratchLibrary(t *testing.T) *library.Library {
	t.Helper()
	root := t.TempDir()
	psxDir := filepath.Join(root, "roms", "psx")
	if err := os.MkdirAll(psxDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(psxDir, "Game (USA) [!].cue"), []byte("cue"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(psxDir, "_extract.leftover"), 0o755); err != nil {
		t.Fatal(err)
	}
	return library.Open(root)
}

func TestCheckNonVerboseOmitsFindingLines(t *testing.T) {
	lib := scratchLibrary(t)
	out := captureStdout(t, func() {
		Check(lib, []string{"psx"}, false, actions.SchemeNoTag, false, "")
	})
	if strings.Contains(out, "naming ->") {
		t.Errorf("non-verbose output should not include per-finding lines, got:\n%s", out)
	}
	if !strings.Contains(out, "Checks:") || !strings.Contains(out, "Fixes:") {
		t.Errorf("expected Checklist and Fixes sections, got:\n%s", out)
	}
}

func TestCheckVerboseIncludesFindingLines(t *testing.T) {
	lib := scratchLibrary(t)
	out := captureStdout(t, func() {
		Check(lib, []string{"psx"}, false, actions.SchemeNoTag, true, "")
	})
	if !strings.Contains(out, "naming ->") {
		t.Errorf("verbose output should include per-finding naming lines, got:\n%s", out)
	}
	if !strings.Contains(out, "leftover extraction directory") {
		t.Errorf("verbose output should include the structure finding, got:\n%s", out)
	}
	if !strings.Contains(out, "raw/archived disc image") {
		t.Errorf("verbose output should include the containers finding, got:\n%s", out)
	}
}

func TestCheckFixesNamesTheRootFlagWhenExplicit(t *testing.T) {
	lib := scratchLibrary(t)
	out := captureStdout(t, func() {
		Check(lib, []string{"psx"}, false, actions.SchemeNoTag, false, "-r "+lib.Root)
	})
	if !strings.Contains(out, "-r "+lib.Root) {
		t.Errorf("expected the Fixes section to include the explicit -r hint, got:\n%s", out)
	}
}

func TestCheckCategorySplitStillFindsEveryKind(t *testing.T) {
	// Regression check for the naming/structure/containers split: every
	// category that used to come out of one combined pass must still
	// report its finding.
	lib := scratchLibrary(t)
	out := captureStdout(t, func() {
		Check(lib, []string{"psx"}, false, actions.SchemeNoTag, true, "")
	})
	for _, want := range []string{
		"naming ->",                     // naming
		"leftover extraction directory", // structure
		"raw/archived disc image",       // containers
	} {
		if !strings.Contains(out, want) {
			t.Errorf("expected output to contain %q, got:\n%s", want, out)
		}
	}
}
