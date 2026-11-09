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

	"github.com/demicloud/romctl/internal/actions"
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

func TestIdentifyRenamesMatchedFileThroughScheme(t *testing.T) {
	content := "gb-rom-bytes"
	lib, sysDir := newTestLib(t, map[string]string{"scrambled_name.gb": content})
	datDir := t.TempDir()
	writeIdentifyDat(t, datDir, "Pocket Monsters (Japan) (Rev 1)", []byte(content), "")

	runner := &actions.Runner{Format: cliutil.FormatPlain}
	ok, err := Identify(lib, runner, []string{"gb"}, datDir, false, actions.SchemeNoTag, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Error("Identify reported failure")
	}

	if _, err := os.Stat(filepath.Join(sysDir, "scrambled_name.gb")); err == nil {
		t.Error("original scrambled filename should no longer exist")
	}
	// SchemeNoTag runs the DAT name through CleanName, which strips both
	// "(Japan)" and "(Rev 1)" (neither is "(Disc N)").
	want := filepath.Join(sysDir, "Pocket Monsters.gb")
	if _, err := os.Stat(want); err != nil {
		t.Errorf("expected renamed file at %s: %v", want, err)
	}
}

func TestIdentifySchemeKeepPreservesDatTags(t *testing.T) {
	content := "gb-rom-bytes-2"
	lib, sysDir := newTestLib(t, map[string]string{"x.gb": content})
	datDir := t.TempDir()
	writeIdentifyDat(t, datDir, "Pocket Monsters (Japan) (Rev 1)", []byte(content), "")

	runner := &actions.Runner{Format: cliutil.FormatPlain}
	if _, err := Identify(lib, runner, []string{"gb"}, datDir, false, actions.SchemeKeep, 0); err != nil {
		t.Fatal(err)
	}

	want := filepath.Join(sysDir, "Pocket Monsters (Japan) (Rev 1).gb")
	if _, err := os.Stat(want); err != nil {
		t.Errorf("expected renamed file at %s (SchemeKeep leaves the DAT name verbatim): %v", want, err)
	}
}

func TestIdentifyLeavesUnmatchedFileAlone(t *testing.T) {
	lib, sysDir := newTestLib(t, map[string]string{"unrelated.gb": "totally-unrelated-content"})
	datDir := t.TempDir()
	writeIdentifyDat(t, datDir, "Some Known Game", []byte("different-content-entirely"), "")

	runner := &actions.Runner{Format: cliutil.FormatPlain}
	ok, err := Identify(lib, runner, []string{"gb"}, datDir, false, actions.SchemeKeep, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Error("Identify reported failure")
	}
	if _, err := os.Stat(filepath.Join(sysDir, "unrelated.gb")); err != nil {
		t.Errorf("unmatched file should be left exactly as it was: %v", err)
	}
}

func TestIdentifyCollisionSkipped(t *testing.T) {
	content := "gb-rom-bytes-3"
	// Both the messy original and a file already sitting at the target
	// name exist - must not overwrite the latter.
	lib, sysDir := newTestLib(t, map[string]string{
		"messy_name.gb": content,
		"Clean Game.gb": "pre-existing-different-content",
	})
	datDir := t.TempDir()
	writeIdentifyDat(t, datDir, "Clean Game", []byte(content), "")

	runner := &actions.Runner{Format: cliutil.FormatPlain}
	if _, err := Identify(lib, runner, []string{"gb"}, datDir, false, actions.SchemeKeep, 0); err != nil {
		t.Fatal(err)
	}

	// messy_name.gb must still exist (rename was skipped, not performed).
	if _, err := os.Stat(filepath.Join(sysDir, "messy_name.gb")); err != nil {
		t.Errorf("source file should remain when the target collides: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(sysDir, "Clean Game.gb"))
	if err != nil {
		t.Fatalf("pre-existing target file should still exist: %v", err)
	}
	if string(got) != "pre-existing-different-content" {
		t.Error("pre-existing target file must not be overwritten")
	}
}

func TestIdentifySkipsMultiROMMatch(t *testing.T) {
	contentA, contentB := "chip-a-bytes", "chip-b-bytes"
	lib, sysDir := newTestLib(t, map[string]string{
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

	runner := &actions.Runner{Format: cliutil.FormatPlain}
	if _, err := Identify(lib, runner, []string{"gb"}, datDir, false, actions.SchemeKeep, 0); err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{"chip_a.gb", "chip_b.gb"} {
		if _, err := os.Stat(filepath.Join(sysDir, name)); err != nil {
			t.Errorf("a multi-ROM match must leave every file alone, %s missing: %v", name, err)
		}
	}
}

func TestIdentifyDryRunRenamesNothing(t *testing.T) {
	content := "gb-rom-bytes-4"
	lib, sysDir := newTestLib(t, map[string]string{"scrambled.gb": content})
	datDir := t.TempDir()
	writeIdentifyDat(t, datDir, "Clean Game", []byte(content), "")

	runner := &actions.Runner{Format: cliutil.FormatPlain, DryRun: true}
	if _, err := Identify(lib, runner, []string{"gb"}, datDir, false, actions.SchemeKeep, 0); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(sysDir, "scrambled.gb")); err != nil {
		t.Errorf("dry-run must not rename anything: %v", err)
	}
	if _, err := os.Stat(filepath.Join(sysDir, "Clean Game.gb")); err == nil {
		t.Error("dry-run must not create the renamed file")
	}
}

func TestIdentifySkipsArcade(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "roms", "arcade"), 0o755); err != nil {
		t.Fatal(err)
	}
	lib := library.Open(root)
	runner := &actions.Runner{Format: cliutil.FormatPlain}
	ok, err := Identify(lib, runner, []string{"arcade"}, "", false, actions.SchemeKeep, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Error("Identify reported failure for an arcade system, which it should skip entirely")
	}
}

// TestIdentifyPrintsResultsTable guards the human-readable summary every
// non-structured run should get: a table naming the matched file, the
// game it was identified as, and the hash that proved it - not just the
// final "N renamed" one-liner.
func TestIdentifyPrintsResultsTable(t *testing.T) {
	content := []byte("gb-rom-bytes-table")
	lib, _ := newTestLib(t, map[string]string{"messy.gb": string(content)})
	datDir := t.TempDir()
	writeIdentifyDat(t, datDir, "Clean Game", content, "")

	runner := &actions.Runner{Format: cliutil.FormatPlain}
	out := captureStdout(t, func() {
		if _, err := Identify(lib, runner, []string{"gb"}, datDir, false, actions.SchemeKeep, 0); err != nil {
			t.Fatal(err)
		}
	})

	if !strings.Contains(out, "messy.gb") || !strings.Contains(out, "Clean Game") {
		t.Errorf("expected the results table to list the matched file and game, got:\n%s", out)
	}
	wantHash := strings.ToUpper(fmt.Sprintf("%08x", crc32.ChecksumIEEE(content)))
	if !strings.Contains(out, wantHash) {
		t.Errorf("expected the results table to include the file's CRC32 %s, got:\n%s", wantHash, out)
	}
}

// TestIdentifyLimitTruncatesPerSystem guards --limit: a system with more
// matches than the limit should still rename everything (the limit only
// caps the printed table), and note how many more rows it held back.
func TestIdentifyLimitTruncatesPerSystem(t *testing.T) {
	c1, c2, c3 := []byte("limit-content-one"), []byte("limit-content-two"), []byte("limit-content-three")
	lib, sysDir := newTestLib(t, map[string]string{
		"messy1.gb": string(c1),
		"messy2.gb": string(c2),
		"messy3.gb": string(c3),
	})
	datDir := t.TempDir()
	extra := gbGameXML("Game Two", c2) + gbGameXML("Game Three", c3)
	writeIdentifyDat(t, datDir, "Game One", c1, extra)

	runner := &actions.Runner{Format: cliutil.FormatPlain}
	out := captureStdout(t, func() {
		if _, err := Identify(lib, runner, []string{"gb"}, datDir, false, actions.SchemeKeep, 2); err != nil {
			t.Fatal(err)
		}
	})

	if !strings.Contains(out, "1 more match(es) not shown (--limit 2)") {
		t.Errorf("expected a truncation note for the 3rd match, got:\n%s", out)
	}
	for _, want := range []string{"Game One.gb", "Game Two.gb", "Game Three.gb"} {
		if !exists(filepath.Join(sysDir, want)) {
			t.Errorf("--limit must not stop a match from actually being renamed, missing %s", want)
		}
	}
}
