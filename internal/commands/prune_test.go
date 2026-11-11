package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/demicloud/romctl/internal/actions"
	"github.com/demicloud/romctl/internal/library"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestPruneRemovesEntryForDeletedROM(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "roms", "psx"), 0o755); err != nil {
		t.Fatal(err)
	}
	lib := library.Open(root)

	gl := filepath.Join(root, "ES-DE", "gamelists", "psx", "gamelist.xml")
	writeFile(t, gl, `<gameList>
	<game><path>./Present Game.chd</path></game>
	<game><path>./Gone Game.chd</path></game>
</gameList>
`)
	writeFile(t, filepath.Join(lib.Roms, "psx", "Present Game.chd"), "fake chd")
	cover := filepath.Join(root, "ES-DE", "downloaded_media", "psx", "covers", "Gone Game.png")
	writeFile(t, cover, "fake png")

	ok, err := Prune(lib, &actions.Runner{}, []string{"psx"})
	if err != nil || !ok {
		t.Fatalf("Prune() = (%v, %v), want (true, nil)", ok, err)
	}

	got, err := os.ReadFile(gl)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "Present Game.chd") {
		t.Errorf("gamelist.xml lost the still-present game's entry:\n%s", got)
	}
	if strings.Contains(string(got), "Gone Game.chd") {
		t.Errorf("gamelist.xml still has the deleted game's entry:\n%s", got)
	}
	if exists(cover) {
		t.Error("expected the orphaned cover to be deleted")
	}
}

func TestPruneDryRunLeavesGamelistUntouched(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "roms", "psx"), 0o755); err != nil {
		t.Fatal(err)
	}
	lib := library.Open(root)

	gl := filepath.Join(root, "ES-DE", "gamelists", "psx", "gamelist.xml")
	content := `<gameList><game><path>./Gone Game.chd</path></game></gameList>`
	writeFile(t, gl, content)

	ok, err := Prune(lib, &actions.Runner{DryRun: true}, []string{"psx"})
	if err != nil || !ok {
		t.Fatalf("Prune() = (%v, %v), want (true, nil)", ok, err)
	}
	got, err := os.ReadFile(gl)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != content {
		t.Errorf("gamelist.xml should be untouched on --dry-run, got:\n%s", got)
	}
}
