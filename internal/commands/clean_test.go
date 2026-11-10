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

// TestCleanIgnoresNonDirectoryToDelete guards a real regression: the
// target-collection check was relaxed from os.Stat+IsDir to a bare
// exists(), so a plain file sitting at roms/<sys>/_to_delete (never
// created by Retire itself, which always makes a directory, but
// possible from an unrelated bug or a user mistake) would silently
// become a clean target instead of being excluded the way the original
// IsDir check guaranteed.
func TestCleanIgnoresNonDirectoryToDelete(t *testing.T) {
	root := t.TempDir()
	sysDir := filepath.Join(root, "roms", "psx")
	if err := os.MkdirAll(sysDir, 0o755); err != nil {
		t.Fatal(err)
	}
	notADir := filepath.Join(sysDir, "_to_delete")
	if err := os.WriteFile(notADir, []byte("not actually a directory"), 0o644); err != nil {
		t.Fatal(err)
	}
	lib := library.Open(root)

	runner := &actions.Runner{Format: cliutil.FormatPlain}
	if err := Clean(lib, runner, []string{"psx"}, true); err != nil {
		t.Fatal(err)
	}
	if !exists(notADir) {
		t.Error("a non-directory _to_delete should have been left alone, not treated as a clean target")
	}
}
