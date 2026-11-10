package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/demicloud/romctl/internal/actions"
	"github.com/demicloud/romctl/internal/audit"
	"github.com/demicloud/romctl/internal/cliutil"
	"github.com/demicloud/romctl/internal/formats/chd"
	"github.com/demicloud/romctl/internal/formats/iso9660"
	"github.com/demicloud/romctl/internal/formats/n64"
	"github.com/demicloud/romctl/internal/formats/signature"
	"github.com/demicloud/romctl/internal/formats/snes"
	"github.com/demicloud/romctl/internal/formats/xiso"
	"github.com/demicloud/romctl/internal/library"
	"github.com/demicloud/romctl/internal/systemdb"
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
			cliutil.Parallel(systems, func(sys string) { checkStructure(report, lib, sys) })
			checkUnknownSystems(report, lib, systems)
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

func checkStructure(report *audit.Report, lib *library.Library, sys string) {
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
		report.Scan(audit.CategoryStructure, 1)
		path := filepath.Join(sysDir, base)
		if e.IsDir() {
			checkStructureDir(report, sys, path)
		} else {
			checkStructureFile(report, sys, class, path)
		}
	}
}

func checkStructureDir(report *audit.Report, sys, dirPath string) {
	base := filepath.Base(dirPath)
	switch {
	case base == "_to_delete":
		report.Add(audit.Info, audit.CategoryStructure, sys, base, "pending deletion", "clean")
	case strings.HasPrefix(base, "_extract."):
		report.Add(audit.Fail, audit.CategoryStructure, sys, base, "leftover extraction directory", "")
	case strings.HasSuffix(base, ".m3u"):
		checkM3uDir(report, sys, dirPath)
	}
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

var discTagRE = regexp.MustCompile(`\(Disc \d+\)`)

func checkStructureFile(report *audit.Report, sys string, class systemdb.Class, filePath string) {
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
	if discTagRE.MatchString(name) {
		report.Add(audit.Warn, audit.CategoryStructure, sys, base,
			"loose multi-disc file; belongs in a .m3u directory", "m3u")
	}
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
