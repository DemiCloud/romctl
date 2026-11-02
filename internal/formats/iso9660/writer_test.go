package iso9660

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBuildAndRead(t *testing.T) {
	src := t.TempDir()
	mustMkdir(t, filepath.Join(src, "PS3_GAME", "USRDIR"))
	mustWrite(t, filepath.Join(src, "PS3_GAME", "USRDIR", "EBOOT.BIN"), []byte("eboot-contents"))
	mustWrite(t, filepath.Join(src, "PS3_GAME", "PARAM.SFO"), []byte("param-sfo-contents-longer-than-eboot"))

	outPath := filepath.Join(t.TempDir(), "test.iso")
	out, err := os.Create(outPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := Build(src, out, "TEST"); err != nil {
		t.Fatalf("Build: %v", err)
	}
	out.Close()

	ok, err := Complete(outPath)
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if !ok {
		t.Fatalf("Complete(%s) = false, want true", outPath)
	}
}

func mustMkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
}

func mustWrite(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}
