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
