package gamelist

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
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

func TestRenamePatchesGamelistPath(t *testing.T) {
	root := t.TempDir()
	gl := filepath.Join(root, "ES-DE", "gamelists", "snes", "gamelist.xml")
	writeFile(t, gl, `<?xml version="1.0"?>
<gameList>
	<game>
		<path>./Super Game (USA).smc</path>
		<name>Super Game</name>
		<desc>A &amp; B's adventure</desc>
	</game>
</gameList>
`)

	if err := Rename(root, "snes", "Super Game (USA).smc", "Super Game.sfc"); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(gl)
	if err != nil {
		t.Fatal(err)
	}
	want := `<?xml version="1.0"?>
<gameList>
	<game>
		<path>./Super Game.sfc</path>
		<name>Super Game</name>
		<desc>A &amp; B's adventure</desc>
	</game>
</gameList>
`
	if string(got) != want {
		t.Errorf("gamelist.xml mismatch:\ngot:  %q\nwant: %q", got, want)
	}
}

func TestRenameNoMatchIsNoop(t *testing.T) {
	root := t.TempDir()
	gl := filepath.Join(root, "ES-DE", "gamelists", "snes", "gamelist.xml")
	content := `<gameList><game><path>./Other Game.sfc</path></game></gameList>`
	writeFile(t, gl, content)

	if err := Rename(root, "snes", "Super Game (USA).smc", "Super Game.sfc"); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(gl)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != content {
		t.Errorf("expected no change, got: %q", got)
	}
}

func TestRenameMovesMedia(t *testing.T) {
	root := t.TempDir()
	cover := filepath.Join(root, "ES-DE", "downloaded_media", "snes", "covers", "Super Game (USA).png")
	writeFile(t, cover, "fake png")

	rom := filepath.Join(root, "roms", "snes", "Super Game.sfc")
	writeFile(t, rom, "fake rom")

	if err := Rename(root, "snes", "Super Game (USA).smc", "Super Game.sfc"); err != nil {
		t.Fatal(err)
	}

	want := filepath.Join(root, "ES-DE", "downloaded_media", "snes", "covers", "Super Game.png")
	if _, err := os.Stat(want); err != nil {
		t.Errorf("expected renamed cover at %s: %v", want, err)
	}
	if _, err := os.Stat(cover); !os.IsNotExist(err) {
		t.Errorf("expected old cover gone, got err=%v", err)
	}
}

func TestRenameMovesDirectoryGameMedia(t *testing.T) {
	root := t.TempDir()
	cover := filepath.Join(root, "ES-DE", "downloaded_media", "ps2", "covers", "Title (Disc 01).chd.png")
	writeFile(t, cover, "fake png")

	dir := filepath.Join(root, "roms", "ps2", "Title.m3u")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := Rename(root, "ps2", "Title (Disc 01).chd", "Title.m3u"); err != nil {
		t.Fatal(err)
	}

	want := filepath.Join(root, "ES-DE", "downloaded_media", "ps2", "covers", "Title.m3u.png")
	if _, err := os.Stat(want); err != nil {
		t.Errorf("expected renamed cover at %s: %v", want, err)
	}
}

// TestRemoveDeletesGamelistEntry guards Remove's whole-entry deletion:
// unlike Rename, which only ever rewrites a <path>'s content in place,
// Remove must drop the entire <game> block (and leave every other game's
// entry untouched), since its caller (relocate, moving a misfiled ROM to
// a different system) has nowhere to re-point the entry to.
func TestRemoveDeletesGamelistEntry(t *testing.T) {
	root := t.TempDir()
	gl := filepath.Join(root, "ES-DE", "gamelists", "dreamcast", "gamelist.xml")
	writeFile(t, gl, `<?xml version="1.0"?>
<gameList>
	<game>
		<path>./Misfiled Genesis Game.bin</path>
		<name>Misfiled Genesis Game</name>
	</game>
	<game>
		<path>./Real Dreamcast Game.chd</path>
		<name>Real Dreamcast Game</name>
	</game>
</gameList>
`)

	if err := Remove(root, "dreamcast", "Misfiled Genesis Game.bin"); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(gl)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(got), "Misfiled Genesis Game") {
		t.Errorf("expected the misfiled game's entry to be gone, got:\n%s", got)
	}
	if !strings.Contains(string(got), "./Real Dreamcast Game.chd") {
		t.Errorf("expected the other game's entry to survive untouched, got:\n%s", got)
	}
}

// TestRemoveLeavesGameListTagIntact guards against gameRE also matching
// the surrounding <gameList> tag itself (a real regression caught
// against a live run: <game[^>]*> on its own matches "<gameList>" just
// as happily as "<game>", silently eating the document's own wrapper
// element along with the entry meant to be removed).
func TestRemoveLeavesGameListTagIntact(t *testing.T) {
	root := t.TempDir()
	gl := filepath.Join(root, "ES-DE", "gamelists", "dreamcast", "gamelist.xml")
	writeFile(t, gl, `<?xml version="1.0"?>
<gameList>
	<game>
		<path>./Misfiled Genesis Game.bin</path>
	</game>
</gameList>
`)

	if err := Remove(root, "dreamcast", "Misfiled Genesis Game.bin"); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(gl)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "<gameList>") || !strings.Contains(string(got), "</gameList>") {
		t.Errorf("expected the <gameList> wrapper to survive intact, got:\n%s", got)
	}
}

// TestRemoveDeletesMedia is TestRenameMovesMedia's counterpart for
// Remove: the misfiled ROM's media should be deleted, not left orphaned
// pointing at a path that no longer exists in this system.
func TestRemoveDeletesMedia(t *testing.T) {
	root := t.TempDir()
	cover := filepath.Join(root, "ES-DE", "downloaded_media", "dreamcast", "covers", "Misfiled Genesis Game.png")
	writeFile(t, cover, "fake png")

	if err := Remove(root, "dreamcast", "Misfiled Genesis Game.bin"); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(cover); !os.IsNotExist(err) {
		t.Errorf("expected the orphaned cover to be deleted, got err=%v", err)
	}
}

// TestRemoveNoMatchIsNoop mirrors TestRenameNoMatchIsNoop: a ROM that was
// never scraped leaves the gamelist untouched rather than erroring.
func TestRemoveNoMatchIsNoop(t *testing.T) {
	root := t.TempDir()
	gl := filepath.Join(root, "ES-DE", "gamelists", "dreamcast", "gamelist.xml")
	content := `<gameList><game><path>./Other Game.chd</path></game></gameList>`
	writeFile(t, gl, content)

	if err := Remove(root, "dreamcast", "Never Scraped.bin"); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(gl)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != content {
		t.Errorf("gamelist.xml should be untouched, got:\n%s", got)
	}
}

func TestRenameWithoutESDEIsNoop(t *testing.T) {
	root := t.TempDir()
	if err := Rename(root, "snes", "Super Game (USA).smc", "Super Game.sfc"); err != nil {
		t.Fatal(err)
	}
}

// TestRenameConcurrentSameSystem guards against the lost-update race
// patchGamelist's read-modify-write would otherwise hit once conversion
// commands started converting a system's files concurrently (see
// renameLocks): every goroutine here targets a distinct <path> in the
// same system's gamelist.xml, so without the per-(libRoot,sys) lock some
// of these edits would be silently overwritten by a sibling goroutine's
// write. Run with -race to also catch the underlying data race itself.
func TestRenameConcurrentSameSystem(t *testing.T) {
	root := t.TempDir()
	const n = 50

	var games strings.Builder
	games.WriteString("<gameList>\n")
	for i := 0; i < n; i++ {
		fmt.Fprintf(&games, "\t<game>\n\t\t<path>./Game %d (USA).smc</path>\n\t</game>\n", i)
	}
	games.WriteString("</gameList>\n")
	gl := filepath.Join(root, "ES-DE", "gamelists", "snes", "gamelist.xml")
	writeFile(t, gl, games.String())

	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			old := fmt.Sprintf("Game %d (USA).smc", i)
			renamed := fmt.Sprintf("Game %d.sfc", i)
			if err := Rename(root, "snes", old, renamed); err != nil {
				t.Errorf("rename %d: %v", i, err)
			}
		}(i)
	}
	wg.Wait()

	got, err := os.ReadFile(gl)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < n; i++ {
		want := fmt.Sprintf("<path>./Game %d.sfc</path>", i)
		if !strings.Contains(string(got), want) {
			t.Errorf("missing edit for game %d (lost update): want %q in gamelist.xml", i, want)
		}
	}
}
