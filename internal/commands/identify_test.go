package commands

import (
	"crypto/md5"
	"crypto/sha1"
	"fmt"
	"hash/crc32"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/demicloud/romctl/internal/cliutil"
	"github.com/demicloud/romctl/internal/library"
)

// writeIdentifyDat writes a Game Boy DAT with one game (named gameName,
// matching haveContent's hash) plus any extraGames XML appended verbatim
// (for multi-ROM/collision fixtures).
func writeIdentifyDat(t *testing.T, dir, gameName string, haveContent []byte, extraGames string) {
	t.Helper()
	hc, hm, hs := crc32.NewIEEE(), md5.New(), sha1.New()
	hc.Write(haveContent)
	hm.Write(haveContent)
	hs.Write(haveContent)

	dat := fmt.Sprintf(`<datafile><header><name>Nintendo - Game Boy</name><version>test</version></header>
<game name="%s"><rom name="rom.gb" size="%d" crc="%08x" md5="%x" sha1="%x"/></game>
%s
</datafile>`, gameName, len(haveContent), hc.Sum32(), hm.Sum(nil), hs.Sum(nil), extraGames)

	datDir := filepath.Join(dir, "no-intro")
	if err := os.MkdirAll(datDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(datDir, "Nintendo - Game Boy.dat"), []byte(dat), 0o644); err != nil {
		t.Fatal(err)
	}
}

func newTestLib(t *testing.T, files map[string]string) (*library.Library, string) {
	t.Helper()
	root := t.TempDir()
	sysDir := filepath.Join(root, "roms", "gb")
	if err := os.MkdirAll(sysDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(sysDir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return library.Open(root), sysDir
}

// TestIdentifyMatchesFileAcrossAnySystem guards identify's main point:
// given an arbitrary path, it finds the match by searching every DAT -
// it's never told which system to check, unlike collection.
func TestIdentifyMatchesFileAcrossAnySystem(t *testing.T) {
	content := []byte("gb-rom-bytes")
	dir := t.TempDir()
	path := filepath.Join(dir, "scrambled_name.gb")
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}
	datDir := t.TempDir()
	writeIdentifyDat(t, datDir, "Pocket Monsters (Japan) (Rev 1)", content, "")

	out := captureStdout(t, func() {
		ok, err := Identify([]string{path}, datDir, cliutil.FormatPlain)
		if err != nil {
			t.Fatal(err)
		}
		if !ok {
			t.Error("Identify reported failure for a clean match")
		}
	})

	if !strings.Contains(out, path) || !strings.Contains(out, "Pocket Monsters (Japan) (Rev 1)") || !strings.Contains(out, "Nintendo - Game Boy") {
		t.Errorf("expected the path, game, and system in the output, got:\n%s", out)
	}
	wantHash := strings.ToUpper(fmt.Sprintf("%08x", crc32.ChecksumIEEE(content)))
	if !strings.Contains(out, wantHash) {
		t.Errorf("expected the CRC32 %s in the output, got:\n%s", wantHash, out)
	}
}

// TestIdentifyReportsNoMatch guards that an unmatched path still gets a
// row (System/Game blank) - it shouldn't just vanish from a multi-path
// run's output.
func TestIdentifyReportsNoMatch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mystery.gb")
	if err := os.WriteFile(path, []byte("totally-unrelated-content"), 0o644); err != nil {
		t.Fatal(err)
	}
	datDir := t.TempDir()
	writeIdentifyDat(t, datDir, "Some Known Game", []byte("different-content-entirely"), "")

	out := captureStdout(t, func() {
		ok, err := Identify([]string{path}, datDir, cliutil.FormatPlain)
		if err != nil {
			t.Fatal(err)
		}
		if !ok {
			t.Error("Identify reported failure for a run that simply found no match")
		}
	})

	if !strings.Contains(out, path) {
		t.Errorf("expected the unmatched path to still appear in the output, got:\n%s", out)
	}
	if strings.Contains(out, "Some Known Game") {
		t.Errorf("an unrelated DAT game must not show up as a match, got:\n%s", out)
	}
}

// TestIdentifyMissingPathReportsFailure guards a path that can't even be
// stat'd: identify reports failure and keeps going rather than aborting.
func TestIdentifyMissingPathReportsFailure(t *testing.T) {
	ok, err := Identify([]string{filepath.Join(t.TempDir(), "does-not-exist.gb")}, t.TempDir(), cliutil.FormatPlain)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Error("Identify should report failure for a path that doesn't exist")
	}
}

// TestIdentifyRejectsDirectory guards passing a directory instead of a
// file - a plain os.Stat success but nothing hashable.
func TestIdentifyRejectsDirectory(t *testing.T) {
	ok, err := Identify([]string{t.TempDir()}, t.TempDir(), cliutil.FormatPlain)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Error("Identify should report failure when given a directory")
	}
}
