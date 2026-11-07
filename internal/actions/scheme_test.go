package actions

import "testing"

func TestParseScheme(t *testing.T) {
	cases := map[string]Scheme{
		"":          SchemeKeep,
		"notag":     SchemeNoTag,
		"NOTAG":     SchemeNoTag,
		"keep":      SchemeKeep,
		"no-intro":  SchemeNoIntro,
		"nointro":   SchemeNoIntro,
		"No-Intro":  SchemeNoIntro,
		"redump":    SchemeRedump,
		"  redump ": SchemeRedump,
	}
	for in, want := range cases {
		got, err := ParseScheme(in)
		if err != nil {
			t.Errorf("ParseScheme(%q) unexpected error: %v", in, err)
		}
		if got != want {
			t.Errorf("ParseScheme(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParseSchemeInvalid(t *testing.T) {
	if _, err := ParseScheme("tosec"); err == nil {
		t.Fatal("expected an error for an unknown scheme")
	}
}

func TestApplySchemeKeep(t *testing.T) {
	in := "Sonic CD (USA) [!]"
	if got := ApplyScheme(in, SchemeKeep); got != in {
		t.Errorf("ApplyScheme(keep) = %q, want unchanged %q", got, in)
	}
}

func TestApplySchemeNoTag(t *testing.T) {
	if got := ApplyScheme("Sonic CD (USA) [!]", SchemeNoTag); got != "Sonic CD" {
		t.Errorf("ApplyScheme(notag) = %q", got)
	}
}

func TestApplySchemeNoIntro(t *testing.T) {
	cases := map[string]string{
		"Super Mario World (Japan) (Rev 1)": "Super Mario World (Japan) (Rev 1)",
		"Super Mario World (Rev 1) (Japan)": "Super Mario World (Japan) (Rev 1)",
		"Sonic CD (USA) [!]":                "Sonic CD (USA)",
		"The Legend of Zelda (USA)":         "Legend of Zelda, The (USA)",
		"Pokemon Crystal (USA, Europe)":     "Pokemon Crystal (USA, Europe)",
		"Game (Disc 1) (USA)":               "Game (USA) (Disc 1)",
		"Game (Weird Flag) (USA)":           "Game (USA) (Weird Flag)",
		"Game":                              "Game",
	}
	for in, want := range cases {
		if got := ApplyScheme(in, SchemeNoIntro); got != want {
			t.Errorf("ApplyScheme(%q, no-intro) = %q, want %q", in, got, want)
		}
	}
}

func TestApplySchemeRedump(t *testing.T) {
	cases := map[string]string{
		"Final Fantasy VII (Disc 1) (USA)": "Final Fantasy VII (USA) (Disc 1)",
		"Game (USA) (Disc 2) (Rev 1)":      "Game (USA) (Disc 2) (Rev 1)",
		"Game (Rev 1) (Disc 2) (USA)":      "Game (USA) (Disc 2) (Rev 1)",
	}
	for in, want := range cases {
		if got := ApplyScheme(in, SchemeRedump); got != want {
			t.Errorf("ApplyScheme(%q, redump) = %q, want %q", in, got, want)
		}
	}
}
