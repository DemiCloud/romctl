package actions

import "testing"

func TestCleanName(t *testing.T) {
	cases := map[string]string{
		"Sonic CD (USA) [!]":                    "Sonic CD",
		"Legend of Zelda, The - The Minish Cap": "Legend of Zelda, The - The Minish Cap",
		"The Legend of Zelda - The Minish Cap":  "Legend of Zelda, The - The Minish Cap",
		"The Legend of Zelda":                   "Legend of Zelda, The",
		"Final Fantasy VII (Disc 1)":            "Final Fantasy VII (Disc 1)",
		"Final Fantasy VII (Disc 1) [Track 01]": "Final Fantasy VII (Disc 1)",
		"A Boy and His Blob":                    "Boy and His Blob, A",
		"An American Tail":                      "American Tail, An",
		"The - Not An Article":                  "The - Not An Article",
		"Game   With    Extra   Spaces":         "Game With Extra Spaces",
	}
	for in, want := range cases {
		if got := CleanName(in); got != want {
			t.Errorf("CleanName(%q) = %q, want %q", in, got, want)
		}
	}
}
