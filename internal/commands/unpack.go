package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/demicloud/romctl/internal/actions"
	"github.com/demicloud/romctl/internal/archive"
	"github.com/demicloud/romctl/internal/cliutil"
	"github.com/demicloud/romctl/internal/gamelist"
	"github.com/demicloud/romctl/internal/library"
	"github.com/demicloud/romctl/internal/systemdb"
)

// unpackJob is one archive to examine/unpack, flattened out of its
// system's directory listing up front - see CHD's doc comment for why
// every system's files run through one shared worker pool instead of one
// per system.
type unpackJob struct {
	sys, base string
}

// Unpack extracts single-file archives in cartridge/N64/SNES systems;
// multi-file archives are left alone (there's no single "the game" file
// to promote).
func Unpack(lib *library.Library, runner *actions.Runner, systems []string) (bool, error) {
	start := time.Now()
	log := cliutil.NewLogger("unpack", runner.Format)
	var unpacked, skipped, failed atomic.Int64

	var jobs []unpackJob
	for _, sys := range systems {
		switch systemdb.ClassOf(sys) {
		case systemdb.ClassCart, systemdb.ClassN64, systemdb.ClassSNES:
		default:
			continue
		}
		sysDir := filepath.Join(lib.Roms, sys)
		entries, err := os.ReadDir(sysDir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			ext := strings.ToLower(filepath.Ext(e.Name()))
			if ext != ".zip" && ext != ".7z" && ext != ".rar" {
				continue
			}
			jobs = append(jobs, unpackJob{sys, e.Name()})
		}
	}

	cliutil.Progress(runner.Format, runner.Verbose, runner.DryRun, "unpack", len(jobs), func(tick func()) {
		cliutil.Parallel(jobs, func(j unpackJob) {
			defer tick()
			sys, base := j.sys, j.base
			sysDir := filepath.Join(lib.Roms, sys)
			archivePath := filepath.Join(sysDir, base)

			// Listing the sole entry's name doesn't require extracting
			// anything, so an already-unpacked archive (very common on a
			// re-run) can be skipped before paying for a full extraction -
			// and, just as importantly, counted separately from a real
			// failure instead of inflating it.
			innerName, single, err := archive.SoleEntry(archivePath)
			if err != nil {
				log.Errf("unreadable archive: %s", archivePath)
				failed.Add(1)
				return
			}
			if !single {
				log.Errf("multi-file archive, skipped: %s/%s", sys, base)
				return
			}
			innerExt := strings.ToLower(filepath.Ext(innerName))
			stem := strings.TrimSuffix(base, filepath.Ext(base))
			out := filepath.Join(sysDir, stem+innerExt)
			if exists(out) {
				skipped.Add(1)
				return
			}
			if !runner.Act("unpack %s/%s", sys, base) {
				return
			}
			if err := unpackOne(sysDir, archivePath, out); err != nil {
				log.Errf("%v", err)
				failed.Add(1)
				return
			}
			if err := gamelist.Rename(lib.Root, sys, base, filepath.Base(out)); err != nil {
				log.Errf("gamelist update failed: %s/%s: %v", sys, base, err)
			}
			unpacked.Add(1)
		})
	})
	if runner.Format.Structured() {
		runner.SetCount("unpacked", unpacked.Load())
		runner.SetCount("skipped", skipped.Load())
		runner.SetCount("failed", failed.Load())
		return failed.Load() == 0, nil
	}
	color, reset := cliutil.StatusColor(runner.Format, failed.Load() > 0)
	fmt.Printf("%sunpack: %d unpacked, %d skipped, %d failed in %s%s\n", color, unpacked.Load(), skipped.Load(), failed.Load(), time.Since(start).Round(time.Second), reset)
	return failed.Load() == 0, nil
}

// unpackOne extracts archivePath's sole entry to out, retiring the
// archive on success. The caller has already confirmed out doesn't
// exist.
func unpackOne(sysDir, archivePath, out string) error {
	tmp, err := os.MkdirTemp(sysDir, "_extract.*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)

	if err := archive.ExtractAll(archivePath, tmp); err != nil {
		return fmt.Errorf("extraction failed: %s: %w", archivePath, err)
	}
	inner, err := soleFile(tmp)
	if err != nil {
		return err
	}
	if err := os.Rename(inner, out); err != nil {
		return err
	}
	return actions.Retire(archivePath)
}

// soleFile finds the single regular file somewhere under root.
func soleFile(root string) (string, error) {
	var found string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		found = path
		return nil
	})
	if err != nil {
		return "", err
	}
	if found == "" {
		return "", fmt.Errorf("no file found in extracted archive: %s", root)
	}
	return found, nil
}
