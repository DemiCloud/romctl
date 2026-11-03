package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/demicloud/romctl/internal/actions"
	"github.com/demicloud/romctl/internal/audit"
	"github.com/demicloud/romctl/internal/cliutil"
	"github.com/demicloud/romctl/internal/formats/chd"
	"github.com/demicloud/romctl/internal/formats/iso9660"
	"github.com/demicloud/romctl/internal/formats/n64"
	"github.com/demicloud/romctl/internal/formats/snes"
	"github.com/demicloud/romctl/internal/formats/xiso"
	"github.com/demicloud/romctl/internal/library"
	"github.com/demicloud/romctl/internal/systemdb"
)

// Check audits every system (structure, naming, formats) and BIOS files,
// prints the FAIL/WARN/INFO findings and a colorized summary, and reports
// whether anything FAILed. dupes additionally scans each system for exact
// (byte-for-byte) duplicate files - off by default since, unlike every
// other check, it has to read every file's full content. scheme
// (`[naming] scheme`) is the naming-audit category's yardstick: a name
// internal/actions.ApplyScheme wouldn't change for the configured scheme
// isn't a finding.
func Check(lib *library.Library, systems []string, dupes bool, scheme actions.Scheme) (bool, error) {
	report := audit.New(os.Stdout)
	if dupes {
		report.EnableDuplicates()
	}
	for _, sys := range systems {
		checkSystem(report, lib, sys, scheme)
		if dupes {
			checkDuplicates(report, sys, filepath.Join(lib.Roms, sys))
		}
	}
	checkBios(report, lib, systems)
	colors := cliutil.NewColors(os.Stdout)
	report.Checklist(os.Stdout, colors)
	report.Summary(os.Stdout, colors)
	return !report.Failed(), nil
}

// Bios audits only BIOS files.
func Bios(lib *library.Library, systems []string) (bool, error) {
	report := audit.New(os.Stdout)
	checkBios(report, lib, systems)
	if report.BiosSkipped() {
		fmt.Println("no bios/ directory; skipping BIOS checks")
	}
	report.Summary(os.Stdout, cliutil.NewColors(os.Stdout))
	return !report.Failed(), nil
}

func checkSystem(report *audit.Report, lib *library.Library, sys string, scheme actions.Scheme) {
	class := systemdb.ClassOf(sys)
	sysDir := filepath.Join(lib.Roms, sys)
	entries, err := os.ReadDir(sysDir)
	if err != nil {
		return
	}
	for _, e := range entries {
		name := e.Name()
		if name == "systeminfo.txt" || name == ".directory" {
			continue
		}
		path := filepath.Join(sysDir, name)
		if e.IsDir() {
			checkDir(report, sys, class, path)
		} else {
			checkFile(report, sys, class, path, scheme)
		}
	}
}

func checkDir(report *audit.Report, sys string, class systemdb.Class, dirPath string) {
	base := filepath.Base(dirPath)
	switch {
	case base == "_to_delete":
		report.Add(audit.Info, audit.CategoryStructure, sys, base, "pending deletion", "clean")
		return
	case strings.HasPrefix(base, "_extract."):
		report.Add(audit.Fail, audit.CategoryStructure, sys, base, "leftover extraction directory", "")
		return
	case strings.HasSuffix(base, ".m3u"):
		checkM3uDir(report, sys, dirPath)
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
		if filepath.Join(dirPath, e.Name()) != playlist {
			discs++
		}
	}
	if lines != discs {
		report.Add(audit.Warn, audit.CategoryStructure, sys, name,
			fmt.Sprintf("%d disc file(s) but %d playlist entries", discs, lines), "")
	}
}

var discTagRE = regexp.MustCompile(`\(Disc \d+\)`)

func checkFile(report *audit.Report, sys string, class systemdb.Class, filePath string, scheme actions.Scheme) {
	base := filepath.Base(filePath)
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(base), "."))
	name := strings.TrimSuffix(base, filepath.Ext(base))

	switch {
	case strings.Contains(base, ".partial.") || strings.Contains(base, ".tmp.") || strings.Contains(base, ".romctl-rename."):
		report.Add(audit.Fail, audit.CategoryStructure, sys, base, "incomplete conversion leftover; delete it", "")
		return
	}
	switch ext {
	case "part", "tmp":
		report.Add(audit.Fail, audit.CategoryStructure, sys, base, "incomplete transfer/conversion leftover", "")
		return
	case "sav", "srm", "state", "sh":
		if class != "" {
			report.Add(audit.Warn, audit.CategoryStructure, sys, base, "does not belong in roms/", "")
		}
		return
	case "txt", "ini", "cheats", "ips", "bps", "ups", "sbi", "dat", "directory":
		return
	}

	if class != systemdb.ClassArcade {
		if clean := actions.ApplyScheme(name, scheme); clean != name {
			report.Add(audit.Warn, audit.CategoryNaming, sys, base, fmt.Sprintf("naming -> %q", clean+"."+ext), "rename")
		}
		if discTagRE.MatchString(name) {
			report.Add(audit.Warn, audit.CategoryStructure, sys, base,
				"loose multi-disc file; belongs in a .m3u directory", "m3u")
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
			report.Add(audit.Warn, audit.CategoryContainers, sys, base, "DVD image stored as CD-type CHD", "dvd")
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

func checkBios(report *audit.Report, lib *library.Library, systems []string) {
	if info, err := os.Stat(lib.Bios); err != nil || !info.IsDir() {
		report.SkipBios()
		return
	}

	for _, sys := range systems {
		groups, ok := systemdb.BiosRequired[sys]
		if !ok || !library.HasGames(filepath.Join(lib.Roms, sys)) {
			continue
		}
		for _, group := range groups {
			found := false
			for _, alt := range group {
				matches, _ := filepath.Glob(filepath.Join(lib.Bios, alt))
				if len(matches) > 0 {
					found = true
				}
			}
			if !found {
				report.Add(audit.Fail, audit.CategoryBios, sys, "bios/"+strings.Join(group, "|"), "required BIOS missing", "")
			}
		}
	}

	for name, wantSum := range systemdb.BiosMD5 {
		path := filepath.Join(lib.Bios, name)
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		sum := md5Hex(data)
		if sum != wantSum {
			report.Add(audit.Fail, audit.CategoryBios, "bios", name,
				fmt.Sprintf("MD5 %s does not match reference %s", sum, wantSum), "")
		}
	}

	if info, err := os.Stat(filepath.Join(lib.Bios, "firmware.bin")); err == nil {
		if info.Size() != 262144 {
			report.Add(audit.Warn, audit.CategoryBios, "bios", "firmware.bin",
				"unexpected size for DS firmware (want "+strconv.Itoa(262144)+")", "")
		}
	}
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
