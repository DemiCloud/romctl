package commands

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
)

// TestFindByExtRecursesIntoSubfolders guards against a regression: many
// redump/no-intro archives wrap a disc image's .cue/.bin pair in one
// extra per-game subfolder (e.g. "Game/Game.cue"), which convertGameDir
// used to miss entirely (a plain top-level os.ReadDir), reporting
// "no cue/gdi/iso in: <dir>" even though the files were right there one
// level down.
func TestFindByExtRecursesIntoSubfolders(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "Some Game (USA)")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	cue := filepath.Join(nested, "Some Game (USA).cue")
	if err := os.WriteFile(cue, []byte("cue"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, "Some Game (USA).bin"), []byte("bin"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := findByExt(root, ".cue", ".gdi")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != cue {
		t.Fatalf("findByExt = %v, want [%s]", got, cue)
	}
}

// TestFindByExtMultiDisc confirms disc images nested under separate
// per-disc subfolders are all found, not just the first.
func TestFindByExtMultiDisc(t *testing.T) {
	root := t.TempDir()
	var want []string
	for i := 1; i <= 2; i++ {
		dir := filepath.Join(root, "Disc"+string(rune('0'+i)))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		cue := filepath.Join(dir, "game.cue")
		if err := os.WriteFile(cue, []byte("cue"), 0o644); err != nil {
			t.Fatal(err)
		}
		want = append(want, cue)
	}

	got, err := findByExt(root, ".cue", ".gdi")
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(got)
	sort.Strings(want)
	if len(got) != len(want) {
		t.Fatalf("findByExt = %v, want %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("findByExt = %v, want %v", got, want)
		}
	}
}
