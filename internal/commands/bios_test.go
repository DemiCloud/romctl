package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/demicloud/romctl/internal/cliutil"
	"github.com/demicloud/romctl/internal/library"
)

// TestBiosLabelUsesPerFileLabelForMultiGroupRequiredSystems is a
// regression test: xbox is the one required system with more than one
// independent required file (three single-alternative groups, not one
// group of three alternatives like everything else), and biosLabel used
// to only try the per-alternative lookup key for optional entries, so
// all three xbox rows fell back to the generic "xbox BIOS" instead of
// their specific systemdb.BiosLabel entries.
func TestBiosLabelUsesPerFileLabelForMultiGroupRequiredSystems(t *testing.T) {
	for _, e := range biosCatalog([]string{"xbox"}) {
		label := biosLabel(e)
		if label == "xbox BIOS" {
			t.Errorf("biosLabel(%v) = %q, want a specific per-file label, not the generic fallback", e.alternatives, label)
		}
	}
}

// TestSplitLabelSeparatesNameFromCaveat guards the Name/Label split
// biosRows relies on: a label with a trailing " (...)" splits into a
// short name and the caveat, one without passes through unchanged.
func TestSplitLabelSeparatesNameFromCaveat(t *testing.T) {
	name, note := splitLabel("DS firmware (per-console MAC address; size-checked, not hashed)")
	if name != "DS firmware" {
		t.Errorf("name = %q, want %q", name, "DS firmware")
	}
	if note != "per-console MAC address; size-checked, not hashed" {
		t.Errorf("note = %q, want the caveat text", note)
	}

	name, note = splitLabel("Dreamcast boot ROM")
	if name != "Dreamcast boot ROM" || note != "" {
		t.Errorf("splitLabel with no caveat = (%q, %q), want (%q, \"\")", name, note, "Dreamcast boot ROM")
	}
}

// TestBiosInventoryTableStaysTerseWhileJSONKeepsFullDetail is the
// end-to-end guard for the table/CSV-vs-structured split: the rendered
// table/CSV must never contain a raw hash or a label's parenthetical
// caveat, but --format=json must still carry both, for a script or a
// closer look.
func TestBiosInventoryTableStaysTerseWhileJSONKeepsFullDetail(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "roms", "nds"), 0o755); err != nil {
		t.Fatal(err)
	}
	biosDir := filepath.Join(root, "bios")
	if err := os.MkdirAll(biosDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(biosDir, "firmware.bin"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	lib := library.Open(root)

	plain := captureStdout(t, func() {
		if _, err := Bios(lib, []string{"nds"}, true, cliutil.FormatPlain); err != nil {
			t.Fatal(err)
		}
	})
	if strings.Contains(plain, "per-console MAC address") {
		t.Errorf("table output should not include the full caveat, got:\n%s", plain)
	}
	if !strings.Contains(plain, "DS firmware") {
		t.Errorf("table output should still include the short name, got:\n%s", plain)
	}

	jsonOut := captureStdout(t, func() {
		if _, err := Bios(lib, []string{"nds"}, true, cliutil.FormatJSON); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(jsonOut, "per-console MAC address") {
		t.Errorf("json output should include the full label detail, got:\n%s", jsonOut)
	}
	if !strings.Contains(jsonOut, `"name": "DS firmware"`) {
		t.Errorf("json output should also expose the short name field, not just the full label, got:\n%s", jsonOut)
	}
}
