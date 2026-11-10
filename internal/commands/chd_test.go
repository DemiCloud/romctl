package commands

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/demicloud/romctl/internal/actions"
	"github.com/demicloud/romctl/internal/cliutil"
	"github.com/demicloud/romctl/internal/library"
	"github.com/demicloud/romctl/internal/systemdb"
)

// writeFakeCDAsDVDImageCHD writes a minimal v5 CHD header plus one CHT2
// metadata entry describing a single MODE1/2048 track - the exact shape
// internal/formats/chd.Classify recognizes as KindCDAsDVDMAGE (a DVD
// image mistakenly stored as a CD-type CHD), without needing a real
// chdman-built file.
func writeFakeCDAsDVDImageCHD(t *testing.T, path string) {
	t.Helper()
	const metaOffset = 124
	data := []byte("TRACK:1 TYPE:MODE1 SUBTYPE:NONE FRAMES:100")
	entry := make([]byte, 16)
	copy(entry[0:4], "CHT2")
	length := len(data)
	entry[5] = byte(length >> 16)
	entry[6] = byte(length >> 8)
	entry[7] = byte(length)

	header := make([]byte, metaOffset)
	copy(header[0:8], "MComprHD")
	binary.BigEndian.PutUint32(header[12:16], 5)
	binary.BigEndian.PutUint64(header[48:56], uint64(metaOffset))

	buf := append(header, entry...)
	buf = append(buf, data...)
	if err := os.WriteFile(path, buf, 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestFindByExtRecursesIntoSubfolders guards against a regression: many
// redump/no-intro archives wrap a disc image's .cue/.bin pair in one
// extra per-game subfolder (e.g. "Game/Game.cue"), which convertGameDir
// used to miss entirely (a plain top-level os.ReadDir), reporting
// "no cue/gdi/iso in: <dir>" even though the files were right there one
// level down.
func TestFindByExtRecursesIntoSubfolders(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "Some Game (USA)")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	cue := filepath.Join(nested, "Some Game (USA).cue")
	if err := os.WriteFile(cue, []byte("cue"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, "Some Game (USA).bin"), []byte("bin"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := findByExt(root, ".cue", ".gdi")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != cue {
		t.Fatalf("findByExt = %v, want [%s]", got, cue)
	}
}

// TestFindByExtMultiDisc confirms disc images nested under separate
// per-disc subfolders are all found, not just the first.
func TestFindByExtMultiDisc(t *testing.T) {
	root := t.TempDir()
	var want []string
	for i := 1; i <= 2; i++ {
		dir := filepath.Join(root, "Disc"+string(rune('0'+i)))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		cue := filepath.Join(dir, "game.cue")
		if err := os.WriteFile(cue, []byte("cue"), 0o644); err != nil {
			t.Fatal(err)
		}
		want = append(want, cue)
	}

	got, err := findByExt(root, ".cue", ".gdi")
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(got)
	sort.Strings(want)
	if len(got) != len(want) {
		t.Fatalf("findByExt = %v, want %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("findByExt = %v, want %v", got, want)
		}
	}
}

// TestConvertGameDirNamesMisfiledCartridgeROM guards against a real
// support report: a Genesis/Mega Drive cartridge ROM archived as a lone
// .bin (no .cue/.gdi/.iso) sitting in a CD-class system's directory (e.g.
// roms/segacd/) used to fail with an opaque "no cue/gdi/iso in: <dir>",
// giving no clue that the file doesn't belong there at all.
func TestConvertGameDirNamesMisfiledCartridgeROM(t *testing.T) {
	dir := t.TempDir()
	data := make([]byte, 0x200)
	copy(data[0x100:], "SEGA MEGA DRIVE ")
	if err := os.WriteFile(filepath.Join(dir, "ALTERED_.bin"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := convertGameDir("segacd", systemdb.ClassCD, dir, dir, "ALTERED_")
	if err == nil {
		t.Fatal("convertGameDir succeeded, want an error naming the misfiled ROM")
	}
	if !strings.Contains(err.Error(), "Genesis/Mega Drive ROM") {
		t.Fatalf("convertGameDir error = %q, want it to name the cartridge ROM", err)
	}
}

// TestCHDRefusesMisfiledCartridgeROM guards the real failure mode that
// prompted signatureMismatch: a bare, loose file sitting directly in a
// CD-class system's directory (no subfolder, so convertGameDir's own
// cartridgeHint never runs) whose content identifies as a cartridge ROM
// must be refused before chdman ever sees it, not handed to chdman for an
// opaque failure (or worse, a "successful" CHD of garbage data).
func TestCHDRefusesMisfiledCartridgeROM(t *testing.T) {
	root := t.TempDir()
	segaDir := filepath.Join(root, "roms", "segacd")
	if err := os.MkdirAll(segaDir, 0o755); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(segaDir, "ALTERED_.iso")
	data := make([]byte, 0x200)
	copy(data[0x100:], "SEGA MEGA DRIVE ")
	if err := os.WriteFile(src, data, 0o644); err != nil {
		t.Fatal(err)
	}
	lib := library.Open(root)

	ok, err := CHD(lib, &actions.Runner{Format: cliutil.FormatPlain}, []string{"segacd"}, actions.SchemeNoTag)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Error("CHD() reported success, want it to report the misfiled ROM as a failure")
	}
	if !exists(src) {
		t.Error("source file should have been left alone, not converted")
	}
	if exists(filepath.Join(segaDir, "ALTERED_.chd")) {
		t.Error("a CHD should never have been produced from the misfiled ROM")
	}
}

// TestCHDDryRunReencodesNestedM3UCHD guards the DVD re-encode pass
// actually looking inside a Title.m3u/ playlist directory for a CD-type-
// mistake CHD, not just among a system's top-level files - previously a
// multi-disc PS2/PSP game already grouped into a playlist was invisible
// to this pass entirely, so it could never get fixed once grouped.
// --dry-run only: the plan: line below comes from chd.Classify (pure Go,
// no chdman needed); actually performing the re-encode does need a real
// chdman, same as every other chdman-backed path in this test file.
func TestCHDDryRunReencodesNestedM3UCHD(t *testing.T) {
	root := t.TempDir()
	m3uDir := filepath.Join(root, "roms", "ps2", "Game.m3u")
	if err := os.MkdirAll(m3uDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFakeCDAsDVDImageCHD(t, filepath.Join(m3uDir, "Game (Disc 01).chd"))
	if err := os.WriteFile(filepath.Join(m3uDir, "Game.m3u"), []byte("Game (Disc 01).chd\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	lib := library.Open(root)

	runner := &actions.Runner{Format: cliutil.FormatPlain, DryRun: true}
	out := captureStdout(t, func() {
		if _, err := CHD(lib, runner, []string{"ps2"}, actions.SchemeKeep); err != nil {
			t.Fatal(err)
		}
	})
	want := "re-encode ps2/Game.m3u/Game (Disc 01).chd as DVD CHD"
	if !strings.Contains(out, want) {
		t.Errorf("expected a plan to re-encode the nested disc (%q), got:\n%s", want, out)
	}
}
