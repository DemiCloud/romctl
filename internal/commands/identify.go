package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/demicloud/romctl/internal/actions"
	"github.com/demicloud/romctl/internal/cliutil"
	"github.com/demicloud/romctl/internal/datdb"
	"github.com/demicloud/romctl/internal/gamelist"
	"github.com/demicloud/romctl/internal/library"
	"github.com/demicloud/romctl/internal/systemdb"
)

// identifyRow is one hash-confirmed rename's shape for the human-readable
// table Identify prints when done - System first (so a multi-system run
// still reads as "one table per system" once sorted/grouped), same
// cliutil.RenderList convention as collectionRow. Hash is the matched
// file's own CRC32, not necessarily the DAT field that actually proved
// the match (Redump entries key on MD5/SHA1 when CRC is absent) - still
// the most recognizable single value to show, and every hashed file has
// one regardless of which field matched.
type identifyRow struct {
	System string `json:"system" yaml:"system" toml:"system" col:"System"`
	File   string `json:"file" yaml:"file" toml:"file" col:"File"`
	Game   string `json:"game" yaml:"game" toml:"game" col:"Game"`
	Hash   string `json:"hash" yaml:"hash" toml:"hash" col:"Hash"`
}

// Identify hash-matches every given system's files against its embedded
// (or --dat-dir-overridden) No-Intro/Redump DAT - the same matching
// collection.Collection already does - and renames a confirmed match to
// its DAT game name, run through ApplyScheme (the same --naming logic
// `rename` already uses), instead of leaving the file under whatever
// name it already had.
//
// Unlike `rename`, this is additive, not a reformat of every file: a
// file identify can't confirm by hash is left completely untouched,
// whatever its current name - run `rename` separately for generic tag
// cleanup on everything identify didn't verify. Only a DAT Game with
// exactly one ROM that unambiguously matches exactly one local file
// qualifies; a multi-ROM game (its match spans more than one file) or a
// file whose hash matches more than one different Game (an actual
// byte-identical ROM shared by two DAT entries) is left alone rather
// than guessed at. ClassArcade systems are skipped entirely, same as
// `rename` already does: arcade filenames are emulator-resolved set
// names. A `.chd` is never hashed or renamed by default, for the same
// reason Collection reports it "unverifiable" - no CHD metadata hash
// lines up with a Redump DAT entry; verifyCHD (--verify-chd) opts into
// extracting and hashing it instead, same cost/caveats as Collection's
// own verifyCHD - see its doc comment.
//
// This is its own command, not a `rename` flag, for the same reason
// `dupes`/`verify` are their own commands rather than `check` flags:
// hashing an entire library is occasionally expensive enough that
// folding it into an otherwise-instant command makes a long run
// indistinguishable from a hang. Hashing runs the same way
// collection/dupes/verify already do: a live per-worker spinner in
// verbose mode on a real terminal, one plain "[i/total] hash sys/file"
// line per file in verbose mode otherwise, or an "identify: done/total"
// counter by default.
func Identify(lib *library.Library, runner *actions.Runner, systems []string, datDir string, verifyCHD bool, scheme actions.Scheme, limit int) (bool, error) {
	start := time.Now()
	log := cliutil.NewLogger("identify", runner.Format)

	var chdmanPath string
	if verifyCHD {
		var err error
		if chdmanPath, err = resolveCHDManIfNeeded(systems); err != nil {
			return false, err
		}
	}

	type sysJob struct {
		sys   string
		class systemdb.Class
		names []string
		df    *datdb.Datafile
	}
	var jobs []sysJob
	var datIssues []datLoadRow
	total := 0
	for _, sys := range systems {
		class := systemdb.ClassOf(sys)
		if class == systemdb.ClassArcade {
			continue
		}
		df, ok, issue := loadDat(sys, datDir)
		if !ok {
			continue
		}
		if issue != nil {
			datIssues = append(datIssues, *issue)
			continue
		}
		if verifyCHD && isDiscClass(class) {
			df = stripSidecarROMs(df)
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
		jobs = append(jobs, sysJob{sys, class, names, df})
		total += len(names)
	}
	printDatLoadIssues("identify", datIssues)

	verbose, format := runner.Verbose, runner.Format
	live := verbose && cliutil.CanAnimate(format)
	var renamed, failed, skippedCHD atomic.Int64
	var rows []identifyRow
	doneSoFar := 0
	process := func(tick func()) {
		for _, job := range jobs {
			sysDir := filepath.Join(lib.Roms, job.sys)
			var mu sync.Mutex
			byCRC, byMD5, bySHA1 := map[string]string{}, map[string]string{}, map[string]string{}
			nameToCRC := map[string]string{}

			hashOne := func(name string) bool {
				defer tick()
				if strings.ToLower(filepath.Ext(name)) == ".chd" {
					if !verifyCHD || !isDiscClass(job.class) {
						skippedCHD.Add(1)
						return true
					}
					hashes, err := hashDiscCHD(chdmanPath, filepath.Join(sysDir, name), job.class)
					if err != nil {
						fmt.Fprintf(os.Stderr, "identify: %s/%s: extraction failed, leaving untouched: %v\n", job.sys, name, err)
						return true
					}
					mu.Lock()
					for _, h := range hashes {
						byCRC[h.crc], byMD5[h.md5sum], bySHA1[h.sha1sum] = name, name, name
						nameToCRC[name] = h.crc
					}
					mu.Unlock()
					return true
				}
				crc, md5sum, sha1sum, err := hashROM(filepath.Join(sysDir, name))
				if err != nil {
					return false
				}
				mu.Lock()
				byCRC[crc], byMD5[md5sum], bySHA1[sha1sum] = name, name, name
				nameToCRC[name] = crc
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

			matched := map[string]string{} // filename -> canonical game name
			ambiguous := map[string]bool{}
			for _, m := range job.df.Match(byCRC, byMD5, bySHA1) {
				if !m.Have {
					continue
				}
				// A multi-track --verify-chd match repeats the same
				// filename once per matched track - dedup before
				// judging "how many files does this span": one unique
				// name is still a legitimate single-file rename target,
				// unlike a genuine multi-ROM match across separate files.
				names := dedupStrings(m.Matched)
				if len(names) != 1 {
					continue
				}
				name := names[0]
				if existing, ok := matched[name]; ok && existing != m.Game {
					ambiguous[name] = true
					continue
				}
				matched[name] = m.Game
			}

			for name, gameName := range matched {
				if ambiguous[name] {
					continue
				}
				ext := filepath.Ext(name)
				clean := actions.ApplyScheme(gameName, scheme)
				if clean+ext == name {
					continue
				}
				src := filepath.Join(sysDir, name)
				dst := filepath.Join(sysDir, clean+ext)

				if same, _ := sameExists(dst, src); exists(dst) && !same {
					log.Errf("collision, skipped: %s/%s -> %s%s", job.sys, name, clean, ext)
					continue
				}
				row := identifyRow{System: job.sys, File: name, Game: gameName, Hash: strings.ToUpper(nameToCRC[name])}
				if !runner.Act("identify %s/%s -> %s%s", job.sys, name, clean, ext) {
					rows = append(rows, row)
					continue
				}
				if err := actions.RenamePath(src, dst); err != nil {
					log.Errf("rename failed: %s/%s: %v", job.sys, name, err)
					failed.Add(1)
					continue
				}
				if err := gamelist.Rename(lib.Root, job.sys, name, clean+ext); err != nil {
					log.Errf("gamelist update failed: %s/%s: %v", job.sys, name, err)
				}
				renamed.Add(1)
				rows = append(rows, row)
			}
		}
	}

	if live || verbose {
		process(func() {})
	} else {
		cliutil.Progress(format, false, false, "identify", total, process)
	}

	if runner.Format.Structured() {
		runner.SetCount("renamed", renamed.Load())
		runner.SetCount("failed", failed.Load())
		runner.SetCount("chd_skipped", skippedCHD.Load())
		return len(datIssues) == 0 && failed.Load() == 0, nil
	}

	if len(rows) > 0 {
		sort.SliceStable(rows, func(i, j int) bool {
			if rows[i].System != rows[j].System {
				return rows[i].System < rows[j].System
			}
			return rows[i].File < rows[j].File
		})
		shown, moreSystems, moreCounts := limitPerSystem(rows, limit)
		cliutil.RenderList(os.Stdout, runner.Format, shown)
		for _, sys := range moreSystems {
			fmt.Printf("identify: %s: %d more match(es) not shown (--limit %d)\n", sys, moreCounts[sys], limit)
		}
	}
	if n := skippedCHD.Load(); n > 0 {
		fmt.Printf("identify: %d .chd file(s) not checked (pass --verify-chd to include them)\n", n)
	}

	color, reset := cliutil.StatusColor(runner.Format, failed.Load() > 0)
	line := fmt.Sprintf("%sidentify: %d renamed, %d failed in %s%s\n", color, renamed.Load(), failed.Load(), time.Since(start).Round(time.Second), reset)
	if runner.Quiet {
		runner.Summarize(line)
	} else {
		fmt.Print(line)
	}
	return len(datIssues) == 0 && failed.Load() == 0, nil
}

// limitPerSystem caps rows (already sorted by System) at limit entries per
// system - a huge system's table would otherwise bury smaller ones below
// it on a real terminal. limit <= 0 means unlimited. It also reports, per
// system that got truncated and in the order each first appears, how many
// more rows that system had, so the caller can note it instead of just
// silently dropping them.
func limitPerSystem(rows []identifyRow, limit int) (kept []identifyRow, truncatedOrder []string, more map[string]int) {
	if limit <= 0 {
		return rows, nil, nil
	}
	kept = make([]identifyRow, 0, len(rows))
	more = map[string]int{}
	seen := map[string]int{}
	for _, r := range rows {
		seen[r.System]++
		if seen[r.System] <= limit {
			kept = append(kept, r)
			continue
		}
		if more[r.System] == 0 {
			truncatedOrder = append(truncatedOrder, r.System)
		}
		more[r.System]++
	}
	return kept, truncatedOrder, more
}
