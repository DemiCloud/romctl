package xiso

import (
	"os"
	"path/filepath"
	"testing"
)

func writeAt(t *testing.T, offset int64, data []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.iso")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := f.Truncate(offset + int64(len(data))); err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteAt(data, offset); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestIsXISO(t *testing.T) {
	path := writeAt(t, magicOffset, magic)
	ok, err := IsXISO(path)
	if err != nil {
		t.Fatalf("IsXISO: %v", err)
	}
	if !ok {
		t.Fatal("IsXISO = false, want true")
	}
}

func TestIsXISOWrongMagic(t *testing.T) {
	path := writeAt(t, magicOffset, []byte("NOT*AN*XBOX*IMAGE*AT*ALL"))
	ok, err := IsXISO(path)
	if err != nil {
		t.Fatalf("IsXISO: %v", err)
	}
	if ok {
		t.Fatal("IsXISO = true, want false")
	}
}

func TestIsXISOTooShort(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tiny.iso")
	if err := os.WriteFile(path, []byte("short"), 0o644); err != nil {
		t.Fatal(err)
	}
	ok, err := IsXISO(path)
	if err != nil {
		t.Fatalf("IsXISO: %v", err)
	}
	if ok {
		t.Fatal("IsXISO = true, want false (file shorter than magicOffset)")
	}
}
