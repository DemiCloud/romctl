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

// Rename applies scheme (`[naming] scheme` in config) to every loose file
// in the given systems (arcade systems are skipped: their filenames are
// emulator-resolved set names). See internal/actions.ApplyScheme.
//
// verifyDat (--verify-dat) opts into hash-matching each file against its
// system's No-Intro/Redump DAT first (datDir/verifyCHD - see
// internal/datdb and Collection's own doc comment on both): a confirmed
// match renames to the DAT's own canonical game name instead of a
// cleaned-up version of whatever name the file already had, and a file
// that can't be confirmed still falls through to the same generic
// cleanup a plain Rename would have done. Unlike identify's own
// (removed) renaming, this is never additive-only: every file ends up
// renamed one way or the other. Off by default - without it, Rename
// never hashes anything and stays exactly as instant as it's always
// been.
func Rename(lib *library.Library, runner *actions.Runner, systems []string, scheme actions.Scheme, verifyDat bool, datDir string, verifyCHD bool) error {
	if verifyDat {
		return renameVerified(lib, runner, systems, scheme, datDir, verifyCHD)
	}
	return renamePlain(lib, runner, systems, scheme)
}

func renamePlain(lib *library.Library, runner *actions.Runner, systems []string, scheme actions.Scheme) error {
	start := time.Now()
	log := cliutil.NewLogger("rename", runner.Format)
	var renamed, failed atomic.Int64
	cliutil.Parallel(systems, func(sys string) {
		if systemdb.ClassOf(sys) == systemdb.ClassArcade {
			return
		}
		sysDir := filepath.Join(lib.Roms, sys)
		entries, err := os.ReadDir(sysDir)
		if err != nil {
			return
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			base := e.Name()
			if base == "systeminfo.txt" || strings.HasPrefix(base, ".") {
				continue
			}
			ext := filepath.Ext(base)
			name := strings.TrimSuffix(base, ext)
			clean := actions.ApplyScheme(name, scheme)
			if clean == name {
				continue
			}
			src := filepath.Join(sysDir, base)
			dst := filepath.Join(sysDir, clean+ext)

			if same, _ := sameExists(dst, src); exists(dst) && !same {
				log.Errf("collision, skipped: %s/%s -> %s%s", sys, base, clean, ext)
				continue
			}
			if !runner.Act("rename %s/%s -> %s%s", sys, base, clean, ext) {
				continue
			}
			if err := actions.RenamePath(src, dst); err != nil {
				log.Errf("rename failed: %s/%s: %v", sys, base, err)
				failed.Add(1)
				continue
			}
			if err := gamelist.Rename(lib.Root, sys, base, clean+ext); err != nil {
				log.Errf("gamelist update failed: %s/%s: %v", sys, base, err)
			}
			renamed.Add(1)
		}
	})
	if runner.Format.Structured() {
		runner.SetCount("renamed", renamed.Load())
		runner.SetCount("failed", failed.Load())
		return nil
	}
	color, reset := cliutil.StatusColor(runner.Format, failed.Load() > 0)
	line := fmt.Sprintf("%srename: %d renamed, %d failed in %s%s\n", color, renamed.Load(), failed.Load(), time.Since(start).Round(time.Second), reset)
	if runner.Quiet {
		runner.Summarize(line)
	} else {
		fmt.Print(line)
	}
	return nil
}

// renameVerified is Rename's --verify-dat path: a per-system hash-and-
// match pass (same shape identify.go's own used to be) ahead of the same
// per-file rename loop renamePlain already has, so every file still gets
// renamed - to its DAT name when confirmed, to a scheme-cleaned version
// of its current name otherwise.
func renameVerified(lib *library.Library, runner *actions.Runner, systems []string, scheme actions.Scheme, datDir string, verifyCHD bool) error {
	start := time.Now()
	log := cliutil.NewLogger("rename", runner.Format)

	var chdmanPath string
	if verifyCHD {
		var err error
		if chdmanPath, err = resolveCHDManIfNeeded(systems); err != nil {
			return err
		}
	}

	type sysJob struct {
		sys   string
		class systemdb.Class
		names []string
		df    *datdb.Datafile // nil: no DAT coverage, every file falls through to plain cleanup
	}
	var jobs []sysJob
	var datIssues []datLoadRow
	total := 0
	for _, sys := range systems {
		class := systemdb.ClassOf(sys)
		if class == systemdb.ClassArcade {
			continue
		}
		sysDir := filepath.Join(lib.Roms, sys)
		entries, err := os.ReadDir(sysDir)
		if err != nil {
			continue
		}
		var names []string
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			base := e.Name()
			if base == "systeminfo.txt" || strings.HasPrefix(base, ".") {
				continue
			}
			names = append(names, base)
		}

		var df *datdb.Datafile
		if loaded, ok, issue := loadDat(sys, datDir); ok {
			if issue != nil {
				datIssues = append(datIssues, *issue)
			} else {
				df = loaded
				if verifyCHD && isDiscClass(class) {
					df = stripSidecarROMs(df)
				}
			}
		}
		jobs = append(jobs, sysJob{sys, class, names, df})
		total += len(names)
	}
	printDatLoadIssues("rename", datIssues)

	verbose, format := runner.Verbose, runner.Format
	live := verbose && cliutil.CanAnimate(format)
	var renamed, failed, skippedCHD atomic.Int64
	doneSoFar := 0
	process := func(tick func()) {
		for _, job := range jobs {
			sysDir := filepath.Join(lib.Roms, job.sys)
			matched := map[string]string{} // filename -> canonical game name
			ambiguous := map[string]bool{}

			if job.df != nil {
				var mu sync.Mutex
				byCRC, byMD5, bySHA1 := map[string]string{}, map[string]string{}, map[string]string{}

				hashOne := func(name string) bool {
					defer tick()
					if !hashable(name) {
						return true
					}
					if strings.ToLower(filepath.Ext(name)) == ".chd" {
						if !verifyCHD || !isDiscClass(job.class) {
							skippedCHD.Add(1)
							return true
						}
						hashes, err := hashDiscCHD(chdmanPath, filepath.Join(sysDir, name), job.class)
						if err != nil {
							fmt.Fprintf(os.Stderr, "rename: %s/%s: extraction failed, leaving untouched: %v\n", job.sys, name, err)
							return true
						}
						mu.Lock()
						for _, h := range hashes {
							byCRC[h.crc], byMD5[h.md5sum], bySHA1[h.sha1sum] = name, name, name
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
					if !m.Have {
						continue
					}
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
			} else {
				for range job.names {
					tick()
				}
				doneSoFar += len(job.names)
			}

			for _, base := range job.names {
				ext := filepath.Ext(base)
				var clean string
				if gameName, ok := matched[base]; ok && !ambiguous[base] {
					clean = actions.ApplyScheme(gameName, scheme)
				} else {
					clean = actions.ApplyScheme(strings.TrimSuffix(base, ext), scheme)
				}
				if clean+ext == base {
					continue
				}
				src := filepath.Join(sysDir, base)
				dst := filepath.Join(sysDir, clean+ext)

				if same, _ := sameExists(dst, src); exists(dst) && !same {
					log.Errf("collision, skipped: %s/%s -> %s%s", job.sys, base, clean, ext)
					continue
				}
				if !runner.Act("rename %s/%s -> %s%s", job.sys, base, clean, ext) {
					continue
				}
				if err := actions.RenamePath(src, dst); err != nil {
					log.Errf("rename failed: %s/%s: %v", job.sys, base, err)
					failed.Add(1)
					continue
				}
				if err := gamelist.Rename(lib.Root, job.sys, base, clean+ext); err != nil {
					log.Errf("gamelist update failed: %s/%s: %v", job.sys, base, err)
				}
				renamed.Add(1)
			}
		}
	}

	if live || verbose {
		process(func() {})
	} else {
		cliutil.Progress(format, false, false, "rename", total, process)
	}

	if !runner.Format.Structured() {
		if n := skippedCHD.Load(); n > 0 {
			fmt.Printf("rename: %d .chd file(s) not checked (pass --verify-chd to include them)\n", n)
		}
	}

	if runner.Format.Structured() {
		runner.SetCount("renamed", renamed.Load())
		runner.SetCount("failed", failed.Load())
		return nil
	}
	color, reset := cliutil.StatusColor(runner.Format, failed.Load() > 0)
	line := fmt.Sprintf("%srename: %d renamed, %d failed in %s%s\n", color, renamed.Load(), failed.Load(), time.Since(start).Round(time.Second), reset)
	if runner.Quiet {
		runner.Summarize(line)
	} else {
		fmt.Print(line)
	}
	return nil
}

func sameExists(dst, src string) (bool, error) {
	di, err := os.Stat(dst)
	if err != nil {
		return false, err
	}
	si, err := os.Stat(src)
	if err != nil {
		return false, err
	}
	return os.SameFile(di, si), nil
}
