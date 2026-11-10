package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
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
// renamed one way or the other.
//
// verifyDatSet reports whether --verify-dat was actually passed on the
// command line (cmd.Flags().Changed), as opposed to merely false by
// default. When it wasn't passed, Rename decides per system instead of
// treating every system the same: cartridge-class systems (isCartridgeClass
// - a few KB to a few MB per file) get --verify-dat's behavior anyway,
// since hashing every file there costs nothing noticeable, while
// disc/Dolphin/Xbox/PS3-class systems (hundreds of MB to several GB per
// file) stay plain, the same reasoning `verify` was split out of `check`
// for in the first place - a full hash pass folded invisibly into an
// otherwise-instant command makes a long run indistinguishable from a
// hang. Passing --verify-dat explicitly (either value) always wins over
// this default, for every requested system.
func Rename(lib *library.Library, runner *actions.Runner, systems []string, scheme actions.Scheme, verifyDat, verifyDatSet bool, datDir string, verifyCHD bool) error {
	start := time.Now()
	verifySystems, plainSystems := splitRenameSystems(systems, verifyDat, verifyDatSet)

	var renamed, failed, skippedCHD int64
	if len(plainSystems) > 0 {
		r, f := renamePlain(lib, runner, plainSystems, scheme)
		renamed += r
		failed += f
	}
	if len(verifySystems) > 0 {
		r, f, sc, err := renameVerified(lib, runner, verifySystems, scheme, datDir, verifyCHD)
		if err != nil {
			return err
		}
		renamed += r
		failed += f
		skippedCHD += sc
	}

	if runner.Format.Structured() {
		runner.SetCount("renamed", renamed)
		runner.SetCount("failed", failed)
		return nil
	}
	if skippedCHD > 0 {
		fmt.Printf("rename: %d .chd file(s) not checked (pass --verify-chd to include them)\n", skippedCHD)
	}
	color, reset := cliutil.StatusColor(runner.Format, failed > 0)
	line := fmt.Sprintf("%srename: %d renamed, %d failed in %s%s\n", color, renamed, failed, time.Since(start).Round(time.Second), reset)
	if runner.Quiet {
		runner.Summarize(line)
	} else {
		fmt.Print(line)
	}
	return nil
}

// splitRenameSystems decides, per system, which of Rename's two paths
// applies - see Rename's own doc comment for the reasoning.
func splitRenameSystems(systems []string, verifyDat, verifyDatSet bool) (verify, plain []string) {
	if verifyDatSet {
		if verifyDat {
			return systems, nil
		}
		return nil, systems
	}
	for _, sys := range systems {
		if isCartridgeClass(systemdb.ClassOf(sys)) {
			verify = append(verify, sys)
		} else {
			plain = append(plain, sys)
		}
	}
	return verify, plain
}

// isCartridgeClass reports whether class's files are small enough that
// splitRenameSystems' default can safely hash every one - the opposite
// of every CHD/RVZ/XISO/PS3 disc-image class (isDiscClass plus Dolphin/
// Xbox/PS3), whose files run from hundreds of MB to several GB each.
func isCartridgeClass(class systemdb.Class) bool {
	switch class {
	case systemdb.ClassCart, systemdb.ClassN64, systemdb.ClassSNES:
		return true
	default:
		return false
	}
}

// renamePlain is Rename's instant path: no hashing, no DAT, just
// scheme cleanup - used for every system splitRenameSystems routes here.
// A Title.m3u/ directory is renamed too (see renameM3UDir): a multi-disc
// game is "one ROM" the same way a bare file is everywhere else in
// romctl's model, so a scheme change should reach it whether or not it's
// already been grouped into a playlist - there's no reason tied to *when*
// chd/m3u created the directory that a later scheme rename shouldn't
// still apply to it.
func renamePlain(lib *library.Library, runner *actions.Runner, systems []string, scheme actions.Scheme) (renamed, failed int64) {
	log := cliutil.NewLogger("rename", runner.Format)
	var renamedCount, failedCount atomic.Int64
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
			base := e.Name()
			if e.IsDir() {
				if suffix, ok := renamableDirSuffix(base); ok {
					var r, f int64
					if suffix == ".m3u" {
						r, f = renameM3UDir(lib, runner, log, sys, sysDir, base, scheme)
					} else {
						r, f = renameSuffixedDir(lib, runner, log, sys, sysDir, base, suffix, scheme)
					}
					renamedCount.Add(r)
					failedCount.Add(f)
				}
				continue
			}
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
				failedCount.Add(1)
				continue
			}
			if err := gamelist.Rename(lib.Root, sys, base, clean+ext); err != nil {
				log.Errf("gamelist update failed: %s/%s: %v", sys, base, err)
			}
			renamedCount.Add(1)
		}
	})
	return renamedCount.Load(), failedCount.Load()
}

// renamableDirSuffix returns the suffix of base if it's a directory
// whose own name (minus that suffix) is its game's title - the single
// source of truth for the "one directory is one game" shapes rename and
// checkNaming both have to treat as a naming unit (Title.m3u/ multi-disc
// playlists, Title.ps3/ disc folders), so the two can't silently drift
// apart on what counts.
func renamableDirSuffix(base string) (suffix string, ok bool) {
	switch {
	case strings.HasSuffix(base, ".m3u"):
		return ".m3u", true
	case strings.HasSuffix(base, ".ps3"):
		return ".ps3", true
	default:
		return "", false
	}
}

// beginDirRename computes scheme's effect on sysDir/base's title (minus
// suffix), checks for a same-but-different-case collision at the
// resulting path, and gates on runner.Act - the shared first half of
// renameSuffixedDir and renameM3UDir. ok is false whenever the caller
// should return (0, 0) immediately without touching the filesystem:
// no rename is actually needed, a collision was found, or this is a
// --dry-run preview.
func beginDirRename(runner *actions.Runner, log *cliutil.Logger, sys, sysDir, base, suffix string, scheme actions.Scheme) (dir, newDir, newBase string, ok bool) {
	title := strings.TrimSuffix(base, suffix)
	clean := actions.ApplyScheme(title, scheme)
	if clean == title {
		return "", "", "", false
	}
	dir = filepath.Join(sysDir, base)
	newBase = clean + suffix
	newDir = filepath.Join(sysDir, newBase)

	if same, _ := sameExists(newDir, dir); exists(newDir) && !same {
		log.Errf("collision, skipped: %s/%s -> %s", sys, base, newBase)
		return "", "", "", false
	}
	if !runner.Act("rename %s/%s -> %s/", sys, base, newBase) {
		return "", "", "", false
	}
	return dir, newDir, newBase, true
}

// renameSuffixedDir applies scheme to a sysDir/base directory whose own
// name (minus suffix, e.g. ".ps3") is the game's title - the same "one
// directory is the naming unit" case renameM3UDir handles for a
// multi-disc playlist, just simpler: nothing inside the directory is
// itself named after the title (a PS3 disc folder's PS3_GAME/USRDIR/
// EBOOT.BIN is RPCS3's own fixed internal layout, not title-derived), so
// only the directory itself ever needs to move. Without this, a
// Title.ps3/ disc folder's name is set once, at unpack time
// (discimage.UnpackArchives), and never revisited - the same "naming is
// creation-time-only" gap renameM3UDir fixed for Title.m3u/.
func renameSuffixedDir(lib *library.Library, runner *actions.Runner, log *cliutil.Logger, sys, sysDir, base, suffix string, scheme actions.Scheme) (renamed, failed int64) {
	dir, newDir, newBase, ok := beginDirRename(runner, log, sys, sysDir, base, suffix, scheme)
	if !ok {
		return 0, 0
	}
	if err := actions.RenamePath(dir, newDir); err != nil {
		log.Errf("rename failed: %s/%s: %v", sys, base, err)
		return 0, 1
	}
	if err := gamelist.Rename(lib.Root, sys, base, newBase); err != nil {
		log.Errf("gamelist update failed: %s/%s: %v", sys, base, err)
	}
	return 1, 0
}

// renameM3UDir applies scheme to the Title.m3u/ playlist directory
// sysDir/base. ApplyScheme already preserves a "(Disc N)" tag verbatim
// wherever it appears in a name (see actions.CleanName/extractSchemeTags),
// so it's applied independently to the bare directory title and to each
// disc's own full filename, same as any other rename in this package -
// no special-case disc-tag handling needed here. Every disc is renamed
// first (while the directory still has its old, stable path), then
// rewriteM3UPlaylist regenerates the playlist from whatever discs are
// actually on disk afterward - never by patching the old playlist text
// in place - so a disc rename failure partway through can't leave the
// playlist referencing a disc under a name that no longer exists: a
// disc this call already renamed is reflected correctly even if a later
// disc in the same loop fails, and a disc that still needs renaming
// next time keeps its current (still playlist-matching) name until it
// does. The playlist file itself is renamed to match the new title, and
// only then is the directory itself renamed - gamelist.Rename follows,
// same as every other rename in this package, since a multi-disc game
// is scraped by its playlist/directory name, not any one disc (see
// gamelist's own doc comment and scrape.go).
func renameM3UDir(lib *library.Library, runner *actions.Runner, log *cliutil.Logger, sys, sysDir, base string, scheme actions.Scheme) (renamed, failed int64) {
	dir, newDir, newBase, ok := beginDirRename(runner, log, sys, sysDir, base, ".m3u", scheme)
	if !ok {
		return 0, 0
	}

	title := strings.TrimSuffix(base, ".m3u")
	playlistName := title + ".m3u"
	discs, err := os.ReadDir(dir)
	if err != nil {
		log.Errf("read failed: %s: %v", dir, err)
		return 0, 1
	}

	renameFailed := false
	for _, d := range discs {
		name := d.Name()
		ext := strings.ToLower(filepath.Ext(name))
		if d.IsDir() || name == playlistName || (ext != ".chd" && ext != ".rvz") {
			continue
		}
		discClean := actions.ApplyScheme(strings.TrimSuffix(name, filepath.Ext(name)), scheme)
		newName := discClean + filepath.Ext(name)
		if newName == name {
			continue
		}
		if err := actions.RenamePath(filepath.Join(dir, name), filepath.Join(dir, newName)); err != nil {
			log.Errf("rename failed: %s/%s/%s: %v", sys, base, name, err)
			renameFailed = true
		}
	}
	if renameFailed {
		return 0, 1
	}

	if err := rewriteM3UPlaylist(dir, playlistName); err != nil {
		log.Errf("rewrite playlist failed: %s: %v", filepath.Join(dir, playlistName), err)
		return 0, 1
	}
	newPlaylist := filepath.Join(dir, newBase)
	if err := actions.RenamePath(filepath.Join(dir, playlistName), newPlaylist); err != nil {
		log.Errf("rename failed: %s/%s: %v", sys, playlistName, err)
		return 0, 1
	}
	if err := actions.RenamePath(dir, newDir); err != nil {
		log.Errf("rename failed: %s/%s: %v", sys, base, err)
		return 0, 1
	}
	if err := gamelist.Rename(lib.Root, sys, base, newBase); err != nil {
		log.Errf("gamelist update failed: %s/%s: %v", sys, base, err)
	}
	return 1, 0
}

// rewriteM3UPlaylist regenerates dir's playlistName file from the disc
// files actually present in dir, sorted by their own "(Disc N)" tag -
// rebuilding from current on-disk reality rather than patching whatever
// the playlist said before, so this is safe to call (and self-heals a
// prior partial failure) regardless of which discs were just renamed,
// already matched the target scheme, or still need another pass.
func rewriteM3UPlaylist(dir, playlistName string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	type disc struct {
		name string
		num  int
	}
	var discs []disc
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || name == playlistName {
			continue
		}
		m := discFileRE.FindStringSubmatch(name)
		if m == nil {
			continue
		}
		num, _ := strconv.Atoi(m[2])
		discs = append(discs, disc{name, num})
	}
	sort.Slice(discs, func(i, j int) bool { return discs[i].num < discs[j].num })
	var lines []string
	for _, d := range discs {
		lines = append(lines, d.name)
	}
	return os.WriteFile(filepath.Join(dir, playlistName), []byte(strings.Join(lines, "\n")+"\n"), 0o644)
}

// renameVerified is Rename's --verify-dat path: a per-system hash-and-
// match pass (same shape identify.go's own used to be) ahead of the same
// per-file rename loop renamePlain already has, so every file still gets
// renamed - to its DAT name when confirmed, to a scheme-cleaned version
// of its current name otherwise.
func renameVerified(lib *library.Library, runner *actions.Runner, systems []string, scheme actions.Scheme, datDir string, verifyCHD bool) (renamed, failed, skippedCHD int64, err error) {
	log := cliutil.NewLogger("rename", runner.Format)

	var chdmanPath string
	if verifyCHD {
		if chdmanPath, err = resolveCHDManIfNeeded(systems); err != nil {
			return 0, 0, 0, err
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
	var renamedCount, failedCount, skippedCHDCount atomic.Int64
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
							skippedCHDCount.Add(1)
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
					failedCount.Add(1)
					continue
				}
				if err := gamelist.Rename(lib.Root, job.sys, base, clean+ext); err != nil {
					log.Errf("gamelist update failed: %s/%s: %v", job.sys, base, err)
				}
				renamedCount.Add(1)
			}
		}
	}

	if live || verbose {
		process(func() {})
	} else {
		cliutil.Progress(format, false, false, "rename", total, process)
	}

	return renamedCount.Load(), failedCount.Load(), skippedCHDCount.Load(), nil
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
