package library

import (
	"os"
	"path/filepath"
	"testing"
)

func TestESDESystemsParsesExistingFile(t *testing.T) {
	root := t.TempDir()
	content := "psx: Sony PlayStation\nsegacd: Sega CD\n"
	if err := os.WriteFile(filepath.Join(root, "systems.txt"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	lib := Open(root)
	names, ok := lib.ESDESystems()
	if !ok {
		t.Fatal("ESDESystems() ok = false, want true")
	}
	if names["psx"] != "Sony PlayStation" {
		t.Errorf("names[psx] = %q, want %q", names["psx"], "Sony PlayStation")
	}
	if names["segacd"] != "Sega CD" {
		t.Errorf("names[segacd] = %q, want %q", names["segacd"], "Sega CD")
	}
}

func TestESDESystemsMissingFile(t *testing.T) {
	lib := Open(t.TempDir())
	names, ok := lib.ESDESystems()
	if ok || names != nil {
		t.Errorf("ESDESystems() = (%v, %v), want (nil, false)", names, ok)
	}
}
