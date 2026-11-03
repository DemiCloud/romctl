package archive

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func buildZip(t *testing.T, files map[string]string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.zip")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	w := zip.NewWriter(f)
	for name, content := range files {
		entry, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestZipEntriesAndExtract(t *testing.T) {
	path := buildZip(t, map[string]string{"game.rom": "rom-bytes"})

	single, err := IsSingleFile(path)
	if err != nil || !single {
		t.Fatalf("IsSingleFile = %v, %v; want true, nil", single, err)
	}

	dest := t.TempDir()
	if err := ExtractAll(path, dest); err != nil {
		t.Fatalf("ExtractAll: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(dest, "game.rom"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, []byte("rom-bytes")) {
		t.Fatalf("got %q, want %q", got, "rom-bytes")
	}
}

func TestZipMultiFileNotSingle(t *testing.T) {
	path := buildZip(t, map[string]string{"a.rom": "a", "b.rom": "b"})
	single, err := IsSingleFile(path)
	if err != nil || single {
		t.Fatalf("IsSingleFile = %v, %v; want false, nil", single, err)
	}
}

// TestExtractAllRejectsPathTraversal locks in ExtractAll's zip-slip guard:
// an entry whose relative path climbs out of destDir (e.g. "../../etc/x")
// must be rejected rather than written outside destDir.
func TestExtractAllRejectsPathTraversal(t *testing.T) {
	path := buildZip(t, map[string]string{"../../../escaped.rom": "pwned"})
	dest := t.TempDir()
	if err := ExtractAll(path, dest); err == nil {
		t.Fatal("ExtractAll: expected a traversal error, got nil")
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(dest), "escaped.rom")); err == nil {
		t.Fatal("ExtractAll: entry escaped destDir")
	}
}

// TestExtractAllContainsAbsolutePathEntry confirms an absolute-path entry
// name doesn't escape destDir either: filepath.Join strips the leading
// separator, so it lands inside destDir rather than at the literal
// filesystem root.
func TestExtractAllContainsAbsolutePathEntry(t *testing.T) {
	path := buildZip(t, map[string]string{"/etc/contained.rom": "contained"})
	dest := t.TempDir()
	if err := ExtractAll(path, dest); err != nil {
		t.Fatalf("ExtractAll: %v", err)
	}
	if _, err := os.Stat("/etc/contained.rom"); err == nil {
		t.Fatal("ExtractAll: entry escaped to filesystem root")
	}
	if _, err := os.Stat(filepath.Join(dest, "etc", "contained.rom")); err != nil {
		t.Fatalf("expected entry contained under destDir: %v", err)
	}
}
