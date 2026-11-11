package commands

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/demicloud/romctl/internal/cliutil"
	"github.com/demicloud/romctl/internal/library"
)

func TestDiscsReportsNothingForAnOrdinaryLibrary(t *testing.T) {
	root := t.TempDir()
	psxDir := filepath.Join(root, "roms", "psx")
	if err := os.MkdirAll(psxDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(psxDir, "Ordinary Game (USA).chd"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	lib := library.Open(root)

	out := captureStdout(t, func() {
		ok, err := Discs(lib, []string{"psx"}, cliutil.FormatPlain)
		if err != nil || !ok {
			t.Fatalf("Discs() = (%v, %v), want (true, nil)", ok, err)
		}
	})
	if out != "" {
		t.Errorf("expected no output for a library with nothing to report, got:\n%s", out)
	}
}

// TestDiscsIgnoresAUnconfirmedLoneDisc guards the "no DAT match" path:
// a lone disc-tagged file whose content doesn't hash-match anything in
// the DAT (or whose system has no DAT at all) is left unconfirmed, not
// reported as an incomplete set - discs only ever reports a genuinely
// confirmed verdict (see checkIncompleteDiscSet).
func TestDiscsIgnoresAnUnconfirmedLoneDisc(t *testing.T) {
	root := t.TempDir()
	sysDir := filepath.Join(root, "roms", "faketestsystem123")
	if err := os.MkdirAll(sysDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sysDir, "Lone Game (USA) (Disc 1).bin"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	lib := library.Open(root)

	out := captureStdout(t, func() {
		ok, err := Discs(lib, []string{"faketestsystem123"}, cliutil.FormatPlain)
		if err != nil || !ok {
			t.Fatalf("Discs() = (%v, %v), want (true, nil)", ok, err)
		}
	})
	if out != "" {
		t.Errorf("expected no output for an unconfirmable lone disc (no DAT for this system), got:\n%s", out)
	}
}

func TestDiscsSkipsArcadeSystems(t *testing.T) {
	root := t.TempDir()
	arcadeDir := filepath.Join(root, "roms", "fbneo")
	if err := os.MkdirAll(arcadeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// An arcade set name that happens to contain "(Disc 1)" would be
	// nonsensical, but prove the class check skips the directory outright
	// regardless, rather than relying on arcade filenames never matching.
	if err := os.WriteFile(filepath.Join(arcadeDir, "Some Set (Disc 1).zip"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	lib := library.Open(root)

	out := captureStdout(t, func() {
		ok, err := Discs(lib, []string{"fbneo"}, cliutil.FormatPlain)
		if err != nil || !ok {
			t.Fatalf("Discs() = (%v, %v), want (true, nil)", ok, err)
		}
	})
	if out != "" {
		t.Errorf("expected arcade systems to be skipped entirely, got:\n%s", out)
	}
}
