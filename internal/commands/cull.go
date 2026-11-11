package commands

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/demicloud/romctl/internal/actions"
	"github.com/demicloud/romctl/internal/audit"
	"github.com/demicloud/romctl/internal/cliutil"
	"github.com/demicloud/romctl/internal/library"
	"github.com/demicloud/romctl/internal/systemdb"
)

// cullMessages is the exact set of check findings (structure or
// containers category, matched by message substring) that mean a file
// is a confirmed dead end - not just unconverted, not just unverified,
// but something check has positively confirmed is broken or
// permanently stuck. Deliberately narrow: an old-version CHD (works
// fine, just not the current chdman format), a DVD-type CHD sitting in
// a CD-class system (misfiled, not necessarily broken - already its
// own plain `check` finding, not duplicated here), a PS3 installed/
// PSN-style folder (may well run fine as-is in RPCS3), and an
// incomplete disc set (the one disc that IS present is itself
// confirmed good - see `discs`/`collection --verify-chd`) are all left
// out on purpose.
var cullMessages = []string{
	"incomplete conversion leftover",
	"incomplete transfer/conversion leftover",
	"leftover extraction directory",
	"no matching .cue sheet",
	"not a valid CHD",
	"ISO is truncated or not ISO9660",
	"not a valid XISO",
	"no EBOOT.BIN found",
}

func cullWorthy(message string) bool {
	for _, m := range cullMessages {
		if strings.Contains(message, m) {
			return true
		}
	}
	return false
}

// knownFormatsPerSystem is, for a romctl system directory name Cull has
// directly verified against RetroDECK's own per-emulator "what file
// formats are supported" documentation
// (https://retrodeck.readthedocs.io/en/latest/wiki_emulator_guides/
// <emulator>/<emulator>-guide/ - the full list of 32 such pages, confirmed
// 2026-11-11 against the site's own sitemap.xml), the complete set of
// extensions that system's real emulator actually plays - not romctl's
// own guess at what it can convert. Replacing the guess with the real
// list caught two confirmed false positives against a real library:
// Duckstation (psx's documented emulator) lists .pbp as directly
// supported, and Flycast (dreamcast) lists .cdi the same way - neither
// should ever have been flagged as "unmanaged" at all. Each entry below
// also unions in whatever archive extension(s) checkContainerFile's own
// switch already treats as a known, needs-conversion case for that
// system's class (cd: zip/7z/rar, dvd: zip/7z/rar) - not because the
// emulator plays them directly, but so this sweep doesn't flag a file
// check already has its own, more specific "convert to CHD" finding for.
//
// RPCS3 (ps3) is deliberately not listed here despite having its own
// guide page: that page documents ES-DE's own launcher convention for
// PS3 (.desktop shortcuts, or a folder renamed .ps3/.ps3dir) - not which
// raw disc formats RPCS3 itself can read - so it isn't the same kind of
// "supported formats" data the other pages give, and ps3 keeps its
// entry in classKnownExts (which does already include .iso correctly)
// instead.
var knownFormatsPerSystem = map[string]map[string]bool{
	// Duckstation - confirmed 2026-11-11 against
	// wiki_emulator_guides/duckstation/duckstation-guide/.
	"psx": extSet("bin", "cue", "img", "iso", "chd", "mds", "pbp", "ecm", "zip", "7z", "rar"),

	// Flycast - confirmed 2026-11-11 against
	// wiki_emulator_guides/flycast/flycast-guide/.
	"dreamcast": extSet("cdi", "chd", "cue", "dat", "elf", "gdi", "iso", "lst", "m3u", "7z", "zip", "rar"),

	// Dolphin - confirmed 2026-11-11 against
	// wiki_emulator_guides/dolphin/dolphin-guide/. Already includes 7z/
	// zip natively (Dolphin plays a compressed disc image directly;
	// checkContainerFile has no dolphin:zip/7z case of its own at all,
	// so this is new recognition, not a union with an existing one).
	"gc":  extSet("ciso", "dff", "dol", "elf", "gcm", "gcz", "iso", "json", "m3u", "rvz", "tgc", "wad", "wbfs", "wia", "7z", "zip"),
	"wii": extSet("ciso", "dff", "dol", "elf", "gcm", "gcz", "iso", "json", "m3u", "rvz", "tgc", "wad", "wbfs", "wia", "7z", "zip"),

	// PCSX2 - confirmed 2026-11-11 against
	// wiki_emulator_guides/pcsx2/pcsx2-guide/.
	"ps2": extSet("iso", "bin", "cue", "chd", "zip", "7z", "rar"),

	// PPSSPP - confirmed 2026-11-11 against
	// wiki_emulator_guides/ppsspp/ppsspp-guide/. No zip/7z/rar listed,
	// and checkContainerFile has no umd:zip/7z/rar case either - a
	// stray archive here really would be unexpected.
	"psp": extSet("elf", "cso", "iso", "pbp", "prx", "chd"),

	// XEMU - confirmed 2026-11-11 against
	// wiki_emulator_guides/xemu/xemu-guide/: only a plain .iso
	// (internally XISO-formatted) is documented. "xiso" is kept too
	// since romctl's own pipeline already treats it as an equivalent
	// output extension (see xiso.go), not because the guide names it.
	"xbox": extSet("iso", "xiso"),
}

// classKnownExts is the fallback for a disc-based system with no entry
// in knownFormatsPerSystem yet - every other ClassCD system (saturn,
// segacd, pcenginecd, 3do, neogeocd, pcfx, amigacd32, cdtv, cdimono1,
// atarijaguarcd, tg-cd, and their regional variants) runs through a
// RetroArch core rather than a standalone emulator, and RetroDECK's
// wiki has no dedicated per-core "what formats are supported" page the
// way it does for the 32 standalone emulators above - so this is still
// the original guess (what checkContainerFile itself already
// recognizes for the class, not a verified emulator capability list),
// pending the same kind of real verification the entries above got.
// ps3 is here rather than in knownFormatsPerSystem for the same reason -
// see that var's own doc comment. Cartridge-shaped classes (cart/n64/
// snes/arcade) have no equivalent closed set of extensions at all -
// every ordinary ROM there would look "unrecognized" by this same test -
// so they're deliberately not covered at all, by either map.
var classKnownExts = map[systemdb.Class]map[string]bool{
	systemdb.ClassCD:  extSet("cue", "gdi", "iso", "bin", "img", "ccd", "mds", "toc", "zip", "7z", "rar", "chd"),
	systemdb.ClassDVD: extSet("iso", "cue", "zip", "7z", "rar", "chd"),
	systemdb.ClassUMD: extSet("iso", "cso", "chd"),
	systemdb.ClassPS3: extSet("iso", "7z", "zip", "rar"),
}

func extSet(exts ...string) map[string]bool {
	m := make(map[string]bool, len(exts))
	for _, e := range exts {
		m[e] = true
	}
	return m
}

// cullFlag is one file Cull reports but doesn't touch - either an
// unmanaged format, or a confirmed dead end that happens to be sitting
// inside a Title.m3u/ directory (see Cull's own doc comment).
type cullFlag struct {
	sys, file, reason string
}

// Cull is meant to run after fix (and scrape, if used) have already had
// their chance to convert and organize everything they can: it looks for
// whatever's left that's a genuine dead end, not just unconverted. Two
// different things can happen to a file, and Cull is careful to keep
// them apart:
//
//   - A confirmed dead end (cullMessages) - check's own structure/
//     containers categories already positively verified it's broken or
//     permanently stuck (a corrupt CHD/XISO/ISO, a loose .bin with no
//     .cue sheet anywhere, a leftover extraction/incomplete-transfer
//     artifact, a PS3 dump missing its EBOOT.BIN) - gets retired, the
//     same actions.Retire into <system>/_to_delete/ every other
//     mutating command uses; nothing here ever deletes anything
//     outright, `clean` still owns that. A file already grouped inside
//     a Title.m3u/ directory is deliberately left out of this, even
//     when it matches: retiring one disc out of a multi-disc set would
//     leave its playlist referencing a file that's gone, and fixing
//     that up isn't attempted here - it's reported instead, for manual
//     handling.
//
//   - An unmanaged format (knownExtsFor) - a file extension outside
//     what this specific system actually supports. Preferably sourced
//     from the system's own roms/<sys>/systeminfo.txt (RetroDECK/ES-DE's
//     own generated metadata - its "Supported file extensions:" line
//     reflects whatever core is actually configured, confirmed against
//     a real one: psx's default core there is swanstation, not the
//     Duckstation standalone guide this was originally written against -
//     see systemInfoKnownExts), falling back to knownFormatsPerSystem/
//     classKnownExts only when no systeminfo.txt is present to read.
//     Covers every class, not just disc-based ones: a stray
//     ES-DE-internal `gamelist.xml` or emulator `.cfg`/`.log` file
//     sitting directly in a cartridge system's directory is just as
//     real a case as a disc-based system's unsupported format, and
//     systeminfo.txt's own list rules those out exactly the same way.
//     Only ever reported, never retired: romctl has no way to confirm
//     it's actually broken.
//
//   - A text-like file misusing a format that IS otherwise known/
//     supported (textLikeFile) - the one case extension matching alone
//     can never catch: Sega Genesis's own ".md" ROM extension is
//     identical to Markdown's, and no extension list, however accurate,
//     could tell them apart. Content is sniffed instead - a control
//     byte anywhere in the first 512 bytes means binary, no control
//     bytes at all means text (the same test git/grep -I use to tell
//     a text file from a binary one; cheap, this never fires on a file
//     already flagged as unmanaged above). Also only ever reported,
//     never retired.
//
// Reuses checkStructure/checkContainers directly (via the new
// audit.Report.Findings) rather than a second implementation of "what
// counts as corrupt/leftover/unconvertible" - the one cost of that reuse
// is that an ambiguous disc set still gets its real chdman/dolphin-tool
// confirmation pass (see checkIncompleteDiscSet) even though Cull never
// acts on that particular finding; running `check`/`discs` first, as
// cull is meant to run after everything else anyway, means that cost
// is nothing new by the time you get here.
func Cull(lib *library.Library, runner *actions.Runner, systems []string) (bool, error) {
	start := time.Now()
	log := cliutil.NewLogger("cull", runner.Format)
	var retired, failed int
	var flagged []cullFlag

	report := audit.New(io.Discard, false, cliutil.FormatPlain, "")
	jobs := &discJobCollector{}
	cliutil.Parallel(systems, func(sys string) { checkStructure(report, lib, sys, jobs) })
	discTools := &discToolResolver{}
	cliutil.Parallel(jobs.jobs, func(j discCompletenessJob) {
		reportDiscCompleteness(report, j.sys, j.class, j.sysDir, j.reportPath, j.num, j.group, discTools, j.fallback)
	})
	esdeNames, _ := lib.ESDESystems()
	cliutil.Parallel(systems, func(sys string) { checkContainers(report, lib, sys, esdeNames) })

	for _, f := range report.Findings() {
		if f.Category != audit.CategoryStructure && f.Category != audit.CategoryContainers {
			continue
		}
		if !cullWorthy(f.Message) {
			continue
		}
		if strings.ContainsAny(f.Path, `/\`) {
			// Nested inside a Title.m3u/ directory - see Cull's own doc
			// comment on why this isn't auto-retired.
			flagged = append(flagged, cullFlag{f.System, f.Path, f.Message})
			continue
		}
		path := filepath.Join(lib.Roms, f.System, f.Path)
		if !runner.Act("retire %s/%s (%s)", f.System, f.Path, f.Message) {
			continue
		}
		if err := actions.Retire(path); err != nil {
			log.Errf("retire failed: %s/%s: %v", f.System, f.Path, err)
			failed++
			continue
		}
		retired++
	}

	for _, sys := range systems {
		flagged = append(flagged, suspiciousFilesFor(lib, sys)...)
	}

	if runner.Format.Structured() {
		runner.SetCount("retired", int64(retired))
		runner.SetCount("flagged", int64(len(flagged)))
		runner.SetCount("failed", int64(failed))
		return failed == 0, nil
	}
	color, reset := cliutil.StatusColor(runner.Format, failed > 0)
	line := fmt.Sprintf("%scull: %d retired, %d flagged for review, %d failed in %s%s\n",
		color, retired, len(flagged), failed, time.Since(start).Round(time.Second), reset)
	if runner.Quiet {
		runner.Summarize(line)
	} else {
		fmt.Print(line)
	}
	if len(flagged) > 0 && !runner.Quiet {
		fmt.Println("\nFlagged for manual review (not retired - unmanaged format, or can't confirm):")
		for _, fl := range flagged {
			fmt.Printf("  %s/%s: %s\n", fl.sys, fl.file, fl.reason)
		}
	}
	return failed == 0, nil
}

// systemInfoExtLabel is the exact label line systemInfoKnownExts looks
// for in a RetroDECK/ES-DE-generated systeminfo.txt, immediately
// preceding the actual space-separated extension list on the next
// non-empty line.
const systemInfoExtLabel = "Supported file extensions:"

// systemInfoKnownExts parses roms/<sys>/systeminfo.txt's own "Supported
// file extensions:" line - RetroDECK/ES-DE's own generated per-system
// metadata, listing exactly what the system's actually-configured
// emulator core supports - into a lowercased extension set (no leading
// dot, duplicates from the file's own upper/lowercase pairs collapsed).
// This is the real, locally-accurate answer suspiciousFilesFor prefers
// over any hardcoded guess: confirmed against a real one, psx's own
// systeminfo.txt names swanstation as its default launch core, not
// Duckstation (the standalone emulator knownFormatsPerSystem's own psx
// entry was verified against) - two different cores can easily disagree
// on what they support, and systeminfo.txt reflects whichever one this
// library's own ES-DE install actually launches. ok is false (caller
// falls back to knownFormatsPerSystem/classKnownExts) when the file
// doesn't exist, doesn't parse, or has no such line at all.
func systemInfoKnownExts(lib *library.Library, sys string) (map[string]bool, bool) {
	data, err := os.ReadFile(filepath.Join(lib.Roms, sys, "systeminfo.txt"))
	if err != nil {
		return nil, false
	}
	lines := strings.Split(string(data), "\n")
	for i, line := range lines {
		if strings.TrimSpace(line) != systemInfoExtLabel {
			continue
		}
		for _, l := range lines[i+1:] {
			l = strings.TrimSpace(l)
			if l == "" {
				continue
			}
			fields := strings.Fields(l)
			exts := make(map[string]bool, len(fields))
			for _, f := range fields {
				exts[strings.ToLower(strings.TrimPrefix(f, "."))] = true
			}
			return exts, len(exts) > 0
		}
		return nil, false
	}
	return nil, false
}

// knownExtsFor returns sys's known/supported extension set, preferring
// its own systeminfo.txt (the real, locally-accurate answer - see
// systemInfoKnownExts) over the hardcoded knownFormatsPerSystem, falling
// back to the even broader classKnownExts guess when neither of those
// has an entry either. ok is false only when nothing at all is
// available - suspiciousFilesFor then skips the extension check
// entirely and relies on content-sniffing alone.
func knownExtsFor(lib *library.Library, sys string) (map[string]bool, bool) {
	if exts, ok := systemInfoKnownExts(lib, sys); ok {
		return exts, true
	}
	if exts, ok := knownFormatsPerSystem[sys]; ok {
		return exts, true
	}
	if exts, ok := classKnownExts[systemdb.ClassOf(sys)]; ok {
		return exts, true
	}
	return nil, false
}

// suspiciousFilesFor scans sys's top-level files (bare files only - a
// Title.m3u/ or Title.ps3/ directory is its own, already-understood
// convention) for either of two things, never both for the same file:
//
//   - An extension knownExtsFor doesn't list for sys at all - flagged
//     as an unmanaged format immediately, no need to also sniff its
//     content: a stray ES-DE-internal gamelist.xml or emulator .cfg/
//     .log file sitting directly in a ROM directory is exactly this
//     case, caught the same way for any system class, cartridge-shaped
//     ones included, now that systeminfo.txt gives every system a real
//     extension list to check against.
//   - An extension that IS known/supported, but content that looks
//     like plain text rather than binary ROM data (textLikeFile) - the
//     one case no extension list, however accurate, could ever catch
//     on its own: Sega Genesis's own ".md" ROM extension is identical
//     to Markdown's.
func suspiciousFilesFor(lib *library.Library, sys string) []cullFlag {
	known, haveKnown := knownExtsFor(lib, sys)
	sysDir := filepath.Join(lib.Roms, sys)
	entries, err := os.ReadDir(sysDir)
	if err != nil {
		return nil
	}
	var flags []cullFlag
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		base := e.Name()
		if base == "systeminfo.txt" || base == ".directory" || isLeftoverInfix(base) {
			continue
		}
		ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(base), "."))
		if nonROMExts[ext] {
			continue
		}
		if haveKnown && !known[ext] {
			flags = append(flags, cullFlag{sys, base, "unrecognized format for this system; romctl has never checked it - verify manually it still plays"})
			continue
		}
		if textFormatExts[ext] {
			continue
		}
		if reason, ok := textLikeFile(filepath.Join(sysDir, base)); ok {
			flags = append(flags, cullFlag{sys, base, reason})
		}
	}
	return flags
}

// textFormatExts is every extension that's expected to be plain text,
// not binary ROM data, despite showing up in a "supported formats" list
// for some system - a cue sheet (.cue), a playlist (.m3u), and (per
// Dolphin's own documented GameCube/Wii support) .json. textLikeFile's
// own test would correctly, but uselessly, flag every one of these -
// they're supposed to be text - so suspiciousFilesFor skips the content
// check for them entirely instead of reporting the obvious.
var textFormatExts = map[string]bool{
	"cue": true, "m3u": true, "json": true,
}

// textSniffWindow is how much of a file textLikeFile samples - comfortably
// past a real, confirmed case this needed to cover: some mid-90s EA/
// Tiburon-developed cartridges (NHL 96, Boxing Legends of the Ring,
// Chavez Boxing 2, ... - all found in one real library) embed a long
// plain-ASCII "help wanted"/developer-credits message at the very start
// of the ROM, ahead of any actual binary code - one real example's own
// text ran to byte 888 before the first control byte appeared. A naive
// 512-byte sample would misread any of these as a stray text file; at
// 32 KiB, a legitimate cartridge ROM's own binary code is certain to
// show up somewhere in the sample no matter how long an embedded text
// header runs, while a stray non-ROM text file (a README, a config, a
// genuinely misplaced Markdown document) is essentially never that
// large to begin with - so there's no real downside to the wider window.
const textSniffWindow = 32 * 1024

// textLikeFile reports whether path's content looks like plain text
// rather than binary ROM data, by the same test git/grep -I use to
// tell the two apart: a control byte anywhere in the sample (something
// other than tab/LF/CR) means binary, no control byte at all means
// text. Deliberately not http.DetectContentType - confirmed unsound for
// this by a real false positive it produced: DetectContentType treats
// a leading 0xFEFF/0xFFFE byte pair as a UTF-16 BOM and reports text
// unconditionally without ever looking further, and a real GBA ROM's
// own header starts with exactly that byte pattern despite being
// unambiguously binary (confirmed via a hex dump - the same bytes
// contain plenty of control bytes this function's own test catches
// correctly). An empty file is deliberately left alone rather than
// flagged - RetroDECK itself uses empty files as placeholders (the
// same convention as .directory), so an empty file is routine, not
// suspicious.
func textLikeFile(path string) (reason string, ok bool) {
	f, err := os.Open(path)
	if err != nil {
		return "", false
	}
	defer f.Close()
	buf := make([]byte, textSniffWindow)
	n, err := io.ReadFull(f, buf)
	if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
		return "", false
	}
	if n == 0 {
		return "", false
	}
	if isLikelyText(buf[:n]) {
		return "looks like a text file, not binary ROM data - check its extension and content", true
	}
	return "", false
}

// isLikelyText reports whether data contains no byte that couldn't
// plausibly appear in a text file: a control character other than
// tab/LF/CR anywhere in the sample disqualifies it. This undersells
// "text" a little (some legitimate text encodings use other control
// bytes) in exchange for near-zero false positives on real binary ROM
// data, which this function's own doc comment explains is the actual
// risk worth avoiding here.
func isLikelyText(data []byte) bool {
	for _, b := range data {
		if b == '\t' || b == '\n' || b == '\r' {
			continue
		}
		if b < 0x20 || b == 0x7F {
			return false
		}
	}
	return true
}
