package commands

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/demicloud/romctl/internal/audit"
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

func TestCheckDuplicates(t *testing.T) {
	dir := t.TempDir()
	write := func(name, content string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("Game A.sfc", "identical-content")
	write("Game A (Copy).sfc", "identical-content")
	write("Game B.sfc", "different-content")
	write("notes.txt", "identical-content") // not hashable, must not be flagged

	var buf bytes.Buffer
	report := audit.New(&buf)
	checkDuplicates(report, "snes", dir)

	out := buf.String()
	lines := strings.Count(out, "\n")
	if lines != 1 {
		t.Fatalf("expected exactly 1 finding (one of the pair flagged, the other kept), got %d:\n%s", lines, out)
	}
	hasA := strings.Contains(out, "Game A.sfc:")
	hasCopy := strings.Contains(out, "Game A (Copy).sfc:")
	if !hasA && !hasCopy {
		t.Errorf("expected one of the duplicate pair to be flagged, got:\n%s", out)
	}
	if hasA && hasCopy {
		t.Errorf("expected only one of the duplicate pair to be flagged, not both, got:\n%s", out)
	}
	if strings.Contains(out, "Game B.sfc") {
		t.Errorf("a unique file should not be flagged, got:\n%s", out)
	}
	if strings.Contains(out, "notes.txt") {
		t.Errorf("a non-hashable file should not be flagged even with matching content, got:\n%s", out)
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
