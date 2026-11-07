package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"

	"github.com/demicloud/romctl/internal/actions"
	"github.com/demicloud/romctl/internal/archive"
	"github.com/demicloud/romctl/internal/cliutil"
	"github.com/demicloud/romctl/internal/executil"
	"github.com/demicloud/romctl/internal/formats/cso"
	"github.com/demicloud/romctl/internal/gamelist"
	"github.com/demicloud/romctl/internal/library"
	"github.com/demicloud/romctl/internal/systemdb"
	"github.com/demicloud/romctl/internal/tools"
)

// CHD converts game directories, archives and loose .cue/.gdi/.iso/.cso
// files to CHD for every CD/DVD/UMD-class system given. Multi-disc games
// become a Title.m3u/ directory. scheme (`[naming] scheme`) controls how
// the source's region/dump tags end up in the resulting filename; see
// internal/actions.ApplyScheme.
func CHD(lib *library.Library, runner *actions.Runner, systems []string, scheme actions.Scheme) (bool, error) {
	log := cliutil.NewLogger("chd")
	var converted, skipped, failed atomic.Int64

	cliutil.Parallel(systems, func(sys string) {
		class := systemdb.ClassOf(sys)
		if class != systemdb.ClassCD && class != systemdb.ClassDVD && class != systemdb.ClassUMD {
			return
		}
		sysDir := filepath.Join(lib.Roms, sys)
		entries, err := os.ReadDir(sysDir)
		if err != nil {
			return
		}
		names := entryNames(entries)
		total := len(names)

		for i, base := range names {
			if strings.HasPrefix(base, "_") || strings.HasSuffix(base, ".m3u") || base == "systeminfo.txt" {
				continue
			}
			path := filepath.Join(sysDir, base)
			title := actions.ApplyScheme(strings.TrimSuffix(base, filepath.Ext(base)), scheme)
			info, err := os.Stat(path)
			if err != nil {
				continue
			}

			if info.IsDir() {
				title = actions.ApplyScheme(base, scheme)
				if !runner.Act("[%d/%d] convert directory %s/%s -> %s", i+1, total, sys, base, title) {
					continue
				}
				newName, err := convertGameDir(class, sysDir, path, title)
				if err != nil {
					log.Errf("failed, source kept: %s/%s: %v", sys, base, err)
					failed.Add(1)
					continue
				}
				if err := actions.Retire(path); err != nil {
					log.Errf("retire failed: %s: %v", path, err)
				}
				if err := gamelist.Rename(lib.Root, sys, base, newName); err != nil {
					log.Errf("gamelist update failed: %s/%s: %v", sys, base, err)
				}
				converted.Add(1)
				continue
			}

			ext := strings.ToLower(filepath.Ext(base))
			dest := filepath.Join(sysDir, title+".chd")
			switch ext {
			case ".cso":
				if exists(dest) {
					skipped.Add(1)
					continue
				}
				if !runner.Act("[%d/%d] convert %s/%s -> %s.chd", i+1, total, sys, base, title) {
					continue
				}
				if err := csoToCHD(path, dest); err != nil {
					log.Errf("failed, source kept: %s/%s: %v", sys, base, err)
					failed.Add(1)
					continue
				}
				_ = actions.Retire(path)
				if err := gamelist.Rename(lib.Root, sys, base, title+".chd"); err != nil {
					log.Errf("gamelist update failed: %s/%s: %v", sys, base, err)
				}
				converted.Add(1)
			case ".cue", ".gdi", ".iso":
				if exists(dest) {
					skipped.Add(1)
					continue
				}
				if !runner.Act("[%d/%d] convert %s/%s -> %s.chd", i+1, total, sys, base, title) {
					continue
				}
				mode := chdMode(class, path)
				if err := makeCHD(mode, path, dest); err != nil {
					log.Errf("failed, source kept: %s/%s: %v", sys, base, err)
					failed.Add(1)
					continue
				}
				_ = actions.Retire(path)
				if err := gamelist.Rename(lib.Root, sys, base, title+".chd"); err != nil {
					log.Errf("gamelist update failed: %s/%s: %v", sys, base, err)
				}
				converted.Add(1)
			case ".zip", ".7z", ".rar":
				if !runner.Act("[%d/%d] extract and convert %s/%s -> %s", i+1, total, sys, base, title) {
					continue
				}
				newName, err := extractAndConvert(class, sysDir, path, title)
				if err != nil {
					log.Errf("failed, source kept: %s/%s: %v", sys, base, err)
					failed.Add(1)
					continue
				}
				_ = actions.Retire(path)
				if err := gamelist.Rename(lib.Root, sys, base, newName); err != nil {
					log.Errf("gamelist update failed: %s/%s: %v", sys, base, err)
				}
				converted.Add(1)
			}
		}
	})

	if c, f := converted.Load(), failed.Load(); c+f > 0 {
		fmt.Printf("chd: %d converted, %d skipped, %d failed\n", c, skipped.Load(), f)
	}
	return failed.Load() == 0, nil
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

// makeCHD creates and verifies one CHD via the embedded/PATH chdman.
func makeCHD(mode, in, out string) error {
	path, err := tools.Resolve("chdman")
	if err != nil {
		return err
	}
	tmp := out + ".tmp"
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
	return os.Rename(tmp, out)
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

// convertGameDir converts a directory of disc images (one game) to one or
// more CHDs, grouping multi-disc games into a Title.m3u/ directory. It
// returns the resulting top-level name under sysDir ("<title>.chd" or
// "<title>.m3u").
func convertGameDir(class systemdb.Class, sysDir, dir, title string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	var imgs []string
	for _, ext := range []string{".cue", ".gdi"} {
		for _, e := range entries {
			if !e.IsDir() && strings.ToLower(filepath.Ext(e.Name())) == ext {
				imgs = append(imgs, filepath.Join(dir, e.Name()))
			}
		}
	}
	if len(imgs) == 0 {
		for _, e := range entries {
			if !e.IsDir() && strings.ToLower(filepath.Ext(e.Name())) == ".iso" {
				imgs = append(imgs, filepath.Join(dir, e.Name()))
			}
		}
	}
	sort.Strings(imgs)
	if len(imgs) == 0 {
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
