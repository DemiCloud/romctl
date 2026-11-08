package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestListUsesSystemsTxtOverride guards the systems.txt cross-check: when
// the library has ES-DE's own generated systems.txt, List should show its
// full name instead of romctl's hand-ported systemdb.FullName.
func TestListUsesSystemsTxtOverride(t *testing.T) {
	lib := scratchLibrary(t)
	if err := os.WriteFile(filepath.Join(lib.Roms, "psx", "Game.cue"), []byte("cue"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(lib.Root, "systems.txt"), []byte("psx: PSX (per this install)\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	out := captureStdout(t, func() {
		if err := List(lib, []string{"psx"}, false); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(out, "PSX (per this install)") {
		t.Errorf("expected List to use the systems.txt override, got:\n%s", out)
	}
}

func TestGameTitle(t *testing.T) {
	cases := map[string]string{
		"Final Fantasy VII.m3u": "Final Fantasy VII",
		"Some Game.ps3":         "Some Game",
		"Sonic CD.chd":          "Sonic CD",
		"sf2.zip":               "sf2",
	}
	for in, want := range cases {
		if got := gameTitle(in); got != want {
			t.Errorf("gameTitle(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestIsLeftover(t *testing.T) {
	cases := map[string]bool{
		"leftover.tmp.1234":      true,
		"foo.partial.chd":        true,
		"foo.romctl-rename.5678": true,
		"foo.part":               true,
		"Sonic CD.chd":           false,
		"Final Fantasy VII.m3u":  false,
	}
	for in, want := range cases {
		if got := isLeftover(in); got != want {
			t.Errorf("isLeftover(%q) = %v, want %v", in, got, want)
		}
	}
}
