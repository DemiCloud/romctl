package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/demicloud/romctl/internal/actions"
	"github.com/demicloud/romctl/internal/library"
)

func cullLibrary(t *testing.T, sys string) *library.Library {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "roms", sys), 0o755); err != nil {
		t.Fatal(err)
	}
	return library.Open(root)
}

func TestCullRetiresLeftoverExtractionDir(t *testing.T) {
	lib := cullLibrary(t, "psx")
	leftover := filepath.Join(lib.Roms, "psx", "_extract.abc123")
	if err := os.MkdirAll(leftover, 0o755); err != nil {
		t.Fatal(err)
	}

	ok, err := Cull(lib, &actions.Runner{}, []string{"psx"})
	if err != nil || !ok {
		t.Fatalf("Cull() = (%v, %v), want (true, nil)", ok, err)
	}
	if exists(leftover) {
		t.Error("leftover extraction dir should have been retired")
	}
	if !exists(filepath.Join(lib.Roms, "psx", "_to_delete", "_extract.abc123")) {
		t.Error("expected the leftover dir in _to_delete/")
	}
}

func TestCullRetiresIncompleteTransferLeftover(t *testing.T) {
	lib := cullLibrary(t, "psx")
	leftover := filepath.Join(lib.Roms, "psx", "Game.chd.tmp.xyz")
	if err := os.WriteFile(leftover, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	ok, err := Cull(lib, &actions.Runner{}, []string{"psx"})
	if err != nil || !ok {
		t.Fatalf("Cull() = (%v, %v), want (true, nil)", ok, err)
	}
	if exists(leftover) {
		t.Error("incomplete-transfer leftover should have been retired")
	}
}

func TestCullRetiresCueLessBin(t *testing.T) {
	lib := cullLibrary(t, "psx")
	bin := filepath.Join(lib.Roms, "psx", "Lone Game (USA).bin")
	if err := os.WriteFile(bin, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	ok, err := Cull(lib, &actions.Runner{}, []string{"psx"})
	if err != nil || !ok {
		t.Fatalf("Cull() = (%v, %v), want (true, nil)", ok, err)
	}
	if exists(bin) {
		t.Error("cue-less bin should have been retired")
	}
	if !exists(filepath.Join(lib.Roms, "psx", "_to_delete", "Lone Game (USA).bin")) {
		t.Error("expected the bin in _to_delete/")
	}
}

func TestCullRetiresCorruptCHD(t *testing.T) {
	lib := cullLibrary(t, "psx")
	chd := filepath.Join(lib.Roms, "psx", "Broken Game.chd")
	if err := os.WriteFile(chd, []byte("not a chd at all"), 0o644); err != nil {
		t.Fatal(err)
	}

	ok, err := Cull(lib, &actions.Runner{}, []string{"psx"})
	if err != nil || !ok {
		t.Fatalf("Cull() = (%v, %v), want (true, nil)", ok, err)
	}
	if exists(chd) {
		t.Error("corrupt CHD should have been retired")
	}
}

// TestCullLeavesPairedBinAlone guards against a false positive: a .bin
// with a real matching .cue (even one referencing it by a different
// filename, same as check's own loadCueReferences) must never be
// retired - it's chd's job to convert it, not cull's to discard it.
func TestCullLeavesPairedBinAlone(t *testing.T) {
	lib := cullLibrary(t, "psx")
	psxDir := filepath.Join(lib.Roms, "psx")
	bin := filepath.Join(psxDir, "Paired Game.bin")
	if err := os.WriteFile(bin, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	cue := "FILE \"Paired Game.bin\" BINARY\n  TRACK 01 MODE1/2048\n    INDEX 01 00:00:00\n"
	if err := os.WriteFile(filepath.Join(psxDir, "Paired Game.cue"), []byte(cue), 0o644); err != nil {
		t.Fatal(err)
	}

	ok, err := Cull(lib, &actions.Runner{}, []string{"psx"})
	if err != nil || !ok {
		t.Fatalf("Cull() = (%v, %v), want (true, nil)", ok, err)
	}
	if !exists(bin) {
		t.Error("a bin paired with a real cue sheet should never be retired")
	}
}

func TestCullDryRunLeavesEverythingInPlace(t *testing.T) {
	lib := cullLibrary(t, "psx")
	bin := filepath.Join(lib.Roms, "psx", "Lone Game (USA).bin")
	if err := os.WriteFile(bin, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	ok, err := Cull(lib, &actions.Runner{DryRun: true}, []string{"psx"})
	if err != nil || !ok {
		t.Fatalf("Cull() = (%v, %v), want (true, nil)", ok, err)
	}
	if !exists(bin) {
		t.Error("--dry-run should never actually retire anything")
	}
}

// TestCullFlagsRatherThanRetiresNestedM3UDisc guards a deliberate
// scope limit: a disc already grouped inside a Title.m3u/ directory is
// never auto-retired even when it matches a cull-worthy finding,
// since removing just one disc would leave the playlist referencing a
// file that's gone - reported for manual review instead.
func TestCullFlagsRatherThanRetiresNestedM3UDisc(t *testing.T) {
	lib := cullLibrary(t, "psx")
	m3uDir := filepath.Join(lib.Roms, "psx", "Game.m3u")
	if err := os.MkdirAll(m3uDir, 0o755); err != nil {
		t.Fatal(err)
	}
	disc1 := filepath.Join(m3uDir, "Game (Disc 01).chd")
	disc2 := filepath.Join(m3uDir, "Game (Disc 02).chd")
	if err := os.WriteFile(disc1, []byte("not a real chd"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(disc2, []byte("not a real chd either"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(m3uDir, "Game.m3u"), []byte("Game (Disc 01).chd\nGame (Disc 02).chd\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	runner := &actions.Runner{Verbose: true}
	out := captureStdout(t, func() {
		ok, err := Cull(lib, runner, []string{"psx"})
		if err != nil || !ok {
			t.Fatalf("Cull() = (%v, %v), want (true, nil)", ok, err)
		}
	})
	if !exists(disc1) || !exists(disc2) {
		t.Error("a disc already grouped inside a Title.m3u/ directory should never be auto-retired")
	}
	if !strings.Contains(out, "Flagged for manual review") || !strings.Contains(out, "Game.m3u/Game (Disc 01).chd") {
		t.Errorf("expected the nested disc to be flagged instead, got:\n%s", out)
	}
}

func TestCullFlagsUnmanagedFormatForDiscBasedSystem(t *testing.T) {
	lib := cullLibrary(t, "psx")
	// .nrg (Nero disc image) is not in Duckstation's own documented
	// supported-format list - unlike .PBP, which is (see
	// TestCullDoesNotFlagVerifiedRealFormats below).
	nrg := filepath.Join(lib.Roms, "psx", "Some Game.nrg")
	if err := os.WriteFile(nrg, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	out := captureStdout(t, func() {
		ok, err := Cull(lib, &actions.Runner{}, []string{"psx"})
		if err != nil || !ok {
			t.Fatalf("Cull() = (%v, %v), want (true, nil)", ok, err)
		}
	})
	if !exists(nrg) {
		t.Error("an unmanaged format should never be retired, only flagged")
	}
	if !strings.Contains(out, "Some Game.nrg") || !strings.Contains(out, "unrecognized format") {
		t.Errorf("expected the .nrg file to be flagged as an unmanaged format, got:\n%s", out)
	}
}

// TestCullDoesNotFlagVerifiedRealFormats guards the real fix this
// session: .PBP (psx) and .cdi (dreamcast) are both directly supported
// by the actual emulator RetroDECK documents for each system
// (Duckstation, Flycast) - confirmed against RetroDECK's own
// wiki_emulator_guides pages, not romctl's own guess at what it can
// convert - so neither should ever be flagged as "unmanaged" at all.
func TestCullDoesNotFlagVerifiedRealFormats(t *testing.T) {
	lib := cullLibrary(t, "psx")
	pbp := filepath.Join(lib.Roms, "psx", "Some Game.PBP")
	if err := os.WriteFile(pbp, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	dreamDir := filepath.Join(lib.Root, "roms", "dreamcast")
	if err := os.MkdirAll(dreamDir, 0o755); err != nil {
		t.Fatal(err)
	}
	cdi := filepath.Join(dreamDir, "Some Game.cdi")
	if err := os.WriteFile(cdi, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	out := captureStdout(t, func() {
		ok, err := Cull(lib, &actions.Runner{}, []string{"psx", "dreamcast"})
		if err != nil || !ok {
			t.Fatalf("Cull() = (%v, %v), want (true, nil)", ok, err)
		}
	})
	if strings.Contains(out, "Flagged") {
		t.Errorf(".PBP (psx) and .cdi (dreamcast) are both verified-supported formats, should never be flagged, got:\n%s", out)
	}
}

func TestCullDoesNotFlagCartridgeExtensionsAsUnmanaged(t *testing.T) {
	lib := cullLibrary(t, "snes")
	rom := filepath.Join(lib.Roms, "snes", "Some Game.sfc")
	if err := os.WriteFile(rom, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	out := captureStdout(t, func() {
		ok, err := Cull(lib, &actions.Runner{}, []string{"snes"})
		if err != nil || !ok {
			t.Fatalf("Cull() = (%v, %v), want (true, nil)", ok, err)
		}
	})
	if strings.Contains(out, "Flagged") {
		t.Errorf("cartridge-shaped systems have no closed extension set and shouldn't be flagged at all, got:\n%s", out)
	}
}
