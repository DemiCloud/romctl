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
		ok, err := Collection(lib, []string{"gb"}, datDir, false, false, true, cliutil.FormatPlain)
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
		ok, err := Collection(lib, []string{"gb"}, datDir, false, false, false, cliutil.FormatPlain)
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

func TestCollectionUnknownSystemSkipped(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "roms", "arcade"), 0o755); err != nil {
		t.Fatal(err)
	}
	lib := library.Open(root)

	out := captureStdout(t, func() {
		ok, err := Collection(lib, []string{"arcade"}, "", false, false, false, cliutil.FormatPlain)
		if err != nil {
			t.Fatal(err)
		}
		if !ok {
			t.Error("Collection reported failure for a system with no DAT coverage")
		}
	})
	if strings.Contains(out, "arcade") {
		t.Errorf("expected arcade (no DatSource entry) to produce no rows, got:\n%s", out)
	}
}
