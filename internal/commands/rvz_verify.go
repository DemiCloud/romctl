package commands

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/demicloud/romctl/internal/executil"
	"github.com/demicloud/romctl/internal/systemdb"
	"github.com/demicloud/romctl/internal/tools"
)

// resolveDolphinToolIfNeeded resolves dolphin-tool once, up front, only
// if at least one of the given systems is GameCube/Wii - same "resolve
// before any work, actionable error" invariant resolveCHDManIfNeeded
// already follows for chdman. Returns "" with no error if no requested
// system needs it, so --verify-chd on a selection with no ClassDolphin
// system never requires dolphin-tool at all.
func resolveDolphinToolIfNeeded(systems []string) (string, error) {
	needed := false
	for _, sys := range systems {
		if systemdb.ClassOf(sys) == systemdb.ClassDolphin {
			needed = true
			break
		}
	}
	if !needed {
		return "", nil
	}
	return tools.Resolve("dolphin-tool")
}

// hashDiscRVZ extracts path (a GameCube/Wii .rvz) via dolphin-tool and
// returns its whole-file hash. Unlike hashDiscCHD's CD case, a GameCube/
// Wii disc image is never split into multiple tracks, so there's always
// exactly one hash. dolphin-tool's own junk-data regeneration reproduces
// the full, byte-exact original disc image even from an NKit-shrunk
// source (confirmed against a real one: its extracted ISO's CRC32
// matched Redump's listed hash exactly, despite dolphin-tool's own
// "Converting an NKit file" warning on the way). Only called from the
// --verify-chd path - see Collection/Identify's doc comments. No
// chdScratchDir-style tmpfs sizing attempt here (unlike hashDiscCHD):
// there's no equivalent of chd.LogicalSize to size the request from
// without decompressing first, so this always lands in a plain OS temp
// directory - a correctness-first default a future change can optimize
// if it turns out to matter in practice.
func hashDiscRVZ(dolphinToolPath, path string) (discTrackHash, error) {
	work, err := os.MkdirTemp("", "romctl-verify-rvz-*")
	if err != nil {
		return discTrackHash{}, err
	}
	defer os.RemoveAll(work)

	iso := filepath.Join(work, "x.iso")
	if out, err := executil.Run(dolphinToolPath, "convert", "-i", path, "-o", iso, "-f", "iso"); err != nil {
		return discTrackHash{}, fmt.Errorf("dolphin-tool convert: %w: %s", err, out)
	}
	return hashRange(iso, 0, -1)
}
