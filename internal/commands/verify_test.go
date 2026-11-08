package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/demicloud/romctl/internal/cliutil"
	"github.com/demicloud/romctl/internal/library"
)

// fakeChdman writes an executable shell script standing in for chdman's
// "verify" subcommand: it exits non-zero (printing a message to stderr)
// for any -i argument whose path contains any of fail, succeeding
// otherwise - enough to drive runVerify without a real CHD or a real
// chdman binary.
func fakeChdman(t *testing.T, fail ...string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "chdman")
	var script strings.Builder
	script.WriteString("#!/bin/sh\n")
	script.WriteString(`target="$3"` + "\n")
	for _, f := range fail {
		script.WriteString("case \"$target\" in *'" + f + "'*) echo bad hunk checksum >&2; exit 1;; esac\n")
	}
	script.WriteString("exit 0\n")
	if err := os.WriteFile(path, []byte(script.String()), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

// verifyScratchLibrary builds roms/psx (CD-class) with a bare .chd and a
// Multi.m3u/ directory of two per-disc .chd files, plus roms/snes
// (cart-class) with a same-extension decoy that verifyJobsFor must never
// pick up - only CD/DVD/UMD-class systems carry CHDs at all.
func verifyScratchLibrary(t *testing.T) *library.Library {
	t.Helper()
	root := t.TempDir()
	psxDir := filepath.Join(root, "roms", "psx")
	if err := os.MkdirAll(psxDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(psxDir, "Game.chd"), []byte("chd"), 0o644); err != nil {
		t.Fatal(err)
	}
	m3uDir := filepath.Join(psxDir, "Multi.m3u")
	if err := os.MkdirAll(m3uDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, disc := range []string{"Multi (Disc 01).chd", "Multi (Disc 02).chd"} {
		if err := os.WriteFile(filepath.Join(m3uDir, disc), []byte("chd"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(m3uDir, "Multi.m3u"), []byte("Multi (Disc 01).chd\nMulti (Disc 02).chd\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	snesDir := filepath.Join(root, "roms", "snes")
	if err := os.MkdirAll(snesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(snesDir, "Decoy.chd"), []byte("chd"), 0o644); err != nil {
		t.Fatal(err)
	}
	return library.Open(root)
}

// TestVerifyJobsForScansBareAndM3uDiscs confirms verifyJobsFor flattens
// both a bare .chd and every disc inside a Title.m3u/ directory into
// jobs, and skips the playlist file itself and any .chd sitting in a
// non-CD/DVD/UMD system.
func TestVerifyJobsForScansBareAndM3uDiscs(t *testing.T) {
	lib := verifyScratchLibrary(t)
	jobs := verifyJobsFor(lib, []string{"psx", "snes"})

	var labels []string
	for _, j := range jobs {
		labels = append(labels, j.sys+"/"+j.label)
	}
	want := []string{"psx/Game.chd", "psx/Multi.m3u/Multi (Disc 01).chd", "psx/Multi.m3u/Multi (Disc 02).chd"}
	if len(labels) != len(want) {
		t.Fatalf("verifyJobsFor jobs = %v, want %v", labels, want)
	}
	for _, w := range want {
		found := false
		for _, l := range labels {
			if l == w {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected job %q, got %v", w, labels)
		}
	}
}

// TestRunVerifyReportsCorruptDisc confirms a chdman verify failure on one
// disc inside a Title.m3u/ directory is reported against that disc's own
// path (not the playlist or the sibling disc), and that it doesn't stop
// the other, healthy files from being treated as fine.
func TestRunVerifyReportsCorruptDisc(t *testing.T) {
	lib := verifyScratchLibrary(t)
	chdman := fakeChdman(t, "Disc 02")
	jobs := verifyJobsFor(lib, []string{"psx"})

	out := captureStdout(t, func() {
		runVerify(jobs, chdman, true, cliutil.FormatPlain)
	})

	if !strings.Contains(out, "Multi.m3u/Multi (Disc 02).chd") || !strings.Contains(out, "failed data verify") {
		t.Errorf("expected a failed-data-verify finding naming Multi.m3u/Multi (Disc 02).chd, got:\n%s", out)
	}
	if n := strings.Count(out, "failed data verify"); n != 1 {
		t.Errorf("expected exactly one failed-data-verify finding, got %d:\n%s", n, out)
	}
}

// TestRunVerifyVerbosePrintsPerFileProgress confirms -v prints a line for
// every file as it's checked, not just the ones that fail - the whole
// point of pulling verify out of check's own --verbose in the first
// place (see Verify's doc comment).
func TestRunVerifyVerbosePrintsPerFileProgress(t *testing.T) {
	lib := verifyScratchLibrary(t)
	chdman := fakeChdman(t)
	jobs := verifyJobsFor(lib, []string{"psx"})

	out := captureStdout(t, func() {
		runVerify(jobs, chdman, true, cliutil.FormatPlain)
	})

	for _, want := range []string{"verify psx/Game.chd", "verify psx/Multi.m3u/Multi (Disc 01).chd", "verify psx/Multi.m3u/Multi (Disc 02).chd"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected verbose output to contain %q, got:\n%s", want, out)
		}
	}
}
