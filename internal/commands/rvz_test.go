package commands

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/demicloud/romctl/internal/actions"
	"github.com/demicloud/romctl/internal/library"
)

// TestRVZRefusesMisfiledCartridgeROM guards the same failure mode as
// chd's signatureMismatch check, for rvz: a cartridge ROM misfiled into
// roms/gc/ with an .iso extension must be refused before dolphin-tool
// ever runs on it.
func TestRVZRefusesMisfiledCartridgeROM(t *testing.T) {
	root := t.TempDir()
	gcDir := filepath.Join(root, "roms", "gc")
	if err := os.MkdirAll(gcDir, 0o755); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(gcDir, "Game.iso")
	data := make([]byte, 0x200)
	copy(data[0x100:], "SEGA MEGA DRIVE ")
	if err := os.WriteFile(src, data, 0o644); err != nil {
		t.Fatal(err)
	}
	lib := library.Open(root)

	ok, err := RVZ(lib, &actions.Runner{}, []string{"gc"})
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Error("RVZ() reported success, want it to report the misfiled ROM as a failure")
	}
	if !exists(src) {
		t.Error("source file should have been left alone")
	}
	if exists(filepath.Join(gcDir, "Game.rvz")) {
		t.Error("an RVZ should never have been produced from the misfiled ROM")
	}
}

func TestRVZOutputPathStripsNKitInfix(t *testing.T) {
	cases := map[string]string{
		"/roms/gc/Title (Disc 1).nkit.iso": "/roms/gc/Title (Disc 1).rvz",
		"/roms/gc/Title (Disc 1).iso":      "/roms/gc/Title (Disc 1).rvz",
		"/roms/wii/Title.NKit.iso":         "/roms/wii/Title.rvz",
	}
	for in, want := range cases {
		if got := rvzOutputPath(in); got != want {
			t.Errorf("rvzOutputPath(%q) = %q, want %q", in, got, want)
		}
	}
}
