package commands

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/demicloud/romctl/internal/actions"
	"github.com/demicloud/romctl/internal/cliutil"
	"github.com/demicloud/romctl/internal/library"
)

// TestCleanRemovesNestedM3UToDelete guards Clean's handling of a
// _to_delete/ nested one level deeper inside a Title.m3u/ multi-disc
// directory - chd's DVD re-encode pass (reencodeAsDVD) retires a
// replaced CD-type CHD right next to it, inside the playlist directory,
// the same "retire beside the file it replaced" rule every other
// conversion command follows. Previously Clean only ever looked for
// roms/<sys>/_to_delete, so a nested one would linger forever even
// though the pending-cleanup reminder (actions.RetiredCount) still
// correctly counted it.
func TestCleanRemovesNestedM3UToDelete(t *testing.T) {
	root := t.TempDir()
	sysDir := filepath.Join(root, "roms", "psx")
	topToDelete := filepath.Join(sysDir, "_to_delete")
	nestedToDelete := filepath.Join(sysDir, "Game.m3u", "_to_delete")
	if err := os.MkdirAll(topToDelete, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(nestedToDelete, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(topToDelete, "old.chd"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nestedToDelete, "old-disc.chd"), []byte("y"), 0o644); err != nil {
		t.Fatal(err)
	}
	lib := library.Open(root)

	runner := &actions.Runner{Format: cliutil.FormatPlain}
	if err := Clean(lib, runner, []string{"psx"}, true); err != nil {
		t.Fatal(err)
	}

	if exists(topToDelete) {
		t.Error("top-level _to_delete/ should have been removed")
	}
	if exists(nestedToDelete) {
		t.Error("nested Game.m3u/_to_delete/ should have been removed too")
	}
}
