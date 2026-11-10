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

// writeDat writes a two-game Logiqx DAT to dir/no-intro/Nintendo - Game
// Boy.dat - "gb" maps there in systemdb.Dat - with haveROM's real
// hash (so the matching local file counts as Have) and a second,
// deliberately unmatched entry (Missing).
func writeDat(t *testing.T, dir string, haveContent []byte) {
	t.Helper()
	hc, hm, hs := crc32.NewIEEE(), md5.New(), sha1.New()
	hc.Write(haveContent)
	hm.Write(haveContent)
	hs.Write(haveContent)

	dat := fmt.Sprintf(`<datafile><header><name>Nintendo - Game Boy</name><version>test</version></header>
<game name="Have Game"><rom name="have.gb" size="%d" crc="%08x" md5="%x" sha1="%x"/></game>
<game name="Missing Game"><rom name="missing.gb" size="1" crc="ffffffff"/></game>
</datafile>`, len(haveContent), hc.Sum32(), hm.Sum(nil), hs.Sum(nil))

	datDir := filepath.Join(dir, "no-intro")
	if err := os.MkdirAll(datDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(datDir, "Nintendo - Game Boy.dat"), []byte(dat), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestCollection(t *testing.T) {
	root := t.TempDir()
	sysDir := filepath.Join(root, "roms", "gb")
	if err := os.MkdirAll(sysDir, 0o755); err != nil {
		t.Fatal(err)
	}
	haveContent := []byte("have-this-rom")
	if err := os.WriteFile(filepath.Join(sysDir, "have.gb"), haveContent, 0o644); err != nil {
		t.Fatal(err)
	}
	// Present but unrelated to either DAT entry - a .chd stub (no system
	// here is actually CHD-converted in practice, but Collection's logic
	// keys off extension alone, not systemdb.Class).
	if err := os.WriteFile(filepath.Join(sysDir, "Some Disc.chd"), []byte("chd-stub"), 0o644); err != nil {
		t.Fatal(err)
	}

	datDir := t.TempDir()
	writeDat(t, datDir, haveContent)

	lib := library.Open(root)
	out := captureStdout(t, func() {
		ok, err := Collection(lib, []string{"gb"}, datDir, false, false, true, nil, cliutil.FormatPlain)
		if err != nil {
			t.Fatal(err)
		}
		if !ok {
			t.Error("Collection reported failure")
		}
	})

	if !strings.Contains(out, "Have Game") || !strings.Contains(out, "have") {
		t.Errorf("expected Have Game to be reported as have, got:\n%s", out)
	}
	if !strings.Contains(out, "Missing Game") || !strings.Contains(out, "missing") {
		t.Errorf("expected Missing Game to be reported as missing with --missing, got:\n%s", out)
	}
	if !strings.Contains(out, "Some Disc.chd") || !strings.Contains(out, "unverifiable") {
		t.Errorf("expected the .chd file to be reported as unverifiable, got:\n%s", out)
	}
	wantHash := strings.ToUpper(fmt.Sprintf("%08x", crc32.ChecksumIEEE(haveContent)))
	if !strings.Contains(out, wantHash) {
		t.Errorf("expected the have row's Hash column to show the CRC32 %s, got:\n%s", wantHash, out)
	}
	if !strings.Contains(out, "Nintendo Game Boy") {
		t.Errorf("expected the System column to show the display name, not just the directory, got:\n%s", out)
	}
	if !strings.Contains(out, "gb") {
		t.Errorf("expected the Directory column to still show the roms/ directory name, got:\n%s", out)
	}
}

// TestCollectionHidesMissingByDefault guards --missing's whole point:
// most DATs list far more games than any one library has, so a plain
// `collection` run should only report have/unverifiable, not bury them
// under every game you don't own.
func TestCollectionHidesMissingByDefault(t *testing.T) {
	root := t.TempDir()
	sysDir := filepath.Join(root, "roms", "gb")
	if err := os.MkdirAll(sysDir, 0o755); err != nil {
		t.Fatal(err)
	}
	haveContent := []byte("have-this-rom-2")
	if err := os.WriteFile(filepath.Join(sysDir, "have.gb"), haveContent, 0o644); err != nil {
		t.Fatal(err)
	}
	datDir := t.TempDir()
	writeDat(t, datDir, haveContent)

	lib := library.Open(root)
	out := captureStdout(t, func() {
		ok, err := Collection(lib, []string{"gb"}, datDir, false, false, false, nil, cliutil.FormatPlain)
		if err != nil {
			t.Fatal(err)
		}
		if !ok {
			t.Error("Collection reported failure")
		}
	})

	if !strings.Contains(out, "Have Game") {
		t.Errorf("expected Have Game to still be reported by default, got:\n%s", out)
	}
	if strings.Contains(out, "Missing Game") {
		t.Errorf("expected Missing Game to be hidden without --missing, got:\n%s", out)
	}
}

func TestGameRegionTags(t *testing.T) {
	got := gameRegionTags("Donkey Kong (Europe, Australia) (Rev 1)")
	want := []string{"Europe", "Australia", "Rev 1"}
	if len(got) != len(want) {
		t.Fatalf("gameRegionTags() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("gameRegionTags() = %v, want %v", got, want)
		}
	}
}

func TestMatchesRegion(t *testing.T) {
	name := "Super Mario Land (USA, Europe)"
	if !matchesRegion(name, []string{"usa"}) {
		t.Errorf("expected %q to match region %q (case-insensitive)", name, "usa")
	}
	if matchesRegion(name, []string{"Japan"}) {
		t.Errorf("expected %q not to match region %q", name, "Japan")
	}
	if !matchesRegion(name, nil) {
		t.Error("expected an empty wanted list to match everything")
	}
}

// writeRegionDat writes a three-game Logiqx DAT, each tagged with a
// different region, all deliberately Missing (no local file hashes
// them) - TestCollectionFiltersByRegion only cares which Games --region
// lets through, not have/missing status.
func writeRegionDat(t *testing.T, dir string) {
	t.Helper()
	dat := `<datafile><header><name>Nintendo - Game Boy</name><version>test</version></header>
<game name="USA Game (USA)"><rom name="usa.gb" size="1" crc="11111111"/></game>
<game name="Europe Game (Europe)"><rom name="europe.gb" size="1" crc="22222222"/></game>
<game name="Japan Game (Japan)"><rom name="japan.gb" size="1" crc="33333333"/></game>
</datafile>`
	datDir := filepath.Join(dir, "no-intro")
	if err := os.MkdirAll(datDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(datDir, "Nintendo - Game Boy.dat"), []byte(dat), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestCollectionFiltersByRegion(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "roms", "gb"), 0o755); err != nil {
		t.Fatal(err)
	}
	datDir := t.TempDir()
	writeRegionDat(t, datDir)

	lib := library.Open(root)
	out := captureStdout(t, func() {
		ok, err := Collection(lib, []string{"gb"}, datDir, false, false, true, []string{"USA"}, cliutil.FormatPlain)
		if err != nil {
			t.Fatal(err)
		}
		if !ok {
			t.Error("Collection reported failure")
		}
	})

	if !strings.Contains(out, "USA Game") {
		t.Errorf("expected USA Game to be reported with --region USA, got:\n%s", out)
	}
	if strings.Contains(out, "Europe Game") || strings.Contains(out, "Japan Game") {
		t.Errorf("expected only the USA-tagged game with --region USA, got:\n%s", out)
	}
}

func TestCollectionUnknownSystemSkipped(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "roms", "arcade"), 0o755); err != nil {
		t.Fatal(err)
	}
	lib := library.Open(root)

	out := captureStdout(t, func() {
		ok, err := Collection(lib, []string{"arcade"}, "", false, false, false, nil, cliutil.FormatPlain)
		if err != nil {
			t.Fatal(err)
		}
		if !ok {
			t.Error("Collection reported failure for a system with no DAT coverage")
		}
	})
	if strings.Contains(out, "arcade") {
		t.Errorf("expected arcade (no DatSource entry, no files) to produce no rows, got:\n%s", out)
	}
}

// TestCollectionListsFilesForSystemWithNoDat is the fix for a system
// with no DAT coverage at all (e.g. `collection naomi`) silently
// returning nothing: it must still list what's actually on disk (as
// "no dat" rows, since there's no have/missing verdict to give them),
// and explain why on stderr rather than leaving an empty result
// indistinguishable from "you own nothing". naomi (not fbneo - see
// systemdb.Dat, fbneo has real coverage now) is a ClassArcade system
// with genuinely no DatSource entry.
func TestCollectionListsFilesForSystemWithNoDat(t *testing.T) {
	root := t.TempDir()
	sysDir := filepath.Join(root, "roms", "naomi")
	if err := os.MkdirAll(sysDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sysDir, "sf2.zip"), []byte("arcade-set"), 0o644); err != nil {
		t.Fatal(err)
	}
	lib := library.Open(root)

	out := captureStdout(t, func() {
		ok, err := Collection(lib, []string{"naomi"}, "", false, false, false, nil, cliutil.FormatPlain)
		if err != nil {
			t.Fatal(err)
		}
		if !ok {
			t.Error("Collection reported failure for a system with no DAT coverage")
		}
	})
	if !strings.Contains(out, "sf2") || !strings.Contains(out, "no dat") {
		t.Errorf("expected sf2.zip listed with a \"no dat\" status, got:\n%s", out)
	}

	errOut := captureStderr(t, func() {
		if _, err := Collection(lib, []string{"naomi"}, "", false, false, false, nil, cliutil.FormatPlain); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(errOut, "naomi") || !strings.Contains(errOut, "no DAT coverage") {
		t.Errorf("expected a stderr note explaining naomi has no DAT coverage, got:\n%s", errOut)
	}
}
