package commands

import (
	"fmt"
	"hash/crc32"
	"os"
	"path/filepath"
	"testing"

	"github.com/demicloud/romctl/internal/actions"
	"github.com/demicloud/romctl/internal/cliutil"
	"github.com/demicloud/romctl/internal/library"
)

// TestRenameVerifyDatUsesConfirmedGameName guards --verify-dat's main
// point: a hash-confirmed file renames straight to the DAT's own
// canonical name (run through scheme), not a cleaned-up guess based on
// whatever name it already had.
func TestRenameVerifyDatUsesConfirmedGameName(t *testing.T) {
	content := []byte("gb-rom-bytes")
	lib, sysDir := newTestLib(t, map[string]string{"scrambled_name.gb": string(content)})
	datDir := t.TempDir()
	writeIdentifyDat(t, datDir, "Pocket Monsters (Japan) (Rev 1)", content, "")

	runner := &actions.Runner{Format: cliutil.FormatPlain}
	if err := Rename(lib, runner, []string{"gb"}, actions.SchemeNoTag, true, true, datDir, false); err != nil {
		t.Fatal(err)
	}

	if exists(filepath.Join(sysDir, "scrambled_name.gb")) {
		t.Error("original scrambled filename should no longer exist")
	}
	want := filepath.Join(sysDir, "Pocket Monsters.gb")
	if _, err := os.Stat(want); err != nil {
		t.Errorf("expected renamed file at %s: %v", want, err)
	}
}

// TestRenameVerifyDatFallsBackToCleanupWhenUnmatched is --verify-dat's
// other half: a file that doesn't hash-confirm against anything still
// gets renamed, same as plain rename would - --verify-dat only changes
// *which* name a confirmed file gets, it never leaves a file untouched
// the way identify's own (removed) renaming used to.
func TestRenameVerifyDatFallsBackToCleanupWhenUnmatched(t *testing.T) {
	lib, sysDir := newTestLib(t, map[string]string{"Messy File (USA) [b1].gb": "totally-unrelated-content"})
	datDir := t.TempDir()
	writeIdentifyDat(t, datDir, "Some Known Game", []byte("different-content-entirely"), "")

	runner := &actions.Runner{Format: cliutil.FormatPlain}
	if err := Rename(lib, runner, []string{"gb"}, actions.SchemeNoTag, true, true, datDir, false); err != nil {
		t.Fatal(err)
	}

	want := filepath.Join(sysDir, "Messy File.gb")
	if _, err := os.Stat(want); err != nil {
		t.Errorf("expected the unmatched file cleaned up via the naming scheme at %s: %v", want, err)
	}
}

// TestRenameVerifyDatCollisionSkipped guards the same collision safety
// plain rename already has: a confirmed match must not overwrite a
// different, already-existing file at the target name.
func TestRenameVerifyDatCollisionSkipped(t *testing.T) {
	content := []byte("gb-rom-bytes-3")
	lib, sysDir := newTestLib(t, map[string]string{
		"messy_name.gb": string(content),
		"Clean Game.gb": "pre-existing-different-content",
	})
	datDir := t.TempDir()
	writeIdentifyDat(t, datDir, "Clean Game", content, "")

	runner := &actions.Runner{Format: cliutil.FormatPlain}
	if err := Rename(lib, runner, []string{"gb"}, actions.SchemeKeep, true, true, datDir, false); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(sysDir, "messy_name.gb")); err != nil {
		t.Errorf("source file should remain when the target collides: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(sysDir, "Clean Game.gb"))
	if err != nil {
		t.Fatalf("pre-existing target file should still exist: %v", err)
	}
	if string(got) != "pre-existing-different-content" {
		t.Error("pre-existing target file must not be overwritten")
	}
}

// TestRenameVerifyDatDryRunRenamesNothing guards --dry-run alongside
// --verify-dat: the hashing/matching still runs (it's read-only anyway),
// but nothing on disk changes.
func TestRenameVerifyDatDryRunRenamesNothing(t *testing.T) {
	content := []byte("gb-rom-bytes-4")
	lib, sysDir := newTestLib(t, map[string]string{"scrambled.gb": string(content)})
	datDir := t.TempDir()
	writeIdentifyDat(t, datDir, "Clean Game", content, "")

	runner := &actions.Runner{Format: cliutil.FormatPlain, DryRun: true}
	if err := Rename(lib, runner, []string{"gb"}, actions.SchemeKeep, true, true, datDir, false); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(sysDir, "scrambled.gb")); err != nil {
		t.Errorf("dry-run must not rename anything: %v", err)
	}
	if exists(filepath.Join(sysDir, "Clean Game.gb")) {
		t.Error("dry-run must not create the renamed file")
	}
}

// TestRenameWithoutVerifyDatIgnoresDat is plain rename's regression
// guard: verifyDat defaulting to false must behave exactly as rename
// always has, even when a DAT that could have matched is sitting right
// there - no hashing, no DAT-driven rename, just the naming scheme.
func TestRenameWithoutVerifyDatIgnoresDat(t *testing.T) {
	content := []byte("gb-rom-bytes-5")
	lib, sysDir := newTestLib(t, map[string]string{"Messy File (USA) [b1].gb": string(content)})
	datDir := t.TempDir()
	writeIdentifyDat(t, datDir, "Clean Game", content, "")

	runner := &actions.Runner{Format: cliutil.FormatPlain}
	if err := Rename(lib, runner, []string{"gb"}, actions.SchemeNoTag, false, true, datDir, false); err != nil {
		t.Fatal(err)
	}

	if exists(filepath.Join(sysDir, "Clean Game.gb")) {
		t.Error("verifyDat=false must never consult the DAT, got the DAT-verified name anyway")
	}
	want := filepath.Join(sysDir, "Messy File.gb")
	if _, err := os.Stat(want); err != nil {
		t.Errorf("expected the plain scheme-cleaned name at %s: %v", want, err)
	}
}

// TestRenameAutoVerifiesCartridgeSystem guards --verify-dat's default:
// when the flag isn't passed at all (verifyDatSet=false), a
// cartridge-class system like gb still gets hash-verified, since
// hashing its small files costs nothing noticeable.
func TestRenameAutoVerifiesCartridgeSystem(t *testing.T) {
	content := []byte("gb-rom-bytes-auto")
	lib, sysDir := newTestLib(t, map[string]string{"scrambled.gb": string(content)})
	datDir := t.TempDir()
	writeIdentifyDat(t, datDir, "Confirmed Game", content, "")

	runner := &actions.Runner{Format: cliutil.FormatPlain}
	if err := Rename(lib, runner, []string{"gb"}, actions.SchemeNoTag, false, false, datDir, false); err != nil {
		t.Fatal(err)
	}

	want := filepath.Join(sysDir, "Confirmed Game.gb")
	if _, err := os.Stat(want); err != nil {
		t.Errorf("expected gb to auto-verify against the DAT without --verify-dat, got: %v", err)
	}
}

// TestRenameAutoSkipsDiscSystem is the other half: a disc-class system
// (psx) must NOT auto-verify - its files are too large for that default
// to be free, so it stays the plain scheme-cleanup path unless
// --verify-dat is passed explicitly.
func TestRenameAutoSkipsDiscSystem(t *testing.T) {
	content := []byte("psx-rom-bytes-auto")
	root := t.TempDir()
	sysDir := filepath.Join(root, "roms", "psx")
	if err := os.MkdirAll(sysDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sysDir, "Messy File (USA) [b1].bin"), content, 0o644); err != nil {
		t.Fatal(err)
	}
	datDir := t.TempDir()
	writePSXDat(t, datDir, "Confirmed Game", content)

	lib := library.Open(root)
	runner := &actions.Runner{Format: cliutil.FormatPlain}
	if err := Rename(lib, runner, []string{"psx"}, actions.SchemeNoTag, false, false, datDir, false); err != nil {
		t.Fatal(err)
	}

	if exists(filepath.Join(sysDir, "Confirmed Game.bin")) {
		t.Error("psx (disc-class) must not auto-verify against the DAT without --verify-dat")
	}
	want := filepath.Join(sysDir, "Messy File.bin")
	if _, err := os.Stat(want); err != nil {
		t.Errorf("expected the plain scheme-cleaned name at %s: %v", want, err)
	}
}

// writePSXDat writes a one-game Redump-shaped DAT to dir/redump/Sony -
// PlayStation.dat - "psx" maps there in systemdb.Dat.
func writePSXDat(t *testing.T, dir, gameName string, content []byte) {
	t.Helper()
	hc := crc32.NewIEEE()
	hc.Write(content)

	dat := fmt.Sprintf(`<datafile><header><name>Sony - PlayStation</name><version>test</version></header>
<game name="%s"><rom name="rom.bin" size="%d" crc="%08x"/></game>
</datafile>`, gameName, len(content), hc.Sum32())

	datDir := filepath.Join(dir, "redump")
	if err := os.MkdirAll(datDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(datDir, "Sony - PlayStation.dat"), []byte(dat), 0o644); err != nil {
		t.Fatal(err)
	}
}
