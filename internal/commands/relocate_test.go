package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/demicloud/romctl/internal/actions"
	"github.com/demicloud/romctl/internal/library"
)

func genesisROMBytes() []byte {
	data := make([]byte, 0x200)
	copy(data[0x100:], "SEGA MEGA DRIVE ")
	return data
}

func segaCDBootBytes() []byte {
	data := make([]byte, 0x200)
	copy(data[0x100:], "SEGA")
	copy(data[0x180:], "BR")
	return data
}

func relocateLibrary(t *testing.T) *library.Library {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "roms", "segacd"), 0o755); err != nil {
		t.Fatal(err)
	}
	return library.Open(root)
}

func TestRelocateMovesMisfiledCartridgeROM(t *testing.T) {
	lib := relocateLibrary(t)
	src := filepath.Join(lib.Roms, "segacd", "ALTERED_.bin")
	if err := os.WriteFile(src, genesisROMBytes(), 0o644); err != nil {
		t.Fatal(err)
	}

	ok, err := Relocate(lib, &actions.Runner{}, []string{"segacd"})
	if err != nil || !ok {
		t.Fatalf("Relocate() = (%v, %v), want (true, nil)", ok, err)
	}
	if exists(src) {
		t.Error("source file should have been moved, still exists")
	}
	dst := filepath.Join(lib.Roms, "genesis", "ALTERED_.bin")
	if !exists(dst) {
		t.Errorf("expected %s to exist after relocate", dst)
	}
}

// TestRelocateRemovesStaleGamelistEntry guards relocate's gamelist
// cleanup: a misfiled ROM that picked up a (wrong-platform) scraped
// gamelist entry under the system it was mistakenly dropped into - a
// real-world case, not a theoretical one (a Genesis cart left unnoticed
// in dreamcast/) - must have that entry removed once relocate moves it
// to its correct system, rather than left behind pointing at a path that
// no longer exists there. Nothing should be added to the destination
// system's gamelist either: there's no legitimate scrape to carry over.
func TestRelocateRemovesStaleGamelistEntry(t *testing.T) {
	lib := relocateLibrary(t)
	src := filepath.Join(lib.Roms, "segacd", "ALTERED_.bin")
	if err := os.WriteFile(src, genesisROMBytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	gl := filepath.Join(lib.Root, "ES-DE", "gamelists", "segacd", "gamelist.xml")
	if err := os.MkdirAll(filepath.Dir(gl), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(gl, []byte(`<gameList><game><path>./ALTERED_.bin</path><name>Altered Beast</name></game></gameList>`), 0o644); err != nil {
		t.Fatal(err)
	}

	ok, err := Relocate(lib, &actions.Runner{}, []string{"segacd"})
	if err != nil || !ok {
		t.Fatalf("Relocate() = (%v, %v), want (true, nil)", ok, err)
	}

	got, err := os.ReadFile(gl)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(got), "ALTERED_.bin") {
		t.Errorf("expected the stale segacd gamelist entry to be removed, got:\n%s", got)
	}
	genesisGL := filepath.Join(lib.Root, "ES-DE", "gamelists", "genesis", "gamelist.xml")
	if exists(genesisGL) {
		t.Error("nothing should have been added to the destination system's gamelist")
	}
}

func TestRelocateLeavesSegaCDBootSectorAlone(t *testing.T) {
	lib := relocateLibrary(t)
	src := filepath.Join(lib.Roms, "segacd", "Game (Track 1).bin")
	if err := os.WriteFile(src, segaCDBootBytes(), 0o644); err != nil {
		t.Fatal(err)
	}

	ok, err := Relocate(lib, &actions.Runner{}, []string{"segacd"})
	if err != nil || !ok {
		t.Fatalf("Relocate() = (%v, %v), want (true, nil)", ok, err)
	}
	if !exists(src) {
		t.Error("Sega CD boot sector should not have been moved")
	}
}

func TestRelocateDryRunMovesNothing(t *testing.T) {
	lib := relocateLibrary(t)
	src := filepath.Join(lib.Roms, "segacd", "ALTERED_.bin")
	if err := os.WriteFile(src, genesisROMBytes(), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := Relocate(lib, &actions.Runner{DryRun: true}, []string{"segacd"}); err != nil {
		t.Fatal(err)
	}
	if !exists(src) {
		t.Error("--dry-run should not have moved the file")
	}
	if exists(filepath.Join(lib.Roms, "genesis", "ALTERED_.bin")) {
		t.Error("--dry-run should not have created the destination")
	}
}

func TestRelocateReportsConflictWithoutOverwriting(t *testing.T) {
	lib := relocateLibrary(t)
	src := filepath.Join(lib.Roms, "segacd", "ALTERED_.bin")
	if err := os.WriteFile(src, genesisROMBytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	genesisDir := filepath.Join(lib.Roms, "genesis")
	if err := os.MkdirAll(genesisDir, 0o755); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(genesisDir, "ALTERED_.bin")
	if err := os.WriteFile(dst, []byte("already here"), 0o644); err != nil {
		t.Fatal(err)
	}

	out := captureStdout(t, func() {
		if _, err := Relocate(lib, &actions.Runner{}, []string{"segacd"}); err != nil {
			t.Fatal(err)
		}
	})
	if !exists(src) {
		t.Error("source should be left in place on a conflict")
	}
	data, err := os.ReadFile(dst)
	if err != nil || string(data) != "already here" {
		t.Error("destination should not have been overwritten")
	}
	if !strings.Contains(out, "Conflicts") {
		t.Errorf("expected a Conflicts section, got:\n%s", out)
	}
}
