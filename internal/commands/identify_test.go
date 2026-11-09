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

// gbGameXML renders one more <game> entry for a Game Boy DAT, for tests
// that need several distinct matches in one file (writeIdentifyDat's own
// extraGames parameter) rather than just the one it sets up directly.
func gbGameXML(name string, content []byte) string {
	hc, hm, hs := crc32.NewIEEE(), md5.New(), sha1.New()
	hc.Write(content)
	hm.Write(content)
	hs.Write(content)
	return fmt.Sprintf(`<game name="%s"><rom name="%s.gb" size="%d" crc="%08x" md5="%x" sha1="%x"/></game>`,
		name, name, len(content), hc.Sum32(), hm.Sum(nil), hs.Sum(nil))
}

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

// TestIdentifyListsConfirmedMatchWithoutRenaming is identify's core
// contract: it reports a hash-confirmed match - file, game, hash - but
// never touches the filesystem. Renaming a confirmed match is `rename
// --verify-dat`'s job (see rename_test.go), not identify's.
func TestIdentifyListsConfirmedMatchWithoutRenaming(t *testing.T) {
	content := []byte("gb-rom-bytes")
	lib, sysDir := newTestLib(t, map[string]string{"scrambled_name.gb": string(content)})
	datDir := t.TempDir()
	writeIdentifyDat(t, datDir, "Pocket Monsters (Japan) (Rev 1)", content, "")

	out := captureStdout(t, func() {
		ok, err := Identify(lib, []string{"gb"}, datDir, false, false, cliutil.FormatPlain, 0)
		if err != nil {
			t.Fatal(err)
		}
		if !ok {
			t.Error("Identify reported failure")
		}
	})

	if _, err := os.Stat(filepath.Join(sysDir, "scrambled_name.gb")); err != nil {
		t.Errorf("identify must never rename anything, original file missing: %v", err)
	}
	if exists(filepath.Join(sysDir, "Pocket Monsters.gb")) || exists(filepath.Join(sysDir, "Pocket Monsters (Japan) (Rev 1).gb")) {
		t.Error("identify must never create a renamed file")
	}
	if !strings.Contains(out, "scrambled_name.gb") || !strings.Contains(out, "Pocket Monsters (Japan) (Rev 1)") {
		t.Errorf("expected the table to list the original filename and the raw DAT game name, got:\n%s", out)
	}
	wantHash := strings.ToUpper(fmt.Sprintf("%08x", crc32.ChecksumIEEE(content)))
	if !strings.Contains(out, wantHash) {
		t.Errorf("expected the table to include the file's CRC32 %s, got:\n%s", wantHash, out)
	}
}

func TestIdentifyLeavesUnmatchedFileAlone(t *testing.T) {
	lib, sysDir := newTestLib(t, map[string]string{"unrelated.gb": "totally-unrelated-content"})
	datDir := t.TempDir()
	writeIdentifyDat(t, datDir, "Some Known Game", []byte("different-content-entirely"), "")

	out := captureStdout(t, func() {
		ok, err := Identify(lib, []string{"gb"}, datDir, false, false, cliutil.FormatPlain, 0)
		if err != nil {
			t.Fatal(err)
		}
		if !ok {
			t.Error("Identify reported failure")
		}
	})
	if _, err := os.Stat(filepath.Join(sysDir, "unrelated.gb")); err != nil {
		t.Errorf("unmatched file should be left exactly as it was: %v", err)
	}
	if strings.Contains(out, "unrelated.gb") {
		t.Errorf("an unmatched file must not appear in the results table, got:\n%s", out)
	}
}

func TestIdentifySkipsMultiROMMatch(t *testing.T) {
	contentA, contentB := "chip-a-bytes", "chip-b-bytes"
	lib, _ := newTestLib(t, map[string]string{
		"chip_a.gb": contentA,
		"chip_b.gb": contentB,
	})
	datDir := t.TempDir()
	hc1, hm1, hs1 := crc32.ChecksumIEEE([]byte(contentA)), md5.Sum([]byte(contentA)), sha1.Sum([]byte(contentA))
	hc2, hm2, hs2 := crc32.ChecksumIEEE([]byte(contentB)), md5.Sum([]byte(contentB)), sha1.Sum([]byte(contentB))
	dat := fmt.Sprintf(`<datafile><header><name>Nintendo - Game Boy</name><version>test</version></header>
<game name="Two Chip Game">
<rom name="a.gb" size="%d" crc="%08x" md5="%x" sha1="%x"/>
<rom name="b.gb" size="%d" crc="%08x" md5="%x" sha1="%x"/>
</game>
</datafile>`, len(contentA), hc1, hm1, hs1, len(contentB), hc2, hm2, hs2)
	datDir2 := filepath.Join(datDir, "no-intro")
	if err := os.MkdirAll(datDir2, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(datDir2, "Nintendo - Game Boy.dat"), []byte(dat), 0o644); err != nil {
		t.Fatal(err)
	}

	out := captureStdout(t, func() {
		if _, err := Identify(lib, []string{"gb"}, datDir, false, false, cliutil.FormatPlain, 0); err != nil {
			t.Fatal(err)
		}
	})

	if strings.Contains(out, "Two Chip Game") {
		t.Errorf("a multi-ROM match spanning more than one file must not be listed, got:\n%s", out)
	}
}

func TestIdentifySkipsArcade(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "roms", "arcade"), 0o755); err != nil {
		t.Fatal(err)
	}
	lib := library.Open(root)
	ok, err := Identify(lib, []string{"arcade"}, "", false, false, cliutil.FormatPlain, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Error("Identify reported failure for an arcade system, which it should skip entirely")
	}
}

// TestIdentifyLimitTruncatesPerSystem guards --limit: it caps the table
// at N rows per system and notes how many more that system had.
func TestIdentifyLimitTruncatesPerSystem(t *testing.T) {
	c1, c2, c3 := []byte("limit-content-one"), []byte("limit-content-two"), []byte("limit-content-three")
	lib, _ := newTestLib(t, map[string]string{
		"messy1.gb": string(c1),
		"messy2.gb": string(c2),
		"messy3.gb": string(c3),
	})
	datDir := t.TempDir()
	extra := gbGameXML("Game Two", c2) + gbGameXML("Game Three", c3)
	writeIdentifyDat(t, datDir, "Game One", c1, extra)

	out := captureStdout(t, func() {
		if _, err := Identify(lib, []string{"gb"}, datDir, false, false, cliutil.FormatPlain, 2); err != nil {
			t.Fatal(err)
		}
	})

	if !strings.Contains(out, "1 more match(es) not shown (--limit 2)") {
		t.Errorf("expected a truncation note for the 3rd match, got:\n%s", out)
	}
}

// TestIdentifyNotesSkippedCHDWithoutVerifyCHD guards the "no output"
// surprise a CHD-converted library used to hit: identify silently skips
// every .chd by default (see the package doc comment), so a run over a
// library that's already all-CHD legitimately identifies nothing - but
// it should say why instead of looking exactly like a run that found no
// matches at all.
func TestIdentifyNotesSkippedCHDWithoutVerifyCHD(t *testing.T) {
	lib, sysDir := newTestLib(t, nil)
	psxDir := filepath.Join(filepath.Dir(sysDir), "psx")
	if err := os.MkdirAll(psxDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(psxDir, "Game.chd"), []byte("chd-stub"), 0o644); err != nil {
		t.Fatal(err)
	}
	datDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(datDir, "redump"), 0o755); err != nil {
		t.Fatal(err)
	}
	dat := `<datafile><header><name>Sony - PlayStation</name><version>test</version></header>
<game name="Game"><rom name="game.bin" size="8" crc="deadbeef" md5="00" sha1="00"/></game>
</datafile>`
	if err := os.WriteFile(filepath.Join(datDir, "redump", "Sony - PlayStation.dat"), []byte(dat), 0o644); err != nil {
		t.Fatal(err)
	}

	out := captureStdout(t, func() {
		ok, err := Identify(lib, []string{"psx"}, datDir, false, false, cliutil.FormatPlain, 0)
		if err != nil {
			t.Fatal(err)
		}
		if !ok {
			t.Error("Identify reported failure for a run that simply had nothing to verify")
		}
	})

	if !strings.Contains(out, "1 .chd file(s) not checked") || !strings.Contains(out, "--verify-chd") {
		t.Errorf("expected a note explaining the skipped .chd, got:\n%s", out)
	}
}
