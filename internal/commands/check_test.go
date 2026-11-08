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

// TestCheckFlagsMisfiledCartridgeROM is a regression test for the real
// support report that prompted internal/formats/signature: a Genesis
// cartridge ROM (bare .bin, no .cue/.gdi/.iso alongside it) sitting in
// roms/segacd/ should be flagged with a "relocate" fix, not silently
// treated as just another disc-image candidate.
func TestCheckFlagsMisfiledCartridgeROM(t *testing.T) {
	lib := scratchLibrary(t)
	segaDir := filepath.Join(lib.Roms, "segacd")
	if err := os.MkdirAll(segaDir, 0o755); err != nil {
		t.Fatal(err)
	}
	data := make([]byte, 0x200)
	copy(data[0x100:], "SEGA MEGA DRIVE ")
	if err := os.WriteFile(filepath.Join(segaDir, "ALTERED_.bin"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	out := captureStdout(t, func() {
		Check(lib, []string{"psx", "segacd"}, false, actions.SchemeNoTag, true, "")
	})
	if !strings.Contains(out, "identifies as a Genesis/Mega Drive ROM") {
		t.Errorf("expected the misfiled-ROM finding, got:\n%s", out)
	}
	if !strings.Contains(out, "relocate all") {
		t.Errorf("expected the Fixes section to suggest relocate, got:\n%s", out)
	}
}

// TestCheckDoesNotFlagSegaCDBootSector guards the false positive
// signature.Identify exists to avoid: a legitimately-dumped Sega CD
// game's boot track reuses the Genesis cartridge header layout and must
// not be reported as misfiled.
func TestCheckDoesNotFlagSegaCDBootSector(t *testing.T) {
	lib := scratchLibrary(t)
	segaDir := filepath.Join(lib.Roms, "segacd")
	if err := os.MkdirAll(segaDir, 0o755); err != nil {
		t.Fatal(err)
	}
	data := make([]byte, 0x200)
	copy(data[0x100:], "SEGA")
	copy(data[0x180:], "BR")
	if err := os.WriteFile(filepath.Join(segaDir, "Game (Track 1).bin"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	out := captureStdout(t, func() {
		Check(lib, []string{"segacd"}, false, actions.SchemeNoTag, true, "")
	})
	if strings.Contains(out, "identifies as a") {
		t.Errorf("Sega CD boot sector should not be flagged as misfiled, got:\n%s", out)
	}
}

// TestCheckFlagsUnknownSystemDirectory guards the systems.txt cross-check:
// a roms/<dir> that neither systemdb nor the install's own ES-DE-generated
// systems.txt recognizes should be flagged - but only once systems.txt is
// actually present, since systemdb's hand-ported list can't make that
// call reliably on its own.
func TestCheckFlagsUnknownSystemDirectory(t *testing.T) {
	lib := scratchLibrary(t)
	bogusDir := filepath.Join(lib.Roms, "totally-bogus-system")
	if err := os.MkdirAll(bogusDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Without systems.txt, systemdb alone can't tell a typo apart from a
	// legitimate system it just doesn't know about yet - no finding.
	out := captureStdout(t, func() {
		Check(lib, []string{"psx", "totally-bogus-system"}, false, actions.SchemeNoTag, true, "")
	})
	if strings.Contains(out, "not a system directory") {
		t.Errorf("should not flag an unknown directory without systems.txt, got:\n%s", out)
	}

	if err := os.WriteFile(filepath.Join(lib.Root, "systems.txt"), []byte("psx: Sony PlayStation\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out = captureStdout(t, func() {
		Check(lib, []string{"psx", "totally-bogus-system"}, false, actions.SchemeNoTag, true, "")
	})
	if !strings.Contains(out, "not a system directory") {
		t.Errorf("expected the unknown-directory finding once systems.txt is present, got:\n%s", out)
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
