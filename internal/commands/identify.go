package commands

import (
	"fmt"
	"os"
	"path/filepath"
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
// names. A `.chd` is never hashed or considered at all, for the same
// reason Collection reports it "unverifiable" - no CHD metadata hash
// lines up with a Redump DAT entry.
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
func Identify(lib *library.Library, runner *actions.Runner, systems []string, datDir string, scheme actions.Scheme) (bool, error) {
	start := time.Now()
	log := cliutil.NewLogger("identify", runner.Format)

	type sysJob struct {
		sys   string
		names []string
		df    *datdb.Datafile
	}
	var jobs []sysJob
	total := 0
	failedLoad := false
	for _, sys := range systems {
		if systemdb.ClassOf(sys) == systemdb.ClassArcade {
			continue
		}
		df, ok, err := datdb.Load(sys, datDir)
		if !ok {
			continue
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "identify: %s: %v\n", sys, err)
			failedLoad = true
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

	verbose, format := runner.Verbose, runner.Format
	live := verbose && cliutil.CanAnimate(format)
	var renamed, failed atomic.Int64
	doneSoFar := 0
	process := func(tick func()) {
		for _, job := range jobs {
			sysDir := filepath.Join(lib.Roms, job.sys)
			var mu sync.Mutex
			byCRC, byMD5, bySHA1 := map[string]string{}, map[string]string{}, map[string]string{}

			hashOne := func(name string) bool {
				defer tick()
				if strings.ToLower(filepath.Ext(name)) == ".chd" {
					return true
				}
				crc, md5sum, sha1sum, err := hashROM(filepath.Join(sysDir, name))
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

			matched := map[string]string{} // filename -> canonical game name
			ambiguous := map[string]bool{}
			for _, m := range job.df.Match(byCRC, byMD5, bySHA1) {
				if !m.Have || len(m.Matched) != 1 {
					continue
				}
				name := m.Matched[0]
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
				if !runner.Act("identify %s/%s -> %s%s", job.sys, name, clean, ext) {
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
		return !failedLoad && failed.Load() == 0, nil
	}
	color, reset := cliutil.StatusColor(runner.Format, failed.Load() > 0)
	line := fmt.Sprintf("%sidentify: %d renamed, %d failed in %s%s\n", color, renamed.Load(), failed.Load(), time.Since(start).Round(time.Second), reset)
	if runner.Quiet {
		runner.Summarize(line)
	} else {
		fmt.Print(line)
	}
	return !failedLoad && failed.Load() == 0, nil
}
