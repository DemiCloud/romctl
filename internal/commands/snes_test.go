package commands

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/demicloud/romctl/internal/actions"
	"github.com/demicloud/romctl/internal/library"
)

// TestSNESRefusesMisfiledCartridgeROM guards the same failure mode as
// chd's signatureMismatch check, for snes: a cartridge ROM from another
// system, misfiled into roms/snes/, must never have its header "stripped"
// (an actual content mutation) just because its size happens to land on
// the same 512-byte boundary a real SNES copier header would.
func TestSNESRefusesMisfiledCartridgeROM(t *testing.T) {
	root := t.TempDir()
	snesDir := filepath.Join(root, "roms", "snes")
	if err := os.MkdirAll(snesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// 1536 % 1024 == 512: looks exactly like a headered SNES ROM by size,
	// but its content is a Genesis cartridge header, not SNES at all.
	data := make([]byte, 1536)
	copy(data[0x100:], "SEGA MEGA DRIVE ")
	src := filepath.Join(snesDir, "Game.smc")
	if err := os.WriteFile(src, data, 0o644); err != nil {
		t.Fatal(err)
	}
	lib := library.Open(root)

	if err := SNES(lib, &actions.Runner{}, []string{"snes"}); err != nil {
		t.Fatal(err)
	}
	if !exists(src) {
		t.Error("source file should have been left alone, not stripped/renamed")
	}
	got, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(data) {
		t.Errorf("file content should not have been modified, got length %d, want %d", len(got), len(data))
	}
}
