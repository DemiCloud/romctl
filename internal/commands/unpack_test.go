package commands

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"

	"github.com/demicloud/romctl/internal/actions"
	"github.com/demicloud/romctl/internal/library"
)

func buildZipFile(t *testing.T, path, innerName string, content []byte) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	w := zip.NewWriter(f)
	entry, err := w.Create(innerName)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := entry.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
}

// TestUnpackRefusesMisfiledCartridgeROM guards the same failure mode as
// chd's signatureMismatch check, for unpack: an archive's sole file is
// opaque until extracted, so the signature check has to run right after
// extraction and before the result is promoted into the library (and the
// archive retired) - a Genesis ROM archived into roms/nes/ must be left
// exactly as found, not silently unpacked under a now-bare, misleading
// name.
func TestUnpackRefusesMisfiledCartridgeROM(t *testing.T) {
	root := t.TempDir()
	nesDir := filepath.Join(root, "roms", "nes")
	if err := os.MkdirAll(nesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	data := make([]byte, 0x200)
	copy(data[0x100:], "SEGA MEGA DRIVE ")
	archivePath := filepath.Join(nesDir, "Game.zip")
	buildZipFile(t, archivePath, "Game.bin", data)

	lib := library.Open(root)
	ok, err := Unpack(lib, &actions.Runner{}, []string{"nes"})
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Error("Unpack() reported success, want it to report the misfiled ROM as a failure")
	}
	if !exists(archivePath) {
		t.Error("archive should have been left alone, not retired")
	}
	if exists(filepath.Join(nesDir, "Game.bin")) {
		t.Error("the misfiled ROM should never have been promoted out of the archive")
	}
}
