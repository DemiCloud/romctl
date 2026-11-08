package commands

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/demicloud/romctl/internal/actions"
	"github.com/demicloud/romctl/internal/library"
)

// TestXisoRefusesMisfiledCartridgeROM guards the same failure mode as
// chd's signatureMismatch check, for xiso: a cartridge ROM misfiled into
// roms/xbox/ with an .iso extension must be refused before extract-xiso
// ever runs on it, instead of handing that tool a file it was never meant
// to read.
func TestXisoRefusesMisfiledCartridgeROM(t *testing.T) {
	root := t.TempDir()
	xboxDir := filepath.Join(root, "roms", "xbox")
	if err := os.MkdirAll(xboxDir, 0o755); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(xboxDir, "Game.iso")
	data := make([]byte, 0x200)
	copy(data[0x100:], "SEGA MEGA DRIVE ")
	if err := os.WriteFile(src, data, 0o644); err != nil {
		t.Fatal(err)
	}
	lib := library.Open(root)

	ok, err := Xiso(lib, &actions.Runner{}, []string{"xbox"})
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Error("Xiso() reported success, want it to report the misfiled ROM as a failure")
	}
	if !exists(src) {
		t.Error("source file should have been left alone")
	}
	if exists(filepath.Join(xboxDir, "Game.xiso")) {
		t.Error("an XISO should never have been produced from the misfiled ROM")
	}
}
