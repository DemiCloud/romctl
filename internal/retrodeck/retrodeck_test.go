package retrodeck

import (
	"os"
	"path/filepath"
	"testing"
)

// realRetrodeckJSON is a real retrodeck.json, pasted from an actual
// RetroDECK install, used to confirm ParseRDHomePath holds up against the
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
	got, err := ParseRDHomePath([]byte(realRetrodeckJSON))
	if err != nil {
		t.Fatal(err)
	}
	want := "/run/media/deck/8bcfc1e6-45d4-4def-890b-9a864228d7d6/retrodeck"
	if got != want {
		t.Errorf("ParseRDHomePath() = %q, want %q", got, want)
	}
}

func TestParseRDHomePathMissing(t *testing.T) {
	if _, err := ParseRDHomePath([]byte(`{"paths": {}}`)); err == nil {
		t.Fatal("expected an error when rd_home_path is missing")
	}
}

func TestParseRDHomePathInvalidJSON(t *testing.T) {
	if _, err := ParseRDHomePath([]byte(`not json`)); err == nil {
		t.Fatal("expected an error for invalid JSON")
	}
}

func TestResolveLocal(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	cfgDir := filepath.Join(home, ".var/app/net.retrodeck.retrodeck/config/retrodeck")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "retrodeck.json"), []byte(realRetrodeckJSON), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := ResolveLocal()
	if err != nil {
		t.Fatal(err)
	}
	want := "/run/media/deck/8bcfc1e6-45d4-4def-890b-9a864228d7d6/retrodeck"
	if got != want {
		t.Errorf("ResolveLocal() = %q, want %q", got, want)
	}
}

func TestResolveLocalMissing(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if _, err := ResolveLocal(); err == nil {
		t.Fatal("expected an error when retrodeck.json is absent")
	}
}
