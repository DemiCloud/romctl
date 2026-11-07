// Package tools resolves the four binaries romctl still shells out to —
// chdman, dolphin-tool, extract-xiso and skyscraper — preferring a binary
// embedded for the current platform at build time (via go:embed, see
// embed_*.go) and extracted to a per-version cache directory, falling back
// to PATH. CHD, RVZ and the Xbox XDVDFS disc format are complex,
// emulator-sensitive codecs not worth reimplementing; everything else the
// historical Bash tool shelled out to (7z, maxcso, makeps3iso) is pure Go
// now (see internal/archive, internal/formats/cso,
// internal/formats/iso9660). Skyscraper (github.com/muldjord/skyscraper)
// is the scraper used by `romctl scrape`: its own ScreenScraper developer
// credentials are baked in, so romctl only ever needs the user's personal
// account, not its own developer registration - see internal/commands's
// Scrape and the "Scraping" section in README.md.
package tools

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"

	"github.com/demicloud/romctl/internal/version"
)

// Resolve returns a path to an executable for tool ("chdman",
// "dolphin-tool", "extract-xiso" or "skyscraper"): an embedded build for
// this platform if one was baked in, extracted to the user's cache
// directory, otherwise whatever is on PATH. It errors with actionable
// instructions if neither is available.
func Resolve(tool string) (string, error) {
	name := binaryName(tool)

	data, err := fs.ReadFile(embedded, "data/"+platformDir()+"/"+name)
	if err == nil {
		path, err := extractToCache(name, data)
		if err != nil {
			return "", fmt.Errorf("extracting embedded %s: %w", tool, err)
		}
		return path, nil
	}

	if path, err := exec.LookPath(tool); err == nil {
		return path, nil
	}

	return "", fmt.Errorf(
		"%s not found: no build embedded for %s/%s, and %s isn't on PATH\n"+
			"get a romctl release for this platform, or build %s yourself (see 'make tools' in the Makefile) and put it on PATH",
		tool, runtime.GOOS, runtime.GOARCH, tool, tool)
}

func binaryName(tool string) string {
	if runtime.GOOS == "windows" {
		return tool + ".exe"
	}
	return tool
}

func platformDir() string {
	return runtime.GOOS + "_" + runtime.GOARCH
}

// extractMu serializes extractToCache: conversion commands now call
// Resolve concurrently (one goroutine per system), and two goroutines
// racing to extract the same not-yet-cached tool would otherwise write the
// same PID-keyed temp path at once.
var extractMu sync.Mutex

// extractToCache writes data to $cache/romctl/tools/<version>/<name>,
// skipping the write if a same-sized copy is already there.
func extractToCache(name string, data []byte) (string, error) {
	extractMu.Lock()
	defer extractMu.Unlock()

	base, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(base, "romctl", "tools", version.Version)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	dst := filepath.Join(dir, name)
	if info, err := os.Stat(dst); err == nil && info.Size() == int64(len(data)) {
		return dst, nil
	}
	tmp := fmt.Sprintf("%s.partial.%d", dst, os.Getpid())
	if err := os.WriteFile(tmp, data, 0o755); err != nil {
		return "", err
	}
	if err := os.Rename(tmp, dst); err != nil {
		os.Remove(tmp)
		return "", err
	}
	return dst, nil
}
