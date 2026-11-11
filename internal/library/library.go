// Package library resolves a romctl library root (roms/, bios/) and the
// system directories within it.
package library

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/demicloud/romctl/internal/cliutil"
)

// Library is a resolved romctl library root.
type Library struct {
	Root string
	Roms string
	Bios string
	// Saves and States are RetroDECK's own save-data directories,
	// siblings of Roms/Bios - romctl never reads them, only writes to
	// them (see commands.Saves, which relocates stray save/state files
	// out of Roms into their proper per-system subfolder here).
	Saves  string
	States string
}

// Open resolves root into a Library. It does not require bios/ to exist,
// but callers that need roms/ should check Roms themselves. root is always
// made absolute: romctl shells out to external tools (Skyscraper in
// particular, which chdirs to ~/.skyscraper before resolving any relative
// -i/-g/-o it's given) that can't be trusted to resolve a relative path
// against romctl's own working directory.
func Open(root string) *Library {
	if abs, err := filepath.Abs(root); err == nil {
		root = abs
	}
	root = strings.TrimRight(filepath.Clean(root), string(filepath.Separator))
	return &Library{
		Root:   root,
		Roms:   filepath.Join(root, "roms"),
		Bios:   filepath.Join(root, "bios"),
		Saves:  filepath.Join(root, "saves"),
		States: filepath.Join(root, "states"),
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
// name is validated to exist. A dot-prefixed directory (RetroDECK's own
// roms/.sync/, holding its favorites-sync staging file - confirmed
// against a real library) is never a real system, regardless: skipped
// the same way a dotfile inside a system's own directory already is.
func (l *Library) ResolveSystems(args []string) ([]string, error) {
	if len(args) == 0 || args[0] == "all" {
		entries, err := os.ReadDir(l.Roms)
		if err != nil {
			return nil, err
		}
		systems := make([]string, 0, len(entries))
		for _, e := range entries {
			if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
				systems = append(systems, e.Name())
			}
		}
		return systems, nil
	}
	systems := make([]string, 0, len(args))
	for _, s := range args {
		info, err := os.Stat(filepath.Join(l.Roms, s))
		if err != nil || !info.IsDir() {
			msg := fmt.Sprintf("no such system directory: %s", filepath.Join(l.Roms, s))
			if suggestion := cliutil.Suggest(s, l.existingSystems()); suggestion != "" {
				msg += fmt.Sprintf(" (did you mean %q?)", suggestion)
			}
			return nil, fmt.Errorf("%s", msg)
		}
		systems = append(systems, s)
	}
	return systems, nil
}

// existingSystems lists the actual directories under Roms, for Suggest to
// match a typo'd system name against - a suggestion naming a directory
// that doesn't actually exist (or isn't really a system - see
// ResolveSystems on dot-prefixed directories) would be worse than no
// suggestion at all.
func (l *Library) existingSystems() []string {
	entries, err := os.ReadDir(l.Roms)
	if err != nil {
		return nil
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
			names = append(names, e.Name())
		}
	}
	return names
}

// ESDESystems reads <Root>/systems.txt - a convenience file ES-DE itself
// generates (one line per system, "<dir>: <full name>") straight from
// RetroDECK's own bundled es_systems.xml, listing every system that
// version of RetroDECK knows about. Unlike systemdb.FullName (hand-ported
// and can drift from whatever RetroDECK version is actually installed),
// this reflects the real, current list - so callers prefer it when
// present. ok is false (not an error) when the file doesn't exist: ES-DE
// only generates it on demand, so "absent" just means "nothing to
// cross-check against," the same way gamelist treats a missing ES-DE/ as
// a no-op rather than a failure.
func (l *Library) ESDESystems() (map[string]string, bool) {
	data, err := os.ReadFile(filepath.Join(l.Root, "systems.txt"))
	if err != nil {
		return nil, false
	}
	out := make(map[string]string)
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimRight(line, "\r")
		dir, name, ok := strings.Cut(line, ": ")
		if !ok {
			continue
		}
		out[dir] = name
	}
	return out, len(out) > 0
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
