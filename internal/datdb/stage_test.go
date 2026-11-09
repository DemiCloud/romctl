package datdb

import (
	"os"
	"path/filepath"
	"testing"
)

func writeStageFixture(t *testing.T, dir, provider, name, headerName string) {
	t.Helper()
	d := filepath.Join(dir, provider)
	if err := os.MkdirAll(d, 0o755); err != nil {
		t.Fatal(err)
	}
	dat := `<datafile><header><name>` + headerName + `</name><version>v</version></header></datafile>`
	if err := os.WriteFile(filepath.Join(d, name), []byte(dat), 0o644); err != nil {
		t.Fatal(err)
	}
}

func resultFor(results []StageResult, sys string) (StageResult, bool) {
	for _, r := range results {
		if r.System == sys {
			return r, true
		}
	}
	return StageResult{}, false
}

func TestStageMatchesTimestampedFilename(t *testing.T) {
	src, dst := t.TempDir(), t.TempDir()
	// "gb" -> {"no-intro", "Nintendo - Game Boy"} in systemdb.Dat. Real
	// packs carry a dynamic "(timestamp)" suffix - this must still match.
	writeStageFixture(t, src, "no-intro", "Nintendo - Game Boy (20261107-104737).dat", "Nintendo - Game Boy")

	results, err := Stage(src, dst)
	if err != nil {
		t.Fatalf("Stage: %v", err)
	}
	r, ok := resultFor(results, "gb")
	if !ok {
		t.Fatal("no result for gb")
	}
	if r.Status != "staged" {
		t.Fatalf("gb status = %q, detail %q, want staged", r.Status, r.Detail)
	}
	if _, err := os.Stat(filepath.Join(dst, "no-intro", "Nintendo - Game Boy.dat")); err != nil {
		t.Fatalf("staged file not found at the stable (timestamp-stripped) path: %v", err)
	}
}

func TestStageAlreadyStrippedFilename(t *testing.T) {
	src, dst := t.TempDir(), t.TempDir()
	// A maintainer who already renamed the file by hand (no timestamp)
	// should still work, not just the glob path.
	writeStageFixture(t, src, "no-intro", "Nintendo - Game Boy.dat", "Nintendo - Game Boy")

	results, err := Stage(src, dst)
	if err != nil {
		t.Fatalf("Stage: %v", err)
	}
	r, _ := resultFor(results, "gb")
	if r.Status != "staged" {
		t.Fatalf("gb status = %q, detail %q, want staged", r.Status, r.Detail)
	}
}

func TestStageHeaderMismatchRejected(t *testing.T) {
	src, dst := t.TempDir(), t.TempDir()
	// The file matched gb's glob pattern by name, but its own <header>
	// says it's actually a different system's DAT - must not be staged.
	writeStageFixture(t, src, "no-intro", "Nintendo - Game Boy (20261107-104737).dat", "Nintendo - Game Boy Color")

	results, err := Stage(src, dst)
	if err != nil {
		t.Fatalf("Stage: %v", err)
	}
	r, _ := resultFor(results, "gb")
	if r.Status != "mismatch" {
		t.Fatalf("gb status = %q, want mismatch", r.Status)
	}
	if _, err := os.Stat(filepath.Join(dst, "no-intro", "Nintendo - Game Boy.dat")); err == nil {
		t.Fatal("a header-mismatched file must not be staged")
	}
}

func TestStageDoesNotConfuseVariantFilesViaGlob(t *testing.T) {
	src, dst := t.TempDir(), t.TempDir()
	// A real pack's own filenames can carry a second, non-timestamp
	// parenthetical before the real timestamp one (e.g. GBA's "Multiboot"
	// variant) - these must not be mistaken for the plain system's file,
	// which a naive "<system> (*).dat" glob would do (`*` matches across
	// the earlier "(...)" group too).
	writeStageFixture(t, src, "no-intro", "Nintendo - Game Boy Advance (20261107-104733).dat", "Nintendo - Game Boy Advance")
	writeStageFixture(t, src, "no-intro", "Nintendo - Game Boy Advance (Multiboot) (20261012-143535).dat", "Nintendo - Game Boy Advance (Multiboot)")
	writeStageFixture(t, src, "no-intro", "Nintendo - Game Boy Advance (e-Reader) (20260920-151102).dat", "Nintendo - Game Boy Advance (e-Reader)")

	results, err := Stage(src, dst)
	if err != nil {
		t.Fatalf("Stage: %v", err)
	}
	r, ok := resultFor(results, "gba")
	if !ok {
		t.Fatal("no result for gba")
	}
	if r.Status != "staged" {
		t.Fatalf("gba status = %q, detail %q, want staged (exactly the plain variant, unambiguous)", r.Status, r.Detail)
	}
}

func TestStageMatchesRedumpDownloadFilename(t *testing.T) {
	src, dst := t.TempDir(), t.TempDir()
	// A real per-system download from http://redump.org/datfile/<slug>/
	// carries a differently-shaped dynamic suffix than No-Intro's own
	// packs - " - Datfile (<id>) (<date> <time>).dat" - confirmed against
	// a real "psx" download. redumpSuffix must recognize this shape too.
	writeStageFixture(t, src, "redump", "Sony - PlayStation - Datfile (10914) (2026-06-15 11-55-46).dat", "Sony - PlayStation")

	results, err := Stage(src, dst)
	if err != nil {
		t.Fatalf("Stage: %v", err)
	}
	r, ok := resultFor(results, "psx")
	if !ok {
		t.Fatal("no result for psx")
	}
	if r.Status != "staged" {
		t.Fatalf("psx status = %q, detail %q, want staged", r.Status, r.Detail)
	}
	if _, err := os.Stat(filepath.Join(dst, "redump", "Sony - PlayStation.dat")); err != nil {
		t.Fatalf("staged file not found at the stable (suffix-stripped) path: %v", err)
	}
}

func TestStageMissingIsNotAnError(t *testing.T) {
	src, dst := t.TempDir(), t.TempDir()
	results, err := Stage(src, dst)
	if err != nil {
		t.Fatalf("Stage: %v", err)
	}
	r, ok := resultFor(results, "gb")
	if !ok || r.Status != "missing" {
		t.Fatalf("gb result = %+v, ok=%v, want status missing", r, ok)
	}
}

func TestStageIgnoresUnrelatedFiles(t *testing.T) {
	src, dst := t.TempDir(), t.TempDir()
	// A real pack bundles hundreds of systems romctl doesn't track - one
	// of those must not be staged or otherwise reported.
	writeStageFixture(t, src, "no-intro", "Acorn - Archimedes (20231029-220453).dat", "Acorn - Archimedes")

	results, err := Stage(src, dst)
	if err != nil {
		t.Fatalf("Stage: %v", err)
	}
	for _, r := range results {
		if r.Name == "Acorn - Archimedes" {
			t.Fatalf("unrelated system should not appear in results: %+v", r)
		}
	}
	entries, _ := os.ReadDir(filepath.Join(dst, "no-intro"))
	for _, e := range entries {
		if e.Name() == "Acorn - Archimedes.dat" {
			t.Fatal("unrelated system should not be staged")
		}
	}
}
