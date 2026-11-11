package commands

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// fakeDolphinToolConvert stands in for dolphin-tool: it writes isoContent
// to whatever path follows "-o" on its command line, regardless of the
// input .rvz (which never needs to be real for this stand-in) - same
// convention as fakeChdmanExtract in chd_verify_test.go.
func fakeDolphinToolConvert(t *testing.T, isoContent []byte) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "dolphin-tool")
	script := fmt.Sprintf(`#!/bin/sh
prev=""
out=""
for arg in "$@"; do
  case "$prev" in
    -o) out="$arg" ;;
  esac
  prev="$arg"
done
echo %s | base64 -d > "$out"
exit 0
`, base64.StdEncoding.EncodeToString(isoContent))
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestHashDiscRVZ(t *testing.T) {
	isoData := []byte("a whole gamecube iso's worth of bytes, pretend")
	dolphinTool := fakeDolphinToolConvert(t, isoData)
	rvzPath := filepath.Join(t.TempDir(), "game.rvz")
	if err := os.WriteFile(rvzPath, []byte("not a real rvz, fake dolphin-tool ignores it"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := hashDiscRVZ(dolphinTool, rvzPath)
	if err != nil {
		t.Fatalf("hashDiscRVZ: %v", err)
	}
	want := mustHashReader(t, isoData)
	if got != want {
		t.Errorf("hashDiscRVZ() = %+v, want %+v", got, want)
	}
}

func TestResolveDolphinToolIfNeededSkipsNonDolphinSystems(t *testing.T) {
	path, err := resolveDolphinToolIfNeeded([]string{"psx", "snes"})
	if err != nil {
		t.Fatalf("resolveDolphinToolIfNeeded() = (%q, %v), want (\"\", nil) with no GameCube/Wii system given", path, err)
	}
	if path != "" {
		t.Errorf("resolveDolphinToolIfNeeded() = %q, want \"\" - dolphin-tool shouldn't be resolved at all", path)
	}
}

func TestHashDiscRVZPropagatesConversionFailure(t *testing.T) {
	dir := t.TempDir()
	failingTool := filepath.Join(dir, "dolphin-tool")
	if err := os.WriteFile(failingTool, []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	rvzPath := filepath.Join(dir, "game.rvz")
	if err := os.WriteFile(rvzPath, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := hashDiscRVZ(failingTool, rvzPath); err == nil {
		t.Error("hashDiscRVZ() with a failing dolphin-tool should return an error")
	}
}
