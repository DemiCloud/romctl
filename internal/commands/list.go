package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/demicloud/romctl/internal/library"
	"github.com/demicloud/romctl/internal/systemdb"
)

// List prints every given system that has at least one game, its full
// name, and how many games it has; showGames additionally lists each
// game's title.
func List(lib *library.Library, systems []string, showGames bool) error {
	type populated struct {
		system string
		games  []string
	}

	esdeNames, _ := lib.ESDESystems()

	var results []populated
	for _, sys := range systems {
		games, err := gameTitles(filepath.Join(lib.Roms, sys))
		if err != nil || len(games) == 0 {
			continue
		}
		results = append(results, populated{system: sys, games: games})
	}
	sort.Slice(results, func(i, j int) bool {
		return systemdb.DisplayNameWith(results[i].system, esdeNames) < systemdb.DisplayNameWith(results[j].system, esdeNames)
	})

	for _, r := range results {
		fmt.Printf("%-40s %-18s %4d game(s)\n", systemdb.DisplayNameWith(r.system, esdeNames), "("+r.system+")", len(r.games))
		if !showGames {
			continue
		}
		names := append([]string(nil), r.games...)
		sort.Strings(names)
		for _, name := range names {
			fmt.Printf("    %s\n", name)
		}
	}
	return nil
}

// gameTitles lists the title of every game entry directly under sysDir,
// using the same "one entry = one game" rule as library.HasGames (a
// directory with a recognized extension, such as a .m3u playlist or a
// .ps3 disc folder, counts as a single game, not its contents).
func gameTitles(sysDir string) ([]string, error) {
	entries, err := os.ReadDir(sysDir)
	if err != nil {
		return nil, err
	}
	var titles []string
	for _, e := range entries {
		name := e.Name()
		if name == "systeminfo.txt" || name == ".directory" || name == "_to_delete" ||
			strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_extract.") ||
			strings.HasSuffix(name, ".txt") || isLeftover(name) {
			continue
		}
		titles = append(titles, gameTitle(name))
	}
	return titles, nil
}

// isLeftover matches the incomplete-conversion leftovers "check" flags as
// FAIL: not a game, just debris from an interrupted run.
func isLeftover(name string) bool {
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(name), "."))
	return strings.Contains(name, ".partial.") || strings.Contains(name, ".tmp.") ||
		strings.Contains(name, ".romctl-rename.") || ext == "part" || ext == "tmp"
}

func gameTitle(name string) string {
	switch {
	case strings.HasSuffix(name, ".m3u"):
		return strings.TrimSuffix(name, ".m3u")
	case strings.HasSuffix(name, ".ps3"):
		return strings.TrimSuffix(name, ".ps3")
	default:
		return strings.TrimSuffix(name, filepath.Ext(name))
	}
}
