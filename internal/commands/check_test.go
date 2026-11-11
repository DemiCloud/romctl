package commands

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/demicloud/romctl/internal/actions"
	"github.com/demicloud/romctl/internal/cliutil"
	"github.com/demicloud/romctl/internal/datdb"
	"github.com/demicloud/romctl/internal/library"
)

// captureStdout runs fn with os.Stdout redirected to a pipe and returns
// everything written to it - Check/Bios/List write straight to os.Stdout,
// so there's no writer to pass in instead.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	orig := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = orig }()

	fn()

	w.Close()
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

// captureStderr is captureStdout's os.Stderr counterpart, for diagnostics
// (e.g. printDatLoadIssues) that deliberately write there instead of
// os.Stdout.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	orig := os.Stderr
	os.Stderr = w
	defer func() { os.Stderr = orig }()

	fn()

	w.Close()
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

// scratchLibrary builds a minimal on-disk library covering a finding in
// each category the naming/structure/containers split (see check.go) now
// handles separately, plus a missing-BIOS finding: psx (class cd) gets a
// tagged filename (naming), a leftover extraction dir (structure), and a
// raw .cue disc image (containers) with no bios/ directory present at all
// (bios, via SkipBios rather than a missing-file Fail - simplest to set up
// and still exercises the bios phase).
func scratchLibrary(t *testing.T) *library.Library {
	t.Helper()
	root := t.TempDir()
	psxDir := filepath.Join(root, "roms", "psx")
	if err := os.MkdirAll(psxDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(psxDir, "Game (USA) [!].cue"), []byte("cue"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(psxDir, "_extract.leftover"), 0o755); err != nil {
		t.Fatal(err)
	}
	return library.Open(root)
}

func TestCheckNonVerboseOmitsFindingLines(t *testing.T) {
	lib := scratchLibrary(t)
	out := captureStdout(t, func() {
		Check(lib, []string{"psx"}, actions.SchemeNoTag, false, cliutil.FormatPlain, "")
	})
	if strings.Contains(out, "naming ->") {
		t.Errorf("non-verbose output should not include per-finding lines, got:\n%s", out)
	}
	if !strings.Contains(out, "CHECK") || !strings.Contains(out, "FIX") {
		t.Errorf("expected the Checks and Fixes tables, got:\n%s", out)
	}
}

func TestCheckVerboseIncludesFindingLines(t *testing.T) {
	lib := scratchLibrary(t)
	out := captureStdout(t, func() {
		Check(lib, []string{"psx"}, actions.SchemeNoTag, true, cliutil.FormatPlain, "")
	})
	if !strings.Contains(out, "naming ->") {
		t.Errorf("verbose output should include per-finding naming lines, got:\n%s", out)
	}
	if !strings.Contains(out, "leftover extraction directory") {
		t.Errorf("verbose output should include the structure finding, got:\n%s", out)
	}
	if !strings.Contains(out, "raw/archived disc image") {
		t.Errorf("verbose output should include the containers finding, got:\n%s", out)
	}
}

func TestCheckFixesNamesTheRootFlagWhenExplicit(t *testing.T) {
	lib := scratchLibrary(t)
	out := captureStdout(t, func() {
		Check(lib, []string{"psx"}, actions.SchemeNoTag, false, cliutil.FormatPlain, "-r "+lib.Root)
	})
	if !strings.Contains(out, "-r "+lib.Root) {
		t.Errorf("expected the Fixes section to include the explicit -r hint, got:\n%s", out)
	}
}

// TestCheckFlagsMisfiledCartridgeROM is a regression test for the real
// support report that prompted internal/formats/signature: a Genesis
// cartridge ROM (bare .bin, no .cue/.gdi/.iso alongside it) sitting in
// roms/segacd/ should be flagged with a "relocate" fix, not silently
// treated as just another disc-image candidate.
// TestCheckFlagsOutOfSchemeM3UDir guards checkNaming's handling of a
// Title.m3u/ multi-disc playlist directory as its own naming unit -
// previously checkNaming skipped every directory outright, so an
// out-of-scheme multi-disc game was never flagged at all, even though
// rename (renameM3UDir) now actually fixes it.
func TestCheckFlagsOutOfSchemeM3UDir(t *testing.T) {
	lib := scratchLibrary(t)
	m3uDir := filepath.Join(lib.Roms, "psx", "Final Fantasy VII (USA).m3u")
	if err := os.MkdirAll(m3uDir, 0o755); err != nil {
		t.Fatal(err)
	}

	out := captureStdout(t, func() {
		Check(lib, []string{"psx"}, actions.SchemeNoTag, true, cliutil.FormatPlain, "")
	})
	if !strings.Contains(out, "Final Fantasy VII (USA).m3u") || !strings.Contains(out, `naming -> "Final Fantasy VII.m3u"`) {
		t.Errorf("expected a naming finding for the out-of-scheme m3u directory, got:\n%s", out)
	}
	if !strings.Contains(out, "rename psx") {
		t.Errorf("expected the Fixes table to suggest rename for psx, got:\n%s", out)
	}
}

// TestCheckFlagsOutOfSchemePS3Dir is TestCheckFlagsOutOfSchemeM3UDir's
// counterpart for a Title.ps3/ disc-folder directory - the same "one
// directory is the naming unit" case, just for PS3 instead of a
// multi-disc playlist.
func TestCheckFlagsOutOfSchemePS3Dir(t *testing.T) {
	root := t.TempDir()
	m3uDir := filepath.Join(root, "roms", "ps3", "Some Game (USA).ps3")
	if err := os.MkdirAll(m3uDir, 0o755); err != nil {
		t.Fatal(err)
	}
	lib := library.Open(root)

	out := captureStdout(t, func() {
		Check(lib, []string{"ps3"}, actions.SchemeNoTag, true, cliutil.FormatPlain, "")
	})
	if !strings.Contains(out, "Some Game (USA).ps3") || !strings.Contains(out, `naming -> "Some Game.ps3"`) {
		t.Errorf("expected a naming finding for the out-of-scheme ps3 directory, got:\n%s", out)
	}
	if !strings.Contains(out, "rename ps3") {
		t.Errorf("expected the Fixes table to suggest rename for ps3, got:\n%s", out)
	}
}

// TestCheckFlagsInvalidCHDInsideM3UDir guards checkContainerDir's
// handling of a Title.m3u/ playlist directory: previously it returned
// immediately for any .m3u directory, so every disc inside an already-
// grouped multi-disc game was invisible to check's containers category -
// a corrupt CHD there could only ever be caught by the much heavier
// verify command.
func TestCheckFlagsInvalidCHDInsideM3UDir(t *testing.T) {
	lib := scratchLibrary(t)
	m3uDir := filepath.Join(lib.Roms, "psx", "Game.m3u")
	if err := os.MkdirAll(m3uDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(m3uDir, "Game (Disc 01).chd"), []byte("not a real chd"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(m3uDir, "Game.m3u"), []byte("Game (Disc 01).chd\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	out := captureStdout(t, func() {
		Check(lib, []string{"psx"}, actions.SchemeNoTag, true, cliutil.FormatPlain, "")
	})
	if !strings.Contains(out, "Game.m3u/Game (Disc 01).chd") || !strings.Contains(out, "not a valid CHD") {
		t.Errorf("expected the nested disc to be flagged as an invalid CHD, got:\n%s", out)
	}
}

// TestCheckFlagsCDAsDVDMistakeInsideM3UDir is the same gap as above, for
// checkCHD's DVD-class-specific finding: a disc inside a Title.m3u/
// directory that's actually a DVD image mistakenly stored as a CD-type
// CHD must still be flagged, with the same "chd" fix hint a top-level
// file would get (see TestCHDDryRunReencodesNestedM3UCHD in chd_test.go
// for the corresponding fix).
func TestCheckFlagsCDAsDVDMistakeInsideM3UDir(t *testing.T) {
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

	out := captureStdout(t, func() {
		Check(lib, []string{"ps2"}, actions.SchemeKeep, true, cliutil.FormatPlain, "")
	})
	if !strings.Contains(out, "Game.m3u/Game (Disc 01).chd") || !strings.Contains(out, "DVD image stored as CD-type CHD") {
		t.Errorf("expected the nested disc to be flagged as a CD-type-mistake CHD, got:\n%s", out)
	}
	if !strings.Contains(out, "chd ps2") {
		t.Errorf("expected the Fixes table to suggest chd for ps2, got:\n%s", out)
	}
}

// TestCheckContainersDoesNotDoubleCountM3UDirectory guards a real
// regression: checkContainers scans every top-level entry once before
// dispatching it, so when that dispatch (for a Title.m3u/ directory)
// went on to additionally scan each disc inside via checkM3uContainers,
// the directory's own scan on top of that double-counted it - inflating
// the Containers category's reported Pass count by one per multi-disc
// game, even though the directory itself never gets a pass/fail verdict
// of its own (only its discs do).
func TestCheckContainersDoesNotDoubleCountM3UDirectory(t *testing.T) {
	root := t.TempDir()
	m3uDir := filepath.Join(root, "roms", "psx", "Game.m3u")
	if err := os.MkdirAll(m3uDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Two discs that pass containers checks cleanly (KindCDAsDVDMAGE only
	// warns for a DVD-class system; psx is CD-class) - correctly counted,
	// this system's containers category should show Pass=2, not 3.
	writeFakeCDAsDVDImageCHD(t, filepath.Join(m3uDir, "Game (Disc 01).chd"))
	writeFakeCDAsDVDImageCHD(t, filepath.Join(m3uDir, "Game (Disc 02).chd"))
	if err := os.WriteFile(filepath.Join(m3uDir, "Game.m3u"), []byte("Game (Disc 01).chd\nGame (Disc 02).chd\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	lib := library.Open(root)

	out := captureStdout(t, func() {
		Check(lib, []string{"psx"}, actions.SchemeKeep, false, cliutil.FormatPlain, "")
	})
	for _, line := range strings.Split(out, "\n") {
		if !strings.Contains(line, "containers") {
			continue
		}
		fields := strings.Split(line, "|")
		if len(fields) < 4 {
			t.Fatalf("unexpected containers row shape: %q", line)
		}
		pass := strings.TrimSpace(fields[3])
		if pass != "2" {
			t.Errorf("containers Pass = %q, want %q (got double-counted if 3)", pass, "2")
		}
		return
	}
	t.Fatalf("no containers row found in output:\n%s", out)
}

// TestCheckM3uDirRecognizesNestedToDelete guards checkM3uDir's handling
// of a _to_delete/ directory nested inside a Title.m3u/ directory (left
// behind by chd's DVD re-encode pass, see TestCHDDryRunReencodesNestedM3UCHD):
// it must get the same dedicated "pending deletion" finding a top-level
// one gets, not be miscounted as an extra disc and reported as a
// confusing "N disc file(s) but M playlist entries" mismatch.
func TestCheckM3uDirRecognizesNestedToDelete(t *testing.T) {
	lib := scratchLibrary(t)
	m3uDir := filepath.Join(lib.Roms, "psx", "Game.m3u")
	if err := os.MkdirAll(filepath.Join(m3uDir, "_to_delete"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(m3uDir, "Game (Disc 01).chd"), []byte("disc"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(m3uDir, "Game.m3u"), []byte("Game (Disc 01).chd\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	out := captureStdout(t, func() {
		Check(lib, []string{"psx"}, actions.SchemeKeep, true, cliutil.FormatPlain, "")
	})
	if !strings.Contains(out, "Game.m3u/_to_delete") || !strings.Contains(out, "pending deletion") {
		t.Errorf("expected a dedicated pending-deletion finding for the nested _to_delete/, got:\n%s", out)
	}
	if strings.Contains(out, "disc file(s)") {
		t.Errorf("the nested _to_delete/ should not be miscounted as a disc, got:\n%s", out)
	}
}

func TestCheckFlagsMisfiledCartridgeROM(t *testing.T) {
	lib := scratchLibrary(t)
	segaDir := filepath.Join(lib.Roms, "segacd")
	if err := os.MkdirAll(segaDir, 0o755); err != nil {
		t.Fatal(err)
	}
	data := make([]byte, 0x200)
	copy(data[0x100:], "SEGA MEGA DRIVE ")
	if err := os.WriteFile(filepath.Join(segaDir, "ALTERED_.bin"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	out := captureStdout(t, func() {
		Check(lib, []string{"psx", "segacd"}, actions.SchemeNoTag, true, cliutil.FormatPlain, "")
	})
	if !strings.Contains(out, "identifies as a Genesis/Mega Drive ROM") {
		t.Errorf("expected the misfiled-ROM finding, got:\n%s", out)
	}
	if !strings.Contains(out, "relocate segacd") {
		t.Errorf("expected the Fixes table to suggest relocate for segacd specifically, got:\n%s", out)
	}
}

// TestCheckDoesNotFlagSegaCDBootSector guards the false positive
// signature.Identify exists to avoid: a legitimately-dumped Sega CD
// game's boot track reuses the Genesis cartridge header layout and must
// not be reported as misfiled.
func TestCheckDoesNotFlagSegaCDBootSector(t *testing.T) {
	lib := scratchLibrary(t)
	segaDir := filepath.Join(lib.Roms, "segacd")
	if err := os.MkdirAll(segaDir, 0o755); err != nil {
		t.Fatal(err)
	}
	data := make([]byte, 0x200)
	copy(data[0x100:], "SEGA")
	copy(data[0x180:], "BR")
	if err := os.WriteFile(filepath.Join(segaDir, "Game (Track 1).bin"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	out := captureStdout(t, func() {
		Check(lib, []string{"segacd"}, actions.SchemeNoTag, true, cliutil.FormatPlain, "")
	})
	if strings.Contains(out, "identifies as a") {
		t.Errorf("Sega CD boot sector should not be flagged as misfiled, got:\n%s", out)
	}
}

// TestCheckFlagsUnknownSystemDirectory guards the systems.txt cross-check:
// a roms/<dir> that neither systemdb nor the install's own ES-DE-generated
// systems.txt recognizes should be flagged - but only once systems.txt is
// actually present, since systemdb's hand-ported list can't make that
// call reliably on its own.
func TestCheckFlagsUnknownSystemDirectory(t *testing.T) {
	lib := scratchLibrary(t)
	bogusDir := filepath.Join(lib.Roms, "totally-bogus-system")
	if err := os.MkdirAll(bogusDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Without systems.txt, systemdb alone can't tell a typo apart from a
	// legitimate system it just doesn't know about yet - no finding.
	out := captureStdout(t, func() {
		Check(lib, []string{"psx", "totally-bogus-system"}, actions.SchemeNoTag, true, cliutil.FormatPlain, "")
	})
	if strings.Contains(out, "not a system directory") {
		t.Errorf("should not flag an unknown directory without systems.txt, got:\n%s", out)
	}

	if err := os.WriteFile(filepath.Join(lib.Root, "systems.txt"), []byte("psx: Sony PlayStation\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out = captureStdout(t, func() {
		Check(lib, []string{"psx", "totally-bogus-system"}, actions.SchemeNoTag, true, cliutil.FormatPlain, "")
	})
	if !strings.Contains(out, "not a system directory") {
		t.Errorf("expected the unknown-directory finding once systems.txt is present, got:\n%s", out)
	}
}

func TestCheckCategorySplitStillFindsEveryKind(t *testing.T) {
	// Regression check for the naming/structure/containers split: every
	// category that used to come out of one combined pass must still
	// report its finding.
	lib := scratchLibrary(t)
	out := captureStdout(t, func() {
		Check(lib, []string{"psx"}, actions.SchemeNoTag, true, cliutil.FormatPlain, "")
	})
	for _, want := range []string{
		"naming ->",                     // naming
		"leftover extraction directory", // structure
		"raw/archived disc image",       // containers
	} {
		if !strings.Contains(out, want) {
			t.Errorf("expected output to contain %q, got:\n%s", want, out)
		}
	}
}

func TestDiscGroupKeyStripsDiscAndTrackTags(t *testing.T) {
	cases := map[string]string{
		"Title (Disc 1)":                           "Title",
		"Title (USA) (Disc 2)":                     "Title (USA)",
		"Title (USA) (Disc 1) (Track 1)":           "Title (USA)",
		"Resident Evil 2 (USA) (Disc 1) (Track 1)": "Resident Evil 2 (USA)",
		"Resident Evil 2 (USA) (Disc 1) (Track 2)": "Resident Evil 2 (USA)",
	}
	for in, want := range cases {
		if got := discGroupKey(in); got != want {
			t.Errorf("discGroupKey(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestBuildDiscGroupsGroupsSiblingDiscsAndTracks(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{
		"Title (USA) (Disc 1).chd",
		"Title (USA) (Disc 2).chd",
		"Other (Disc 1) (Track 1).bin",
		"Other (Disc 1) (Track 2).bin",
	} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	groups := buildDiscGroups(entries)

	titleGroup := groups[discGroupKey("Title (USA) (Disc 1)")]
	if len(titleGroup) != 2 {
		t.Fatalf("Title group = %+v, want 2 entries (Disc 1, Disc 2)", titleGroup)
	}

	otherGroup := groups[discGroupKey("Other (Disc 1) (Track 1)")]
	if len(otherGroup) != 2 {
		t.Fatalf("Other group = %+v, want 2 entries (Track 1, Track 2 of the same disc)", otherGroup)
	}
	for _, f := range otherGroup {
		if f.num != 1 {
			t.Errorf("Other group entry %+v, want disc num 1 for both tracks", f)
		}
	}
}

// discSetTestDAT is a synthetic 2-disc Redump-shaped DAT: each disc is
// its own <game>, crc filled in per-test with the real CRC32 of whatever
// content that test writes to disk, so matchIncompleteDiscSet's hashing
// is exercised for real rather than mocked.
func discSetTestDAT(disc1CRC, disc2CRC string) string {
	return `<?xml version="1.0"?>
<datafile>
  <header><name>test</name></header>
  <game name="Test Game (USA) (Disc 1)">
    <rom name="Test Game (USA) (Disc 1).bin" size="4" crc="` + disc1CRC + `"/>
  </game>
  <game name="Test Game (USA) (Disc 2)">
    <rom name="Test Game (USA) (Disc 2).bin" size="4" crc="` + disc2CRC + `"/>
  </game>
</datafile>`
}

func TestMatchIncompleteDiscSetConfirmsMissingDisc(t *testing.T) {
	disc1 := []byte("disc one data")
	disc2 := []byte("disc two data")
	df, err := datdb.Parse([]byte(discSetTestDAT(crc32Hex(disc1), crc32Hex(disc2))))
	if err != nil {
		t.Fatal(err)
	}

	sysDir := t.TempDir()
	base := "Test Game (USA) (Disc 1).bin"
	if err := os.WriteFile(filepath.Join(sysDir, base), disc1, 0o644); err != nil {
		t.Fatal(err)
	}

	msg, confirmed := matchIncompleteDiscSet(df, sysDir, 1, []discFile{{num: 1, base: base}})
	if !confirmed {
		t.Fatal("matchIncompleteDiscSet() confirmed = false, want true (hash should match disc 1)")
	}
	if !strings.Contains(msg, "incomplete disc set") || !strings.Contains(msg, "missing 2") {
		t.Errorf("msg = %q, want it to mention the incomplete set and missing disc 2", msg)
	}
}

func TestMatchIncompleteDiscSetSingleDiscGameIsClean(t *testing.T) {
	// The DAT's matched title has no sibling disc entries at all (a
	// single-disc game whose local filename just happens to carry a
	// "(Disc 1)" tag) - confirmed, but nothing wrong to report.
	disc1 := []byte("solo disc data")
	df, err := datdb.Parse([]byte(`<?xml version="1.0"?>
<datafile><header><name>test</name></header>
<game name="Solo Game (USA)"><rom name="Solo Game (USA).bin" size="4" crc="` + crc32Hex(disc1) + `"/></game>
</datafile>`))
	if err != nil {
		t.Fatal(err)
	}

	sysDir := t.TempDir()
	base := "Solo Game (USA) (Disc 1).bin"
	if err := os.WriteFile(filepath.Join(sysDir, base), disc1, 0o644); err != nil {
		t.Fatal(err)
	}

	msg, confirmed := matchIncompleteDiscSet(df, sysDir, 1, []discFile{{num: 1, base: base}})
	if !confirmed {
		t.Fatal("matchIncompleteDiscSet() confirmed = false, want true")
	}
	if msg != "" {
		t.Errorf("msg = %q, want empty - DAT shows this is a single-disc game", msg)
	}
}

func TestMatchIncompleteDiscSetUnconfirmedWhenNoHashMatch(t *testing.T) {
	df, err := datdb.Parse([]byte(discSetTestDAT("deadbeef", "cafef00d")))
	if err != nil {
		t.Fatal(err)
	}

	sysDir := t.TempDir()
	base := "Test Game (USA) (Disc 1).bin"
	if err := os.WriteFile(filepath.Join(sysDir, base), []byte("unrelated content"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, confirmed := matchIncompleteDiscSet(df, sysDir, 1, []discFile{{num: 1, base: base}})
	if confirmed {
		t.Error("matchIncompleteDiscSet() confirmed = true, want false - no hash in the DAT matches this file")
	}
}

// TestCheckStillFlagsSiblingDiscPairAsNeedingM3U is a regression guard
// for the DAT-aware incomplete-disc-set check added alongside this: two
// loose discs of the same title, different disc numbers both present,
// must still get the plain, cheap "belongs in a .m3u directory" message
// - the DAT lookup is only for the no-sibling case.
func TestCheckStillFlagsSiblingDiscPairAsNeedingM3U(t *testing.T) {
	root := t.TempDir()
	psxDir := filepath.Join(root, "roms", "psx")
	if err := os.MkdirAll(psxDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"Game (USA) (Disc 1).chd", "Game (USA) (Disc 2).chd"} {
		if err := os.WriteFile(filepath.Join(psxDir, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	lib := library.Open(root)
	out := captureStdout(t, func() {
		Check(lib, []string{"psx"}, actions.SchemeNoTag, true, cliutil.FormatPlain, "")
	})
	if !strings.Contains(out, "belongs in a .m3u directory") {
		t.Errorf("expected the sibling-disc pair to still be flagged as needing m3u, got:\n%s", out)
	}
	if strings.Contains(out, "incomplete disc set") {
		t.Errorf("sibling-disc pair should not be flagged as an incomplete set, got:\n%s", out)
	}
}

// TestCheckFallsBackToM3UMessageWithNoDAT guards the fallback path: a
// lone disc-tagged file in a system with no DatSource entry at all must
// still get the old, unconditional "belongs in a .m3u directory"
// message - there's nothing to confirm or rule out without a DAT.
func TestCheckFallsBackToM3UMessageWithNoDAT(t *testing.T) {
	root := t.TempDir()
	sysDir := filepath.Join(root, "roms", "faketestsystem123")
	if err := os.MkdirAll(sysDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sysDir, "Lone Game (USA) (Disc 1).bin"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	lib := library.Open(root)
	out := captureStdout(t, func() {
		Check(lib, []string{"faketestsystem123"}, actions.SchemeNoTag, true, cliutil.FormatPlain, "")
	})
	if !strings.Contains(out, "belongs in a .m3u directory") {
		t.Errorf("expected the lone disc to fall back to the m3u message with no DAT available, got:\n%s", out)
	}
}

// TestCheckReportsLoneDiscOnceDespiteMultipleTrackFiles guards against
// double-reporting: a raw multi-track CD dump with no .cue sheet splits
// one disc into several files (see discTrackTagRE) - both share the
// same disc number, so they're one incomplete-set verdict, not two
// identical findings.
func TestCheckReportsLoneDiscOnceDespiteMultipleTrackFiles(t *testing.T) {
	root := t.TempDir()
	sysDir := filepath.Join(root, "roms", "faketestsystem123")
	if err := os.MkdirAll(sysDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{
		"Lone Game (USA) (Disc 1) (Track 1).bin",
		"Lone Game (USA) (Disc 1) (Track 2).bin",
	} {
		if err := os.WriteFile(filepath.Join(sysDir, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	lib := library.Open(root)
	out := captureStdout(t, func() {
		Check(lib, []string{"faketestsystem123"}, actions.SchemeNoTag, true, cliutil.FormatPlain, "")
	})
	if n := strings.Count(out, "belongs in a .m3u directory"); n != 1 {
		t.Errorf("expected exactly one finding for the two track files of one disc, got %d in:\n%s", n, out)
	}
}
