package commands

import (
	"crypto/md5"
	"crypto/sha1"
	"fmt"
	"hash/crc32"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/demicloud/romctl/internal/archive"
	"github.com/demicloud/romctl/internal/cliutil"
	"github.com/demicloud/romctl/internal/datdb"
	"github.com/demicloud/romctl/internal/library"
)

// collectionRow is one known game's shape for every output form, same
// cliutil.RenderList convention as list/bios -v.
type collectionRow struct {
	System  string `json:"system" yaml:"system" toml:"system" col:"System"`
	Game    string `json:"game" yaml:"game" toml:"game" col:"Game"`
	Status  string `json:"status" yaml:"status" toml:"status" col:"Status"` // "have", "missing", "unverifiable"
	Matched string `json:"matched" yaml:"matched" toml:"matched" col:"Matched File"`
}

// Collection compares every given system's games against its embedded (or
// --dat-dir-overridden) No-Intro/Redump DAT, reporting one row per known
// game: have, missing, or unverifiable.
//
// A system with no DatSource entry at all (arcade, and a few niches - see
// systemdb.Dat) is silently skipped, same as a system with no games at
// all would be; a system with an entry but no DAT actually staged (the
// common case until `make dats` has been run - see the embedded data
// directories' MANIFEST placeholders) gets a one-line note printed to
// stderr instead of failing the whole run, so asking for "all" still
// reports every system that does have coverage.
//
// CHD-converted files are reported as "unverifiable", not matched or
// missing: no CHD metadata stores a hash that lines up with a Redump DAT
// entry (chdman verify only checks a CHD against itself), and
// reconstructing one by re-extracting every CHD back to raw tracks was
// ruled out as too slow and not even guaranteed byte-exact for multi-track
// CDs. This only affects disc-based (Redump) systems; cartridge systems
// (No-Intro) are never CHD-converted, so they always get a real have/
// missing verdict. An archived (.zip/.7z/.rar) ROM is hashed by streaming
// straight through the archive reader (its sole entry - a multi-file
// archive is skipped, same as hashable callers elsewhere expect one ROM
// per archive), no extraction to disk needed.
//
// No parent/clone (1G1R) grouping: every DAT entry, including every
// region/revision/translation variant, is its own row. A future --group
// flag could collapse these; v1 doesn't, to keep the matching logic
// itself simple.
//
// Hashing runs the same way dupes/verify already do: a live per-worker
// spinner display in verbose mode on a real terminal, one plain
// "[i/total] hash sys/file" line per file in verbose mode otherwise, or a
// "collection: done/total" counter by default - across a worker pool
// either way.
func Collection(lib *library.Library, systems []string, datDir string, verbose bool, format cliutil.Format) (bool, error) {
	colors := cliutil.ColorsFor(format)
	failed := false

	type sysJob struct {
		sys   string
		names []string
		df    *datdb.Datafile
	}
	var jobs []sysJob
	total := 0
	for _, sys := range systems {
		df, ok, err := datdb.Load(sys, datDir)
		if !ok {
			continue
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "collection: %s: %v\n", sys, err)
			failed = true
			continue
		}
		entries, err := os.ReadDir(filepath.Join(lib.Roms, sys))
		if err != nil {
			continue
		}
		var names []string
		for _, e := range entries {
			if e.IsDir() || !hashable(e.Name()) {
				continue
			}
			names = append(names, e.Name())
		}
		jobs = append(jobs, sysJob{sys, names, df})
		total += len(names)
	}

	live := verbose && cliutil.CanAnimate(format)
	var rows []collectionRow
	doneSoFar := 0
	process := func(tick func()) {
		for _, job := range jobs {
			sysDir := filepath.Join(lib.Roms, job.sys)
			var mu sync.Mutex
			byCRC, byMD5, bySHA1 := map[string]string{}, map[string]string{}, map[string]string{}
			var unverifiable []string

			hashOne := func(name string) bool {
				defer tick()
				path := filepath.Join(sysDir, name)
				if strings.ToLower(filepath.Ext(name)) == ".chd" {
					mu.Lock()
					unverifiable = append(unverifiable, name)
					mu.Unlock()
					return true
				}
				crc, md5sum, sha1sum, err := hashROM(path)
				if err != nil {
					return false
				}
				mu.Lock()
				byCRC[crc], byMD5[md5sum], bySHA1[sha1sum] = name, name, name
				mu.Unlock()
				return true
			}

			switch {
			case live:
				cliutil.ParallelLive(format, job.names, func(name string) string { return job.sys + "/" + name }, hashOne)
			case verbose:
				for i, name := range job.names {
					fmt.Printf("[%d/%d] hash %s/%s\n", doneSoFar+i+1, total, job.sys, name)
					hashOne(name)
				}
				doneSoFar += len(job.names)
			default:
				cliutil.Parallel(job.names, func(name string) { hashOne(name) })
			}

			for _, m := range job.df.Match(byCRC, byMD5, bySHA1) {
				rows = append(rows, collectionRowFor(job.sys, m, colors))
			}
			for _, name := range unverifiable {
				rows = append(rows, collectionRow{System: job.sys, Game: name, Status: statusColor("unverifiable", colors), Matched: "-"})
			}
		}
	}

	if live || verbose {
		process(func() {})
	} else {
		cliutil.Progress(format, false, false, "collection", total, process)
	}

	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].System != rows[j].System {
			return rows[i].System < rows[j].System
		}
		return rows[i].Game < rows[j].Game
	})

	if err := cliutil.RenderList(os.Stdout, format, rows); err != nil {
		return false, err
	}
	return !failed, nil
}

func collectionRowFor(sys string, m datdb.MatchResult, colors cliutil.Colors) collectionRow {
	status, matched := "missing", "-"
	if m.Have {
		status, matched = "have", strings.Join(m.Matched, ", ")
	}
	return collectionRow{System: sys, Game: m.Game, Status: statusColor(status, colors), Matched: matched}
}

func statusColor(status string, colors cliutil.Colors) string {
	switch status {
	case "have":
		return colors.Green + status + colors.Reset
	case "missing":
		return colors.Red + status + colors.Reset
	default:
		return colors.Yellow + status + colors.Reset
	}
}

// hashROM returns path's lowercase-hex CRC32/MD5/SHA1, streamed in one
// pass - or, for a .zip/.7z/.rar, its sole entry's hashes, extracted
// straight into the hashers with no temp file. A multi-file archive
// errors (nothing sensible to hash as "the ROM"), same as archive.SoleEntry
// reports it.
func hashROM(path string) (crc, md5sum, sha1sum string, err error) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".zip", ".7z", ".rar":
		return hashArchiveROM(path)
	default:
		f, err := os.Open(path)
		if err != nil {
			return "", "", "", err
		}
		defer f.Close()
		return hashReader(f)
	}
}

func hashArchiveROM(path string) (crc, md5sum, sha1sum string, err error) {
	name, ok, err := archive.SoleEntry(path)
	if err != nil {
		return "", "", "", err
	}
	if !ok {
		return "", "", "", fmt.Errorf("%s: not a single-file archive", path)
	}
	a, err := archive.Open(path)
	if err != nil {
		return "", "", "", err
	}
	defer a.Close()

	hc, hm, hs := crc32.NewIEEE(), md5.New(), sha1.New()
	if err := a.Extract(name, io.MultiWriter(hc, hm, hs)); err != nil {
		return "", "", "", err
	}
	return fmt.Sprintf("%08x", hc.Sum32()), fmt.Sprintf("%x", hm.Sum(nil)), fmt.Sprintf("%x", hs.Sum(nil)), nil
}

func hashReader(r io.Reader) (crc, md5sum, sha1sum string, err error) {
	hc, hm, hs := crc32.NewIEEE(), md5.New(), sha1.New()
	if _, err := io.Copy(io.MultiWriter(hc, hm, hs), r); err != nil {
		return "", "", "", err
	}
	return fmt.Sprintf("%08x", hc.Sum32()), fmt.Sprintf("%x", hm.Sum(nil)), fmt.Sprintf("%x", hs.Sum(nil)), nil
}
