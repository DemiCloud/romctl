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
//   - An unmanaged format (discClassKnownExts) - a file extension
//     romctl doesn't recognize at all for a disc-based system (.PBP in
//     a psx/ directory is the real case this was written for) - is only
//     ever reported, never retired: romctl has no way to confirm it's
//     actually broken, and plenty of such formats play just fine as-is
//     in the right emulator core without ever being converted. Deciding
//     what to do with these is left to you.
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
		flagged = append(flagged, unmanagedFormatsFor(lib, sys)...)
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

// unmanagedFormatsFor scans sys's top-level files (bare files only - a
// Title.m3u/ or Title.ps3/ directory is its own, already-understood
// convention, not an unrecognized format) for an extension
// discClassKnownExts doesn't list for sys's class.
func unmanagedFormatsFor(lib *library.Library, sys string) []cullFlag {
	known, ok := knownFormatsPerSystem[sys]
	if !ok {
		known, ok = classKnownExts[systemdb.ClassOf(sys)]
		if !ok {
			return nil
		}
	}
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
		if ext == "" || nonROMExts[ext] || known[ext] {
			continue
		}
		flags = append(flags, cullFlag{sys, base, "unrecognized format for this system; romctl has never checked it - verify manually it still plays"})
	}
	return flags
}
