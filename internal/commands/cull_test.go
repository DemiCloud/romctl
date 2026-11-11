package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/demicloud/romctl/internal/actions"
	"github.com/demicloud/romctl/internal/library"
)

func cullLibrary(t *testing.T, sys string) *library.Library {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "roms", sys), 0o755); err != nil {
		t.Fatal(err)
	}
	return library.Open(root)
}

func TestCullRetiresLeftoverExtractionDir(t *testing.T) {
	lib := cullLibrary(t, "psx")
	leftover := filepath.Join(lib.Roms, "psx", "_extract.abc123")
	if err := os.MkdirAll(leftover, 0o755); err != nil {
		t.Fatal(err)
	}

	ok, err := Cull(lib, &actions.Runner{}, []string{"psx"})
	if err != nil || !ok {
		t.Fatalf("Cull() = (%v, %v), want (true, nil)", ok, err)
	}
	if exists(leftover) {
		t.Error("leftover extraction dir should have been retired")
	}
	if !exists(filepath.Join(lib.Roms, "psx", "_to_delete", "_extract.abc123")) {
		t.Error("expected the leftover dir in _to_delete/")
	}
}

func TestCullRetiresIncompleteTransferLeftover(t *testing.T) {
	lib := cullLibrary(t, "psx")
	leftover := filepath.Join(lib.Roms, "psx", "Game.chd.tmp.xyz")
	if err := os.WriteFile(leftover, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	ok, err := Cull(lib, &actions.Runner{}, []string{"psx"})
	if err != nil || !ok {
		t.Fatalf("Cull() = (%v, %v), want (true, nil)", ok, err)
	}
	if exists(leftover) {
		t.Error("incomplete-transfer leftover should have been retired")
	}
}

func TestCullRetiresCueLessBin(t *testing.T) {
	lib := cullLibrary(t, "psx")
	bin := filepath.Join(lib.Roms, "psx", "Lone Game (USA).bin")
	if err := os.WriteFile(bin, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	ok, err := Cull(lib, &actions.Runner{}, []string{"psx"})
	if err != nil || !ok {
		t.Fatalf("Cull() = (%v, %v), want (true, nil)", ok, err)
	}
	if exists(bin) {
		t.Error("cue-less bin should have been retired")
	}
	if !exists(filepath.Join(lib.Roms, "psx", "_to_delete", "Lone Game (USA).bin")) {
		t.Error("expected the bin in _to_delete/")
	}
}

func TestCullRetiresCorruptCHD(t *testing.T) {
	lib := cullLibrary(t, "psx")
	chd := filepath.Join(lib.Roms, "psx", "Broken Game.chd")
	if err := os.WriteFile(chd, []byte("not a chd at all"), 0o644); err != nil {
		t.Fatal(err)
	}

	ok, err := Cull(lib, &actions.Runner{}, []string{"psx"})
	if err != nil || !ok {
		t.Fatalf("Cull() = (%v, %v), want (true, nil)", ok, err)
	}
	if exists(chd) {
		t.Error("corrupt CHD should have been retired")
	}
}

// TestCullLeavesPairedBinAlone guards against a false positive: a .bin
// with a real matching .cue (even one referencing it by a different
// filename, same as check's own loadCueReferences) must never be
// retired - it's chd's job to convert it, not cull's to discard it.
func TestCullLeavesPairedBinAlone(t *testing.T) {
	lib := cullLibrary(t, "psx")
	psxDir := filepath.Join(lib.Roms, "psx")
	bin := filepath.Join(psxDir, "Paired Game.bin")
	if err := os.WriteFile(bin, fakeBinaryROM(), 0o644); err != nil {
		t.Fatal(err)
	}
	cue := "FILE \"Paired Game.bin\" BINARY\n  TRACK 01 MODE1/2048\n    INDEX 01 00:00:00\n"
	if err := os.WriteFile(filepath.Join(psxDir, "Paired Game.cue"), []byte(cue), 0o644); err != nil {
		t.Fatal(err)
	}

	ok, err := Cull(lib, &actions.Runner{}, []string{"psx"})
	if err != nil || !ok {
		t.Fatalf("Cull() = (%v, %v), want (true, nil)", ok, err)
	}
	if !exists(bin) {
		t.Error("a bin paired with a real cue sheet should never be retired")
	}
}

func TestCullDryRunLeavesEverythingInPlace(t *testing.T) {
	lib := cullLibrary(t, "psx")
	bin := filepath.Join(lib.Roms, "psx", "Lone Game (USA).bin")
	if err := os.WriteFile(bin, fakeBinaryROM(), 0o644); err != nil {
		t.Fatal(err)
	}

	ok, err := Cull(lib, &actions.Runner{DryRun: true}, []string{"psx"})
	if err != nil || !ok {
		t.Fatalf("Cull() = (%v, %v), want (true, nil)", ok, err)
	}
	if !exists(bin) {
		t.Error("--dry-run should never actually retire anything")
	}
}

// TestCullFlagsRatherThanRetiresNestedM3UDisc guards a deliberate
// scope limit: a disc already grouped inside a Title.m3u/ directory is
// never auto-retired even when it matches a cull-worthy finding,
// since removing just one disc would leave the playlist referencing a
// file that's gone - reported for manual review instead.
func TestCullFlagsRatherThanRetiresNestedM3UDisc(t *testing.T) {
	lib := cullLibrary(t, "psx")
	m3uDir := filepath.Join(lib.Roms, "psx", "Game.m3u")
	if err := os.MkdirAll(m3uDir, 0o755); err != nil {
		t.Fatal(err)
	}
	disc1 := filepath.Join(m3uDir, "Game (Disc 01).chd")
	disc2 := filepath.Join(m3uDir, "Game (Disc 02).chd")
	if err := os.WriteFile(disc1, []byte("not a real chd"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(disc2, []byte("not a real chd either"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(m3uDir, "Game.m3u"), []byte("Game (Disc 01).chd\nGame (Disc 02).chd\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	runner := &actions.Runner{Verbose: true}
	out := captureStdout(t, func() {
		ok, err := Cull(lib, runner, []string{"psx"})
		if err != nil || !ok {
			t.Fatalf("Cull() = (%v, %v), want (true, nil)", ok, err)
		}
	})
	if !exists(disc1) || !exists(disc2) {
		t.Error("a disc already grouped inside a Title.m3u/ directory should never be auto-retired")
	}
	if !strings.Contains(out, "Flagged for manual review") || !strings.Contains(out, "Game.m3u/Game (Disc 01).chd") {
		t.Errorf("expected the nested disc to be flagged instead, got:\n%s", out)
	}
}

func TestCullFlagsUnmanagedFormatForDiscBasedSystem(t *testing.T) {
	lib := cullLibrary(t, "psx")
	// .nrg (Nero disc image) is not in Duckstation's own documented
	// supported-format list - unlike .PBP, which is (see
	// TestCullDoesNotFlagVerifiedRealFormats below).
	nrg := filepath.Join(lib.Roms, "psx", "Some Game.nrg")
	if err := os.WriteFile(nrg, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	out := captureStdout(t, func() {
		ok, err := Cull(lib, &actions.Runner{}, []string{"psx"})
		if err != nil || !ok {
			t.Fatalf("Cull() = (%v, %v), want (true, nil)", ok, err)
		}
	})
	if !exists(nrg) {
		t.Error("an unmanaged format should never be retired, only flagged")
	}
	if !strings.Contains(out, "Some Game.nrg") || !strings.Contains(out, "unrecognized format") {
		t.Errorf("expected the .nrg file to be flagged as an unmanaged format, got:\n%s", out)
	}
}

// TestCullDoesNotFlagVerifiedRealFormats guards the real fix this
// session: .PBP (psx) and .cdi (dreamcast) are both directly supported
// by the actual emulator RetroDECK documents for each system
// (Duckstation, Flycast) - confirmed against RetroDECK's own
// wiki_emulator_guides pages, not romctl's own guess at what it can
// convert - so neither should ever be flagged as "unmanaged" at all.
func TestCullDoesNotFlagVerifiedRealFormats(t *testing.T) {
	lib := cullLibrary(t, "psx")
	pbp := filepath.Join(lib.Roms, "psx", "Some Game.PBP")
	if err := os.WriteFile(pbp, fakeBinaryROM(), 0o644); err != nil {
		t.Fatal(err)
	}
	dreamDir := filepath.Join(lib.Root, "roms", "dreamcast")
	if err := os.MkdirAll(dreamDir, 0o755); err != nil {
		t.Fatal(err)
	}
	cdi := filepath.Join(dreamDir, "Some Game.cdi")
	if err := os.WriteFile(cdi, fakeBinaryROM(), 0o644); err != nil {
		t.Fatal(err)
	}

	out := captureStdout(t, func() {
		ok, err := Cull(lib, &actions.Runner{}, []string{"psx", "dreamcast"})
		if err != nil || !ok {
			t.Fatalf("Cull() = (%v, %v), want (true, nil)", ok, err)
		}
	})
	if strings.Contains(out, "Flagged") {
		t.Errorf(".PBP (psx) and .cdi (dreamcast) are both verified-supported formats, should never be flagged, got:\n%s", out)
	}
}

// fakeBinaryROM is a small, ROM-shaped byte payload for cartridge-class
// tests: mostly zeroed with a non-text byte present, so it reliably
// fails http.DetectContentType's text sniff the same way real ROM data
// does (see textLikeFile), unlike a short literal string such as "x"
// (which IS valid, if meaningless, text).
func fakeBinaryROM() []byte {
	buf := make([]byte, 64)
	copy(buf, "FAKE ROM HEADER")
	buf[32] = 0x00
	buf[33] = 0xFF
	buf[34] = 0x01
	return buf
}

func TestCullDoesNotFlagCartridgeExtensionsAsUnmanaged(t *testing.T) {
	lib := cullLibrary(t, "snes")
	rom := filepath.Join(lib.Roms, "snes", "Some Game.sfc")
	if err := os.WriteFile(rom, fakeBinaryROM(), 0o644); err != nil {
		t.Fatal(err)
	}

	out := captureStdout(t, func() {
		ok, err := Cull(lib, &actions.Runner{}, []string{"snes"})
		if err != nil || !ok {
			t.Fatalf("Cull() = (%v, %v), want (true, nil)", ok, err)
		}
	})
	if strings.Contains(out, "Flagged") {
		t.Errorf("cartridge-shaped systems have no closed extension set and shouldn't be flagged at all, got:\n%s", out)
	}
}

// TestCullFlagsTextFileMasqueradingAsROM guards the real case extension
// matching can never catch on its own: Sega Genesis's own ".md" ROM
// extension is identical to Markdown's, and no fixed "known extensions"
// list could tell them apart - only content can.
func TestCullFlagsTextFileMasqueradingAsROM(t *testing.T) {
	lib := cullLibrary(t, "genesis")
	notes := filepath.Join(lib.Roms, "genesis", "notes.md")
	if err := os.WriteFile(notes, []byte("# Notes\n\nThis is a markdown file, not a Genesis ROM.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	out := captureStdout(t, func() {
		ok, err := Cull(lib, &actions.Runner{}, []string{"genesis"})
		if err != nil || !ok {
			t.Fatalf("Cull() = (%v, %v), want (true, nil)", ok, err)
		}
	})
	if !exists(notes) {
		t.Error("a text-like file should only ever be flagged, never retired")
	}
	if !strings.Contains(out, "notes.md") || !strings.Contains(out, "looks like a text file") {
		t.Errorf("expected notes.md to be flagged as text-like despite its ROM-shaped extension, got:\n%s", out)
	}
}

// TestCullDoesNotFlagRealGenesisROM is
// TestCullFlagsTextFileMasqueradingAsROM's negative case: a real binary
// ROM using the exact same ".md" extension must never be flagged, even
// though the extension alone is indistinguishable from Markdown's.
func TestCullDoesNotFlagRealGenesisROM(t *testing.T) {
	lib := cullLibrary(t, "genesis")
	rom := filepath.Join(lib.Roms, "genesis", "Some Game.md")
	if err := os.WriteFile(rom, fakeBinaryROM(), 0o644); err != nil {
		t.Fatal(err)
	}

	out := captureStdout(t, func() {
		ok, err := Cull(lib, &actions.Runner{}, []string{"genesis"})
		if err != nil || !ok {
			t.Fatalf("Cull() = (%v, %v), want (true, nil)", ok, err)
		}
	})
	if strings.Contains(out, "Flagged") {
		t.Errorf("a real binary ROM with the .md extension should never be flagged, got:\n%s", out)
	}
}

// TestCullIgnoresEmptyPlaceholderFiles guards a real RetroDECK
// convention: an empty file (same idea as .directory) is a routine
// placeholder, not a suspicious stray text file - even though
// http.DetectContentType's own default for zero bytes is "text/plain".
func TestCullIgnoresEmptyPlaceholderFiles(t *testing.T) {
	lib := cullLibrary(t, "genesis")
	placeholder := filepath.Join(lib.Roms, "genesis", "Some Game.md")
	if err := os.WriteFile(placeholder, nil, 0o644); err != nil {
		t.Fatal(err)
	}

	out := captureStdout(t, func() {
		ok, err := Cull(lib, &actions.Runner{}, []string{"genesis"})
		if err != nil || !ok {
			t.Fatalf("Cull() = (%v, %v), want (true, nil)", ok, err)
		}
	})
	if strings.Contains(out, "Flagged") {
		t.Errorf("an empty placeholder file should never be flagged, got:\n%s", out)
	}
}

// realPSXSystemInfo is a real roms/psx/systeminfo.txt, as RetroDECK/
// ES-DE actually generates it - confirmed against a real library, not
// invented for the test.
const realPSXSystemInfo = `System name:
psx

Full system name:
Sony PlayStation

Supported file extensions:
.bin .BIN .cbn .CBN .ccd .CCD .chd .CHD .cue .CUE .ecm .ECM .exe .EXE .img .IMG .iso .ISO .m3u .M3U .mdf .MDF .mds .MDS .minipsf .MINIPSF .pbp .PBP .psexe .PSEXE .psf .PSF .toc .TOC .z .Z .znx .ZNX .7z .7Z .zip .ZIP

Launch command:
%EMULATOR_RETROARCH% -L %CORE_RETROARCH%/swanstation_libretro.so %ROM%

Alternative launch commands:
%EMULATOR_RETROARCH% -L %CORE_RETROARCH%/mednafen_psx_libretro.so %ROM%
%EMULATOR_RETROARCH% -L %CORE_RETROARCH%/mednafen_psx_hw_libretro.so %ROM%
%EMULATOR_RETROARCH% -L %CORE_RETROARCH%/pcsx_rearmed_libretro.so %ROM%
%EMULATOR_DUCKSTATION% -batch %ROM%

Platform (for scraping):
psx

Theme folder:
psx
`

func TestSystemInfoKnownExtsParsesRealFile(t *testing.T) {
	lib := cullLibrary(t, "psx")
	if err := os.WriteFile(filepath.Join(lib.Roms, "psx", "systeminfo.txt"), []byte(realPSXSystemInfo), 0o644); err != nil {
		t.Fatal(err)
	}

	exts, ok := systemInfoKnownExts(lib, "psx")
	if !ok {
		t.Fatal("systemInfoKnownExts() ok = false, want true")
	}
	for _, want := range []string{"bin", "cue", "pbp", "mdf", "znx", "zip"} {
		if !exts[want] {
			t.Errorf("exts[%q] = false, want true (from the real systeminfo.txt list)", want)
		}
	}
	if exts["nrg"] {
		t.Error(`exts["nrg"] = true, want false - not in the real list`)
	}
}

func TestSystemInfoKnownExtsNoFileIsNotOk(t *testing.T) {
	lib := cullLibrary(t, "psx")
	if _, ok := systemInfoKnownExts(lib, "psx"); ok {
		t.Error("systemInfoKnownExts() ok = true with no systeminfo.txt present, want false")
	}
}

// TestCullPrefersSystemInfoOverHardcodedTables guards the real fix this
// session: knownExtsFor must prefer a real, locally-present
// systeminfo.txt over knownFormatsPerSystem/classKnownExts, since it
// reflects whatever core this library's own ES-DE install actually has
// configured (confirmed against a real one: psx's default launch core
// there is swanstation, not Duckstation, which knownFormatsPerSystem's
// own psx entry was verified against) - the two can genuinely disagree.
func TestCullPrefersSystemInfoOverHardcodedTables(t *testing.T) {
	lib := cullLibrary(t, "psx")
	psxDir := filepath.Join(lib.Roms, "psx")
	// .mdf is real (listed in realPSXSystemInfo) but isn't in
	// knownFormatsPerSystem's own psx entry - if systeminfo.txt weren't
	// actually being preferred, this would be (wrongly) flagged.
	if err := os.WriteFile(filepath.Join(psxDir, "systeminfo.txt"), []byte(realPSXSystemInfo), 0o644); err != nil {
		t.Fatal(err)
	}
	mdf := filepath.Join(psxDir, "Some Game.mdf")
	if err := os.WriteFile(mdf, fakeBinaryROM(), 0o644); err != nil {
		t.Fatal(err)
	}

	out := captureStdout(t, func() {
		ok, err := Cull(lib, &actions.Runner{}, []string{"psx"})
		if err != nil || !ok {
			t.Fatalf("Cull() = (%v, %v), want (true, nil)", ok, err)
		}
	})
	if strings.Contains(out, "Flagged") {
		t.Errorf(".mdf is real per this library's own systeminfo.txt, should not be flagged, got:\n%s", out)
	}
}

// TestCullDoesNotFlagCueSheets guards textFormatExts: a .cue sheet is
// legitimately plain text, and flagging it as "looks like a text file"
// would be correct but useless noise - it's supposed to be text.
func TestCullDoesNotFlagCueSheets(t *testing.T) {
	lib := cullLibrary(t, "psx")
	cue := filepath.Join(lib.Roms, "psx", "Some Game.cue")
	if err := os.WriteFile(cue, []byte("FILE \"Some Game.bin\" BINARY\n  TRACK 01 MODE1/2048\n    INDEX 01 00:00:00\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	out := captureStdout(t, func() {
		ok, err := Cull(lib, &actions.Runner{}, []string{"psx"})
		if err != nil || !ok {
			t.Fatalf("Cull() = (%v, %v), want (true, nil)", ok, err)
		}
	})
	if strings.Contains(out, "Flagged") {
		t.Errorf("a .cue sheet should never be flagged as text-like, got:\n%s", out)
	}
}

// TestIsLikelyTextRejectsUTF16BOMLikeBinaryData is the actual regression
// this session found against a real library: a real GBA ROM's own
// header starts with the exact byte pair (0xFE, 0xFF) http's own
// DetectContentType treats as an unconditional UTF-16 BOM, despite the
// rest of the sampled bytes being unambiguously binary (confirmed via a
// real hex dump) - isLikelyText must not make that same mistake.
func TestIsLikelyTextRejectsUTF16BOMLikeBinaryData(t *testing.T) {
	data := append([]byte{0xFE, 0xFF, 0x0E, 0xEA, 0x24, 0xFF}, fakeBinaryROM()...)
	if isLikelyText(data) {
		t.Error("isLikelyText() = true for real ROM-shaped binary data starting with a BOM-like byte pair, want false")
	}
}

func TestIsLikelyTextAcceptsRealText(t *testing.T) {
	if !isLikelyText([]byte("# Notes\n\nThis is a markdown file, not a ROM.\n")) {
		t.Error("isLikelyText() = false for genuine plain text, want true")
	}
}

// TestCullDoesNotFlagROMWithEmbeddedTextCredits is the second real
// regression this session found against a real library: some mid-90s
// EA/Tiburon cartridges embed a long plain-ASCII "help wanted"/
// developer-credits message at the very start of the ROM, ahead of any
// binary code - one real example's own text ran to byte 888 before the
// first control byte appeared. textSniffWindow (32 KiB) exists
// specifically so a sample never stops short of the real binary data
// that follows.
func TestCullDoesNotFlagROMWithEmbeddedTextCredits(t *testing.T) {
	lib := cullLibrary(t, "snes")
	credits := strings.Repeat("We develop entertainment software. ", 30) // > 888 bytes of plain text
	rom := append([]byte(credits), fakeBinaryROM()...)
	path := filepath.Join(lib.Roms, "snes", "Some Game.sfc")
	if err := os.WriteFile(path, rom, 0o644); err != nil {
		t.Fatal(err)
	}

	out := captureStdout(t, func() {
		ok, err := Cull(lib, &actions.Runner{}, []string{"snes"})
		if err != nil || !ok {
			t.Fatalf("Cull() = (%v, %v), want (true, nil)", ok, err)
		}
	})
	if strings.Contains(out, "Flagged") {
		t.Errorf("a real ROM with a long embedded text header should not be flagged once binary data follows, got:\n%s", out)
	}
}
