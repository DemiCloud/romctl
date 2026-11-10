package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/demicloud/romctl/internal/cliutil"
	"github.com/demicloud/romctl/internal/library"
)

func TestHashable(t *testing.T) {
	cases := map[string]bool{
		"Sonic CD.chd":           true,
		"Final Fantasy VII.m3u":  true,
		"systeminfo.txt":         false,
		".directory":             false,
		"notes.txt":              false,
		"game.sav":               false,
		"foo.partial.chd":        false,
		"foo.romctl-rename.1234": false,
	}
	for in, want := range cases {
		if got := hashable(in); got != want {
			t.Errorf("hashable(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestDupes(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "roms", "snes")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(name, content string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("Game A.sfc", "identical-content")
	write("Game A (Copy).sfc", "identical-content")
	write("Game B.sfc", "different-content")
	write("notes.txt", "identical-content") // not hashable, must not be flagged
	lib := library.Open(root)

	out := captureStdout(t, func() {
		Dupes(lib, []string{"snes"}, true, cliutil.FormatPlain)
	})

	hasA := strings.Contains(out, "Game A.sfc:")
	hasCopy := strings.Contains(out, "Game A (Copy).sfc:")
	if !hasA && !hasCopy {
		t.Errorf("expected one of the duplicate pair to be flagged, got:\n%s", out)
	}
	if hasA && hasCopy {
		t.Errorf("expected only one of the duplicate pair to be flagged, not both, got:\n%s", out)
	}
	if strings.Contains(out, "Game B.sfc:") {
		t.Errorf("a unique file should not be flagged, got:\n%s", out)
	}
	if strings.Contains(out, "notes.txt") {
		t.Errorf("a non-hashable file should not be flagged even with matching content, got:\n%s", out)
	}
}

// TestDupesFindsDuplicateInsideM3U confirms dupes walks into a Title.m3u/
// playlist directory the same way verify's own CHD walk already does
// (see verifyJobsFor), instead of silently skipping every multi-disc
// game the way a plain os.ReadDir + skip-dirs loop would. A duplicate
// disc is flagged with its "Title.m3u/Disc.chd" label, and the playlist
// text file itself is never hashed as if it were game content.
func TestDupesFindsDuplicateInsideM3U(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "roms", "psx")
	m3uDir := filepath.Join(dir, "Game.m3u")
	if err := os.MkdirAll(m3uDir, 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(path, content string) {
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(dir, "Loose.chd"), "identical-content")
	write(filepath.Join(m3uDir, "Game (Disc 1).chd"), "identical-content")
	write(filepath.Join(m3uDir, "Game (Disc 2).chd"), "different-content")
	write(filepath.Join(m3uDir, "Game.m3u"), "identical-content") // playlist text, must not be flagged
	lib := library.Open(root)

	out := captureStdout(t, func() {
		Dupes(lib, []string{"psx"}, true, cliutil.FormatPlain)
	})

	hasLoose := strings.Contains(out, "Loose.chd:")
	hasDisc1 := strings.Contains(out, "Game.m3u/Game (Disc 1).chd:")
	if !hasLoose && !hasDisc1 {
		t.Errorf("expected one of the duplicate pair (loose file vs. disc inside Game.m3u/) to be flagged, got:\n%s", out)
	}
	if hasLoose && hasDisc1 {
		t.Errorf("expected only one of the duplicate pair to be flagged, not both, got:\n%s", out)
	}
	if strings.Contains(out, "Game (Disc 2).chd:") {
		t.Errorf("a unique disc should not be flagged, got:\n%s", out)
	}
	if strings.Contains(out, "Game.m3u:") {
		t.Errorf("the playlist file itself should never be hashed as game content, got:\n%s", out)
	}
}

// TestDupesVerbosePrintsPerFileProgress confirms -v prints a line for
// every file as it's hashed, not just the ones that turn out to be
// duplicates - the whole point of pulling dupes out of check's own
// --verbose in the first place (see Dupes' doc comment).
func TestDupesVerbosePrintsPerFileProgress(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "roms", "snes")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "Game A.sfc"), []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "Game B.sfc"), []byte("b"), 0o644); err != nil {
		t.Fatal(err)
	}
	lib := library.Open(root)

	out := captureStdout(t, func() {
		Dupes(lib, []string{"snes"}, true, cliutil.FormatPlain)
	})

	for _, want := range []string{"hash snes/Game A.sfc", "hash snes/Game B.sfc"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected verbose output to contain %q, got:\n%s", want, out)
		}
	}
}

func TestCrc32File(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a")
	if err := os.WriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	sum, err := crc32File(path)
	if err != nil {
		t.Fatal(err)
	}
	// crc32.ChecksumIEEE([]byte("hello")) == 0x3610a686
	if sum != 0x3610a686 {
		t.Errorf("crc32File = %08x, want 3610a686", sum)
	}
}
