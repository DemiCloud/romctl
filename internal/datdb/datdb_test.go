package datdb

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/demicloud/romctl/internal/systemdb"
)

const sampleDat = `<?xml version="1.0"?>
<datafile>
  <header>
    <name>Nintendo - Game Boy</name>
    <version>20241101-000000</version>
    <date>2024-11-01</date>
  </header>
  <game name="Some Game (World)">
    <rom name="Some Game (World).gb" size="32768" crc="DEADBEEF" md5="0123456789abcdef0123456789abcdef" sha1="0123456789abcdef0123456789abcdef01234567"/>
  </game>
  <game name="Other Game (USA)">
    <rom name="Other Game (USA).gb" size="32768" crc="CAFEF00D" md5="fedcba9876543210fedcba9876543210fedcba9" sha1="fedcba9876543210fedcba9876543210fedcba98765"/>
  </game>
</datafile>`

func TestParse(t *testing.T) {
	df, err := Parse([]byte(sampleDat))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if df.Header.Name != "Nintendo - Game Boy" {
		t.Errorf("Header.Name = %q", df.Header.Name)
	}
	if df.Header.Version != "20241101-000000" {
		t.Errorf("Header.Version = %q", df.Header.Version)
	}
	if df.Header.Date != "2024-11-01" {
		t.Errorf("Header.Date = %q", df.Header.Date)
	}
	if len(df.Games) != 2 {
		t.Fatalf("len(Games) = %d, want 2", len(df.Games))
	}
	g := df.Games[0]
	if g.Name != "Some Game (World)" || len(g.ROMs) != 1 {
		t.Fatalf("Games[0] = %+v", g)
	}
	// CRC/MD5/SHA1 are lowercased on parse, since local hashing always
	// produces lowercase hex and comparisons must match case-insensitively
	// without re-lowercasing on every lookup.
	if g.ROMs[0].CRC != "deadbeef" {
		t.Errorf("ROMs[0].CRC = %q, want lowercased", g.ROMs[0].CRC)
	}
}

func TestMatch(t *testing.T) {
	df, err := Parse([]byte(sampleDat))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	byCRC := map[string]string{"deadbeef": "some_game.gb"}
	results := df.Match(byCRC, nil, nil)
	if len(results) != 2 {
		t.Fatalf("len(results) = %d, want 2", len(results))
	}
	if !results[0].Have || results[0].Matched[0] != "some_game.gb" {
		t.Errorf("results[0] = %+v, want Have with some_game.gb", results[0])
	}
	if results[1].Have {
		t.Errorf("results[1] = %+v, want not Have", results[1])
	}
}

func TestMatchRequiresEveryROM(t *testing.T) {
	// A game with two ROMs (e.g. a multi-chip cart) only counts as Have
	// when every one of its ROMs is matched.
	multiDat := `<datafile><header><name>x</name></header>
<game name="Multi"><rom name="a" size="1" crc="aaaaaaaa"/><rom name="b" size="1" crc="bbbbbbbb"/></game>
</datafile>`
	df, err := Parse([]byte(multiDat))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	onlyFirst := map[string]string{"aaaaaaaa": "a.bin"}
	if results := df.Match(onlyFirst, nil, nil); results[0].Have {
		t.Fatalf("Match with only one of two ROMs present reported Have: %+v", results[0])
	}
	both := map[string]string{"aaaaaaaa": "a.bin", "bbbbbbbb": "b.bin"}
	if results := df.Match(both, nil, nil); !results[0].Have {
		t.Fatalf("Match with both ROMs present did not report Have: %+v", results[0])
	}
}

func TestLoadUnknownSystem(t *testing.T) {
	df, ok, err := Load("not-a-real-system", "")
	if ok || err != nil || df != nil {
		t.Fatalf("Load(unknown) = %v, %v, %v, want nil, false, nil", df, ok, err)
	}
}

func TestLoadOverrideDir(t *testing.T) {
	dir := t.TempDir()
	// "gb" maps to {"no-intro", "Nintendo - Game Boy"} in systemdb.Dat.
	src := filepath.Join(dir, "no-intro")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "Nintendo - Game Boy.dat"), []byte(sampleDat), 0o644); err != nil {
		t.Fatal(err)
	}

	df, ok, err := Load("gb", dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !ok {
		t.Fatal("Load: ok = false, want true (gb has a DatSource entry)")
	}
	if len(df.Games) != 2 {
		t.Fatalf("len(Games) = %d, want 2", len(df.Games))
	}
}

func TestLoadOverrideDirMissingFile(t *testing.T) {
	dir := t.TempDir()
	_, ok, err := Load("gb", dir)
	if !ok {
		t.Fatal("Load: ok = false, want true (gb has a mapping even if the file is missing)")
	}
	if err == nil {
		t.Fatal("Load: err = nil, want an error for a missing override file")
	}
}

// TestLoadSourceMatchesLoad guards LoadSource against drifting from
// Load's own behavior - identify's by-path lookup calls LoadSource
// directly (it has no romctl system name to look up), so the two must
// stay equivalent once a DatSource is in hand.
func TestLoadSourceMatchesLoad(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "no-intro")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "Nintendo - Game Boy.dat"), []byte(sampleDat), 0o644); err != nil {
		t.Fatal(err)
	}

	df, err := LoadSource(systemdb.DatSource{Provider: "no-intro", System: "Nintendo - Game Boy"}, dir)
	if err != nil {
		t.Fatalf("LoadSource: %v", err)
	}
	if len(df.Games) != 2 {
		t.Fatalf("len(Games) = %d, want 2", len(df.Games))
	}
}

func TestLoadSourceMissingFile(t *testing.T) {
	if _, err := LoadSource(systemdb.DatSource{Provider: "redump", System: "Sony - PlayStation"}, t.TempDir()); err == nil {
		t.Fatal("LoadSource: err = nil, want an error for a missing override file")
	}
}

// TestDistinctSourcesDedupes guards the whole reason DistinctSources
// exists: several romctl system keys (saturn/saturnjp, segacd/megacd/
// megacdjp, ...) share one real DatSource - identify's by-path lookup
// must only load and check each actual DAT once, not once per alias.
func TestDistinctSourcesDedupes(t *testing.T) {
	sources := DistinctSources()
	seen := map[systemdb.DatSource]bool{}
	for _, src := range sources {
		if seen[src] {
			t.Fatalf("duplicate DatSource in DistinctSources: %+v", src)
		}
		seen[src] = true
	}
	if !seen[systemdb.DatSource{Provider: "redump", System: "Sega - Saturn"}] {
		t.Fatal("expected the Sega - Saturn DatSource (saturn/saturnjp's shared pack) to be present")
	}
	if len(sources) >= len(systemdb.Dat) {
		t.Fatalf("len(sources) = %d, want fewer than len(systemdb.Dat) = %d (aliases should collapse)", len(sources), len(systemdb.Dat))
	}
}
