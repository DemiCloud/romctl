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

// scrapMessages is the exact set of check findings (structure or
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
var scrapMessages = []string{
	"incomplete conversion leftover",
	"incomplete transfer/conversion leftover",
	"leftover extraction directory",
	"no matching .cue sheet",
	"not a valid CHD",
	"ISO is truncated or not ISO9660",
	"not a valid XISO",
	"no EBOOT.BIN found",
}

func scrapWorthy(message string) bool {
	for _, m := range scrapMessages {
		if strings.Contains(message, m) {
			return true
		}
	}
	return false
}

// discClassKnownExts is, for each disc-based system class Scrap's
// unmanaged-format sweep covers, the complete set of extensions
// checkContainerFile already recognizes for it (every key sharing that
// class in its own switch - see check.go) plus whichever "already
// final, nothing left to check" format has no case there for exactly
// that reason (Dolphin's own .rvz - unlike .chd, .xiso and PS3's own
// .iso, there's no checkRVZ validation at all yet, so .rvz never
// appears as a switch key). Anything else found in one of these
// classes' directories is a format Scrap has never seen before - not
// necessarily broken (it might just work as-is, like a .PBP a libretro
// PS1 core can often load directly without any conversion), but worth
// surfacing rather than staying silent about. Cartridge-shaped classes
// (cart/n64/snes/arcade) have no equivalent closed set of extensions -
// every ordinary ROM there would look "unrecognized" by this same
// test, so they're deliberately not covered at all.
var discClassKnownExts = map[systemdb.Class]map[string]bool{
	systemdb.ClassCD:      extSet("cue", "gdi", "iso", "bin", "img", "ccd", "mds", "toc", "zip", "7z", "rar", "chd"),
	systemdb.ClassDVD:     extSet("iso", "cue", "zip", "7z", "rar", "chd"),
	systemdb.ClassUMD:     extSet("iso", "cso", "chd"),
	systemdb.ClassDolphin: extSet("iso", "gcm", "wbfs", "ciso", "gcz", "rvz"),
	systemdb.ClassXbox:    extSet("iso", "xiso"),
	systemdb.ClassPS3:     extSet("iso", "7z", "zip", "rar"),
}

func extSet(exts ...string) map[string]bool {
	m := make(map[string]bool, len(exts))
	for _, e := range exts {
		m[e] = true
	}
	return m
}

// scrapFlag is one file Scrap reports but doesn't touch - either an
// unmanaged format, or a confirmed dead end that happens to be sitting
// inside a Title.m3u/ directory (see Scrap's own doc comment).
type scrapFlag struct {
	sys, file, reason string
}

// Scrap is meant to run after fix (and scrape, if used) have already had
// their chance to convert and organize everything they can: it looks for
// whatever's left that's a genuine dead end, not just unconverted. Two
// different things can happen to a file, and Scrap is careful to keep
// them apart:
//
//   - A confirmed dead end (scrapMessages) - check's own structure/
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
// confirmation pass (see checkIncompleteDiscSet) even though Scrap never
// acts on that particular finding; running `check`/`discs` first, as
// scrap is meant to run after everything else anyway, means that cost
// is nothing new by the time you get here.
func Scrap(lib *library.Library, runner *actions.Runner, systems []string) (bool, error) {
	start := time.Now()
	log := cliutil.NewLogger("scrap", runner.Format)
	var retired, failed int
	var flagged []scrapFlag

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
		if !scrapWorthy(f.Message) {
			continue
		}
		if strings.ContainsAny(f.Path, `/\`) {
			// Nested inside a Title.m3u/ directory - see Scrap's own doc
			// comment on why this isn't auto-retired.
			flagged = append(flagged, scrapFlag{f.System, f.Path, f.Message})
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
	line := fmt.Sprintf("%sscrap: %d retired, %d flagged for review, %d failed in %s%s\n",
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
func unmanagedFormatsFor(lib *library.Library, sys string) []scrapFlag {
	class := systemdb.ClassOf(sys)
	known, ok := discClassKnownExts[class]
	if !ok {
		return nil
	}
	sysDir := filepath.Join(lib.Roms, sys)
	entries, err := os.ReadDir(sysDir)
	if err != nil {
		return nil
	}
	var flags []scrapFlag
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
		flags = append(flags, scrapFlag{sys, base, "unrecognized format for this system; romctl has never checked it - verify manually it still plays"})
	}
	return flags
}
