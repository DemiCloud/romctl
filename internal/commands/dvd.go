package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/demicloud/romctl/internal/actions"
	"github.com/demicloud/romctl/internal/cliutil"
	"github.com/demicloud/romctl/internal/executil"
	"github.com/demicloud/romctl/internal/formats/chd"
	"github.com/demicloud/romctl/internal/library"
	"github.com/demicloud/romctl/internal/systemdb"
	"github.com/demicloud/romctl/internal/tools"
)

// dvdJob is one CD-as-DVD CHD to re-encode, flattened out of its system's
// directory listing up front - see CHD's doc comment for why every
// system's files run through one shared worker pool instead of one per
// system.
type dvdJob struct {
	sys, base string
}

// DVD re-encodes DVD images that got stored as CD-type CHDs (e.g. PS2
// titles dumped before this distinction mattered) as proper DVD-type CHDs.
func DVD(lib *library.Library, runner *actions.Runner, systems []string) (bool, error) {
	start := time.Now()
	log := cliutil.NewLogger("dvd")
	var converted, failed atomic.Int64

	var jobs []dvdJob
	for _, sys := range systems {
		if systemdb.ClassOf(sys) != systemdb.ClassDVD {
			continue
		}
		sysDir := filepath.Join(lib.Roms, sys)
		entries, err := os.ReadDir(sysDir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() || strings.ToLower(filepath.Ext(e.Name())) != ".chd" {
				continue
			}
			jobs = append(jobs, dvdJob{sys, e.Name()})
		}
	}

	cliutil.Parallel(jobs, func(j dvdJob) {
		path := filepath.Join(lib.Roms, j.sys, j.base)
		kind, err := chd.Classify(path)
		if err != nil || kind != chd.KindCDAsDVDMAGE {
			return
		}
		if !runner.Act("re-encode %s/%s as DVD CHD", j.sys, j.base) {
			return
		}
		if err := reencodeAsDVD(path); err != nil {
			log.Errf("failed, original kept: %s: %v", path, err)
			failed.Add(1)
			return
		}
		converted.Add(1)
	})
	fmt.Printf("dvd: %d converted, %d failed in %s\n", converted.Load(), failed.Load(), time.Since(start).Round(time.Second))
	return failed.Load() == 0, nil
}

// reencodeAsDVD re-encodes one CHD via its own fresh scratch directory -
// each call (now possibly concurrent with others re-encoding the same
// system) needs its own, since chdman's extractcd always writes to fixed
// filenames. The scratch directory is created next to path, inside its
// own system directory, rather than in the OS's global temp dir: the
// final os.Rename below needs to land on the same filesystem as path, and
// the system directory and the global temp dir (often tmpfs, or another
// partition entirely) are frequently not the same filesystem - e.g. a
// library living on an SD card. The "_extract." prefix matches the one
// chd/unpack's own scratch directories already use, so an interrupted run
// leaves a directory check already recognizes as a leftover to clean up.
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
