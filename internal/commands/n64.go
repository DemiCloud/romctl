package commands

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/demicloud/romctl/internal/actions"
	"github.com/demicloud/romctl/internal/cliutil"
	"github.com/demicloud/romctl/internal/formats/n64"
	"github.com/demicloud/romctl/internal/gamelist"
	"github.com/demicloud/romctl/internal/library"
	"github.com/demicloud/romctl/internal/systemdb"
)

// N64 normalizes N64 ROMs to big-endian .z64.
func N64(lib *library.Library, runner *actions.Runner, systems []string) error {
	log := cliutil.NewLogger("n64")
	for _, sys := range systems {
		if systemdb.ClassOf(sys) != systemdb.ClassN64 {
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
			base := e.Name()
			ext := strings.ToLower(filepath.Ext(base))
			if ext != ".z64" && ext != ".v64" && ext != ".n64" {
				continue
			}
			src := filepath.Join(sysDir, base)
			order, err := n64.Detect(src)
			if err != nil || order == n64.OrderUnknown {
				log.Errf("unknown N64 header: %s/%s", sys, base)
				continue
			}
			out := strings.TrimSuffix(src, filepath.Ext(src)) + ".z64"
			if order == n64.OrderZ64 && src == out {
				continue
			}
			if exists(out) && src != out {
				log.Errf("exists, skipped: %s", out)
				continue
			}
			outBase := filepath.Base(out)
			if !runner.Act("normalize %s/%s -> %s", sys, base, outBase) {
				continue
			}
			if err := normalizeN64(src, out, order); err != nil {
				log.Errf("conversion failed: %s/%s: %v", sys, base, err)
				continue
			}
			if err := gamelist.Rename(lib.Root, sys, base, outBase); err != nil {
				log.Errf("gamelist update failed: %s/%s: %v", sys, base, err)
			}
		}
	}
	return nil
}

func normalizeN64(src, out string, order n64.Order) error {
	if order == n64.OrderZ64 {
		return os.Rename(src, out)
	}
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	converted, err := n64.ToBigEndian(data, order)
	if err != nil {
		return err
	}
	tmp := out + ".tmp"
	if err := os.WriteFile(tmp, converted, 0o644); err != nil {
		os.Remove(tmp)
		return err
	}
	if src == out {
		if err := actions.Retire(src); err != nil {
			os.Remove(tmp)
			return err
		}
		return os.Rename(tmp, out)
	}
	if err := os.Rename(tmp, out); err != nil {
		os.Remove(tmp)
		return err
	}
	return actions.Retire(src)
}
