package commands

import (
	"archive/zip"
	"fmt"
	"hash/crc32"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/demicloud/romctl/internal/cliutil"
	"github.com/demicloud/romctl/internal/library"
)

// writeZip writes a zip at path containing one entry per name->content
// pair - a stand-in for an arcade ROM set, which (unlike a cartridge or
// disc dump) holds one file per ROM chip rather than a single ROM.
func writeZip(t *testing.T, path string, files map[string][]byte) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	w := zip.NewWriter(f)
	for name, content := range files {
		e, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := e.Write(content); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
}

func crc32Hex(content []byte) string {
	return fmt.Sprintf("%08x", crc32.ChecksumIEEE(content))
}

// writeArcadeDat writes an FBNeo-shaped DAT (description child alongside
// the terse name attribute, no md5/sha1 - matching the real file's own
// shape) to dir/fbneo/FinalBurn Neo - Arcade Games.dat, with a "parent"
// game (self-contained) and a "clone" game whose shared ROM (bContent)
// is the same content as one of parent's own ROMs - mirroring a real
// split-set clone that relies on its parent's zip for a shared chip.
func writeArcadeDat(t *testing.T, dir string, aContent, bContent, cContent []byte) {
	t.Helper()
	dat := fmt.Sprintf(`<datafile><header><name>FinalBurn Neo - Arcade Games</name><version>test</version></header>
<game name="parent"><description>Parent Game</description><rom name="a.bin" size="%d" crc="%s"/><rom name="b.bin" size="%d" crc="%s"/></game>
<game name="clone"><description>Parent Game (Clone)</description><rom name="b.bin" size="%d" crc="%s"/><rom name="c.bin" size="%d" crc="%s"/></game>
</datafile>`,
		len(aContent), crc32Hex(aContent),
		len(bContent), crc32Hex(bContent),
		len(bContent), crc32Hex(bContent),
		len(cContent), crc32Hex(cContent))

	datDir := filepath.Join(dir, "fbneo")
	if err := os.MkdirAll(datDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(datDir, "FinalBurn Neo - Arcade Games.dat"), []byte(dat), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestCollectionArcadeParentClone is the point of fbneo DAT support: a
// clone set's zip is routinely missing ROMs that only live in its
// parent's zip (the real FBNeo DAT declares this via romof/cloneof/merge
// on ~74% of its games). collection resolves this for free by hashing
// every zip's entries into one shared per-system hash pool rather than
// one hash per zip - no romof/cloneof parsing needed - so both games
// must report "have" as long as both zips are present, and clone's own
// Matched column must list both zip names (proving the parent's zip
// really is what supplied the shared ROM).
func TestCollectionArcadeParentClone(t *testing.T) {
	aContent, bContent, cContent := []byte("rom-a-data"), []byte("rom-b-shared"), []byte("rom-c-clone-only")

	root := t.TempDir()
	sysDir := filepath.Join(root, "roms", "fbneo")
	if err := os.MkdirAll(sysDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeZip(t, filepath.Join(sysDir, "parent.zip"), map[string][]byte{"a.bin": aContent, "b.bin": bContent})
	writeZip(t, filepath.Join(sysDir, "clone.zip"), map[string][]byte{"c.bin": cContent})

	datDir := t.TempDir()
	writeArcadeDat(t, datDir, aContent, bContent, cContent)

	lib := library.Open(root)
	out := captureStdout(t, func() {
		ok, err := Collection(lib, []string{"fbneo"}, datDir, false, false, false, nil, cliutil.FormatPlain)
		if err != nil {
			t.Fatal(err)
		}
		if !ok {
			t.Error("Collection reported failure")
		}
	})

	if !strings.Contains(out, "Parent Game") || !strings.Contains(out, "have") {
		t.Errorf("expected Parent Game to report have, got:\n%s", out)
	}
	cloneLine := ""
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "Parent Game (Clone)") {
			cloneLine = line
		}
	}
	if cloneLine == "" || !strings.Contains(cloneLine, "have") {
		t.Errorf("expected clone to report have (its shared ROM comes from parent.zip), got:\n%s", out)
	}
	if !strings.Contains(cloneLine, "parent.zip") || !strings.Contains(cloneLine, "clone.zip") {
		t.Errorf("expected clone's Matched column to list both parent.zip and clone.zip, got: %s", cloneLine)
	}
	// Hash isn't meaningful for a multi-chip zip (see Collection's doc
	// comment) - confirm no per-chip CRC leaked into the row instead of "-".
	if strings.Contains(cloneLine, strings.ToUpper(crc32Hex(bContent))) || strings.Contains(cloneLine, strings.ToUpper(crc32Hex(cContent))) {
		t.Errorf("expected no per-chip CRC in an arcade row's Hash column, got: %s", cloneLine)
	}
}

// TestCollectionArcadeCloneMissingWithoutParent guards the other half:
// the shared hash pool must not over-match either - if the parent zip
// that actually holds the shared ROM isn't present, the clone is
// genuinely incomplete and must report missing, not a false have.
func TestCollectionArcadeCloneMissingWithoutParent(t *testing.T) {
	aContent, bContent, cContent := []byte("rom-a-data-2"), []byte("rom-b-shared-2"), []byte("rom-c-clone-only-2")

	root := t.TempDir()
	sysDir := filepath.Join(root, "roms", "fbneo")
	if err := os.MkdirAll(sysDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// parent.zip deliberately absent.
	writeZip(t, filepath.Join(sysDir, "clone.zip"), map[string][]byte{"c.bin": cContent})

	datDir := t.TempDir()
	writeArcadeDat(t, datDir, aContent, bContent, cContent)

	lib := library.Open(root)
	out := captureStdout(t, func() {
		ok, err := Collection(lib, []string{"fbneo"}, datDir, false, false, true, nil, cliutil.FormatPlain)
		if err != nil {
			t.Fatal(err)
		}
		if !ok {
			t.Error("Collection reported failure")
		}
	})

	cloneLine := ""
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "Parent Game (Clone)") {
			cloneLine = line
		}
	}
	if cloneLine == "" || !strings.Contains(cloneLine, "missing") {
		t.Errorf("expected clone to report missing without parent.zip present, got:\n%s", out)
	}
}

func TestHashArcadeEntries(t *testing.T) {
	files := map[string][]byte{"a.bin": []byte("chip-a"), "b.bin": []byte("chip-b-longer")}
	path := filepath.Join(t.TempDir(), "set.zip")
	writeZip(t, path, files)

	hashes, err := hashArcadeEntries(path)
	if err != nil {
		t.Fatalf("hashArcadeEntries: %v", err)
	}
	if len(hashes) != len(files) {
		t.Fatalf("len(hashes) = %d, want %d", len(hashes), len(files))
	}
	want := map[string]bool{crc32Hex([]byte("chip-a")): true, crc32Hex([]byte("chip-b-longer")): true}
	for _, h := range hashes {
		if !want[h.crc] {
			t.Errorf("unexpected crc %s in hashArcadeEntries result", h.crc)
		}
	}
}
