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

// TestResolveSystemsAllExcludesDotDirectories guards a real gap found
// against a real library: RetroDECK keeps its own internal state in a
// roms/.sync/ directory (its favorites-sync staging file) - "all" must
// never treat that as a system the way it would any ordinary directory.
func TestResolveSystemsAllExcludesDotDirectories(t *testing.T) {
	root := t.TempDir()
	for _, dir := range []string{"roms/psx", "roms/.sync"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	lib := Open(root)

	systems, err := lib.ResolveSystems([]string{"all"})
	if err != nil {
		t.Fatal(err)
	}
	for _, sys := range systems {
		if sys == ".sync" {
			t.Errorf("ResolveSystems([\"all\"]) included .sync, want it excluded: %v", systems)
		}
	}
	found := false
	for _, sys := range systems {
		if sys == "psx" {
			found = true
		}
	}
	if !found {
		t.Errorf("ResolveSystems([\"all\"]) = %v, want psx included", systems)
	}
}

func TestESDESystemsMissingFile(t *testing.T) {
	lib := Open(t.TempDir())
	names, ok := lib.ESDESystems()
	if ok || names != nil {
		t.Errorf("ESDESystems() = (%v, %v), want (nil, false)", names, ok)
	}
}
