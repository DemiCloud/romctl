package commands

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"github.com/demicloud/romctl/internal/actions"
	"github.com/demicloud/romctl/internal/archive"
	"github.com/demicloud/romctl/internal/cliutil"
	"github.com/demicloud/romctl/internal/executil"
	"github.com/demicloud/romctl/internal/formats/chd"
	"github.com/demicloud/romctl/internal/formats/cso"
	"github.com/demicloud/romctl/internal/formats/signature"
	"github.com/demicloud/romctl/internal/gamelist"
	"github.com/demicloud/romctl/internal/library"
	"github.com/demicloud/romctl/internal/systemdb"
	"github.com/demicloud/romctl/internal/tools"
)

// chdJob is one file (or game directory) to convert, flattened out of its
// system's directory listing up front so every system's files run through
// one shared worker pool instead of one pool per system - see CHD's doc
// comment on why that matters.
type chdJob struct {
	sys        string
	class      systemdb.Class
	base       string
	idx, total int
}

// CHD converts game directories, archives and loose .cue/.gdi/.iso/.cso
// files to CHD for every CD/DVD/UMD-class system given. Multi-disc games
// become a Title.m3u/ directory. scheme (`[naming] scheme`) controls how
// the source's region/dump tags end up in the resulting filename; see
// internal/actions.ApplyScheme.
//
// Every chdman invocation is CPU-bound and independent per file, so all
// matching files across every system are flattened into one job list and
// run through a single GOMAXPROCS-sized worker pool (cliutil.Parallel),
// rather than one pool per system - the latter would only ever use as many
// concurrent workers as there are systems given, which is 1 whenever the
// request is for a single large system (the common case: "romctl chd
// psx"), wasting every other core. Running a system's files concurrently
// is safe: gamelist.Rename serializes its own per-system gamelist.xml
// read-modify-write internally, and actions.Retire/convertGameDir/
// extractAndConvert each only ever touch paths derived from their own
// job's unique source file or a freshly made temp directory.
func CHD(lib *library.Library, runner *actions.Runner, systems []string, scheme actions.Scheme) (bool, error) {
	start := time.Now()
	log := cliutil.NewLogger("chd", runner.Format)
	var converted, skipped, failed atomic.Int64

	var jobs []chdJob
	for _, sys := range systems {
		class := systemdb.ClassOf(sys)
		if class != systemdb.ClassCD && class != systemdb.ClassDVD && class != systemdb.ClassUMD {
			continue
		}
		sysDir := filepath.Join(lib.Roms, sys)
		entries, err := os.ReadDir(sysDir)
		if err != nil {
			continue
		}
		names := entryNames(entries)
		for i, base := range names {
			jobs = append(jobs, chdJob{sys, class, base, i, len(names)})
		}
	}

	cliutil.Progress(runner.Format, runner.Verbose, runner.DryRun, "chd", len(jobs), func(tick func()) {
		cliutil.Parallel(jobs, func(j chdJob) {
			defer tick()
			sys, class, base, i, total := j.sys, j.class, j.base, j.idx, j.total
			if strings.HasPrefix(base, "_") || strings.HasSuffix(base, ".m3u") || base == "systeminfo.txt" {
				return
			}
			sysDir := filepath.Join(lib.Roms, sys)
			path := filepath.Join(sysDir, base)
			title := actions.ApplyScheme(strings.TrimSuffix(base, filepath.Ext(base)), scheme)
			info, err := os.Stat(path)
			if err != nil {
				return
			}

			if info.IsDir() {
				title = actions.ApplyScheme(base, scheme)
				if !runner.Act("[%d/%d] convert directory %s/%s -> %s", i+1, total, sys, base, title) {
					return
				}
				newName, err := convertGameDir(class, sysDir, path, title)
				if err != nil {
					log.Errf("failed, source kept: %s/%s: %v", sys, base, err)
					failed.Add(1)
					return
				}
				if err := actions.Retire(path); err != nil {
					log.Errf("retire failed: %s: %v", path, err)
				}
				if err := gamelist.Rename(lib.Root, sys, base, newName); err != nil {
					log.Errf("gamelist update failed: %s/%s: %v", sys, base, err)
				}
				converted.Add(1)
				return
			}

			ext := strings.ToLower(filepath.Ext(base))
			dest := filepath.Join(sysDir, title+".chd")
			switch ext {
			case ".cso":
				if exists(dest) {
					skipped.Add(1)
					return
				}
				if !runner.Act("[%d/%d] convert %s/%s -> %s.chd", i+1, total, sys, base, title) {
					return
				}
				if err := csoToCHD(path, dest); err != nil {
					log.Errf("failed, source kept: %s/%s: %v", sys, base, err)
					failed.Add(1)
					return
				}
				_ = actions.Retire(path)
				if err := gamelist.Rename(lib.Root, sys, base, title+".chd"); err != nil {
					log.Errf("gamelist update failed: %s/%s: %v", sys, base, err)
				}
				converted.Add(1)
			case ".cue", ".gdi", ".iso":
				if exists(dest) {
					skipped.Add(1)
					return
				}
				if !runner.Act("[%d/%d] convert %s/%s -> %s.chd", i+1, total, sys, base, title) {
					return
				}
				mode := chdMode(class, path)
				if err := makeCHD(mode, path, dest); err != nil {
					log.Errf("failed, source kept: %s/%s: %v", sys, base, err)
					failed.Add(1)
					return
				}
				_ = actions.Retire(path)
				if err := gamelist.Rename(lib.Root, sys, base, title+".chd"); err != nil {
					log.Errf("gamelist update failed: %s/%s: %v", sys, base, err)
				}
				converted.Add(1)
			case ".zip", ".7z", ".rar":
				if !runner.Act("[%d/%d] extract and convert %s/%s -> %s", i+1, total, sys, base, title) {
					return
				}
				newName, err := extractAndConvert(class, sysDir, path, title)
				if err != nil {
					log.Errf("failed, source kept: %s/%s: %v", sys, base, err)
					failed.Add(1)
					return
				}
				_ = actions.Retire(path)
				if err := gamelist.Rename(lib.Root, sys, base, newName); err != nil {
					log.Errf("gamelist update failed: %s/%s: %v", sys, base, err)
				}
				converted.Add(1)
			case ".chd":
				// Not a fresh conversion - an existing CHD that was made
				// as CD-type back when a DVD-class system (e.g. PS2)
				// didn't yet have that distinction. Re-encoding it is
				// still squarely "make this system's CHDs correct," so
				// it belongs in the same pass as every other case above
				// rather than its own separate command.
				if class != systemdb.ClassDVD {
					return
				}
				kind, err := chd.Classify(path)
				if err != nil || kind != chd.KindCDAsDVDMAGE {
					return
				}
				if !runner.Act("[%d/%d] re-encode %s/%s as DVD CHD", i+1, total, sys, base) {
					return
				}
				if err := reencodeAsDVD(path); err != nil {
					log.Errf("failed, original kept: %s/%s: %v", sys, base, err)
					failed.Add(1)
					return
				}
				converted.Add(1)
			}
		})
	})

	if runner.Format.Structured() {
		runner.SetCount("converted", converted.Load())
		runner.SetCount("skipped", skipped.Load())
		runner.SetCount("failed", failed.Load())
		return failed.Load() == 0, nil
	}
	color, reset := cliutil.StatusColor(runner.Format, failed.Load() > 0)
	fmt.Printf("%schd: %d converted, %d skipped, %d failed in %s%s\n", color, converted.Load(), skipped.Load(), failed.Load(), time.Since(start).Round(time.Second), reset)
	return failed.Load() == 0, nil
}

// findByExt walks dir recursively collecting every file whose extension
// (case-insensitive) matches one of exts. A redump/no-intro archive
// commonly wraps its disc image in one extra per-game subfolder once
// extracted, so a plain top-level directory listing misses it.
func findByExt(dir string, exts ...string) ([]string, error) {
	var out []string
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		ext := strings.ToLower(filepath.Ext(info.Name()))
		for _, want := range exts {
			if ext == want {
				out = append(out, path)
				return nil
			}
		}
		return nil
	})
	return out, err
}

func entryNames(entries []os.DirEntry) []string {
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)
	return names
}

// chdMode picks createcd vs createdvd for an image in a given system class.
func chdMode(class systemdb.Class, imagePath string) string {
	ext := strings.ToLower(filepath.Ext(imagePath))
	if (class == systemdb.ClassDVD || class == systemdb.ClassUMD) && ext == ".iso" {
		return "dvd"
	}
	return "cd"
}

// makeCHD creates and verifies one CHD via the embedded/PATH chdman. The
// candidate file is staged on a RAM-backed scratch directory when
// chdScratchDir judges it safe to (see its own doc comment for why: it
// saves a slow-medium read during chdman's own verify pass), falling back
// to staging next to out exactly as before otherwise. The two cases differ
// only in how the verified candidate reaches out: a same-filesystem rename
// when staged next to out, a copy through out+".tmp" (to keep the same
// atomic-write guarantee) when staged elsewhere.
func makeCHD(mode, in, out string) error {
	path, err := tools.Resolve("chdman")
	if err != nil {
		return err
	}

	var scratch string
	release := func() {}
	if info, statErr := os.Stat(in); statErr == nil {
		if dir, rel := chdScratchDir(info.Size()); dir != "" {
			scratch, release = dir, rel
		}
	}
	defer release()

	tmp := out + ".tmp"
	if scratch != "" {
		f, err := os.CreateTemp(scratch, "romctl-chd-*.tmp")
		if err != nil {
			return err
		}
		tmp = f.Name()
		f.Close()
		os.Remove(tmp) // reserve the name, let chdman create the file itself
		defer os.Remove(tmp)
	}

	args := []string{"createcd"}
	if mode == "dvd" {
		args = []string{"createdvd", "-hs", "2048", "-c", "lzma,zlib"}
	}
	args = append(args, "-i", in, "-o", tmp)
	if cmdOut, err := executil.Run(path, args...); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("chdman %s: %w: %s", mode, err, cmdOut)
	}
	if cmdOut, err := executil.Run(path, "verify", "-i", tmp); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("chdman verify: %w: %s", err, cmdOut)
	}
	if scratch == "" {
		return os.Rename(tmp, out)
	}
	return copyToDest(tmp, out)
}

// copyToDest copies src - staged on a different filesystem than dst, so
// os.Rename can't cross it - into dst, through dst+".tmp" first to keep
// the atomic-write guarantee a same-filesystem rename would otherwise give
// for free.
func copyToDest(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	tmp := dst + ".tmp"
	out, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(tmp)
		return err
	}
	if err := out.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, dst)
}

// reencodeAsDVD re-encodes one existing CD-type CHD (chd.KindCDAsDVDMAGE -
// a DVD-class system's title dumped back when CD/DVD CHD wasn't yet a
// distinction chdman made) in place as a proper DVD-type CHD, via its own
// fresh scratch directory - each call (possibly concurrent with others
// re-encoding the same system) needs its own, since chdman's extractcd
// always writes to fixed filenames. The scratch directory is created next
// to path, inside its own system directory, rather than in the OS's
// global temp dir: the final os.Rename below needs to land on the same
// filesystem as path, and the system directory and the global temp dir
// (often tmpfs, or another partition entirely) are frequently not the
// same filesystem - e.g. a library living on an SD card. The "_extract."
// prefix matches the one chd/unpack's own scratch directories already
// use, so an interrupted run leaves a directory check already recognizes
// as a leftover to clean up.
func reencodeAsDVD(path string) error {
	chdmanPath, err := tools.Resolve("chdman")
	if err != nil {
		return err
	}
	work, err := os.MkdirTemp(filepath.Dir(path), "_extract.*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(work)

	iso := filepath.Join(work, "x.iso")
	cue := filepath.Join(work, "x.cue")
	out := filepath.Join(work, "out.chd")

	if cmdOut, err := executil.Run(chdmanPath, "extractcd", "-i", path, "-o", cue, "-ob", iso); err != nil {
		return fmt.Errorf("extractcd: %w: %s", err, cmdOut)
	}
	if err := makeCHD("dvd", iso, out); err != nil {
		return err
	}
	if err := actions.Retire(path); err != nil {
		return err
	}
	return os.Rename(out, path)
}

// csoToCHD decompresses a PSP CSO image and converts the result to a
// DVD-type CHD.
func csoToCHD(csoPath, out string) error {
	tmp, err := os.MkdirTemp("", "romctl-cso")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)

	isoPath := filepath.Join(tmp, "x.iso")
	f, err := os.Create(isoPath)
	if err != nil {
		return err
	}
	err = cso.Decompress(csoPath, f)
	f.Close()
	if err != nil {
		return err
	}
	return makeCHD("dvd", isoPath, out)
}

// cartridgeHint checks dir's loose .bin files (the ones left unaccounted
// for once cue/gdi/iso search comes up empty) for a known cartridge/
// handheld ROM signature (see internal/formats/signature). A redump/
// no-intro CD dump's .bin always has a matching .cue; a lone .bin with
// none is far more often a cartridge ROM that got misfiled into a
// CD-class system's directory than an actual cue-less disc dump, so this
// turns chd's otherwise-opaque "no cue/gdi/iso" error into one that names
// the real problem.
func cartridgeHint(dir string) string {
	bins, err := findByExt(dir, ".bin")
	if err != nil {
		return ""
	}
	for _, b := range bins {
		if fam, ok := signature.Identify(b); ok {
			return fmt.Sprintf("%s is a %s ROM, not a disc image - move it to the correct system", filepath.Base(b), signature.Name[fam])
		}
	}
	return ""
}

// convertGameDir converts a directory of disc images (one game) to one or
// more CHDs, grouping multi-disc games into a Title.m3u/ directory. It
// returns the resulting top-level name under sysDir ("<title>.chd" or
// "<title>.m3u").
func convertGameDir(class systemdb.Class, sysDir, dir, title string) (string, error) {
	imgs, err := findByExt(dir, ".cue", ".gdi")
	if err != nil {
		return "", err
	}
	if len(imgs) == 0 {
		imgs, err = findByExt(dir, ".iso")
		if err != nil {
			return "", err
		}
	}
	sort.Strings(imgs)
	if len(imgs) == 0 {
		if hint := cartridgeHint(dir); hint != "" {
			return "", fmt.Errorf("no cue/gdi/iso in: %s (%s)", dir, hint)
		}
		return "", fmt.Errorf("no cue/gdi/iso in: %s", dir)
	}

	if len(imgs) == 1 {
		out := filepath.Join(sysDir, title+".chd")
		if exists(out) {
			return "", fmt.Errorf("exists, skipped: %s", out)
		}
		if err := makeCHD(chdMode(class, imgs[0]), imgs[0], out); err != nil {
			return "", err
		}
		return title + ".chd", nil
	}

	m3uDir := filepath.Join(sysDir, title+".m3u")
	if exists(m3uDir) {
		return "", fmt.Errorf("exists, skipped: %s", m3uDir)
	}
	if err := os.Mkdir(m3uDir, 0o755); err != nil {
		return "", err
	}
	var playlist []string
	for n, img := range imgs {
		disc := fmt.Sprintf("%s (Disc %02d).chd", title, n+1)
		if err := makeCHD(chdMode(class, img), img, filepath.Join(m3uDir, disc)); err != nil {
			os.RemoveAll(m3uDir)
			return "", err
		}
		playlist = append(playlist, disc)
	}
	if err := os.WriteFile(filepath.Join(m3uDir, title+".m3u"), []byte(strings.Join(playlist, "\n")+"\n"), 0o644); err != nil {
		return "", err
	}
	return title + ".m3u", nil
}

// extractAndConvert extracts an archived disc image/game directory and
// converts the result, cleaning up the extraction scratch directory. It
// returns the resulting top-level name under sysDir, same as
// convertGameDir.
func extractAndConvert(class systemdb.Class, sysDir, archivePath, title string) (string, error) {
	tmp, err := os.MkdirTemp(sysDir, "_extract.*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmp)

	if err := archive.ExtractAll(archivePath, tmp); err != nil {
		return "", err
	}
	return convertGameDir(class, sysDir, tmp, title)
}
