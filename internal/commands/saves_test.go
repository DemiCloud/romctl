package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/demicloud/romctl/internal/actions"
	"github.com/demicloud/romctl/internal/library"
)

// TestSavesRelocatesByExtension exercises every extension saveExtSubdir
// knows about, confirming sav/srm/sh land under saves/<system>/ and state
// lands under states/<system>/, both siblings of roms/ at the library
// root - and that the actual ROM file next to them is left untouched.
func TestSavesRelocatesByExtension(t *testing.T) {
	root := t.TempDir()
	sysDir := filepath.Join(root, "roms", "n64")
	if err := os.MkdirAll(sysDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"Game.z64", "Game.sav", "Game.srm", "Game.sh", "Game.state"} {
		if err := os.WriteFile(filepath.Join(sysDir, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	lib := library.Open(root)
	ok, err := Saves(lib, &actions.Runner{}, []string{"n64"}, "")
	if err != nil || !ok {
		t.Fatalf("Saves failed: ok=%v err=%v", ok, err)
	}

	for _, want := range []string{
		filepath.Join(root, "saves", "n64", "Game.sav"),
		filepath.Join(root, "saves", "n64", "Game.srm"),
		filepath.Join(root, "saves", "n64", "Game.sh"),
		filepath.Join(root, "states", "n64", "Game.state"),
	} {
		if !exists(want) {
			t.Errorf("expected relocated file: %s", want)
		}
	}
	if !exists(filepath.Join(sysDir, "Game.z64")) {
		t.Error("ROM file should not have moved")
	}
	for _, gone := range []string{"Game.sav", "Game.srm", "Game.sh", "Game.state"} {
		if exists(filepath.Join(sysDir, gone)) {
			t.Errorf("stray file should have moved out of roms/: %s", gone)
		}
	}
}

// TestSavesCustomDest confirms --dest (the dest parameter) relocates
// into DIR/saves, DIR/states instead of the library root - letting a
// user keep save data somewhere other than the RetroDECK-managed tree.
func TestSavesCustomDest(t *testing.T) {
	root := t.TempDir()
	customDest := t.TempDir()
	sysDir := filepath.Join(root, "roms", "n64")
	if err := os.MkdirAll(sysDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sysDir, "Game.srm"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	lib := library.Open(root)
	if _, err := Saves(lib, &actions.Runner{}, []string{"n64"}, customDest); err != nil {
		t.Fatal(err)
	}

	if !exists(filepath.Join(customDest, "saves", "n64", "Game.srm")) {
		t.Error("expected file under the custom --dest, not the library root")
	}
	if exists(filepath.Join(root, "saves", "n64", "Game.srm")) {
		t.Error("should not have written under the library root when --dest was given")
	}
}

// TestSavesConflictIdenticalContent confirms a destination collision
// where both files have identical content is left in place (never
// overwritten or deleted) and reported as identical.
func TestSavesConflictIdenticalContent(t *testing.T) {
	root := t.TempDir()
	sysDir := filepath.Join(root, "roms", "snes")
	destDir := filepath.Join(root, "saves", "snes")
	if err := os.MkdirAll(sysDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sysDir, "Game.srm"), []byte("same bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(destDir, "Game.srm"), []byte("same bytes"), 0o644); err != nil {
		t.Fatal(err)
	}

	lib := library.Open(root)
	out := captureStdout(t, func() {
		if _, err := Saves(lib, &actions.Runner{}, []string{"snes"}, ""); err != nil {
			t.Fatal(err)
		}
	})

	if !exists(filepath.Join(sysDir, "Game.srm")) {
		t.Error("source should be left in place on a conflict")
	}
	if !strings.Contains(out, "identical") {
		t.Errorf("expected the conflict to be reported as identical, got: %s", out)
	}
}

// TestSavesConflictDifferentContent confirms a destination collision
// where the two files differ is reported as needing manual resolution,
// not silently treated as safe to discard.
func TestSavesConflictDifferentContent(t *testing.T) {
	root := t.TempDir()
	sysDir := filepath.Join(root, "roms", "snes")
	destDir := filepath.Join(root, "saves", "snes")
	if err := os.MkdirAll(sysDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sysDir, "Game.srm"), []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(destDir, "Game.srm"), []byte("existing"), 0o644); err != nil {
		t.Fatal(err)
	}

	lib := library.Open(root)
	out := captureStdout(t, func() {
		if _, err := Saves(lib, &actions.Runner{}, []string{"snes"}, ""); err != nil {
			t.Fatal(err)
		}
	})

	if !exists(filepath.Join(sysDir, "Game.srm")) {
		t.Error("source should be left in place when destination already exists")
	}
	data, err := os.ReadFile(filepath.Join(destDir, "Game.srm"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "existing" {
		t.Error("pre-existing destination file should not be overwritten")
	}
	if !strings.Contains(out, "DIFFERENT") {
		t.Errorf("expected the conflict to be reported as different, got: %s", out)
	}
}

// TestSavesDryRunMovesNothing confirms --dry-run previews without
// touching the filesystem, same contract as every other mutating command.
func TestSavesDryRunMovesNothing(t *testing.T) {
	root := t.TempDir()
	sysDir := filepath.Join(root, "roms", "snes")
	if err := os.MkdirAll(sysDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sysDir, "Game.srm"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	lib := library.Open(root)
	if _, err := Saves(lib, &actions.Runner{DryRun: true}, []string{"snes"}, ""); err != nil {
		t.Fatal(err)
	}

	if !exists(filepath.Join(sysDir, "Game.srm")) {
		t.Error("dry-run should not have moved anything")
	}
	if exists(filepath.Join(root, "saves", "snes", "Game.srm")) {
		t.Error("dry-run should not have created the destination")
	}
}
