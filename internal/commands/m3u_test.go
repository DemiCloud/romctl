package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/demicloud/romctl/internal/actions"
	"github.com/demicloud/romctl/internal/library"
)

// TestM3UGroupsCHD covers the original behavior: loose "Title (Disc
// N).chd" files (CD/DVD/UMD-class systems) get grouped into Title.m3u/.
func TestM3UGroupsCHD(t *testing.T) {
	root := t.TempDir()
	psxDir := filepath.Join(root, "roms", "psx")
	if err := os.MkdirAll(psxDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, n := range []int{1, 2} {
		p := filepath.Join(psxDir, fmt.Sprintf("Game (Disc %d).chd", n))
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	lib := library.Open(root)

	if err := M3U(lib, &actions.Runner{}, []string{"psx"}); err != nil {
		t.Fatal(err)
	}

	assertM3UGrouped(t, psxDir, "Game", "chd", 2)
}

// TestM3ULeavesLoneDiscUngrouped guards the fix for a real bug: m3u used
// to group any "(Disc N).chd/.rvz" file regardless of whether a sibling
// disc number existed, wrapping a single-disc game whose name just
// happens to carry a "(Disc 1)" tag in a pointless one-file .m3u/
// directory - confirmed against a real library (a GameCube title ended
// up exactly this way from an earlier m3u run, which then made it
// invisible to check/collection's own disc-completeness logic since
// nothing else expects game content inside a .m3u/ directory with only
// one disc).
func TestM3ULeavesLoneDiscUngrouped(t *testing.T) {
	root := t.TempDir()
	psxDir := filepath.Join(root, "roms", "psx")
	if err := os.MkdirAll(psxDir, 0o755); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(psxDir, "Game (Disc 1).chd")
	if err := os.WriteFile(src, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	lib := library.Open(root)

	if err := M3U(lib, &actions.Runner{}, []string{"psx"}); err != nil {
		t.Fatal(err)
	}

	if !exists(src) {
		t.Error("lone disc file should have been left in place, not grouped")
	}
	if exists(filepath.Join(psxDir, "Game.m3u")) {
		t.Error("a lone disc should never produce a one-file .m3u/ directory")
	}
}

// TestM3UGroupsRVZ covers GameCube/Wii multi-disc games: they never go
// through CHD (see rvz.go), so the loose files are "Title (Disc N).rvz"
// instead - m3u must recognize that extension too, not just .chd.
func TestM3UGroupsRVZ(t *testing.T) {
	root := t.TempDir()
	gcDir := filepath.Join(root, "roms", "gc")
	if err := os.MkdirAll(gcDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, n := range []int{1, 2} {
		p := filepath.Join(gcDir, fmt.Sprintf("Resident Evil (Disc %d).rvz", n))
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	lib := library.Open(root)

	if err := M3U(lib, &actions.Runner{}, []string{"gc"}); err != nil {
		t.Fatal(err)
	}

	assertM3UGrouped(t, gcDir, "Resident Evil", "rvz", 2)
}

// TestM3UIgnoresPreConversionFormat guards the ordering check.go/fix.go
// rely on: a disc-tagged file not yet converted to its system's final
// container (.bin here, instead of .chd) must be left alone - grouping it
// is chd's/rvz's job to set up for, not m3u's to improvise around.
func TestM3UIgnoresPreConversionFormat(t *testing.T) {
	root := t.TempDir()
	psxDir := filepath.Join(root, "roms", "psx")
	if err := os.MkdirAll(psxDir, 0o755); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(psxDir, "Game (Disc 1).bin")
	if err := os.WriteFile(src, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	lib := library.Open(root)

	if err := M3U(lib, &actions.Runner{}, []string{"psx"}); err != nil {
		t.Fatal(err)
	}

	if !exists(src) {
		t.Error("loose pre-conversion file should have been left alone")
	}
	if exists(filepath.Join(psxDir, "Game.m3u")) {
		t.Error("should not have grouped a file that was never converted")
	}
}

func assertM3UGrouped(t *testing.T, sysDir, title, ext string, discs int) {
	t.Helper()
	dir := filepath.Join(sysDir, title+".m3u")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("%s.m3u/ was not created: %v", title, err)
	}
	wantFiles := discs + 1 // N discs + the .m3u playlist itself
	if len(entries) != wantFiles {
		t.Fatalf("%s.m3u/ has %d entries, want %d", title, len(entries), wantFiles)
	}
	playlist, err := os.ReadFile(filepath.Join(dir, title+".m3u"))
	if err != nil {
		t.Fatalf("playlist missing: %v", err)
	}
	for n := 1; n <= discs; n++ {
		name := fmt.Sprintf("%s (Disc %02d).%s", title, n, ext)
		if !exists(filepath.Join(dir, name)) {
			t.Errorf("missing moved disc file %s", name)
		}
		if !strings.Contains(string(playlist), name) {
			t.Errorf("playlist missing entry %s, got:\n%s", name, playlist)
		}
	}
}
