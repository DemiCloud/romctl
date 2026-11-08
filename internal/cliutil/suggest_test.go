package cliutil

import "testing"

func TestSuggest(t *testing.T) {
	candidates := []string{"psx", "snes", "n64", "segacd"}
	cases := []struct {
		input, want string
	}{
		{"pxs", "psx"},
		{"ps", "psx"},
		{"snez", "snes"},
		{"totally-unrelated-name", ""},
		{"", ""},
	}
	for _, c := range cases {
		if got := Suggest(c.input, candidates); got != c.want {
			t.Errorf("Suggest(%q) = %q, want %q", c.input, got, c.want)
		}
	}
}
