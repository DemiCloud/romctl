package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/demicloud/romctl/internal/actions"
	"github.com/demicloud/romctl/internal/cliutil"
	"github.com/demicloud/romctl/internal/executil"
	"github.com/demicloud/romctl/internal/formats/chd"
	"github.com/demicloud/romctl/internal/library"
	"github.com/demicloud/romctl/internal/systemdb"
	"github.com/demicloud/romctl/internal/tools"
)

// DVD re-encodes DVD images that got stored as CD-type CHDs (e.g. PS2
// titles dumped before this distinction mattered) as proper DVD-type CHDs.
func DVD(lib *library.Library, runner *actions.Runner, systems []string) (bool, error) {
	log := cliutil.NewLogger("dvd")
	failed := false

	work, err := os.MkdirTemp("", "romctl-dvd")
	if err != nil {
		return false, err
	}
	defer os.RemoveAll(work)

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
			path := filepath.Join(sysDir, e.Name())
			kind, err := chd.Classify(path)
			if err != nil || kind != chd.KindCDAsDVDMAGE {
				continue
			}
			if !runner.Act("re-encode %s/%s as DVD CHD", sys, e.Name()) {
				continue
			}
			if err := reencodeAsDVD(path, work); err != nil {
				log.Errf("failed, original kept: %s: %v", path, err)
				failed = true
			}
		}
	}
	return !failed, nil
}

func reencodeAsDVD(path, work string) error {
	chdmanPath, err := tools.Resolve("chdman")
	if err != nil {
		return err
	}
	iso := filepath.Join(work, "x.iso")
	cue := filepath.Join(work, "x.cue")
	out := filepath.Join(work, "out.chd")
	os.Remove(iso)
	os.Remove(cue)
	os.Remove(out)
	defer func() {
		os.Remove(iso)
		os.Remove(cue)
		os.Remove(out)
	}()

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
