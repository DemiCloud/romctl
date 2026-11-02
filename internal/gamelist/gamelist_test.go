package gamelist

import (
	"os"
	"path/filepath"
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

func TestRenameWithoutESDEIsNoop(t *testing.T) {
	root := t.TempDir()
	if err := Rename(root, "snes", "Super Game (USA).smc", "Super Game.sfc"); err != nil {
		t.Fatal(err)
	}
}
