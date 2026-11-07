// Package library resolves a romctl library root (roms/, bios/) and the
// system directories within it.
package library

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Library is a resolved romctl library root.
type Library struct {
	Root string
	Roms string
	Bios string
}

// Open resolves root into a Library. It does not require bios/ to exist,
// but callers that need roms/ should check Roms themselves.
func Open(root string) *Library {
	root = strings.TrimRight(filepath.Clean(root), string(filepath.Separator))
	return &Library{
		Root: root,
		Roms: filepath.Join(root, "roms"),
		Bios: filepath.Join(root, "bios"),
	}
}

// HasRoms reports whether root itself already has a roms/ directory,
// without needing a resolved Library - used to decide root precedence
// before a Library is even opened (see cmd/romctl's resolveRoot).
func HasRoms(root string) bool {
	info, err := os.Stat(filepath.Join(root, "roms"))
	return err == nil && info.IsDir()
}

// RequireRoms errors unless roms/ exists under the library root.
func (l *Library) RequireRoms() error {
	if info, err := os.Stat(l.Roms); err != nil || !info.IsDir() {
		return fmt.Errorf("no roms/ directory under %s (use -r)", l.Root)
	}
	return nil
}

// ResolveSystems expands system arguments to directory names under roms/:
// no arguments or "all" means every directory under roms/, otherwise each
// name is validated to exist.
func (l *Library) ResolveSystems(args []string) ([]string, error) {
	if len(args) == 0 || args[0] == "all" {
		entries, err := os.ReadDir(l.Roms)
		if err != nil {
			return nil, err
		}
		systems := make([]string, 0, len(entries))
		for _, e := range entries {
			if e.IsDir() {
				systems = append(systems, e.Name())
			}
		}
		return systems, nil
	}
	systems := make([]string, 0, len(args))
	for _, s := range args {
		info, err := os.Stat(filepath.Join(l.Roms, s))
		if err != nil || !info.IsDir() {
			return nil, fmt.Errorf("no such system directory: %s", filepath.Join(l.Roms, s))
		}
		systems = append(systems, s)
	}
	return systems, nil
}

// HasGames reports whether a system directory holds any game entries,
// ignoring systeminfo.txt, other .txt files, _to_delete, and .directory.
func HasGames(sysDir string) bool {
	entries, err := os.ReadDir(sysDir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		name := e.Name()
		if name == "_to_delete" || name == ".directory" || strings.HasSuffix(name, ".txt") {
			continue
		}
		return true
	}
	return false
}
