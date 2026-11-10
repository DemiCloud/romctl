package discimage

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/demicloud/romctl/internal/actions"
	"github.com/demicloud/romctl/internal/cliutil"
	"github.com/demicloud/romctl/internal/systemdb"
)

// TestBuildISOsUpdatesGamelist guards BuildISOs calling gamelist.Rename
// after converting a disc folder to an ISO - unlike UnpackArchives' own
// archive-to-folder step (an archive is never itself a scraped entry),
// the disc folder here is a real ES-DE game entry in its own right, so
// it can already carry scraped metadata (scraped on-device by ES-DE, or
// pulled down locally) before this command ever runs. Without this, that
// metadata would be orphaned - left pointing at a path that no longer
// exists - the moment the folder became an ISO.
func TestBuildISOsUpdatesGamelist(t *testing.T) {
	root := t.TempDir()
	sys := "ps3"
	sysDir := filepath.Join(root, "roms", sys)
	spec, ok := SpecFor(systemdb.ClassPS3)
	if !ok {
		t.Fatal("no Spec registered for ClassPS3")
	}
	discDir := filepath.Join(sysDir, "Some Game.ps3")
	usrDir := filepath.Join(discDir, spec.MarkerDir, "USRDIR")
	if err := os.MkdirAll(usrDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(usrDir, "EBOOT.BIN"), []byte("eboot-content"), 0o644); err != nil {
		t.Fatal(err)
	}

	gl := filepath.Join(root, "ES-DE", "gamelists", sys, "gamelist.xml")
	if err := os.MkdirAll(filepath.Dir(gl), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(gl, []byte(`<?xml version="1.0"?>
<gameList>
	<game>
		<path>./Some Game.ps3</path>
		<name>Some Game</name>
	</game>
</gameList>
`), 0o644); err != nil {
		t.Fatal(err)
	}

	runner := &actions.Runner{Format: cliutil.FormatPlain}
	log := cliutil.NewLogger("ps3", cliutil.FormatPlain)
	built, err := BuildISOs(root, sysDir, sys, spec, runner, log)
	if err != nil {
		t.Fatal(err)
	}
	if built != 1 {
		t.Fatalf("built = %d, want 1", built)
	}

	got, err := os.ReadFile(gl)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "./Some Game.iso") {
		t.Errorf("expected gamelist.xml to now point at ./Some Game.iso, got:\n%s", got)
	}
	if strings.Contains(string(got), "./Some Game.ps3") {
		t.Errorf("gamelist.xml still points at the old .ps3 path, got:\n%s", got)
	}
}
