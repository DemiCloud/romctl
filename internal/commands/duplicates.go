package commands

import (
	"fmt"
	"hash/crc32"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/demicloud/romctl/internal/audit"
	"github.com/demicloud/romctl/internal/cliutil"
	"github.com/demicloud/romctl/internal/library"
)

// Dupes scans every given system for exact (byte-for-byte) duplicate
// files, flagging all but the first of each group as a WARN finding. Its
// own command, rather than a check flag: unlike every other check, this
// means reading every file's full content, which can take long enough on
// a library with large CHD/ISO files that folding it into check's own
// instant, all-categories-at-once output made a long run indistinguishable
// from a hang - the same reasoning that already split `bios` out as its
// own command.
//
// This only catches exact duplicates. A game present under two different
// region dumps (different hashes, same underlying game) isn't detected -
// doing that precisely needs No-Intro/Redump-style DAT clone-group data,
// which is a deliberately separate, bigger scope question.
//
// Default shows a live "dupes: done/total" counter while it runs
// (cliutil.Progress, the same primitive every mutating command already
// uses for this), hashing every system's files across a worker pool.
// -v/--verbose, on a real Fancy terminal (cliutil.CanAnimate), instead
// drives cliutil.ParallelLive per system: one spinning line per active
// worker naming the file it's hashing right now, finalized to a
// permanent colored ✓/✗ the moment that worker moves to its next file.
// Verbose without a usable terminal (plain/structured output, or
// redirected to a file) falls back to one plain "[i/total] hash sys/file"
// line per file instead, processed one at a time to keep the printed
// order sane. Either way, an actual duplicate is only ever found after
// every file in its system has already finished hashing (the grouping
// needs every hash first), so it never has to interleave with a live
// display still in progress - it always prints via the ordinary
// Report.Add, same as every other check finding.
func Dupes(lib *library.Library, systems []string, verbose bool, format cliutil.Format) (bool, error) {
	report := audit.New(os.Stdout, verbose, format, "")

	type sysFiles struct {
		sys   string
		files []gameFile
	}
	var bySys []sysFiles
	total := 0
	for _, sys := range systems {
		files := listGameFiles(filepath.Join(lib.Roms, sys), hashable)
		report.Scan(audit.CategoryDuplicates, len(files))
		bySys = append(bySys, sysFiles{sys, files})
		total += len(files)
	}

	live := verbose && cliutil.CanAnimate(format)
	doneSoFar := 0
	process := func(tick func()) {
		for _, sf := range bySys {
			var mu sync.Mutex
			byHash := map[uint32][]string{}
			hashOne := func(gf gameFile) bool {
				sum, err := crc32File(gf.path)
				tick()
				if err != nil {
					return false
				}
				mu.Lock()
				byHash[sum] = append(byHash[sum], gf.label)
				mu.Unlock()
				return true
			}

			switch {
			case live:
				cliutil.ParallelLive(format, sf.files, func(gf gameFile) string { return sf.sys + "/" + gf.label }, hashOne)
			case verbose:
				for i, gf := range sf.files {
					fmt.Printf("[%d/%d] hash %s/%s\n", doneSoFar+i+1, total, sf.sys, gf.label)
					hashOne(gf)
				}
				doneSoFar += len(sf.files)
			default:
				cliutil.Parallel(sf.files, func(gf gameFile) { hashOne(gf) })
			}

			hashes := make([]uint32, 0, len(byHash))
			for h := range byHash {
				hashes = append(hashes, h)
			}
			sort.Slice(hashes, func(i, j int) bool { return hashes[i] < hashes[j] })
			for _, h := range hashes {
				labels := byHash[h]
				if len(labels) < 2 {
					continue
				}
				sort.Strings(labels)
				for _, label := range labels[1:] {
					report.Add(audit.Warn, audit.CategoryDuplicates, sf.sys, label,
						fmt.Sprintf("identical to %s (CRC32 %08X)", labels[0], h), "")
				}
			}
		}
	}

	if live || verbose {
		process(func() {})
	} else {
		cliutil.Progress(format, false, false, "dupes", total, process)
	}

	if format.Structured() {
		return !report.Failed(), report.Encode(os.Stdout, format)
	}
	report.Table(os.Stdout)
	report.Fixes(os.Stdout)
	return !report.Failed(), nil
}

// hashable reports whether a filename is worth hashing for duplicate
// detection - the same non-ROM extensions and leftover markers the rest
// of check already ignores.
func hashable(name string) bool {
	if name == "systeminfo.txt" || name == ".directory" {
		return false
	}
	if strings.Contains(name, ".partial.") || strings.Contains(name, ".tmp.") || strings.Contains(name, ".romctl-rename.") {
		return false
	}
	switch strings.ToLower(strings.TrimPrefix(filepath.Ext(name), ".")) {
	case "txt", "ini", "cheats", "ips", "bps", "ups", "sbi", "dat", "directory",
		"sav", "srm", "state", "sh", "part", "tmp":
		return false
	}
	return true
}

func crc32File(path string) (uint32, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	h := crc32.NewIEEE()
	if _, err := io.Copy(h, f); err != nil {
		return 0, err
	}
	return h.Sum32(), nil
}
