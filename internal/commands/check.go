package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/demicloud/romctl/internal/actions"
	"github.com/demicloud/romctl/internal/audit"
	"github.com/demicloud/romctl/internal/cliutil"
	"github.com/demicloud/romctl/internal/datdb"
	"github.com/demicloud/romctl/internal/formats/chd"
	"github.com/demicloud/romctl/internal/formats/iso9660"
	"github.com/demicloud/romctl/internal/formats/n64"
	"github.com/demicloud/romctl/internal/formats/signature"
	"github.com/demicloud/romctl/internal/formats/snes"
	"github.com/demicloud/romctl/internal/formats/xiso"
	"github.com/demicloud/romctl/internal/library"
	"github.com/demicloud/romctl/internal/systemdb"
	"github.com/demicloud/romctl/internal/tools"
)

// Check audits every system (structure, naming, formats) and BIOS files,
// then prints the Checks table and a Fixes table (how to resolve what was
// found), and reports whether anything FAILed. Exact-duplicate detection
// and full CHD data verification live in their own dedicated commands
// (`dupes`, `verify` - see commands.Dupes/commands.Verify), not here: both
// mean reading every file's full content, which can take long enough on a
// library with large CHD/ISO files that folding them into check's own
// instant, all-categories-at-once output made a slow pass indistinguishable
// from a hang - the same reasoning that already split `bios` out as its
// own command. scheme (`[naming] scheme`) is the naming-audit category's
// yardstick: a name internal/actions.ApplyScheme wouldn't change for the
// configured scheme isn't a finding. verbose prints every finding as it's
// found (and skips the live all-phases-at-once display, which would
// otherwise collide with that output); rootHint, if non-empty (an
// explicitly-passed "-r PATH"), is spliced into every suggested command.
// Each category runs across every system in parallel - independent,
// I/O-bound work. Under a structured format (json/yaml/toml/csv), the
// tables are skipped entirely and every finding plus the summary tally is
// encoded instead - see audit.Report.Encode.
//
// The structure category is the one place this isn't always instant:
// confirming an "incomplete disc set" finding for an already-converted
// .chd/.rvz needs a real chdman/dolphin-tool extraction (see
// discCompletenessJob/discFileHashes), gated behind the same cheap,
// directory-listing-only ambiguity check that's instant for everything
// else. Every such candidate across every system is collected first and
// resolved through one shared worker pool afterward (discJobCollector),
// not inline as each is found - see that type's own doc comment for why.
func Check(lib *library.Library, systems []string, scheme actions.Scheme, verbose bool, format cliutil.Format, rootHint string) (bool, error) {
	report := audit.New(os.Stdout, verbose, format, rootHint)
	report.CollapseBios()
	esdeNames, _ := lib.ESDESystems()

	phases := []cliutil.Phase{
		{Label: string(audit.CategoryNaming), Fn: func() bool {
			cliutil.Parallel(systems, func(sys string) { checkNaming(report, lib, sys, scheme) })
			return true
		}},
		{Label: string(audit.CategoryStructure), Fn: func() bool {
			jobs := &discJobCollector{}
			cliutil.Parallel(systems, func(sys string) { checkStructure(report, lib, sys, jobs) })
			checkUnknownSystems(report, lib, systems)
			// Confirming an ambiguous disc (see discCompletenessJob) is the
			// one part of this phase that can be genuinely slow - a real
			// chdman/dolphin-tool extraction, not a filename check. The
			// scan above only collects candidates; running them through
			// one shared worker pool here, instead of resolving each
			// inline as it's found, means two ambiguous discs in the same
			// system - or across different systems - run concurrently
			// rather than one after another (confirmed against a real
			// library: two such discs in one system took ~30s run
			// serially this way before this change).
			discTools := &discToolResolver{}
			cliutil.Parallel(jobs.jobs, func(j discCompletenessJob) {
				reportDiscCompleteness(report, j.sys, j.class, j.sysDir, j.reportPath, j.num, j.group, discTools, j.fallback)
			})
			return true
		}},
		{Label: string(audit.CategoryContainers), Fn: func() bool {
			cliutil.Parallel(systems, func(sys string) { checkContainers(report, lib, sys, esdeNames) })
			return true
		}},
		{Label: string(audit.CategoryBios), Fn: func() bool {
			checkBios(report, lib, systems)
			return true
		}},
	}
	cliutil.RunPhases(format, verbose, false, phases)

	if format.Structured() {
		return !report.Failed(), report.Encode(os.Stdout, format)
	}

	report.Table(os.Stdout)
	report.Fixes(os.Stdout)
	return !report.Failed(), nil
}

// nonROMExts are extensions check never flags on their own: sidecar/save
// data (sav/srm/state/sh get their own structure finding first, see
// checkStructureFile; everything else here is silently ignored) and
// incomplete-transfer markers (part/tmp, likewise flagged first in
// checkStructureFile). Shared by checkNaming and checkContainerFile, which
// have no file-type-specific finding of their own for any of these.
var nonROMExts = map[string]bool{
	"part": true, "tmp": true,
	"sav": true, "srm": true, "state": true, "sh": true,
	"txt": true, "ini": true, "cheats": true, "ips": true, "bps": true, "ups": true, "sbi": true, "dat": true, "directory": true,
}

// isLeftoverInfix reports whether base is an atomic-write-in-progress
// leftover (see AGENTS.md's Atomic outputs invariant): *.partial.*,
// *.tmp.*, or *.romctl-rename.*. Narrower than list.go's isLeftover (which
// also matches a bare .part/.tmp extension): checkStructureFile reports
// those two cases with different messages, so it needs to tell them apart.
func isLeftoverInfix(base string) bool {
	return strings.Contains(base, ".partial.") || strings.Contains(base, ".tmp.") || strings.Contains(base, ".romctl-rename.")
}

func checkNaming(report *audit.Report, lib *library.Library, sys string, scheme actions.Scheme) {
	if systemdb.ClassOf(sys) == systemdb.ClassArcade {
		return // arcade filenames are set names the emulator resolves, never renamed
	}
	sysDir := filepath.Join(lib.Roms, sys)
	entries, err := os.ReadDir(sysDir)
	if err != nil {
		return
	}
	for _, e := range entries {
		base := e.Name()
		if e.IsDir() {
			// A Title.m3u/ playlist directory or a Title.ps3/ disc folder
			// is its game's own naming unit, same as a bare file is for
			// everything else - rename handles both (see renamableDirSuffix,
			// renameM3UDir/renameSuffixedDir).
			suffix, ok := renamableDirSuffix(base)
			if !ok {
				continue
			}
			report.Scan(audit.CategoryNaming, 1)
			title := strings.TrimSuffix(base, suffix)
			if clean := actions.ApplyScheme(title, scheme); clean != title {
				report.Add(audit.Warn, audit.CategoryNaming, sys, base, fmt.Sprintf("naming -> %q", clean+suffix), "rename")
			}
			continue
		}
		if base == "systeminfo.txt" || base == ".directory" || isLeftoverInfix(base) {
			continue
		}
		ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(base), "."))
		if nonROMExts[ext] {
			continue
		}
		report.Scan(audit.CategoryNaming, 1)
		name := strings.TrimSuffix(base, filepath.Ext(base))
		if clean := actions.ApplyScheme(name, scheme); clean != name {
			report.Add(audit.Warn, audit.CategoryNaming, sys, base, fmt.Sprintf("naming -> %q", clean+"."+ext), "rename")
		}
	}
}

// discCompletenessJob is one ambiguous disc - loose (checkStructureFile)
// or already grouped inside a Title.m3u/ directory (checkM3uDiscSet) -
// deferred for confirmation rather than resolved on the spot: see
// discJobCollector.
type discCompletenessJob struct {
	sys        string
	class      systemdb.Class
	sysDir     string
	reportPath string
	num        int
	group      []discFile
	fallback   func()
}

// discJobCollector gathers every discCompletenessJob found across every
// system's cheap structure scan (checkStructure runs one per system,
// concurrently via cliutil.Parallel - hence the mutex), so Check can run
// all of them through one shared worker pool afterward instead of
// resolving each inline as it's found. Without this, two ambiguous
// discs in the very same system would confirm one after another even
// though confirming one is a real chdman/dolphin-tool extraction, not a
// filename check - confirmed against a real library, where exactly that
// (two such discs in one system) took roughly twice as long run this
// way as run concurrently.
type discJobCollector struct {
	mu   sync.Mutex
	jobs []discCompletenessJob
}

func (c *discJobCollector) add(j discCompletenessJob) {
	c.mu.Lock()
	c.jobs = append(c.jobs, j)
	c.mu.Unlock()
}

func checkStructure(report *audit.Report, lib *library.Library, sys string, jobs *discJobCollector) {
	class := systemdb.ClassOf(sys)
	sysDir := filepath.Join(lib.Roms, sys)
	entries, err := os.ReadDir(sysDir)
	if err != nil {
		return
	}
	discGroups := buildDiscGroups(entries)
	for _, e := range entries {
		base := e.Name()
		if base == "systeminfo.txt" || base == ".directory" {
			continue
		}
		report.Scan(audit.CategoryStructure, 1)
		path := filepath.Join(sysDir, base)
		if e.IsDir() {
			checkStructureDir(report, sys, class, path, jobs)
		} else {
			checkStructureFile(report, sys, class, path, sysDir, discGroups, jobs)
		}
	}
}

func checkStructureDir(report *audit.Report, sys string, class systemdb.Class, dirPath string, jobs *discJobCollector) {
	base := filepath.Base(dirPath)
	switch {
	case base == "_to_delete":
		report.Add(audit.Info, audit.CategoryStructure, sys, base, "pending deletion", "clean")
	case strings.HasPrefix(base, "_extract."):
		report.Add(audit.Fail, audit.CategoryStructure, sys, base, "leftover extraction directory", "")
	case strings.HasSuffix(base, ".m3u"):
		checkM3uDir(report, sys, dirPath)
		checkM3uDiscSet(sys, class, dirPath, jobs)
	}
}

// checkM3uDiscSet is checkStructureFile's lone-disc ambiguity check,
// extended to a disc already grouped inside a Title.m3u/ directory: a
// directory holding only one disc file is just as ambiguous as a loose
// one with no sibling (see m3uLoneDiscGroup), and romctl's own `m3u`
// command refusing to create a one-file group going forward (see its
// own doc comment) doesn't retroactively fix one that already exists.
// A directory already holding more than one distinct disc number is
// left alone - not re-verified against the DAT - same proportional
// scope as the loose-file case. No fallback job field set: unlike a
// loose file, there's no pre-existing "needs grouping" warning to fall
// back to for something that's already correctly grouped.
func checkM3uDiscSet(sys string, class systemdb.Class, dirPath string, jobs *discJobCollector) {
	name := filepath.Base(dirPath)
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return
	}
	group, ok := m3uLoneDiscGroup(name, entries)
	if !ok {
		return
	}
	jobs.add(discCompletenessJob{sys: sys, class: class, sysDir: dirPath, reportPath: name, num: group[0].num, group: group})
}

func checkM3uDir(report *audit.Report, sys, dirPath string) {
	name := filepath.Base(dirPath)
	playlist := filepath.Join(dirPath, name)

	data, err := os.ReadFile(playlist)
	if err != nil {
		report.Add(audit.Fail, audit.CategoryStructure, sys, name, "missing playlist "+name, "")
		return
	}
	lines := 0
	for _, line := range strings.Split(strings.TrimRight(string(data), "\n"), "\n") {
		line = strings.TrimSuffix(line, "\r")
		if line == "" {
			continue
		}
		lines++
		if !exists(filepath.Join(dirPath, line)) {
			report.Add(audit.Fail, audit.CategoryStructure, sys, name, "playlist entry not found: "+line, "")
		}
	}
	entries, _ := os.ReadDir(dirPath)
	discs := 0
	for _, e := range entries {
		base := e.Name()
		switch {
		case filepath.Join(dirPath, base) == playlist:
			continue
		case base == "_to_delete":
			report.Add(audit.Info, audit.CategoryStructure, sys, filepath.Join(name, base), "pending deletion", "clean")
		case strings.HasPrefix(base, "_extract."):
			report.Add(audit.Fail, audit.CategoryStructure, sys, filepath.Join(name, base), "leftover extraction directory", "")
		default:
			discs++
		}
	}
	if lines != discs {
		report.Add(audit.Warn, audit.CategoryStructure, sys, name,
			fmt.Sprintf("%d disc file(s) but %d playlist entries", discs, lines), "")
	}
}

// checkUnknownSystems flags a roms/<dir> that neither romctl's own
// systemdb.SystemClass nor the install's own ES-DE-generated systems.txt
// recognizes at all - most likely a typo'd directory name or a leftover
// from a system that's since been removed. Only runs when systems.txt is
// present: systemdb.SystemClass alone can't make this call reliably on
// its own, since it's a hand-ported subset and a directory it doesn't
// know about might still be a perfectly legitimate, newer RetroDECK
// system that just hasn't been added to it yet.
func checkUnknownSystems(report *audit.Report, lib *library.Library, systems []string) {
	esdeNames, ok := lib.ESDESystems()
	if !ok {
		return
	}
	for _, sys := range systems {
		report.Scan(audit.CategoryStructure, 1)
		if _, known := systemdb.SystemClass[sys]; known {
			continue
		}
		if _, known := esdeNames[sys]; known {
			continue
		}
		report.Add(audit.Warn, audit.CategoryStructure, sys, "roms/"+sys,
			"not a system directory RetroDECK or romctl recognizes", "")
	}
}

var discTagRE = regexp.MustCompile(`\(Disc (\d+)\)`)

// discTrackTagRE matches a "(Track N)" sidecar tag - a raw multi-track CD
// dump with no .cue sheet splits one disc into several bin files this
// way (e.g. "Title (Disc 1) (Track 1).bin", "... (Track 2).bin"), which
// discGroupKey strips alongside the disc tag so every track of the same
// disc collapses into one group entry instead of looking like several
// different discs.
var discTrackTagRE = regexp.MustCompile(`\(Track \d+\)`)

// discFile is one "(Disc N)"-tagged file found by buildDiscGroups: num is
// its own disc number, base its filename.
type discFile struct {
	num  int
	base string
}

// discGroupKey is name (without extension) with its "(Disc N)" tag and
// any "(Track M)" sidecar tag removed, collapsing the whitespace either
// leaves behind - the key buildDiscGroups and checkIncompleteDiscSet
// group same-title files/DAT entries under, regardless of disc or track
// number.
func discGroupKey(name string) string {
	key := discTagRE.ReplaceAllString(name, "")
	key = discTrackTagRE.ReplaceAllString(key, "")
	return strings.Join(strings.Fields(key), " ")
}

// buildDiscGroups scans sysDir's top-level files for "(Disc N)"-tagged
// names, grouping them by discGroupKey. checkStructureFile uses this to
// tell a loose disc that already has a sibling disc number present (an
// ordinary "needs m3u" case) apart from one with no sibling at all - the
// anomaly checkIncompleteDiscSet is gated behind, since that's the only
// case cheap filename matching alone can't resolve on its own.
func buildDiscGroups(entries []os.DirEntry) map[string][]discFile {
	groups := map[string][]discFile{}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		base := e.Name()
		name := strings.TrimSuffix(base, filepath.Ext(base))
		m := discTagRE.FindStringSubmatch(name)
		if m == nil {
			continue
		}
		num, err := strconv.Atoi(m[1])
		if err != nil {
			continue
		}
		key := discGroupKey(name)
		groups[key] = append(groups[key], discFile{num, base})
	}
	return groups
}

// m3uLoneDiscGroup returns the disc files in a Title.m3u/ directory's
// already-read entries (skipping the playlist itself, a nested
// _to_delete, and a leftover extraction dir) when ok is true: all of
// them share one disc number, the same ambiguous "single-disc game, or
// missing the rest" case checkStructureFile already resolves for a
// loose file - just for one already grouped. ok is false when there's
// nothing to anchor on (a disc file with no "(Disc N)" tag at all, or
// no disc files at all) or the directory already holds more than one
// distinct disc number - an already-healthy-looking group this doesn't
// re-verify. playlistName is the directory's own base name (also the
// playlist file's name inside it, ES-DE's convention).
func m3uLoneDiscGroup(playlistName string, entries []os.DirEntry) (group []discFile, ok bool) {
	for _, e := range entries {
		base := e.Name()
		if e.IsDir() || base == playlistName || base == "_to_delete" || strings.HasPrefix(base, "_extract.") {
			continue
		}
		name := strings.TrimSuffix(base, filepath.Ext(base))
		m := discTagRE.FindStringSubmatch(name)
		if m == nil {
			return nil, false
		}
		num, err := strconv.Atoi(m[1])
		if err != nil {
			return nil, false
		}
		group = append(group, discFile{num, base})
	}
	if len(group) == 0 {
		return nil, false
	}
	nums := map[int]bool{}
	for _, f := range group {
		nums[f.num] = true
	}
	return group, len(nums) == 1
}

func checkStructureFile(report *audit.Report, sys string, class systemdb.Class, filePath, sysDir string, discGroups map[string][]discFile, jobs *discJobCollector) {
	base := filepath.Base(filePath)
	if isLeftoverInfix(base) {
		report.Add(audit.Fail, audit.CategoryStructure, sys, base, "incomplete conversion leftover; delete it", "")
		return
	}
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(base), "."))
	switch ext {
	case "part", "tmp":
		report.Add(audit.Fail, audit.CategoryStructure, sys, base, "incomplete transfer/conversion leftover", "")
		return
	case "sav", "srm", "state", "sh":
		if class != "" {
			report.Add(audit.Warn, audit.CategoryStructure, sys, base, "does not belong in roms/", "saves")
		}
		return
	}
	if nonROMExts[ext] || class == systemdb.ClassArcade {
		return
	}
	name := strings.TrimSuffix(base, filepath.Ext(base))
	m := discTagRE.FindStringSubmatch(name)
	if m == nil {
		return
	}
	key := discGroupKey(name)
	group := discGroups[key]
	nums := map[int]bool{}
	for _, f := range group {
		nums[f.num] = true
	}
	if len(nums) > 1 {
		report.Add(audit.Warn, audit.CategoryStructure, sys, base,
			"loose multi-disc file; belongs in a .m3u directory", "m3u")
		return
	}
	// Lone disc, no sibling disc number present - worth the one DAT
	// lookup checkIncompleteDiscSet does (see its own doc comment): find
	// out whether this game genuinely has more discs than this one,
	// rather than assuming every "(Disc N)"-tagged file is mid-group.
	//
	// group holds every file of this one disc number (os.ReadDir's own
	// sorted order, preserved by buildDiscGroups) - more than one for a
	// raw multi-track CD dump with no .cue sheet (see discTrackTagRE).
	// Only group[0] reports anything: the whole group shares one verdict
	// (complete/incomplete), so a second identical finding for its other
	// track file(s) would just be noise, not new information.
	if base != group[0].base {
		return
	}
	num, _ := strconv.Atoi(m[1])
	jobs.add(discCompletenessJob{
		sys: sys, class: class, sysDir: sysDir, reportPath: base, num: num, group: group,
		fallback: func() {
			report.Add(audit.Warn, audit.CategoryStructure, sys, base,
				"loose multi-disc file; belongs in a .m3u directory", "m3u")
		},
	})
}

// reportDiscCompleteness runs checkIncompleteDiscSet for one ambiguous
// disc (loose, via checkStructureFile, or already grouped inside a
// Title.m3u/ directory, via checkM3uDiscSet) and reports the result: a
// confirmed, genuinely incomplete set as a Warn finding at reportPath: a
// bare filename for a loose disc, a Title.m3u directory name for one
// already grouped. Nothing at all when the DAT confirms the title
// really is single-disc. onFallback, if non-nil, runs when nothing
// could be confirmed either way (no DAT, no hash match, or chdman/
// dolphin-tool unavailable) - checkM3uDiscSet passes nil: unlike a loose
// file, there's no pre-existing "needs grouping" warning to fall back to
// for something that's already correctly grouped.
func reportDiscCompleteness(report *audit.Report, sys string, class systemdb.Class, sysDir, reportPath string, num int, filesForDisc []discFile, discTools *discToolResolver, onFallback func()) {
	status := checkIncompleteDiscSet(sys, class, sysDir, num, filesForDisc, discTools)
	if status.confirmed {
		if status.incomplete() {
			report.Add(audit.Warn, audit.CategoryStructure, sys, reportPath, status.message(), "")
		}
		return
	}
	if onFallback != nil {
		onFallback()
	}
}

// discSetStatus is checkIncompleteDiscSet/matchIncompleteDiscSet's
// structured verdict for a lone "(Disc N)"-tagged file - shared between
// check's own finding message (message) and commands.Discs' dedicated
// report, so neither has to parse the other's formatted text back apart.
// confirmed is false (caller falls back to the generic "belongs in a
// .m3u directory" message, or nothing at all for an already-grouped
// disc - see reportDiscCompleteness) when there's no embedded DAT for
// the system, extraction of a .chd/.rvz failed or chdman/dolphin-tool
// isn't available (a .cso is left unsupported entirely - no extraction
// path for it today), or none of the disc's own file(s) hash-match
// anything in the DAT at all. When confirmed is true, incomplete() tells
// apart a genuinely missing disc (len(missing) > 0) from the DAT itself
// showing this title has only ever had one disc - nothing wrong to
// report, an improvement over the old unconditional "belongs in a .m3u
// directory" guess for that case.
type discSetStatus struct {
	confirmed  bool
	title      string
	present    int   // the one disc number already known to be on disk
	totalDiscs int   // the DAT's own count of discs for title
	missing    []int // totalDiscs' disc numbers not on disk, sorted
}

func (s discSetStatus) incomplete() bool { return s.confirmed && len(s.missing) > 0 }

// message is the check finding text for an incomplete set; empty (and
// never called) when !incomplete().
func (s discSetStatus) message() string {
	return fmt.Sprintf("incomplete disc set (%s): have disc %d of %d, missing %s",
		s.title, s.present, s.totalDiscs, joinInts(s.missing))
}

func joinInts(nums []int) string {
	parts := make([]string, len(nums))
	for i, n := range nums {
		parts[i] = strconv.Itoa(n)
	}
	return strings.Join(parts, ", ")
}

// discToolResolver lazily resolves chdman/dolphin-tool, each exactly
// once, the first time discFileHashes actually needs to extract a .chd/
// .rvz file to confirm an ambiguous disc. Deliberately not resolved up
// front the way chd/rvz/scrape/verify resolve their own tool before any
// work starts (see AGENTS.md's "check tool availability before acting"
// invariant): unlike those, check must stay usable with neither tool
// installed on the common path, since most runs never hit a disc
// ambiguous enough to need one at all. Scoped to one Check()/Discs()
// call (each creates its own), not process-global, so concurrent or
// repeated calls (tests) never share stale state; sync.Once makes it
// safe to share across checkStructure's own per-system goroutines.
type discToolResolver struct {
	chdmanOnce  sync.Once
	chdmanPath  string
	chdmanErr   error
	dolphinOnce sync.Once
	dolphinPath string
	dolphinErr  error
}

func (r *discToolResolver) chdman() (string, error) {
	r.chdmanOnce.Do(func() { r.chdmanPath, r.chdmanErr = tools.Resolve("chdman") })
	return r.chdmanPath, r.chdmanErr
}

func (r *discToolResolver) dolphinTool() (string, error) {
	r.dolphinOnce.Do(func() { r.dolphinPath, r.dolphinErr = tools.Resolve("dolphin-tool") })
	return r.dolphinPath, r.dolphinErr
}

// discFileHashes returns every hash discoverable for f: a plain
// whole-file hash for anything still in a raw/pre-conversion format,
// chdman-extracted per-track hashes for a .chd (hashDiscCHD; class picks
// CD/DVD/UMD extraction mode - see collection's own --verify-chd path,
// which this reuses directly), or a dolphin-tool-extracted whole-file
// hash for a .rvz (hashDiscRVZ, same reuse). A .cso has no equivalent
// "just extract this one file" path today and is left unsupported.
func discFileHashes(sysDir string, f discFile, class systemdb.Class, discTools *discToolResolver) ([]discTrackHash, error) {
	path := filepath.Join(sysDir, f.base)
	switch strings.ToLower(filepath.Ext(f.base)) {
	case ".chd":
		chdmanPath, err := discTools.chdman()
		if err != nil {
			return nil, err
		}
		return hashDiscCHD(chdmanPath, path, class)
	case ".rvz":
		dolphinToolPath, err := discTools.dolphinTool()
		if err != nil {
			return nil, err
		}
		h, err := hashDiscRVZ(dolphinToolPath, path)
		if err != nil {
			return nil, err
		}
		return []discTrackHash{h}, nil
	case ".cso":
		return nil, fmt.Errorf("%s: .cso extraction isn't supported here", path)
	default:
		crc, md5sum, sha1sum, err := hashROM(path)
		if err != nil {
			return nil, err
		}
		return []discTrackHash{{crc, md5sum, sha1sum}}, nil
	}
}

// checkIncompleteDiscSet is the gated, DAT-aware follow-up for an
// ambiguous disc - a lone "(Disc N)"-tagged file with no sibling disc
// number, loose or already grouped inside a Title.m3u/ directory: a
// disc-tagged file in that position is just as likely to be a
// single-disc game whose name happens to carry a "(Disc 1)" tag as it
// is to be genuinely missing the rest of a multi-disc set, and
// filenames alone can't tell those apart. Hashing+DAT-matching only
// this one disc (never a whole system's worth) keeps this cheap enough
// to run unconditionally even though it now extracts a .chd/.rvz when
// that's what it finds: the expensive part only ever happens on the
// rare disc it's actually needed for. filesForDisc is every file
// sharing the same title and num's disc number - more than one for a
// raw multi-track CD dump with no .cue sheet (see discTrackTagRE).
func checkIncompleteDiscSet(sys string, class systemdb.Class, sysDir string, num int, filesForDisc []discFile, discTools *discToolResolver) discSetStatus {
	df, ok, err := datdb.Load(sys, "")
	if !ok || err != nil {
		return discSetStatus{}
	}
	return matchIncompleteDiscSet(df, class, sysDir, num, filesForDisc, discTools)
}

// matchIncompleteDiscSet is checkIncompleteDiscSet's DAT-independent
// core (split out so it's testable against a synthetic datdb.Datafile,
// not just whatever's actually embedded/staged): hash filesForDisc,
// find which df Game they belong to, then compare the DAT's own total
// disc count for that title against the single disc number already
// known to be present.
func matchIncompleteDiscSet(df *datdb.Datafile, class systemdb.Class, sysDir string, num int, filesForDisc []discFile, discTools *discToolResolver) discSetStatus {
	var title string
	matched := false
outer:
	for _, f := range filesForDisc {
		hashes, err := discFileHashes(sysDir, f, class, discTools)
		if err != nil {
			continue
		}
		for _, h := range hashes {
			if name, ok := datMatchByHash(df, h.crc, h.md5sum, h.sha1sum); ok {
				title, matched = name, true
				break outer
			}
		}
	}
	if !matched {
		return discSetStatus{}
	}

	canonical := discGroupKey(title)
	expected := map[int]bool{}
	for _, g := range df.Games {
		name := g.Name
		if g.Description != "" {
			name = g.Description
		}
		if discGroupKey(name) != canonical {
			continue
		}
		if m := discTagRE.FindStringSubmatch(name); m != nil {
			if n, err := strconv.Atoi(m[1]); err == nil {
				expected[n] = true
			}
		}
	}
	if len(expected) <= 1 {
		return discSetStatus{confirmed: true, title: canonical, present: num, totalDiscs: len(expected)}
	}

	var missing []int
	for n := range expected {
		if n != num {
			missing = append(missing, n)
		}
	}
	sort.Ints(missing)
	return discSetStatus{
		confirmed: true, title: canonical, present: num,
		totalDiscs: len(expected), missing: missing,
	}
}

// datMatchByHash finds the Game in df whose any single ROM's CRC/MD5/
// SHA1 matches crc/md5sum/sha1sum, preferring Description over Name the
// same way datdb.Match does. Unlike Match's own have() check (every ROM
// of a Game must be present to count as a match), one matching track of
// a multi-track disc is enough here: the goal is only "what title does
// this already-flagged file belong to," not a full ownership verdict.
func datMatchByHash(df *datdb.Datafile, crc, md5sum, sha1sum string) (name string, ok bool) {
	for _, g := range df.Games {
		for _, r := range g.ROMs {
			if (crc != "" && r.CRC == crc) || (md5sum != "" && r.MD5 == md5sum) || (sha1sum != "" && r.SHA1 == sha1sum) {
				if g.Description != "" {
					return g.Description, true
				}
				return g.Name, true
			}
		}
	}
	return "", false
}

func checkContainers(report *audit.Report, lib *library.Library, sys string, esdeNames map[string]string) {
	class := systemdb.ClassOf(sys)
	sysDir := filepath.Join(lib.Roms, sys)
	entries, err := os.ReadDir(sysDir)
	if err != nil {
		return
	}
	for _, e := range entries {
		base := e.Name()
		if base == "systeminfo.txt" || base == ".directory" {
			continue
		}
		path := filepath.Join(sysDir, base)
		if e.IsDir() {
			if strings.HasSuffix(base, ".m3u") {
				// checkM3uContainers (via checkContainerDir) scans each
				// disc inside individually - scanning the directory entry
				// itself here too would double-count it against that.
				checkContainerDir(report, sys, class, path, esdeNames)
				continue
			}
			report.Scan(audit.CategoryContainers, 1)
			checkContainerDir(report, sys, class, path, esdeNames)
			continue
		}
		report.Scan(audit.CategoryContainers, 1)
		checkContainerFile(report, sys, class, path, base, esdeNames)
	}
}

func checkContainerDir(report *audit.Report, sys string, class systemdb.Class, dirPath string, esdeNames map[string]string) {
	base := filepath.Base(dirPath)
	switch {
	case base == "_to_delete", strings.HasPrefix(base, "_extract."):
		return // checkStructureDir's territory
	case strings.HasSuffix(base, ".m3u"):
		checkM3uContainers(report, sys, class, dirPath, esdeNames)
		return
	}
	switch class {
	case systemdb.ClassCD, systemdb.ClassDVD:
		report.Add(audit.Warn, audit.CategoryContainers, sys, base, "game directory; convert to CHD", "chd")
	case systemdb.ClassPS3:
		switch {
		case exists(filepath.Join(dirPath, "PS3_GAME", "USRDIR", "EBOOT.BIN")):
			report.Add(audit.Info, audit.CategoryContainers, sys, base, "disc folder; could be a single ISO", "ps3")
		case exists(filepath.Join(dirPath, "USRDIR", "EBOOT.BIN")):
			report.Add(audit.Warn, audit.CategoryContainers, sys, base,
				"installed/PSN-style folder; needs a license (RAP) and cannot become an ISO", "")
		default:
			report.Add(audit.Fail, audit.CategoryContainers, sys, base, "no EBOOT.BIN found", "")
		}
	}
}

// checkM3uContainers runs checkContainerFile against every disc inside a
// Title.m3u/ playlist directory (dirPath) - the same container/format
// validation (invalid CHD header, wrong CD/DVD type, a wrong-system
// signature, ...) a bare file gets, just labeled "Title.m3u/Disc.ext" so
// a finding still names exactly which disc. The playlist text file
// itself is skipped (nothing to validate as a disc); checkStructureDir's
// own checkM3uDir already covers playlist/disc-count consistency
// separately. Previously checkContainerDir returned immediately for any
// .m3u directory, so every disc inside an already-grouped multi-disc
// game - often a library's biggest, most valuable titles - was entirely
// invisible to check: a corrupt header or a CD/DVD-type mismatch there
// could only ever be caught by the much heavier verify command, and
// verify doesn't even check CD/DVD type at all.
func checkM3uContainers(report *audit.Report, sys string, class systemdb.Class, dirPath string, esdeNames map[string]string) {
	base := filepath.Base(dirPath)
	playlistName := strings.TrimSuffix(base, ".m3u") + ".m3u"
	discs, err := os.ReadDir(dirPath)
	if err != nil {
		return
	}
	for _, d := range discs {
		name := d.Name()
		if d.IsDir() || name == playlistName {
			continue
		}
		report.Scan(audit.CategoryContainers, 1)
		checkContainerFile(report, sys, class, filepath.Join(dirPath, name), filepath.Join(base, name), esdeNames)
	}
}

func checkContainerFile(report *audit.Report, sys string, class systemdb.Class, filePath, label string, esdeNames map[string]string) {
	base := label
	if isLeftoverInfix(base) {
		return // checkStructureFile's territory
	}
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(base), "."))
	if nonROMExts[ext] {
		return
	}

	// Checked ahead of (and independent from) the extension-keyed switch
	// below, so it still catches a lone, cue-less .bin that the switch's
	// own "cd:bin"/"cd:img" case would otherwise return out of silently -
	// exactly the shape of the real misfiled Genesis cartridge ROM this
	// check exists to catch. Bare files only; archive contents aren't
	// inspected here (see check's own --dupes cost tradeoff) - a misfiled
	// ROM still inside an archive is instead named by chd/unpack's own
	// conversion errors.
	if class != systemdb.ClassArcade {
		if fam, ok := signature.Identify(filePath); ok && !signature.ValidFor(fam, sys) {
			report.Add(audit.Warn, audit.CategoryContainers, sys, base,
				fmt.Sprintf("identifies as a %s ROM, not %s", signature.Name[fam], systemdb.DisplayNameWith(sys, esdeNames)), "relocate")
		}
	}

	key := string(class) + ":" + ext
	switch key {
	case "cart:zip", "cart:7z", "cart:rar", "n64:zip", "n64:7z", "n64:rar", "snes:zip", "snes:7z", "snes:rar":
		report.Add(audit.Warn, audit.CategoryContainers, sys, base, "archive; store bare", "unpack")
	case "cd:cue", "cd:gdi", "cd:iso", "cd:bin", "cd:img", "cd:ccd", "cd:mds", "cd:toc", "cd:zip", "cd:7z", "cd:rar":
		if ext == "bin" || ext == "img" {
			return
		}
		report.Add(audit.Warn, audit.CategoryContainers, sys, base, "raw/archived disc image; convert to CHD", "chd")
	case "dvd:iso", "dvd:cue", "dvd:zip", "dvd:7z", "dvd:rar":
		report.Add(audit.Warn, audit.CategoryContainers, sys, base, "raw/archived disc image; convert to CHD", "chd")
	case "cd:chd", "dvd:chd", "umd:chd":
		checkCHD(report, sys, class, base, filePath)
	case "umd:iso", "umd:cso":
		report.Add(audit.Warn, audit.CategoryContainers, sys, base, "convert to CHD", "chd")
	case "dolphin:iso", "dolphin:gcm", "dolphin:wbfs", "dolphin:ciso", "dolphin:gcz":
		report.Add(audit.Warn, audit.CategoryContainers, sys, base, "convert to RVZ", "rvz")
	case "xbox:iso":
		checkXbox(report, sys, base, filePath, true)
	case "xbox:xiso":
		checkXbox(report, sys, base, filePath, false)
	case "ps3:iso":
		if ok, err := iso9660.Complete(filePath); err != nil || !ok {
			report.Add(audit.Fail, audit.CategoryContainers, sys, base, "ISO is truncated or not ISO9660", "")
		}
	case "ps3:7z", "ps3:zip", "ps3:rar":
		report.Add(audit.Warn, audit.CategoryContainers, sys, base, "archived PS3 dump", "ps3")
	case "n64:v64", "n64:n64":
		report.Add(audit.Warn, audit.CategoryContainers, sys, base, "byte-swapped N64 ROM", "n64")
	case "n64:z64":
		if order, err := n64.Detect(filePath); err == nil && order != n64.OrderZ64 {
			report.Add(audit.Warn, audit.CategoryContainers, sys, base, ".z64 with non-native byte order", "n64")
		}
	case "snes:smc":
		report.Add(audit.Warn, audit.CategoryContainers, sys, base, ".smc; use headerless .sfc", "snes")
	case "snes:sfc":
		if info, err := os.Stat(filePath); err == nil && snes.HasHeader(info.Size()) {
			report.Add(audit.Warn, audit.CategoryContainers, sys, base, "has 512-byte copier header", "snes")
		}
	}
}

func checkCHD(report *audit.Report, sys string, class systemdb.Class, base, filePath string) {
	kind, err := chd.Classify(filePath)
	if err != nil {
		return
	}
	switch {
	case kind == chd.KindBad:
		report.Add(audit.Fail, audit.CategoryContainers, sys, base, "not a valid CHD", "")
	case strings.HasPrefix(string(kind), "v"):
		report.Add(audit.Warn, audit.CategoryContainers, sys, base,
			fmt.Sprintf("old CHD %s; re-create with current chdman", kind), "")
	case kind == chd.KindCDAsDVDMAGE:
		if class == systemdb.ClassDVD {
			report.Add(audit.Warn, audit.CategoryContainers, sys, base, "DVD image stored as CD-type CHD", "chd")
		}
	case kind == chd.KindDVD:
		if class == systemdb.ClassCD {
			report.Add(audit.Fail, audit.CategoryContainers, sys, base, "DVD-type CHD in a CD system", "")
		}
	}
}

func checkXbox(report *audit.Report, sys, base, filePath string, isISO bool) {
	ok, err := xiso.IsXISO(filePath)
	if err != nil {
		return
	}
	if isISO {
		if ok {
			report.Add(audit.Info, audit.CategoryContainers, sys, base, "XISO with .iso extension", "xiso")
		} else {
			report.Add(audit.Warn, audit.CategoryContainers, sys, base,
				"full/redump disc image; convert to XISO", "xiso")
		}
		return
	}
	if !ok {
		report.Add(audit.Fail, audit.CategoryContainers, sys, base, "not a valid XISO", "")
	}
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
