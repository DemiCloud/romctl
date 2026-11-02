package commands

import "testing"

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
