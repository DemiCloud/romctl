package sync

import "testing"

// realRetrodeckJSON is a real retrodeck.json, pasted from an actual
// RetroDECK install, used to confirm parseRDHomePath holds up against the
// real schema rather than a hand-guessed fixture.
const realRetrodeckJSON = `{
  "version": "0.10.9b",
  "paths": {
    "rd_home_path": "/run/media/deck/8bcfc1e6-45d4-4def-890b-9a864228d7d6/retrodeck",
    "roms_path": "/run/media/deck/8bcfc1e6-45d4-4def-890b-9a864228d7d6/retrodeck/roms",
    "bios_path": "/run/media/deck/8bcfc1e6-45d4-4def-890b-9a864228d7d6/retrodeck/bios",
    "downloaded_media_path": "/run/media/deck/8bcfc1e6-45d4-4def-890b-9a864228d7d6/retrodeck/ES-DE/downloaded_media",
    "sdcard": "/run/media/deck/8bcfc1e6-45d4-4def-890b-9a864228d7d6"
  },
  "options": {}
}`

func TestParseRDHomePath(t *testing.T) {
	got, err := parseRDHomePath([]byte(realRetrodeckJSON))
	if err != nil {
		t.Fatal(err)
	}
	want := "/run/media/deck/8bcfc1e6-45d4-4def-890b-9a864228d7d6/retrodeck"
	if got != want {
		t.Errorf("parseRDHomePath() = %q, want %q", got, want)
	}
}

func TestParseRDHomePathMissing(t *testing.T) {
	if _, err := parseRDHomePath([]byte(`{"paths": {}}`)); err == nil {
		t.Fatal("expected an error when rd_home_path is missing")
	}
}

func TestParseRDHomePathInvalidJSON(t *testing.T) {
	if _, err := parseRDHomePath([]byte(`not json`)); err == nil {
		t.Fatal("expected an error for invalid JSON")
	}
}
